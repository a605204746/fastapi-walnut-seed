package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// QuerySemicolonCompat 复刻 Starlette 对查询串中分号的语义。
//
// Go 1.17 起 net/url.ParseQuery 把 ';' 当非法分隔符，**整个查询串解析失败**，
// gin 的 c.Query 随之全部返回空串；而 Python 的 parse_qsl 把 ';' 当值内普通
// 字符（"orderByColumn=a;DROP" 原样到达业务层，再由排序字符集校验拦下）。
// 不补这一层，带分号的请求在 Go 端会静默丢掉所有查询参数（过滤/排序全失效）。
//
// 必须在任何读取查询参数的中间件/handler 之前执行（gin 惰性解析并缓存结果）。
func QuerySemicolonCompat() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.Contains(c.Request.URL.RawQuery, ";") {
			c.Request.URL.RawQuery = strings.ReplaceAll(c.Request.URL.RawQuery, ";", "%3B")
		}
		c.Next()
	}
}