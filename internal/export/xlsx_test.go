package export

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"qgen/internal/gen"
)

func TestWriteXLSX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.xlsx")

	qs := []gen.Question{
		{Round: 1, Scene: "信息解读", Question: "员工宿舍租住时间最长不得超过多少年？", UploadDoc: "无（知识库检索）", SourceDoc: "员工宿舍管理细则", AnswerHint: "第3条"},
		{Round: 2, Scene: "以稿写稿", Question: "把以下原文压缩为150字动态信息：……", UploadDoc: "无（知识库检索）", SourceDoc: "员工宿舍管理细则"},
	}
	if err := WriteXLSX(path, qs); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("文件未生成: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("打开失败: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("问题清单")
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(rows) != 3 { // 1 表头 + 2 数据
		t.Fatalf("行数不符: got %d want 3", len(rows))
	}
	if rows[0][0] != "轮次" || rows[0][2] != "问题" || rows[0][5] != "答案位置提示" {
		t.Fatalf("表头不符: %v", rows[0])
	}
	if rows[1][2] != qs[0].Question || rows[1][1] != "信息解读" {
		t.Fatalf("内容不符: %v", rows[1])
	}
}
