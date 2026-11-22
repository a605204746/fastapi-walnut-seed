// 本文件集中维护路由声明，handler.go 只保留请求处理函数。
package social

import (
	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册路由。挂载点是 /system/social（Python 端 router 前缀自带 /system）。
func (h *Handler) Mount(b routex.Binder) {
	b.GET("/list", h.List)
}
