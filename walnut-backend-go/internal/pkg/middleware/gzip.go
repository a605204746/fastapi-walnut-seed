package middleware

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// GZipConfig GZip 配置。
type GZipConfig struct {
	// MinSize 保留配置兼容。Python 端 minimum_size=1000 实际**失效**：
	// 内层 CorrelationId 是 BaseHTTPMiddleware，把所有响应变成流式
	// （more_body=True），starlette 的小响应跳过分支永远走不到——
	// 实测 70 字节的 401 信封也被压缩。故这里按可观测行为压缩全部响应。
	MinSize int
	// CompressLevel 压缩级别 1-9（对应 Python 端 compresslevel）。
	CompressLevel int
}

// gzipExcludedTypes 与 starlette DEFAULT_EXCLUDED_CONTENT_TYPES 逐条一致。
// 前缀匹配项（audio/、video/）对应 starlette 的 "audio/*"、"video/*"。
var gzipExcludedTypes = []string{
	"application/gzip",
	"application/x-gzip",
	"application/zip",
	"audio/",
	"font/woff",
	"font/woff2",
	"image/avif",
	"image/gif",
	"image/jpeg",
	"image/png",
	"image/webp",
	"text/event-stream",
	"video/",
}

// GZip 响应压缩中间件，对齐 Python CustomGZipMiddleware 的可观测行为：
//   - 所有响应都带 Vary: Accept-Encoding（实测无 Accept-Encoding 时也带）；
//   - 请求 Accept-Encoding 含 gzip 且响应类型不在排除列表 → 无条件压缩
//     （即使压缩后更大，starlette 不做收益判断）；
//   - SSE/图片等排除类型保持流式直通，不缓冲。
func GZip(cfg GZipConfig) gin.HandlerFunc {
	level := cfg.CompressLevel
	if level < 1 || level > 9 {
		level = gzip.BestCompression
	}
	return func(c *gin.Context) {
		c.Writer.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(c.Request.Header.Get("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}
		gw := &gzipWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}, level: level}
		c.Writer = gw
		c.Next()
		gw.finish()
	}
}

type gzipWriter struct {
	gin.ResponseWriter
	body        *bytes.Buffer
	level       int
	status      int
	decided     bool
	passthrough bool
}

// WriteHeader 只记录状态码：gin 的 c.Status() 先于 render 设置 Content-Type，
// 排除判定必须推迟到首次 Write/Flush（那时 CT 才就绪）。
func (w *gzipWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *gzipWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true
	if w.status == 0 {
		w.status = http.StatusOK
	}
	h := w.Header()
	if w.status == http.StatusPartialContent || h.Get("Content-Encoding") != "" || gzipExcluded(h.Get("Content-Type")) {
		w.passthrough = true
		w.ResponseWriter.WriteHeader(w.status)
	}
}

func (w *gzipWriter) WriteHeaderNow() {
	w.decide()
}

func (w *gzipWriter) Write(b []byte) (int, error) {
	w.decide()
	if w.passthrough {
		return w.ResponseWriter.Write(b)
	}
	return w.body.Write(b)
}

func (w *gzipWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *gzipWriter) Status() int {
	if w.status != 0 {
		return w.status
	}
	return http.StatusOK
}

func (w *gzipWriter) Written() bool {
	return w.decided
}

// Flush 直通模式（SSE）立即刷出；缓冲模式无操作（finish 统一写出）。
func (w *gzipWriter) Flush() {
	w.decide()
	if w.passthrough {
		w.ResponseWriter.Flush()
	}
}

func (w *gzipWriter) finish() {
	w.decide()
	if w.passthrough {
		return
	}
	body := w.body.Bytes()
	h := w.ResponseWriter.Header()
	if len(body) > 0 {
		var buf bytes.Buffer
		gz, err := gzip.NewWriterLevel(&buf, w.level)
		if err == nil {
			_, _ = gz.Write(body)
			_ = gz.Close()
			h.Set("Content-Encoding", "gzip")
			h.Set("Content-Length", strconv.Itoa(buf.Len()))
			w.ResponseWriter.WriteHeader(w.status)
			_, _ = w.ResponseWriter.Write(buf.Bytes())
			return
		}
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.ResponseWriter.WriteHeader(w.status)
	if len(body) > 0 {
		_, _ = w.ResponseWriter.Write(body)
	}
}

func gzipExcluded(ct string) bool {
	media := strings.TrimSpace(strings.ToLower(strings.SplitN(ct, ";", 2)[0]))
	if media == "" {
		return false
	}
	for _, ex := range gzipExcludedTypes {
		if strings.HasSuffix(ex, "/") {
			if strings.HasPrefix(media, ex) {
				return true
			}
		} else if media == ex {
			return true
		}
	}
	return false
}