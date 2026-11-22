// 本文件集中维护本模块的路由声明与权限/防重守卫，handler.go 只保留请求处理函数。

package log

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/operlog/list", require("monitor:operlog:list"), h.OperLogList)
	b.POST("/operlog/export", require("monitor:operlog:export"), h.OperLogExport)
	b.DELETE("/operlog/clean", require("monitor:operlog:remove"), h.OperLogClean)
	b.DELETE("/operlog/:oper_ids", require("monitor:operlog:remove"), h.OperLogDelete)

	b.GET("/logininfor/list", require("monitor:logininfor:list"), h.LogininforList)
	b.POST("/logininfor/export", require("monitor:logininfor:export"), h.LogininforExport)
	b.DELETE("/logininfor/clean", require("monitor:logininfor:remove"), h.LogininforClean)
	b.DELETE("/logininfor/:info_ids", require("monitor:logininfor:remove"), h.LogininforDelete)
	b.GET("/logininfor/unlock/:user_name", require("monitor:logininfor:unlock"), repeat(), h.LogininforUnlock)
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
