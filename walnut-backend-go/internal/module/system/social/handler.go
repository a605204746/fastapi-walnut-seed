// 本文件实现社交授权关系的 HTTP 端点，对应 Python 端 module_system/social/controller.py。
//
// ⚠️ 仅 /list 一个端点：需登录 + AuthPermission()（**无权限码**，等价 permguard 的仅登录守卫）。

package social

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/routex"
)

// Handler 社交授权关系端点。
type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
}

// NewHandler 构造 handler。
func NewHandler(db *gorm.DB, eh *errs.Handler) *Handler {
	return &Handler{db: db, errs: eh}
}

// Mount 注册路由。挂载点是 /system/social（Python 端 router 前缀自带 /system）。
func (h *Handler) Mount(b routex.Binder) {
	b.GET("/list", h.List)
}

// List GET /system/social/list（仅登录，无权限码）
func (h *Handler) List(c *gin.Context) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return
	}
	svc := NewService(h.db, ac)
	rows, err := svc.ListByUserID(c.Request.Context(), ac.User.ID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, rows)
}
