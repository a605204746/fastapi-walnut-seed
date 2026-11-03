package middleware

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/i18nx"
)

// ctxKeyLocale 存入 gin.Context 的语言环境键。
const ctxKeyLocale = "locale"

// Locale 按请求头解析语言环境并写入 context，对齐 Python 端 LocaleMiddleware。
//
// 两处都写：
//   - c.Request.Context()：供下游**接受 ctx 的函数**读取（service / repo 层）
//   - c.Set(ctxKeyLocale)：供 gin handler 直接读取，免去 ctx 取值断言
//
// Python 端用 ContextVar 自动传播；Go 必须显式把新 ctx 塞回 c.Request，
// 否则下游 c.Request.Context() 拿到的还是没有 locale 的旧 ctx——
// 这是 ASGI ContextVar 移植到 Go 时最容易漏的一步。
//
// 无需在请求结束时"重置"：每个请求有独立的 context，随请求结束自然回收，
// 不存在 Python 端 ContextVar 复用导致的上下文串用问题。
func Locale() gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := i18nx.ResolveLocale(
			c.GetHeader("Accept-Language"),
			c.GetHeader("Content-Language"),
		)
		c.Request = c.Request.WithContext(i18nx.WithLocale(c.Request.Context(), locale))
		c.Set(ctxKeyLocale, locale)
		c.Next()
	}
}

// LocaleFrom 取出当前请求的语言环境；未设置时返回默认 zh_CN。
func LocaleFrom(c *gin.Context) string {
	if v, ok := c.Get(ctxKeyLocale); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return i18nx.DefaultLocale
}

// T 按当前请求语言解析消息键，args 填充 {0}/{1} 占位符。
//
// 业务层写错误文案时应统一走这个函数，而不是硬编码中文——
// 否则 en_US 请求会拿到中文提示，与另两端行为不一致。
func T(c *gin.Context, code string, args ...any) string {
	return i18nx.MessageIn(LocaleFrom(c), code, args...)
}
