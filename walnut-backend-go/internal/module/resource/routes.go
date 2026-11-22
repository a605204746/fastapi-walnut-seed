// 本文件集中维护路由声明，handler.go 只保留请求处理函数。
package resource

import (
	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册资源路由。/sse 需要登录，/sse/close 在白名单内无需登录。
// /websocket 在白名单内（浏览器 WebSocket 无法设自定义头）。
func (h *Handler) Mount(protected routex.Binder, public routex.Binder) {
	protected.GET("/sse", h.SSEConnect)
	public.GET("/sse/close", h.SSEClose)
	public.GET("/websocket", h.WebSocket)
}
