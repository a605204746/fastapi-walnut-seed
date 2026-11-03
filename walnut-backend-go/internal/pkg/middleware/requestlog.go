package middleware

import (
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/logx"
)

// RequestLogConfig 请求日志配置。
type RequestLogConfig struct {
	// Logger 目标 logger；nil 时用 slog.Default()
	Logger *slog.Logger
	// SkipPaths 完全不记录的路径前缀。
	// 健康探针默认跳过：compose 每 2-10s 探一次，记下来会淹没真实流量
	SkipPaths []string
	// LogRequestBody 是否记录请求体（P1 接入 ApiDecrypt 后才有意义，
	// 且必须在解密**之后**记录，否则记的是密文）
	LogRequestBody bool
}

// RequestLog 记录每个请求的方法、路径、状态码、耗时与客户端 IP。
//
// 对齐 Python 端 RequestLogMiddleware。两个关键点：
//
//  1. **位置**：在链中位于 ApiDecrypt 与 XSS 之后，因此记录到的参数是解密后的明文。
//     装配顺序错了会记成密文，排障时完全无法使用。
//  2. **客户端 IP**：用 c.ClientIP()，它遵守 engine.SetTrustedProxies 的配置。
//     ⚠️ Gin 默认信任所有代理，若不显式设置 TrustedProxies，任何客户端都能
//     伪造 X-Forwarded-For 让日志与限流 IP 失真（见计划 R6）。
func RequestLog(cfg RequestLogConfig) gin.HandlerFunc {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	skips := make([]string, 0, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		if p != "" {
			skips = append(skips, p)
		}
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if shouldSkip(path, skips) {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		cost := time.Since(start)

		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("cost_ms", cost.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
			slog.Int("body_size", c.Writer.Size()),
		}
		if id := CorrelationIDFrom(c); id != "" {
			attrs = append(attrs, slog.String("request_id", id))
		}
		if raw := c.Request.URL.RawQuery; raw != "" {
			// 查询串同样可能带敏感参数，走一遍脱敏再记录。
			// logx.scrubAny 已内置 url.Values 分支，能按键名剔除 password 等字段
			if values, err := url.ParseQuery(raw); err == nil {
				attrs = append(attrs, slog.Any("query", logx.Scrub(values)))
			} else {
				attrs = append(attrs, slog.String("query", raw), slog.String("query_parse_err", err.Error()))
			}
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("errors", c.Errors.String()))
		}

		// 按状态码分级：5xx 记 Error，4xx 记 Warn，其余 Info
		switch {
		case c.Writer.Status() >= 500:
			logger.Error("[请求]", attrs...)
		case c.Writer.Status() >= 400:
			logger.Warn("[请求]", attrs...)
		default:
			logger.Info("[请求]", attrs...)
		}
	}
}

// shouldSkip 判断路径是否命中跳过前缀。
func shouldSkip(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
