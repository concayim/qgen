// qgen 是一个基于 golang + eino 的智能体：扫描知识库清单，自动为每篇文档生成检索型问题，并导出为 xlsx。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"qgen/internal/config"
	"qgen/internal/export"
	"qgen/internal/gen"
	"qgen/internal/kb"
)

func main() {
	var (
		configPath  string
		rootsFlag   stringList
		outPath     string
		perDoc      int
		concurrency int
		maxDocs     int
		dryRun      bool
	)
	flag.StringVar(&configPath, "config", "", "配置文件路径(yaml)，可选")
	flag.Var(&rootsFlag, "root", "知识库根目录，可多次指定（覆盖配置）")
	flag.StringVar(&outPath, "out", "", "输出 xlsx 路径（覆盖配置）")
	flag.IntVar(&perDoc, "n", 0, "覆盖 information_interpretation 意图的条数（其余意图按配置）")
	flag.IntVar(&concurrency, "c", 0, "并发文档数（覆盖配置）")
	flag.IntVar(&maxDocs, "max", -1, "最多处理的文档数，0/负数为不限制（覆盖配置）")
	flag.BoolVar(&dryRun, "dry-run", false, "仅打印扫描到的知识库清单，不调用模型")
	flag.Parse()

	cfg, err := loadConfig(configPath, rootsFlag, outPath, perDoc, concurrency, maxDocs, dryRun)
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}

	docs, err := kb.Scan(&cfg.KB)
	if err != nil {
		log.Fatalf("扫描知识库失败: %v", err)
	}
	if len(docs) == 0 {
		log.Fatalf("未在 %v 中扫描到任何文档（后缀: %v）", cfg.KB.Roots, cfg.KB.Extensions)
	}
	if cfg.Gen.MaxDocs > 0 && len(docs) > cfg.Gen.MaxDocs {
		docs = docs[:cfg.Gen.MaxDocs]
	}

	fmt.Printf("📚 知识库清单：共扫描到 %d 篇文档\n", len(docs))
	for i, d := range docs {
		fmt.Printf("  %3d. %s  (%s)\n", i+1, d.Title, d.RelPath)
	}

	if dryRun {
		fmt.Println("\n[dry-run] 仅展示清单，未调用模型。")
		return
	}

	ctx := context.Background()
	generator, err := gen.New(ctx, cfg)
	if err != nil {
		log.Fatalf("初始化生成器失败: %v", err)
	}

	var mix []string
	for _, it := range cfg.Gen.Intents {
		if it.Count > 0 {
			mix = append(mix, fmt.Sprintf("%s×%d", it.Intent, it.Count))
		}
	}
	fmt.Printf("\n🤖 开始生成问题（模型: %s，每篇 %d 条 [%s]，并发 %d）...\n",
		cfg.Model.Model, cfg.Gen.TotalPerDoc(), strings.Join(mix, " + "), cfg.Gen.Concurrency)

	results := runPool(ctx, generator, cfg, docs)

	// 按文档顺序汇总并编号。
	var questions []gen.Question
	round := 1
	for _, r := range results {
		for _, q := range r.questions {
			q.Round = round
			round++
			questions = append(questions, q)
		}
	}

	if len(questions) == 0 {
		log.Fatalf("未生成任何问题，请检查模型配置或网络。")
	}

	if err := export.WriteXLSX(cfg.Output.Path, questions); err != nil {
		log.Fatalf("导出失败: %v", err)
	}

	fmt.Printf("\n✅ 完成：共生成 %d 条问题，已写入 %s\n", len(questions), cfg.Output.Path)
}

// docResult 保存单篇文档的生成结果，idx 用于恢复文档顺序。
type docResult struct {
	idx       int
	questions []gen.Question
}

func runPool(ctx context.Context, generator *gen.Generator, cfg *config.Config, docs []*kb.Document) []docResult {
	type job struct {
		idx int
		doc *kb.Document
	}

	jobs := make(chan job)
	resultsCh := make(chan docResult)
	var done int64

	var wg sync.WaitGroup
	for w := 0; w < cfg.Gen.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				qs := processDoc(ctx, generator, cfg, j.doc)
				n := atomic.AddInt64(&done, 1)
				fmt.Printf("  [%d/%d] %s -> %d 条\n", n, len(docs), truncateTitle(j.doc.Title), len(qs))
				resultsCh <- docResult{idx: j.idx, questions: qs}
			}
		}()
	}

	go func() {
		for i, d := range docs {
			jobs <- job{idx: i, doc: d}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	results := make([]docResult, 0, len(docs))
	for r := range resultsCh {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].idx < results[j].idx })
	return results
}

func processDoc(ctx context.Context, generator *gen.Generator, cfg *config.Config, doc *kb.Document) []gen.Question {
	if err := kb.Load(doc, cfg.Model.MaxDocChars); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️ 读取失败 %s: %v\n", doc.RelPath, err)
		return nil
	}

	// 简单重试，缓解偶发网络/限流问题。
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		qs, err := generator.Generate(ctx, doc)
		if err == nil {
			return qs
		}
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	fmt.Fprintf(os.Stderr, "  ⚠️ 生成失败 %s: %v\n", doc.RelPath, lastErr)
	return nil
}

func loadConfig(path string, roots stringList, out string, perDoc, concurrency, maxDocs int, dryRun bool) (*config.Config, error) {
	// dry-run 不需要模型 Key，使用 Default 跳过校验。
	var cfg *config.Config
	if dryRun {
		cfg = config.Default()
		if path != "" {
			if loaded, err := config.Load(path); err == nil {
				cfg = loaded
			}
		}
	} else {
		var err error
		cfg, err = config.Load(path)
		if err != nil {
			return nil, err
		}
	}

	if len(roots) > 0 {
		cfg.KB.Roots = roots
	}
	if out != "" {
		cfg.Output.Path = out
	}
	if perDoc > 0 {
		setIntentCount(cfg, "information_interpretation", "信息解读", perDoc)
	}
	if concurrency > 0 {
		cfg.Gen.Concurrency = concurrency
	}
	if maxDocs >= 0 {
		cfg.Gen.MaxDocs = maxDocs
	}
	if cfg.Gen.Concurrency < 1 {
		cfg.Gen.Concurrency = 1
	}
	return cfg, nil
}

// setIntentCount 覆盖指定意图的条数；若配置中不存在该意图则追加。
func setIntentCount(cfg *config.Config, intent, scene string, count int) {
	for i := range cfg.Gen.Intents {
		if cfg.Gen.Intents[i].Intent == intent {
			cfg.Gen.Intents[i].Count = count
			return
		}
	}
	cfg.Gen.Intents = append(cfg.Gen.Intents, config.IntentSpec{Intent: intent, Scene: scene, Count: count})
}

func truncateTitle(s string) string {
	r := []rune(s)
	if len(r) > 30 {
		return string(r[:30]) + "…"
	}
	return s
}

// stringList 支持 -root 多次指定。
type stringList []string

func (s *stringList) String() string { return fmt.Sprintf("%v", []string(*s)) }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}
