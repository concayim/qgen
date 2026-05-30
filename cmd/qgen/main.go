// qgen 是一个基于 golang + eino 的智能体：扫描知识库清单，自动为每篇文档生成检索型问题，并导出为 xlsx。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
		guidanceN   int
		draftN      int
		concurrency int
		maxDocs     int
		dryRun      bool
		jsonMode    bool
	)
	flag.StringVar(&configPath, "config", "", "配置文件路径(yaml)，可选")
	flag.Var(&rootsFlag, "root", "知识库根目录，可多次指定（覆盖配置）")
	flag.StringVar(&outPath, "out", "", "输出 xlsx 路径（覆盖配置）")
	flag.IntVar(&perDoc, "n", 0, "覆盖 information_interpretation 意图的条数（其余意图按配置）")
	flag.IntVar(&guidanceN, "guidance", -1, "覆盖 guidance 意图的条数")
	flag.IntVar(&draftN, "draft", -1, "覆盖 draft_from_doc 意图的条数")
	flag.IntVar(&concurrency, "c", 0, "并发文档数（覆盖配置）")
	flag.IntVar(&maxDocs, "max", -1, "最多处理的文档数，0/负数为不限制（覆盖配置）")
	flag.BoolVar(&dryRun, "dry-run", false, "仅打印扫描到的知识库清单，不调用模型")
	flag.BoolVar(&jsonMode, "json", false, "以 JSON 行输出进度事件（供客户端解析）")
	flag.Parse()

	em := &emitter{json: jsonMode}

	cfg, err := loadConfig(configPath, rootsFlag, outPath, perDoc, guidanceN, draftN, concurrency, maxDocs, dryRun)
	if err != nil {
		em.fatal("配置错误: " + err.Error())
	}

	docs, err := kb.Scan(&cfg.KB)
	if err != nil {
		em.fatal("扫描知识库失败: " + err.Error())
	}
	if len(docs) == 0 {
		em.fatal(fmt.Sprintf("未在 %v 中扫描到任何文档（后缀: %v）", cfg.KB.Roots, cfg.KB.Extensions))
	}
	if cfg.Gen.MaxDocs > 0 && len(docs) > cfg.Gen.MaxDocs {
		docs = docs[:cfg.Gen.MaxDocs]
	}

	em.scan(docs)

	if dryRun {
		em.event(event{Type: "done", DryRun: true, Docs: len(docs)})
		if !em.json {
			fmt.Println("\n[dry-run] 仅展示清单，未调用模型。")
		}
		return
	}

	ctx := context.Background()
	generator, err := gen.New(ctx, cfg)
	if err != nil {
		em.fatal("初始化生成器失败: " + err.Error())
	}

	var mix []string
	for _, it := range cfg.Gen.Intents {
		if it.Count > 0 {
			mix = append(mix, fmt.Sprintf("%s×%d", it.Intent, it.Count))
		}
	}
	em.start(cfg.Model.Model, cfg.Gen.TotalPerDoc(), cfg.Gen.Concurrency, strings.Join(mix, " + "))

	results := runPool(ctx, generator, cfg, docs, em)

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
		em.fatal("未生成任何问题，请检查模型配置或网络。")
	}

	if err := export.WriteXLSX(cfg.Output.Path, questions); err != nil {
		em.fatal("导出失败: " + err.Error())
	}

	em.done(len(questions), cfg.Output.Path)
}

// docResult 保存单篇文档的生成结果，idx 用于恢复文档顺序。
type docResult struct {
	idx       int
	questions []gen.Question
}

