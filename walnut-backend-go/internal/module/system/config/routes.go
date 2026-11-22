// 本文件集中维护参数配置域的路由声明，handler.go 只保留请求处理函数。

package config

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册参数配置路由。挂载点是 /system/config（由 bootstrap 决定）。
func (h *Handler) Mount(b routex.Binder) {
	require := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return func(c *gin.Context) { c.Next() }
		}
		return h.perm.Require(perm)
	}

	b.GET("/list", require("system:config:list"), h.List)
	b.POST("/export", require("system:config:export"), h.Export)
	b.GET("/configKey/:config_key", h.GetByKey)
	b.PUT("/updateByKey", require("system:config:edit"), h.UpdateByKey)
	b.DELETE("/refreshCache", require("system:config:remove"), h.RefreshCache)
	b.GET("/:config_id", require("system:config:query"), h.GetByID)
	b.POST("", require("system:config:add"), h.Add)
	b.PUT("", require("system:config:edit"), h.Update)
	b.DELETE("/:config_ids", require("system:config:remove"), h.Delete)
}
