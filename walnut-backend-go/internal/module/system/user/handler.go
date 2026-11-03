// 本文件实现用户域的 HTTP 端点，对应 Python 端 module_system/user/controller.py。
//
// P3 窄切片只落地 GET /system/user/getInfo（前端登录后立刻调用）；
// 其余 20 个端点随 user 模块整体补齐。

package user

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/routex"
)

// msgNoUserDataScope 与 Python 端 controller.get_info 的字面量一致。
const msgNoUserDataScope = "没有权限访问用户数据!"

// Handler 用户域端点，对应 Python 端 module_system/user/controller.py。
type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
}

// NewHandler 构造 handler。
func NewHandler(db *gorm.DB, eh *errs.Handler) *Handler {
	return &Handler{db: db, errs: eh}
}

// Mount 注册用户路由。挂载点是 /system/user（由 bootstrap 决定），
// 故这里的 path 都是相对路径。
//
// ⚠️ 全部端点都在受保护分组内，必须经 b.Protected 注册——
// 启动期路由认证审计会对白名单外的公开路由直接拒绝启动。
func (h *Handler) Mount(b routex.Binder) {
	b.GET("/getInfo", h.GetInfo)
}

// GetInfo GET /system/user/getInfo
//
// 对应 Python 端 controller.get_info：**需登录但不查权限**
// （依赖是 get_current_user 而非 AuthPermission），且忽略数据权限。
func (h *Handler) GetInfo(c *gin.Context) {
	ac, ok := authctx.From(c)
	if !ok {
		// 受保护分组必然已挂认证中间件，走到这里说明路由被错误地注册成了公开组
		h.errs.Abort(c, errs.NotLogin())
		return
	}

	data, err := NewService(h.db, ac).GetInfo(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if data == nil {
		envelope.Fail(c, msgNoUserDataScope)
		return
	}
	envelope.OK(c, data)
}
