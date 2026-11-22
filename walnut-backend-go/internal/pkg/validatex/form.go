// 本文件托管导出 form 校验的共享件（原 post/req.go 逐字迁入）：
// pydantic ValidationError 的明细结构与原文排版，post/user 的 export 端点共用。
// 只依赖标准库——DateErrType 按消息字符串匹配错误类型，不 import bindx（无导入环）。

package validatex

import (
	"fmt"
	"strings"
	"unicode"
)

// FormFieldError 一条 ValidationError 明细（Loc 用驼峰别名，与实测一致）。
// 导出供其他模块的 export 端点复用同款 pydantic 错误排版。
type FormFieldError struct {
	Loc   string
	Msg   string
	Typ   string
	Input string
}

// DateErrType 由 bindx.ParseDateQuery 的原文反推 pydantic 错误类型（实测钉住）：
// 零时间判定 → date_from_datetime_inexact；year 0 → date_parsing；其余 → date_from_datetime_parsing。
func DateErrType(msg string) string {
	switch {
	case strings.HasPrefix(msg, "Datetimes provided to dates"):
		return "date_from_datetime_inexact"
	case strings.HasPrefix(msg, "Input should be a valid date in the format"):
		return "date_parsing"
	default:
		return "date_from_datetime_parsing"
	}
}

// ValidationErrorText 复刻 pydantic ValidationError.__str__ 的排版（含错误详情 URL）。
func ValidationErrorText(model string, ferrs []FormFieldError) string {
	var b strings.Builder
	if len(ferrs) == 1 {
		b.WriteString("1 validation error for ")
	} else {
		fmt.Fprintf(&b, "%d validation errors for ", len(ferrs))
	}
	b.WriteString(model)
	for _, e := range ferrs {
		b.WriteString("\n")
		b.WriteString(e.Loc)
		b.WriteString("\n  ")
		b.WriteString(e.Msg)
		b.WriteString(" [type=")
		b.WriteString(e.Typ)
		b.WriteString(", input_value=")
		b.WriteString(PyRepr(e.Input))
		b.WriteString(", input_type=str]\n    For further information visit https://errors.pydantic.dev/2.13/v/")
		b.WriteString(e.Typ)
	}
	return b.String()
}

// PyRepr 复刻 Python repr(str)：优先单引号，含 ' 且不含 " 时改用双引号；
// 转义反斜杠与常见控制字符，可打印 Unicode（如中文）原样保留。
func PyRepr(s string) string {
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteRune(quote)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case quote:
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			switch {
			case r < 0x20 || r == 0x7f:
				fmt.Fprintf(&b, `\x%02x`, r)
			case !unicode.IsPrint(r):
				if r <= 0xffff {
					fmt.Fprintf(&b, `\u%04x`, r)
				} else {
					fmt.Fprintf(&b, `\U%08x`, r)
				}
			default:
				b.WriteRune(r)
			}
		}
	}
	b.WriteRune(quote)
	return b.String()
}
