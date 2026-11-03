// Package excelx 实现 Excel 导出，对应 Python 端 app/utils/excel_util.py。
//
// # 字节不可能逐字节一致，契约比的是「响应头 + 解析后的单元格」
//
// Python 用 openpyxl、Go 用 excelize，两者生成的 xlsx 是不同的 zip
// （压缩级别、内部条目顺序、样式表 XML 都不同）。故导出接口的契约断言只能是：
//
//   - HTTP 200
//   - Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//   - Content-Disposition / download-filename 的**编码形态**一致
//   - Access-Control-Expose-Headers 一致
//   - 用各自的解析器读回后，**表头与单元格内容逐格一致**
//
// 归一化规则同验证码图片（diff 脚本把它替换成占位符），见 docs/go/01-续作交接.md §6。
package excelx

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// ContentType 是 xlsx 的 MIME，对应 ExcelUtil.EXCEL_CONTENT_TYPE。
const ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ExposeHeaders 对应 Python 端显式设置的 Access-Control-Expose-Headers。
//
// ⚠️ 必须设置：前端下载组件要读 Content-Disposition 与 download-filename 来定名，
// 浏览器默认不暴露这两个跨域响应头，缺了就拿不到文件名。
const ExposeHeaders = "Content-Disposition,download-filename"

// 表头样式，对应 openpyxl 的 PatternFill(start_color="ababab") + 居中 + 列宽 16。
const (
	headerFill  = "ABABAB"
	headerAlign = "center"
	columnWidth = 16.0
	// TemplateColumnWidth 是导入模板的列宽（get_excel_template 用 14，与导出的 16 不同）
	TemplateColumnWidth = 14.0
)

// Column 是导出的一列：字段取值函数 + 中文表头。
//
// 用「取值函数」而不是「字段名 + 反射」：Python 端是 header_mapping 的
// **键序即列序**（dict 保序），Go 用切片天然保序，且取值逻辑显式可见，
// 不会因结构体字段重排而悄悄改变列序。
type Column struct {
	// Header 中文表头，必须与 Python 端 _EXPORT_HEADERS 的值逐字一致
	Header string
	// Value 取单元格值。返回 nil 表示空单元格（Python 端 _fmt_cell 把 None 转空串）
	Value func(row any) any
}

// Build 生成工作簿字节，对应 ExcelUtil.build_workbook。
func Build(rows []any, cols []Column, sheetName string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// excelize 默认工作表名是 Sheet1，需改成业务名（Python 端 ws.title = sheet_name）
	const defaultSheet = "Sheet1"
	if sheetName != defaultSheet {
		if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
			return nil, fmt.Errorf("重命名工作表失败: %w", err)
		}
	}

	// 表头样式：灰底 + 居中 + 列宽 16
	styleID, err := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{headerFill}},
		Alignment: &excelize.Alignment{Horizontal: headerAlign},
	})
	if err != nil {
		return nil, fmt.Errorf("构造表头样式失败: %w", err)
	}

	for i, col := range cols {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return nil, err
		}
		if err := f.SetCellValue(sheetName, cell, col.Header); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(sheetName, cell, cell, styleID); err != nil {
			return nil, err
		}
		letter, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, err
		}
		if err := f.SetColWidth(sheetName, letter, letter, columnWidth); err != nil {
			return nil, err
		}
	}

	for r, row := range rows {
		for c, col := range cols {
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			if err != nil {
				return nil, err
			}
			if err := f.SetCellValue(sheetName, cell, cellValue(col.Value(row))); err != nil {
				return nil, err
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("写出工作簿失败: %w", err)
	}
	return buf.Bytes(), nil
}

// cellValue 对应 Python 端 _fmt_cell：None → 空串，时间 → 展示格式字符串。
//
// ⚠️ 时间必须转成**字符串**再写入。若让 excelize 写成日期类型，
// 单元格存的是序列号 + 数字格式，读回时与 Python 端的字符串不一致。
func cellValue(v any) any {
	switch t := v.(type) {
	case nil:
		return ""
	case time.Time:
		return t.Format("2006-01-02 15:04:05")
	case *time.Time:
		if t == nil {
			return ""
		}
		return t.Format("2006-01-02 15:04:05")
	default:
		return v
	}
}

// Export 写出下载响应，对应 ExcelUtil.export_excel_response。
//
// 文件名规则：`{sheetName}_{yyyy-MM-dd HH:mm:ss}.xlsx`（date_time_now 的默认格式）。
func Export(c *gin.Context, rows []any, cols []Column, sheetName string) error {
	data, err := Build(rows, cols, sheetName)
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("%s_%s.xlsx", sheetName, time.Now().Format("2006-01-02 15:04:05"))
	encoded := Quote(filename)

	c.Header("Content-Disposition", "attachment;filename*=utf-8''"+encoded)
	c.Header("download-filename", encoded)
	c.Header("Access-Control-Expose-Headers", ExposeHeaders)
	c.Data(200, ContentType, data)
	return nil
}

// Quote 复刻 Python 的 urllib.parse.quote(s)（默认 safe='/'）。
//
// 与 Go 标准库的差异只有一处需要处理：**空格**。
//   - Python quote → %20
//   - url.QueryEscape → +
//
// 两者的「不转义集合」都是 字母/数字/`-_.~`，故把 `+` 换回 `%20` 即完全一致。
// `*` 在两端都会被转义成 %2A，无需特殊处理。
func Quote(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
