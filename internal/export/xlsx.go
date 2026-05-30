// Package export 将生成的问题写入与测试模板一致的 xlsx。
package export

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"qgen/internal/gen"
)

// 列顺序对齐"文档提问20条-测试模板.xlsx"。
var headers = []string{
	"轮次", "场景", "问题", "上传文档", "来源文档", "答案位置提示",
}

// WriteXLSX 将问题列表写入指定路径的 xlsx 文件。
func WriteXLSX(path string, questions []gen.Question) error {
	f := excelize.NewFile()
	defer f.Close()

	const sheet = "问题清单"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return fmt.Errorf("创建工作表失败: %w", err)
	}
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"D9E1F2"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return fmt.Errorf("创建表头样式失败: %w", err)
	}
	bodyStyle, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	})
	if err != nil {
		return fmt.Errorf("创建正文样式失败: %w", err)
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	_ = f.SetCellStyle(sheet, "A1", lastCol(1), headerStyle)

	for r, q := range questions {
		row := r + 2
		round := q.Round
		if round == 0 {
			round = r + 1
		}
		values := []any{round, q.Scene, q.Question, q.UploadDoc, q.SourceDoc, q.AnswerHint}
		for c, v := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	if len(questions) > 0 {
		_ = f.SetCellStyle(sheet, "A2", lastCol(len(questions)+1), bodyStyle)
	}

	// 设置列宽，问题/来源列更宽。
	widths := map[string]float64{"A": 8, "B": 12, "C": 60, "D": 20, "E": 40, "F": 40}
	for col, w := range widths {
		_ = f.SetColWidth(sheet, col, col, w)
	}

	if err := f.SaveAs(path); err != nil {
		return fmt.Errorf("保存 xlsx 失败: %w", err)
	}
	return nil
}

func lastCol(row int) string {
	cell, _ := excelize.CoordinatesToCellName(len(headers), row)
	return cell
}
