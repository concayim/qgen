package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config 描述问题生成器的全部运行参数。
type Config struct {
	// Model 大模型相关配置（OpenAI 兼容协议，可对接 火山方舟/vLLM/OpenAI 等）。
	Model ModelConfig `yaml:"model"`

	// KB 知识库扫描配置。
	KB KBConfig `yaml:"kb"`

	// Gen 问题生成策略。
	Gen GenConfig `yaml:"gen"`

	// Output 导出配置。
	Output OutputConfig `yaml:"output"`
}

type ModelConfig struct {
	BaseURL     string  `yaml:"base_url"`
	APIKey      string  `yaml:"api_key"`
	Model       string  `yaml:"model"`
	Temperature float32 `yaml:"temperature"`
	// MaxDocChars 单篇文档送入模型的最大字符数，超出截断，避免超长上下文。
	MaxDocChars int `yaml:"max_doc_chars"`
}

type KBConfig struct {
	// Roots 知识库根目录列表，递归扫描。
	Roots []string `yaml:"roots"`
	// Extensions 参与扫描的文件后缀（小写，含点）。
	Extensions []string `yaml:"extensions"`
	// ExcludeDirs 需要跳过的目录名。
	ExcludeDirs []string `yaml:"exclude_dirs"`
}

// IntentSpec 描述某一场景要生成的问题数及其展示用的场景名。
// Intent 取值与政务意图分类器一致：
//
//	intelligent_qa 智能问答（自然语言事实问答，检索定位）
//	draft_from_doc 以稿写稿（改写嵌入的原文片段）
//	doc_extraction 上传文件萃取（从本篇文档直接抽取/列出/归集）
//	info_writing   信息撰写（依据文档写新的正式文稿）
type IntentSpec struct {
	Intent string `yaml:"intent"`
	Scene  string `yaml:"scene"`
	Count  int    `yaml:"count"`
}

type GenConfig struct {
	// Intents 各意图要生成的问题数分布。
	Intents []IntentSpec `yaml:"intents"`
	// Concurrency 并发处理的文档数。
	Concurrency int `yaml:"concurrency"`
	// MaxDocs 最多处理的文档数，<=0 表示不限制。
	MaxDocs int `yaml:"max_docs"`
}

// TotalPerDoc 返回每篇文档生成的问题总数。
func (g GenConfig) TotalPerDoc() int {
	n := 0
	for _, it := range g.Intents {
		if it.Count > 0 {
			n += it.Count
		}
	}
	return n
}

type OutputConfig struct {
	// Path 导出的 xlsx 文件路径。
	Path string `yaml:"path"`
	// UploadDocLabel 模板"上传文档"列的默认值。
	UploadDocLabel string `yaml:"upload_doc_label"`
}

// Default 返回带合理默认值的配置。
func Default() *Config {
	return &Config{
		Model: ModelConfig{
			BaseURL:     "https://ark.cn-beijing.volces.com/api/v3",
			Model:       "",
			Temperature: 0.7,
			MaxDocChars: 12000,
		},
		KB: KBConfig{
			Roots:       []string{"."},
			Extensions:  []string{".md", ".txt", ".markdown"},
			ExcludeDirs: []string{".git", "node_modules", "qgen"},
		},
		Gen: GenConfig{
			Intents: []IntentSpec{
				{Intent: "intelligent_qa", Scene: "智能问答", Count: 4},
				{Intent: "draft_from_doc", Scene: "以稿写稿", Count: 2},
				{Intent: "doc_extraction", Scene: "上传文件萃取", Count: 2},
				{Intent: "info_writing", Scene: "信息撰写", Count: 2},
			},
			Concurrency: 4,
			MaxDocs:     0,
		},
		Output: OutputConfig{
			Path:           "知识库问题清单.xlsx",
			UploadDocLabel: "无（知识库检索）",
		},
	}
}

// Load 从 yaml 文件加载配置（文件可选），再用环境变量覆盖敏感/常变项。
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	if cfg.Model.APIKey == "" {
		return nil, fmt.Errorf("缺少模型 API Key：请在配置文件 model.api_key 或环境变量 QGEN_API_KEY/ARK_API_KEY 中设置")
	}
	if cfg.Model.Model == "" {
		return nil, fmt.Errorf("缺少模型名：请在配置文件 model.model 或环境变量 QGEN_MODEL 中设置")
	}
	if cfg.Gen.TotalPerDoc() == 0 {
		return nil, fmt.Errorf("gen.intents 为空或全部 count=0，请至少配置一个意图")
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("QGEN_BASE_URL"); v != "" {
		cfg.Model.BaseURL = v
	}
	// 兼容方舟常用变量名。
	if v := firstNonEmpty(os.Getenv("QGEN_API_KEY"), os.Getenv("ARK_API_KEY"), os.Getenv("OPENAI_API_KEY")); v != "" {
		cfg.Model.APIKey = v
	}
	if v := os.Getenv("QGEN_MODEL"); v != "" {
		cfg.Model.Model = v
	}
	if v := os.Getenv("QGEN_TEMPERATURE"); v != "" {
		if t, err := strconv.ParseFloat(v, 32); err == nil {
			cfg.Model.Temperature = float32(t)
		}
	}
	if v := os.Getenv("QGEN_OUTPUT"); v != "" {
		cfg.Output.Path = v
	}
	if v := os.Getenv("QGEN_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Gen.Concurrency = n
		}
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
