package middleware

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// GZipConfig GZip 配置。
type GZipConfig struct {
	// MinSize 响应体小于此字节数不压缩（对应 Python 端 minimum_size）。
	MinSize int
	// CompressLevel 压缩级别 1-9（对应 Python 端 compresslevel）。
	CompressLevel int
}

// GZip 响应压缩中间件，对应 Python 端 CustomGZipMiddleware。
func GZip(cfg GZipConfig) gin.HandlerFunc {
	level := cfg.CompressLevel
	if level < 1 || level > 9 {
		level = gzip.BestCompression
	}
	opts := []gzip.Option{}
	if cfg.MinSize > 0 {
		opts = append(opts, gzip.WithMinLength(cfg.MinSize))
	}
	return gzip.Gzip(level, opts...)
}
