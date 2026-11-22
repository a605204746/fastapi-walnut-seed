// 本文件集中维护本模块的路由声明与权限/防重守卫，handler.go 只保留请求处理函数。

package notice

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册通知公告路由。挂载点是 /system/notice（由 bootstrap 决定）。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:notice:list"), h.List)
	b.GET("/:notice_id", require("system:notice:query"), h.GetByID)
	b.POST("", require("system:notice:add"), repeat(), h.Add)
	b.PUT("", require("system:notice:edit"), repeat(), h.Update)
	b.DELETE("/:notice_ids", require("system:notice:remove"), h.Delete)
}

// guards 返回「权限守卫」与「防重提交」两个中间件工厂（同 post/routes.go）。
func (h *Handler) guards() (func(string) gin.HandlerFunc, func() gin.HandlerFunc) {
	pass := func(c *gin.Context) { c.Next() }
	require := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return pass
		}
		return h.perm.Require(perm)
	}
	repeat := func() gin.HandlerFunc {
		if h.idem == nil {
			return pass
		}
		return h.idem.Default()
	}
	return require, repeat
}
