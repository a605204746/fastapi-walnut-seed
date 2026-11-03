package middleware

import (
	"strconv"
	"strings"
)

// joinOr 把切片拼成逗号分隔串；结果为空时返回 fallback。
// 用于 CORS 头拼装——空值不应写出 "Access-Control-Allow-Methods: " 这样的空头。
func joinOr(parts []string, fallback string) string {
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		return fallback
	}
	return strings.Join(cleaned, ", ")
}

func itoa(n int) string { return strconv.Itoa(n) }
