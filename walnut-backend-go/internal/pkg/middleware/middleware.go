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
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CorrelationHeader 关联 ID 的请求/响应头名，与 Python CorrelationIdMiddleware
// 的 "X-Correlation-ID" 逐字一致（注意 CORS expose 配置里写的是 X-Request-ID，
// 那是 Python 配置自身的历史遗留，两端 expose 列表保持相同字符串即可）。
const CorrelationHeader = "X-Correlation-ID"

// ctxKeyCorrelation 存入 gin.Context 的键。
const ctxKeyCorrelation = "correlation_id"

// CorrelationID 为每个请求分配关联 ID：优先复用上游传入的同名头，
// 缺失时生成 uuid4（与 Python str(uuid.uuid4()) 同形态）。回写响应头供前端排障。
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

// newCorrelationID 生成 uuid4 字符串（36 字符含连字符），与 Python 端一致。
// 随机源失败时退化为固定形态，确保永不返回空 ID（空值会让日志检索失去抓手）。
func newCorrelationID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
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

// CORS 逐条复刻 starlette CORSMiddleware 的可观测行为：
//   - 无 Origin → 完全不加 CORS 头，OPTIONS 也交给路由（405 信封）；
//   - 预检 = OPTIONS + Origin + Access-Control-Request-Method 三者齐备，
//     响应 200 "OK"（text/plain），失败 400 "Disallowed CORS ..."；
//   - AllowMethods 含 "*" 展开为 ALL_METHODS 显式列表；AllowHeaders 含 "*"
//     时回显请求的 Access-Control-Request-Headers（通配符在 credentials 模式下
//     不被浏览器接受，必须回显）；
//   - 简单响应只加 ACAO/ACAC/Expose/Vary:Origin，**不加 Max-Age**。
func CORS(cfg CORSConfig) gin.HandlerFunc {
	methods := cfg.AllowMethods
	for _, m := range methods {
		if m == "*" {
			methods = []string{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"}
			break
		}
	}
	allowMethods := strings.Join(methods, ", ")
	methodSet := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		methodSet[m] = struct{}{}
	}
	allowAllHeaders := false
	for _, h := range cfg.AllowHeaders {
		if h == "*" {
			allowAllHeaders = true
		}
	}
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := itoa(cfg.MaxAge)
	if cfg.MaxAge <= 0 {
		maxAge = "1800"
	}
	originSet := make(map[string]struct{}, len(cfg.AllowOrigins))
	for _, o := range cfg.AllowOrigins {
		if o != "" {
			originSet[o] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}
		h := c.Writer.Header()
		allowed := allowedOrigin(origin, cfg.AllowAllOrigins, originSet)

		// 预检请求
		if c.Request.Method == http.MethodOptions && c.Request.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", allowMethods)
			h.Set("Access-Control-Max-Age", maxAge)
			if cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			var failures []string
			if allowed {
				h.Set("Access-Control-Allow-Origin", origin)
			} else {
				failures = append(failures, "origin")
			}
			reqMethod := c.Request.Header.Get("Access-Control-Request-Method")
			if _, ok := methodSet[reqMethod]; !ok {
				failures = append(failures, "method")
			}
			reqHeaders := c.Request.Header.Get("Access-Control-Request-Headers")
			if allowAllHeaders {
				if reqHeaders != "" {
					h.Set("Access-Control-Allow-Headers", reqHeaders)
				}
			} else if reqHeaders != "" {
				h.Set("Access-Control-Allow-Headers", allowHeaders)
				for _, rh := range strings.Split(reqHeaders, ",") {
					if !headerAllowed(strings.TrimSpace(strings.ToLower(rh)), cfg.AllowHeaders) {
						failures = append(failures, "headers")
						break
					}
				}
			}
			if c.Request.Header.Get("Access-Control-Request-Private-Network") != "" {
				failures = append(failures, "private-network")
			}
			if len(failures) > 0 {
				c.Data(http.StatusBadRequest, "text/plain; charset=utf-8", []byte("Disallowed CORS "+strings.Join(failures, ", ")))
				c.Abort()
				return
			}
			c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte("OK"))
			c.Abort()
			return
		}

		// 简单响应：回显具体 Origin（credentials 模式唯一合法写法）
		if allowed {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			if cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			if exposeHeaders != "" {
				h.Set("Access-Control-Expose-Headers", exposeHeaders)
			}
		}
		c.Next()
	}
}

// headerAllowed 非通配模式下校验请求头是否在允许列表（含 starlette 安全列表头）。
func headerAllowed(h string, allow []string) bool {
	switch h {
	case "accept", "accept-language", "content-language", "content-type":
		return true
	}
	for _, a := range allow {
		if strings.ToLower(a) == h {
			return true
		}
	}
	return false
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
