// 本文件集中维护路由声明，handler.go 只保留请求处理函数。
package file

import (
	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册文件路由。挂载点是 /common/file。
func (h *Handler) Mount(b routex.Binder) {
	b.POST("/upload", h.Upload)
	b.GET("/download", h.Download)
	b.DELETE("/delete", h.Delete)
}
