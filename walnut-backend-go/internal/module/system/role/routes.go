// 本文件集中维护路由声明与权限/防重守卫，handler.go 只保留请求处理函数。
package role

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:role:list"), h.List)
	b.GET("/optionselect", require("system:role:query"), h.OptionSelect)
	b.GET("/authUser/allocatedList", require("system:role:list"), h.AllocatedList)
	b.GET("/authUser/unallocatedList", require("system:role:list"), h.UnallocatedList)
	b.GET("/deptTree/:role_id", require("system:role:list"), h.DeptTree)
	b.POST("/export", require("system:role:export"), h.Export)
	b.POST("", require("system:role:add"), repeat(), h.Add)
	b.PUT("", require("system:role:edit"), repeat(), h.Update)
	b.PUT("/dataScope", require("system:role:edit"), repeat(), h.DataScope)
	b.PUT("/changeStatus", require("system:role:edit"), repeat(), h.ChangeStatus)
	b.PUT("/authUser/cancel", require("system:role:edit"), repeat(), h.CancelAuthUser)
	b.PUT("/authUser/cancelAll", require("system:role:edit"), repeat(), h.CancelAll)
	b.PUT("/authUser/selectAll", require("system:role:edit"), repeat(), h.SelectAll)
	b.GET("/:role_id", require("system:role:query"), h.GetByID)
	b.DELETE("/:role_ids", require("system:role:remove"), h.Delete)
}

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
