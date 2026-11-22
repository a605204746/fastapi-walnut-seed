// 本文件集中维护本模块的路由声明与权限/防重守卫，handler.go 只保留请求处理函数。

package menu

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册菜单路由。挂载点是 /system/menu（由 bootstrap 决定）。
//
// ⚠️ 动态段 /:menu_id 必须注册在所有静态路由与 /cascade/:menu_ids 之后。
func (h *Handler) Mount(b routex.Binder) {
	require, requireSuper, repeat := h.guards()

	b.GET("/getRouters", h.GetRouters)
	b.GET("/list", requireSuper("system:menu:list"), h.List)
	b.GET("/treeselect", require("system:menu:query"), h.TreeSelect)
	b.GET("/roleMenuTreeselect/:role_id", require("system:menu:query"), h.RoleMenuTreeSelect)
	b.GET("/:menu_id", requireSuper("system:menu:query"), h.GetByID)
	b.POST("", requireSuper("system:menu:add"), repeat(), h.Add)
	b.PUT("", requireSuper("system:menu:edit"), repeat(), h.Update)
	b.DELETE("/cascade/:menu_ids", requireSuper("system:menu:remove"), h.DeleteCascade)
	b.DELETE("/:menu_id", requireSuper("system:menu:remove"), h.Delete)
}

// guards 返回「权限守卫」「权限+超管角色守卫」「防重提交」三个中间件工厂。
//
// requireSuper 对应 Python 端 AuthPermission(permissions=[...], roles=[SUPER_ADMIN])：
// 角色校验先于权限校验，非超管即使持有权限串也必须 403（NotRole）。
// treeselect / roleMenuTreeselect 在 Python 端是 MenuQueryAuth（无角色要求），用 require。
func (h *Handler) guards() (func(string) gin.HandlerFunc, func(string) gin.HandlerFunc, func() gin.HandlerFunc) {
	pass := func(c *gin.Context) { c.Next() }
	require := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return pass
		}
		return h.perm.Require(perm)
	}
	requireSuper := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return pass
		}
		return h.perm.Check(permguard.Rule{
			Permissions: []string{perm},
			Roles:       []string{modelx.SuperAdminRoleKey},
		})
	}
	repeat := func() gin.HandlerFunc {
		if h.idem == nil {
			return pass
		}
		return h.idem.Default()
	}
	return require, requireSuper, repeat
}