func runPool(ctx context.Context, generator *gen.Generator, cfg *config.Config, docs []*kb.Document, em *emitter) []docResult {
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
				qs := processDoc(ctx, generator, cfg, j.doc, em)
				n := atomic.AddInt64(&done, 1)
				em.progress(int(n), len(docs), j.doc.Title, len(qs))
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

func processDoc(ctx context.Context, generator *gen.Generator, cfg *config.Config, doc *kb.Document, em *emitter) []gen.Question {
	if err := kb.Load(doc, cfg.Model.MaxDocChars); err != nil {
		em.warn(fmt.Sprintf("读取失败 %s: %v", doc.RelPath, err))
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
	em.warn(fmt.Sprintf("生成失败 %s: %v", doc.RelPath, lastErr))
	return nil
}

func loadConfig(path string, roots stringList, out string, perDoc, guidanceN, draftN, concurrency, maxDocs int, dryRun bool) (*config.Config, error) {
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
	if guidanceN >= 0 {
		setIntentCount(cfg, "guidance", "文稿撰写", guidanceN)
	}
	if draftN >= 0 {
		setIntentCount(cfg, "draft_from_doc", "以稿写稿", draftN)
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

// stringList 支持 -root 多次指定。
type stringList []string

func (s *stringList) String() string { return fmt.Sprintf("%v", []string(*s)) }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// ---- 进度事件输出（人类可读 / JSON 行） ----

type docMeta struct {
	Title string `json:"title"`
	Rel   string `json:"rel"`
}

type event struct {
	Type        string    `json:"type"`
	Total       int       `json:"total,omitempty"`
	Docs        int       `json:"docs,omitempty"`
	DocList     []docMeta `json:"doc_list,omitempty"`
	Done        int       `json:"done,omitempty"`
	Title       string    `json:"title,omitempty"`
	Count       int       `json:"count,omitempty"`
	Model       string    `json:"model,omitempty"`
	TotalPerDoc int       `json:"total_per_doc,omitempty"`
	Concurrency int       `json:"concurrency,omitempty"`
	Mix         string    `json:"mix,omitempty"`
	Questions   int       `json:"questions,omitempty"`
	Output      string    `json:"output,omitempty"`
	Message     string    `json:"message,omitempty"`
	DryRun      bool      `json:"dry_run,omitempty"`
}

type emitter struct {
	json bool
	mu   sync.Mutex
}

func (e *emitter) event(ev event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.Marshal(ev)
	fmt.Println(string(b))
}

func (e *emitter) scan(docs []*kb.Document) {
	if e.json {
		list := make([]docMeta, len(docs))
		for i, d := range docs {
			list[i] = docMeta{Title: d.Title, Rel: d.RelPath}
		}
		e.event(event{Type: "scan", Total: len(docs), DocList: list})
		return
	}
	fmt.Printf("📚 知识库清单：共扫描到 %d 篇文档\n", len(docs))
	for i, d := range docs {
		fmt.Printf("  %3d. %s  (%s)\n", i+1, d.Title, d.RelPath)
	}
}

func (e *emitter) start(model string, perDoc, concurrency int, mix string) {
	if e.json {
		e.event(event{Type: "start", Model: model, TotalPerDoc: perDoc, Concurrency: concurrency, Mix: mix})
		return
	}
	fmt.Printf("\n🤖 开始生成问题（模型: %s，每篇 %d 条 [%s]，并发 %d）...\n", model, perDoc, mix, concurrency)
}

func (e *emitter) progress(done, total int, title string, count int) {
	if e.json {
		e.event(event{Type: "progress", Done: done, Total: total, Title: title, Count: count})
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	fmt.Printf("  [%d/%d] %s -> %d 条\n", done, total, truncateTitle(title), count)
}

func (e *emitter) warn(msg string) {
	if e.json {
		e.event(event{Type: "warn", Message: msg})
		return
	}
	fmt.Fprintf(os.Stderr, "  ⚠️ %s\n", msg)
}

func (e *emitter) done(questions int, output string) {
	if e.json {
		e.event(event{Type: "done", Questions: questions, Output: output})
		return
	}
	fmt.Printf("\n✅ 完成：共生成 %d 条问题，已写入 %s\n", questions, output)
}

func (e *emitter) fatal(msg string) {
	if e.json {
		e.event(event{Type: "error", Message: msg})
	} else {
		fmt.Fprintln(os.Stderr, "错误: "+msg)
	}
	os.Exit(1)
}

func truncateTitle(s string) string {
	r := []rune(s)
	if len(r) > 30 {
		return string(r[:30]) + "…"
	}
	return s
}
