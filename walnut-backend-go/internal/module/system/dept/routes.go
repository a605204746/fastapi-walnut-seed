// 本文件集中维护部门域的路由声明与守卫，handler.go 只保留请求处理函数。

package dept

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册部门路由。挂载点是 /system/dept（由 bootstrap 决定）。
//
// ⚠️ 权限串与 Python 端 AuthPermission(permissions=[...]) 逐条对应。
// ⚠️ 动态段路由（/:dept_id、/list/exclude/:dept_id）必须注册在静态路由之后。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:dept:list"), h.List)
	b.GET("/list/exclude/:dept_id", require("system:dept:list"), h.ListExclude)
	b.GET("/treeselect", require("system:dept:query"), h.TreeSelect)
	b.GET("/optionselect", require("system:dept:query"), h.OptionSelect)
	b.GET("/:dept_id", require("system:dept:query"), h.GetByID)
	b.POST("", require("system:dept:add"), repeat(), h.Add)
	b.PUT("", require("system:dept:edit"), repeat(), h.Update)
	b.DELETE("/:dept_id", require("system:dept:remove"), h.Delete)
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
