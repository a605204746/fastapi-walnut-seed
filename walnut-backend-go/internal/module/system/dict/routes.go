// 本文件集中维护字典域的路由声明，handler.go 只保留请求处理函数。

package dict

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册字典路由。挂载点是 /system/dict（由 bootstrap 决定）。
//
// ⚠️ 动态段（/type/:dict_id 等）必须注册在同方法静态路由之后。
func (h *Handler) Mount(b routex.Binder) {
	require := func(perm string) gin.HandlerFunc {
		if h.perm == nil {
			return func(c *gin.Context) { c.Next() }
		}
		return h.perm.Require(perm)
	}

	b.GET("/type/list", require("system:dict:list"), h.TypeList)
	b.POST("/type/export", require("system:dict:export"), h.TypeExport)
	b.GET("/type/optionselect", h.TypeOptionSelect)
	b.DELETE("/type/refreshCache", require("system:dict:remove"), h.RefreshCache)
	b.GET("/type/:dict_id", require("system:dict:query"), h.TypeGet)
	b.POST("/type", require("system:dict:add"), h.TypeAdd)
	b.PUT("/type", require("system:dict:edit"), h.TypeUpdate)
	b.DELETE("/type/:dict_ids", require("system:dict:remove"), h.TypeDelete)

	b.GET("/data/list", require("system:dict:list"), h.DataList)
	b.POST("/data/export", require("system:dict:export"), h.DataExport)
	b.GET("/data/type/:dict_type", h.DataByType)
	b.GET("/data/:dict_code", require("system:dict:query"), h.DataGet)
	b.POST("/data", require("system:dict:add"), h.DataAdd)
	b.PUT("/data", require("system:dict:edit"), h.DataUpdate)
	b.DELETE("/data/:dict_codes", require("system:dict:remove"), h.DataDelete)
}
