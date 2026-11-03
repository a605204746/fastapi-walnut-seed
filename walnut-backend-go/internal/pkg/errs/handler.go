package errs

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/envelope"
)

// MessageResolver 由 i18nx 注入，把消息键解析为对应语言的文案。
// 为 nil 时使用 AuthErrorCode.Fallback（中文）。
// 用函数类型而非接口，避免 errs 反向依赖 i18n 包（也便于测试注入）。
type MessageResolver func(locale, key string, args ...any) string

// LocaleExtractor 从请求上下文取语言环境，由 middleware.LocaleFrom 注入。
//
// 用注入而非直接 import middleware：那样会让错误层耦合到 HTTP 层的具体实现，
// 注入函数则保持 errs 可独立测试。
type LocaleExtractor func(c *gin.Context) string

// DefaultLocaleFallback 未注入 LocaleOf 时使用的语言环境。
// 与 i18nx.DefaultLocale 一致（zh_CN）；此处硬编码而非 import，避免依赖环。
const DefaultLocaleFallback = "zh_CN"

// Handler 负责把异常渲染为信封响应，并承载全局兜底中间件。
type Handler struct {
	// ExposeDetail 为 true（非生产）时把 detail 写入 body.data，
	// 对齐 Python 端 service_exception_handler 里对 ENVIRONMENT != PROD 的判断。
	ExposeDetail bool
	// Resolve 可选的 i18n 解析器；nil 时用 AuthErrorCode.Fallback（中文）。
	Resolve MessageResolver
	// LocaleOf 可选的语言环境提取器；nil 时用 DefaultLocaleFallback。
	LocaleOf LocaleExtractor
	// Logger 用于记录异常；nil 时退化为 slog.Default()。
	Logger *slog.Logger
}

func (h *Handler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// localeOf 取当前请求的语言环境。
func (h *Handler) localeOf(c *gin.Context) string {
	if h.LocaleOf != nil {
		if loc := h.LocaleOf(c); loc != "" {
			return loc
		}
	}
	return DefaultLocaleFallback
}

// resolveMsg 若异常带 i18n 键则按请求语言解析，解析不出则用兜底文案。
//
// 回退逻辑对齐 Python 端 AppBaseException.message 的判断
// `if resolved and resolved != self.message_code`：
// i18nx 在**未命中时返回消息键本身**，故"返回值等于键"就等价于"没翻译"。
func (h *Handler) resolveMsg(c *gin.Context, se *ServiceError) string {
	if se.MessageKey == "" || h.Resolve == nil {
		return se.Msg
	}
	if resolved := h.Resolve(h.localeOf(c), se.MessageKey, se.KeyArgs...); resolved != "" && resolved != se.MessageKey {
		return resolved
	}
	return se.Msg
}

// Render 把任意 error 渲染为信封响应。
//
// 非 ServiceError 一律按 500 兜底，且**不把原始错误内容回传客户端**
// （对齐 Python 端 all_exception_handler：只返回"发生系统异常，请联系管理员"）。
func (h *Handler) Render(c *gin.Context, err error) {
	if err == nil {
		return
	}
	se := As(err)
	if se == nil {
		h.logger().Error("[未捕获异常]",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("err", err.Error()),
		)
		se = &ServiceError{Code: CodeError, Msg: MsgSystemError}
	} else {
		h.logger().Error("[业务异常]",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("code", se.Code),
			slog.String("msg", se.Msg),
		)
	}

	var data any
	if h.ExposeDetail && se.Detail != "" {
		data = se.Detail
	}
	envelope.FailWith(c, se.Status(), se.Code, h.resolveMsg(c, se), data)
}

// Abort 渲染并中止后续 handler（gin 中必须显式 Abort，否则链路会继续执行）。
func (h *Handler) Abort(c *gin.Context, err error) {
	h.Render(c, err)
	c.Abort()
}

// Abortf 是 Abort 的便捷封装，直接构造 code=500 的业务异常。
func (h *Handler) Abortf(c *gin.Context, format string, args ...any) {
	h.Abort(c, Newf(format, args...))
}

// Recovery 捕获 panic 并渲染为 500 信封。
//
// 必须存在：否则 panic 会让 gin 返回裸 500 空响应，前端拦截器会当成网络错误处理，
// 而不是走业务错误分支。
func (h *Handler) Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				h.logger().Error("[panic 恢复]",
					slog.String("method", c.Request.Method),
					slog.String("path", c.Request.URL.Path),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				// 已写过响应头时不能再写（gin 会告警），此时仅中止链路
				if c.Writer.Written() {
					c.Abort()
					return
				}
				envelope.FailWith(c, http.StatusOK, CodeError, MsgSystemError, nil)
				c.Abort()
			}
		}()
		c.Next()
	}
}

// NoRoute 处理未匹配路由：真实 HTTP 404 + 信封体。
// 对齐 Python 端 http_exception_handler 的 404 分支（msg="请求地址不存在"）。
func (h *Handler) NoRoute() gin.HandlerFunc {
	return func(c *gin.Context) {
		envelope.FailWith(c, http.StatusNotFound, CodeNotFound, MsgNotFound, nil)
	}
}

// NoMethod 处理方法不匹配：真实 HTTP 405 + 信封体。
// 对齐 Python 端 http_exception_handler 的 405 分支（msg=str(exc.detail)）。
func (h *Handler) NoMethod() gin.HandlerFunc {
	return func(c *gin.Context) {
		envelope.FailWith(c, http.StatusMethodNotAllowed, CodeBadMethod, http.StatusText(http.StatusMethodNotAllowed), nil)
	}
}
