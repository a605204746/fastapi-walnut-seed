package middleware

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// XSSConfig XSS 过滤配置。
type XSSConfig struct {
	// Enabled 总开关。
	Enabled bool
	// ExcludeURLs 排除路径前缀（支持 * 通配符后缀，去掉 * 后做前缀匹配）。
	ExcludeURLs []string
}

// reStripHTML 与 Python 端 xss_util.RE_STRIP_HTML = re.compile(r"<[^>]+>", re.IGNORECASE) 逐字一致。
var reStripHTML = regexp.MustCompile(`(?i)<[^>]+>`)

// XSS 过滤中间件，对应 Python 端 XssMiddleware。
//
// GET / DELETE 不过滤；命中排除路径不过滤；其余请求对 application/json
// 请求体剥离全部 HTML 标签（正则全剥离，与 Python 端 clean_html_tag 一致）。
func XSS(cfg XSSConfig) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "GET" || method == "DELETE" {
			c.Next()
			return
		}
		if isExcluded(c.Request.URL.Path, cfg.ExcludeURLs) {
			c.Next()
			return
		}
		ct := c.ContentType()
		if !strings.HasPrefix(ct, "application/json") {
			c.Next()
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil || len(body) == 0 {
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			c.Next()
			return
		}
		cleaned := reStripHTML.ReplaceAll(body, nil)
		cleaned = bytes.TrimSpace(cleaned)
		c.Request.Body = io.NopCloser(bytes.NewReader(cleaned))
		c.Request.ContentLength = int64(len(cleaned))
		c.Next()
	}
}

// isExcluded 路径是否在排除列表内。
func isExcluded(path string, excludes []string) bool {
	for _, item := range excludes {
		if item == "" {
			continue
		}
		prefix := strings.TrimSuffix(item, "*")
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
