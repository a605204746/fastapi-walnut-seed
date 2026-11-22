package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// QueryCompat 复刻 Starlette 对查询串/表单的两处语义差异：
//
//  1. 分号：Go 1.17 起 net/url.ParseQuery 把 ';' 当非法分隔符，**整个查询串
//     解析失败**，gin 的 c.Query 随之全部返回空串；Python parse_qsl 把 ';'
//     当值内普通字符。这里先转义为 %3B。
//
//  2. 重复键：Starlette 的 QueryParams/FormData 是 multidict，.get 取**最后一个**
//     值；Go url.Values.Get / gin c.Query / c.PostForm 取**第一个**。这里把
//     查询串与表单值都归一为「每键保留末值」。
//
// 必须在任何读取查询参数/表单的中间件之前执行（gin 惰性解析并缓存结果）。
// 带 encrypt-key 的请求跳过表单预解析：body 此时还是密文，ApiDecrypt 解密后
// 会把 Content-Type 改成 application/json，表单语义不再适用。
func QueryCompat() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rq := c.Request.URL.RawQuery; rq != "" {
			c.Request.URL.RawQuery = compatRawQuery(rq)
		}

		method := c.Request.Method
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
			ct := c.ContentType()
			formLike := strings.HasPrefix(ct, "application/x-www-form-urlencoded") || strings.HasPrefix(ct, "multipart/form-data")
			if formLike && c.GetHeader("encrypt-key") == "" && c.Request.Body != nil {
				bodyBytes, err := io.ReadAll(c.Request.Body)
				if err == nil {
					c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					if strings.HasPrefix(ct, "multipart/form-data") {
						_ = c.Request.ParseMultipartForm(32 << 20)
					} else {
						_ = c.Request.ParseForm()
					}
					lastValueWins(c.Request.PostForm)
					lastValueWins(c.Request.Form)
					if c.Request.MultipartForm != nil {
						lastValueWins(c.Request.MultipartForm.Value)
					}
					// 还原 body 供下游（操作日志采集）读取原文
					c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}
		}
		c.Next()
	}
}

// compatRawQuery 转义分号并按键保留末值（保留原始编码与键序）。
func compatRawQuery(rq string) string {
	rq = strings.ReplaceAll(rq, ";", "%3B")
	pairs := strings.Split(rq, "&")
	lastIdx := make(map[string]int, len(pairs))
	for i, p := range pairs {
		if p == "" {
			continue
		}
		key := p
		if eq := strings.IndexByte(p, '='); eq >= 0 {
			key = p[:eq]
		}
		lastIdx[key] = i
	}
	var b strings.Builder
	for i, p := range pairs {
		if p == "" {
			continue
		}
		key := p
		if eq := strings.IndexByte(p, '='); eq >= 0 {
			key = p[:eq]
		}
		if lastIdx[key] != i {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(p)
	}
	return b.String()
}

// lastValueWins 把 url.Values 归一为每键单值（末值）。
func lastValueWins(v map[string][]string) {
	for k, vals := range v {
		if len(vals) > 1 {
			v[k] = vals[len(vals)-1:]
		}
	}
}