// 本文件集中维护客户端域的路由声明与守卫，handler.go 只保留请求处理函数。

package client

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册客户端路由。挂载点是 /system/client（由 bootstrap 决定）。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:client:list"), h.List)
	b.POST("/export", require("system:client:export"), h.Export)
	b.GET("/:client_id", require("system:client:query"), h.GetByID)
	b.POST("", require("system:client:add"), repeat(), h.Add)
	b.PUT("", require("system:client:edit"), repeat(), h.Update)
	b.PUT("/changeStatus", require("system:client:edit"), h.ChangeStatus)
	b.DELETE("/:ids", require("system:client:remove"), h.Delete)
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
