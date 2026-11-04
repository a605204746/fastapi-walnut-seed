package excelx

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// BuildTemplate 生成导入模板工作簿，对应 Python 端 ExcelUtil.get_excel_template。
//
// headers: 列序即表头序（与 _IMPORT_HEADERS 一致）
// selectorHeaders: 需要下拉验证的列名（如 "用户性别"、"账号状态"）
// options: 与 selectorHeaders 一一对应的选项列表（如 {"男","女","未知"}）
func BuildTemplate(headers []string, selectorHeaders []string, options [][]string, sheetName string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const defaultSheet = "Sheet1"
	if sheetName != defaultSheet {
		if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
			return nil, fmt.Errorf("重命名工作表失败: %w", err)
		}
	}

	// 表头样式：灰底 + 居中 + 列宽 14
	styleID, err := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{headerFill}},
		Alignment: &excelize.Alignment{Horizontal: headerAlign},
	})
	if err != nil {
		return nil, fmt.Errorf("构造表头样式失败: %w", err)
	}

	// 建立 selectorHeader → options 的映射
	selectorMap := make(map[string][]string, len(selectorHeaders))
	for i, sh := range selectorHeaders {
		if i < len(options) {
			selectorMap[sh] = options[i]
		}
	}

	for i, header := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return nil, err
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(sheetName, cell, cell, styleID); err != nil {
			return nil, err
		}
		letter, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, err
		}
		if err := f.SetColWidth(sheetName, letter, letter, TemplateColumnWidth); err != nil {
			return nil, err
		}

		// 为 selector 列添加下拉验证（rows 2–1001）
		if opts, ok := selectorMap[header]; ok && len(opts) > 0 {
			formula := strings.Join(opts, ",")
			if err := f.AddDataValidation(sheetName, &excelize.DataValidation{
				Type:             "list",
				AllowBlank:       true,
				Sqref:            fmt.Sprintf("%s2:%s1001", letter, letter),
				Formula1:         formula,
				ShowInputMessage: true,
				ShowErrorMessage: true,
			}); err != nil {
				return nil, fmt.Errorf("添加数据验证失败: %w", err)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("写出工作簿失败: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportTemplate 写出导入模板下载响应。
func ExportTemplate(c *gin.Context, headers []string, selectorHeaders []string, options [][]string, sheetName string) error {
	data, err := BuildTemplate(headers, selectorHeaders, options, sheetName)
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

// ReadRows 读取上传的 Excel 工作簿，返回按表头索引的行数据。
// 跳过全空行。对应 Python 端 ExcelUtil.read_excel_to_dicts。
func ReadRows(data []byte) ([][]string, []string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("打开工作簿失败: %w", err)
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, nil, fmt.Errorf("读取行失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}

	headers := rows[0]
	dataRows := make([][]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		// 跳过全空行
		allEmpty := true
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}
		// 补齐列数（excelize 会省略尾部空列）
		for len(row) < len(headers) {
			row = append(row, "")
		}
		dataRows = append(dataRows, row)
	}
	return dataRows, headers, nil
}
