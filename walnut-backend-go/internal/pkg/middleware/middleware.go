// Package middleware 实现全局中间件链。
//
// 链顺序是**隐式契约**，必须与 Python 端 setting.py 的 MIDDLEWARE_LIST 一致
// （从外到内）：
//
//	HTTPSRedirect → TrustedHost → CORS → Locale → ApiDecrypt → XSS
//	  → RequestLog → GZip → CorrelationId → 业务路由
//
// ⚠️ Gin 的 Use() 调用顺序**就是**执行顺序，与 FastAPI「逆序叠加」相反。
// 移植时不要照抄 Python 的 [::-1] 逻辑。
//
// P0 只落地 CorrelationId、RequestLog、CORS 三个（其余在 P1/P2 补齐），
// 但装配顺序在 bootstrap/middleware.go 中已按完整链预留位置。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CorrelationHeader 关联 ID 的请求/响应头名。
// 必须出现在 CORS 的 expose headers 中，否则前端跨域读不到。
const CorrelationHeader = "X-Request-ID"

// ctxKeyCorrelation 存入 gin.Context 的键。
const ctxKeyCorrelation = "correlation_id"

// CorrelationID 为每个请求分配关联 ID：优先复用上游传入的 X-Request-ID，
// 缺失时生成 16 字节随机十六进制串。回写到响应头供前端排障。
//
// 对齐 Python 端 CorrelationIdMiddleware。注意它在链的**最内层**（紧邻业务路由），
// 因此外层中间件记日志时可能还没有关联 ID——这一行为两端一致，不要"优化"顺序。
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(CorrelationHeader)
		if id == "" {
			id = newCorrelationID()
		}
		c.Set(ctxKeyCorrelation, id)
		c.Writer.Header().Set(CorrelationHeader, id)
		c.Next()
	}
}

// CorrelationIDFrom 取出当前请求的关联 ID，无则返回空串。
func CorrelationIDFrom(c *gin.Context) string {
	v, ok := c.Get(ctxKeyCorrelation)
	if !ok {
		return ""
	}
	id, _ := v.(string)
	return id
}

// newCorrelationID 生成 32 字符十六进制串。
// 随机源失败时退化为时间戳形态，确保永不返回空 ID（空值会让日志检索失去抓手）。
func newCorrelationID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "0000000000000000deadbeefdeadbeef"
	}
	return hex.EncodeToString(buf)
}

// CORSConfig CORS 参数，由 config.Config 映射而来。
type CORSConfig struct {
	// AllowOrigins 显式允许的源列表；含 "*" 表示允许全部
	AllowOrigins []string
	// AllowAllOrigins 为 true 时回显请求的 Origin（配合 credentials 使用，
	// 因为 Access-Control-Allow-Origin: * 与 Allow-Credentials: true 互斥）
	AllowAllOrigins bool
	AllowMethods    []string
	AllowHeaders    []string
	// ExposeHeaders 必须包含 encrypt-key 与 X-Request-ID，
	// 否则前端跨域时读不到加密协商头与关联 ID
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// CORS 实现跨域处理。
//
// 不用 gin-contrib/cors：该库对「AllowCredentials + 通配源」的组合处理与
// Starlette 的 CORSMiddleware 存在差异（前者会拒绝，后者回显具体 Origin），
// 而本项目 ALLOW_CREDENTIALS 默认 true、ALLOW_ORIGINS 默认 *，必须回显 Origin。
func CORS(cfg CORSConfig) gin.HandlerFunc {
	allowMethods := joinOr(cfg.AllowMethods, "*")
	allowHeaders := joinOr(cfg.AllowHeaders, "*")
	exposeHeaders := joinOr(cfg.ExposeHeaders, "")
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = 1800
	}
	originSet := make(map[string]struct{}, len(cfg.AllowOrigins))
	for _, o := range cfg.AllowOrigins {
		if o != "" {
			originSet[o] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			if allowedOrigin(origin, cfg.AllowAllOrigins, originSet) {
				// 回显具体 Origin 而非 "*"：这是 credentials 模式下的唯一合法写法
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Add("Vary", "Origin")
				if cfg.AllowCredentials {
					c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if exposeHeaders != "" {
					c.Writer.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
				}
				c.Writer.Header().Set("Access-Control-Max-Age", itoa(maxAge))
			}
		}

		// 预检请求直接短路返回 204，与 Starlette 行为一致
		if c.Request.Method == http.MethodOptions {
			c.Writer.Header().Set("Access-Control-Allow-Methods", allowMethods)
			c.Writer.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// allowedOrigin 判断源是否放行。
func allowedOrigin(origin string, allowAll bool, set map[string]struct{}) bool {
	if allowAll {
		return true
	}
	if _, ok := set["*"]; ok {
		return true
	}
	_, ok := set[origin]
	return ok
}
