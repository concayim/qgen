// Package gen 使用 eino 编排（ChatTemplate -> ChatModel）基于文档按意图生成问题。
package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"qgen/internal/config"
	"qgen/internal/kb"
)

// Question 表示一条生成的问题。
type Question struct {
	Round      int    // 轮次
	Scene      string // 场景（展示名，对应意图类别）
	Question   string // 问题/指令
	UploadDoc  string // 上传文档
	SourceDoc  string // 来源文档
	AnswerHint string // 答案位置/改写要点
}

// rawQuestion 对应模型返回的单条问题。
type rawQuestion struct {
	Question   string `json:"question"`
	AnswerHint string `json:"answer_hint"`
}

// Generator 封装 eino runnable 与生成策略。
type Generator struct {
	runnable compose.Runnable[map[string]any, *schema.Message]
	cfg      *config.Config
}

// New 构建一个基于 eino Chain 的问题生成器。
func New(ctx context.Context, cfg *config.Config) (*Generator, error) {
	modelCfg := &openai.ChatModelConfig{
		BaseURL: cfg.Model.BaseURL,
		APIKey:  cfg.Model.APIKey,
		Model:   cfg.Model.Model,
	}
	// temperature <= 0 视为"不发送"，以兼容 gpt-5.x 等强制固定采样参数的 beta 模型。
	if cfg.Model.Temperature > 0 {
		temp := cfg.Model.Temperature
		modelCfg.Temperature = &temp
	}
	chatModel, err := openai.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, fmt.Errorf("初始化 ChatModel 失败: %w", err)
	}

	tpl := prompt.FromMessages(schema.GoTemplate,
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userPromptTmpl),
	)

	// eino Chain：模板渲染 -> 大模型生成。
	chain := compose.NewChain[map[string]any, *schema.Message]()
	chain.AppendChatTemplate(tpl)
	chain.AppendChatModel(chatModel)

	runnable, err := chain.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("编译 eino Chain 失败: %w", err)
	}

	return &Generator{runnable: runnable, cfg: cfg}, nil
}

// Generate 针对单篇文档按配置的意图分布生成问题列表。
func (g *Generator) Generate(ctx context.Context, doc *kb.Document) ([]Question, error) {
	input := map[string]any{
		"title":     doc.Title,
		"source":    doc.RelPath,
		"content":   doc.Content,
		"truncated": doc.Truncated,
		"intents":   g.cfg.Gen.Intents,
	}

	msg, err := g.runnable.Invoke(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("模型生成失败: %w", err)
	}

	buckets, err := parseBuckets(msg.Content)
	if err != nil {
		return nil, fmt.Errorf("解析模型输出失败: %w（原始输出：%s）", err, truncateForErr(msg.Content))
	}

	var out []Question
	// 按配置意图顺序汇总，并裁剪到各自要求的条数。
	for _, spec := range g.cfg.Gen.Intents {
		if spec.Count <= 0 {
			continue
		}
		items := buckets[spec.Intent]
		for i, r := range items {
			if i >= spec.Count {
				break
			}
			q := strings.TrimSpace(r.Question)
			if q == "" {
				continue
			}
			// 上传文件萃取：用户上传了该文档，"上传文档"列应标明来源文档；其余场景为知识库检索。
			uploadDoc := g.cfg.Output.UploadDocLabel
			if spec.Intent == "doc_extraction" {
				uploadDoc = doc.Title
			}
			out = append(out, Question{
				Scene:      spec.Scene,
				Question:   q,
				UploadDoc:  uploadDoc,
				SourceDoc:  doc.Title,
				AnswerHint: strings.TrimSpace(r.AnswerHint),
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("模型未返回任何有效问题")
	}
	return out, nil
}

// parseBuckets 从模型文本输出中稳健提取「意图 -> 问题数组」的 JSON 对象。
func parseBuckets(content string) (map[string][]rawQuestion, error) {
	s := stripCodeFence(strings.TrimSpace(content))

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		var m map[string][]rawQuestion
		if err := json.Unmarshal([]byte(s[start:end+1]), &m); err == nil && len(m) > 0 {
			return m, nil
		}
	}
	return nil, fmt.Errorf("未找到合法的 JSON 意图对象")
}

func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

func truncateForErr(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return string(r)
}
