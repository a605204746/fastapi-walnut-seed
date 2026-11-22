// 本文件集中维护本模块的路由声明与权限/防重守卫，handler.go 只保留请求处理函数。

package post

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册岗位路由。挂载点是 /system/post（由 bootstrap 决定）。
//
// ⚠️ 每个端点都必须挂权限守卫：前端隐藏按钮不是权限控制。
// 权限串与 Python 端 AuthPermission(permissions=[...]) 逐条对应。
//
// 动态段路由（/:post_id）必须注册在全部静态路由之后，避免 gin 路由树冲突。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:post:list"), h.List)
	b.POST("/export", require("system:post:export"), h.Export)
	b.GET("/optionselect", require("system:post:query"), h.OptionSelect)
	b.GET("/deptTree", require("system:post:list"), h.DeptTree)
	b.GET("/:post_id", require("system:post:query"), h.GetByID)
	b.POST("", require("system:post:add"), repeat(), h.Add)
	b.PUT("", require("system:post:edit"), repeat(), h.Update)
	b.DELETE("/:post_ids", require("system:post:remove"), h.Delete)
}

// guards 返回「权限守卫」与「防重提交」两个中间件工厂。
//
// 依赖为 nil 时返回放行中间件，让 handler 能脱离 bootstrap 做单元测试。
// ⚠️ 生产装配路径上两者都非 nil（见 bootstrap.mountSystemRoutes）；
// 若这里静默放行掩盖了装配遗漏，契约 diff 的 403 用例会立刻暴露。
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
