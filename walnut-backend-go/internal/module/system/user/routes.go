package user

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册本模块端点：路由声明与权限/防重守卫集中在此维护，
// handler.go 只保留请求处理函数。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/getInfo", h.GetInfo)
	b.GET("/profile", h.GetProfile)
	b.PUT("/profile", repeat(), h.UpdateProfile)
	b.PUT("/profile/updatePwd", repeat(), h.UpdatePwd)
	b.POST("/profile/avatar", repeat(), h.Avatar)
	b.GET("/list", require("system:user:list"), h.List)
	b.GET("/list/dept/:dept_id", require("system:user:list"), h.ListByDept)
	b.POST("/export", require("system:user:export"), h.Export)
	b.POST("/importData", require("system:user:import"), h.ImportData)
	b.POST("/importTemplate", h.ImportTemplate)
	b.GET("/optionselect", require("system:user:query"), h.OptionSelect)
	b.GET("/deptTree", require("system:user:list"), h.DeptTree)
	b.GET("/authRole/:user_id", require("system:user:query"), h.GetAuthRole)
	b.PUT("/resetPwd", require("system:user:resetPwd"), repeat(), h.ResetPwd)
	b.PUT("/changeStatus", require("system:user:edit"), repeat(), h.ChangeStatus)
	b.PUT("/authRole", require("system:user:edit"), repeat(), h.AuthRole)
	b.POST("", require("system:user:add"), repeat(), h.Add)
	b.PUT("", require("system:user:edit"), repeat(), h.Update)
	b.DELETE("/:user_ids", require("system:user:remove"), h.Delete)
	b.GET("/", require("system:user:query"), h.GetUserInfo)
	b.GET("/:user_id", require("system:user:query"), h.GetByID)
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
