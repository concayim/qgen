// Package kb 负责扫描知识库清单并读取文档正文。
package kb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"qgen/internal/config"
)

// Document 表示知识库中的一篇文档。
type Document struct {
	// Title 文档标题，用于"来源文档"列：优先取正文首个一级标题，否则取去后缀的文件名。
	Title string
	// Path 文件绝对/相对路径。
	Path string
	// RelPath 相对扫描根目录的路径，便于展示来源。
	RelPath string
	// Content 文档正文（可能已按 MaxDocChars 截断）。
	Content string
	// Truncated 标记正文是否被截断。
	Truncated bool
}

var (
	// 匹配 markdown 一级/二级标题作为标题候选。
	headingRe = regexp.MustCompile(`(?m)^#{1,2}\s+(.+?)\s*$`)
	// 折叠多余空白行。
	multiBlank = regexp.MustCompile(`\n{3,}`)
)

// Scan 按配置递归扫描知识库根目录，返回文档清单（仅元信息，不含正文）。
func Scan(cfg *config.KBConfig) ([]*Document, error) {
	extSet := make(map[string]struct{}, len(cfg.Extensions))
	for _, e := range cfg.Extensions {
		extSet[strings.ToLower(e)] = struct{}{}
	}
	excludeSet := make(map[string]struct{}, len(cfg.ExcludeDirs))
	for _, d := range cfg.ExcludeDirs {
		excludeSet[d] = struct{}{}
	}

	var docs []*Document
	seen := make(map[string]struct{})

	for _, root := range cfg.Roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("解析知识库根目录失败 %q: %w", root, err)
		}
		walkErr := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if _, skip := excludeSet[d.Name()]; skip {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if _, ok := extSet[ext]; !ok {
				return nil
			}
			if _, dup := seen[path]; dup {
				return nil
			}
			seen[path] = struct{}{}

			rel, relErr := filepath.Rel(absRoot, path)
			if relErr != nil {
				rel = path
			}
			docs = append(docs, &Document{
				Path:    path,
				RelPath: rel,
				Title:   titleFromFilename(path),
			})
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("扫描知识库失败 %q: %w", absRoot, walkErr)
		}
	}
	return docs, nil
}

// Load 读取文档正文并填充到 doc 中，按 maxChars 截断。
func Load(doc *Document, maxChars int) error {
	raw, err := os.ReadFile(doc.Path)
	if err != nil {
		return fmt.Errorf("读取文档失败 %q: %w", doc.Path, err)
	}
	content := normalize(string(raw))

	// 用正文首个标题覆盖标题（更贴近真实文档名）。
	if h := headingRe.FindStringSubmatch(content); len(h) == 2 {
		if t := strings.TrimSpace(h[1]); t != "" {
			doc.Title = t
		}
	}

	if maxChars > 0 {
		runes := []rune(content)
		if len(runes) > maxChars {
			content = string(runes[:maxChars])
			doc.Truncated = true
		}
	}
	doc.Content = content
	return nil
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = multiBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func titleFromFilename(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	// 去掉常见的日期后缀，如 -2026-04-01。
	name = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`).ReplaceAllString(name, "")
	return strings.TrimSpace(name)
}
