// 本文件实现菜单域的 HTTP 端点，对应 Python 端 menu/controller.py。

package menu

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/routex"
)

// Handler 菜单域端点。
type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
}

// NewHandler 构造 handler。
func NewHandler(db *gorm.DB, eh *errs.Handler) *Handler {
	return &Handler{db: db, errs: eh}
}

// Mount 注册菜单路由。挂载点是 /system/menu（由 bootstrap 决定）。
func (h *Handler) Mount(b routex.Binder) {
	b.GET("/getRouters", h.GetRouters)
}

// GetRouters GET /system/menu/getRouters
//
// 对应 Python 端 controller.get_routers：依赖是 get_current_user（AuthUser）
// 而**不是** AuthPermission —— 只要登录就能取自己的路由，无需 system:menu:* 权限。
// 否则普通用户登录后拿不到菜单，主页会是空白。
func (h *Handler) GetRouters(c *gin.Context) {
	ac, ok := authctx.From(c)
	if !ok {
		// 受保护分组必然已挂认证中间件，走到这里说明路由被错误地注册成了公开组
		h.errs.Abort(c, errs.NotLogin())
		return
	}

	routers, err := NewService(h.db, ac).GetRouters(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, routers)
}
