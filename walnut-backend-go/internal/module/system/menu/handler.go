// 本文件实现菜单域的 HTTP 端点，对应 Python 端 menu/controller.py。
//
// ⚠️ 契约点：
//   - getRouters 只挂登录（前端动态路由命脉）；
//   - 删除有两条：/cascade/{menu_ids}（批量级联）与 /{menu_id}（单条，两道 Warn 前置）；
//   - Warn 响应 code=601：「存在子菜单,不允许删除」「菜单已分配,不允许删除」；
//   - POST/PUT 挂 RepeatSubmit。

package menu

import (
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
)

// Handler 菜单域端点。
type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
	perm *permguard.Guard
	idem *idempotentx.Guard
}

// NewHandler 构造 handler。perm / idem 可为 nil（仅单测）。
func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem}
}

// Mount 注册菜单路由。挂载点是 /system/menu（由 bootstrap 决定）。
//
// ⚠️ 动态段 /:menu_id 必须注册在所有静态路由与 /cascade/:menu_ids 之后。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/getRouters", h.GetRouters)
	b.GET("/list", require("system:menu:list"), h.List)
	b.GET("/treeselect", require("system:menu:query"), h.TreeSelect)
	b.GET("/roleMenuTreeselect/:role_id", require("system:menu:query"), h.RoleMenuTreeSelect)
	b.GET("/:menu_id", require("system:menu:query"), h.GetByID)
	b.POST("", require("system:menu:add"), repeat(), h.Add)
	b.PUT("", require("system:menu:edit"), repeat(), h.Update)
	b.DELETE("/cascade/:menu_ids", require("system:menu:remove"), h.DeleteCascade)
	b.DELETE("/:menu_id", require("system:menu:remove"), h.Delete)
}

// guards 返回「权限守卫」与「防重提交」两个中间件工厂（同 post/handler.go）。
//
// ⚠️ Python 端部分守卫带 roles=[SUPER_ADMIN]（AND/OR 组合）；admin 是超管且
// 持有 *:*:*，permguard 的权限串校验在 diff 覆盖范围内与两端一致。
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

// service 取出当前请求的业务层。
func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac), true
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

// List GET /system/menu/list（不分页数组）
func (h *Handler) List(c *gin.Context) {
	q, err := BindQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.ListOut(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, rows)
}

// TreeSelect GET /system/menu/treeselect
func (h *Handler) TreeSelect(c *gin.Context) {
	q, err := BindQuery(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.TreeSelectOut(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, rows)
}

// RoleMenuTreeSelect GET /system/menu/roleMenuTreeselect/{role_id}
func (h *Handler) RoleMenuTreeSelect(c *gin.Context) {
	roleID, err := pathInt(c, "role_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.RoleMenuTreeSelect(c.Request.Context(), roleID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

// GetByID GET /system/menu/{menu_id}
func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathInt(c, "menu_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.GetOut(c.Request.Context(), id)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

// Add POST /system/menu
//
// ⚠️ 三道前置校验的顺序与文案照抄：名称唯一 → 外链地址 http(s) → 路由配置唯一。
func (h *Handler) Add(c *gin.Context) {
	in, err := DecodeCreate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	name := deref(in.MenuName)

	if err := svc.CheckNameUnique(ctx, in); err != nil {
		if err == errDupName {
			h.errs.Abort(c, errs.Newf("新增菜单'%s'失败，菜单名称已存在", name))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if deref(in.IsFrame) == YesFrame && !isHTTP(deref(in.Path)) {
		h.errs.Abort(c, errs.Newf("新增菜单'%s'失败，地址必须以http(s)://开头", name))
		return
	}
	if err := svc.CheckRouteConfigUnique(ctx, in); err != nil {
		if err == errDupRoute {
			h.errs.Abort(c, errs.Newf("新增菜单'%s'失败，路由名称或地址已存在", name))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	n, err := svc.Insert(ctx, in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if n <= 0 {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// Update PUT /system/menu
func (h *Handler) Update(c *gin.Context) {
	in, err := DecodeUpdate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	name := deref(in.MenuName)

	if err := svc.CheckNameUnique(ctx, in.CreateIn); err != nil {
		if err == errDupName {
			h.errs.Abort(c, errs.Newf("修改菜单'%s'失败，菜单名称已存在", name))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	if deref(in.IsFrame) == YesFrame && !isHTTP(deref(in.Path)) {
		h.errs.Abort(c, errs.Newf("修改菜单'%s'失败，地址必须以http(s)://开头", name))
		return
	}
	if in.ID != nil && in.ParentID != nil && *in.ID == *in.ParentID {
		h.errs.Abort(c, errs.Newf("修改菜单'%s'失败，上级菜单不能选择自己", name))
		return
	}
	if err := svc.CheckRouteConfigUnique(ctx, in.CreateIn); err != nil {
		if err == errDupRoute {
			h.errs.Abort(c, errs.Newf("修改菜单'%s'失败，路由名称或地址已存在", name))
			return
		}
		h.errs.Abort(c, err)
		return
	}
	n, err := svc.Update(ctx, in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if n <= 0 {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// DeleteCascade DELETE /system/menu/cascade/{menu_ids}
//
// ⚠️ ID 串解析在 Python 端是裸 int()（无 try/except）：非整数会抛未捕获异常。
// 这里镜像为 500；空串解析为空列表（has_child 检查直接放过、删除 0 条成功）。
func (h *Handler) DeleteCascade(c *gin.Context) {
	ids, okParse := parseIDsLoose(c.Param("menu_ids"))
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if !okParse {
		h.errs.Abort(c, errs.New(errs.MsgSystemError))
		return
	}
	ctx := c.Request.Context()
	hasChild, err := svc.repo.HasChildByMenuIDs(ctx, ids)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if hasChild {
		envelope.Warn(c, nil, "存在子菜单,不允许删除")
		return
	}
	if err := svc.DeleteCascade(ctx, ids); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// Delete DELETE /system/menu/{menu_id}
func (h *Handler) Delete(c *gin.Context) {
	id, err := pathInt(c, "menu_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	hasChild, err := svc.repo.HasChildByMenuID(ctx, id)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if hasChild {
		envelope.Warn(c, nil, "存在子菜单,不允许删除")
		return
	}
	assigned, err := svc.repo.CheckMenuExistRole(ctx, id)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if assigned {
		envelope.Warn(c, nil, "菜单已分配,不允许删除")
		return
	}
	n, err := svc.DeleteByID(ctx, id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if n <= 0 {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// ---- 内部工具 ----

func readBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	return buf
}

func pathInt(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Param(name))
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errs.BadRequest(msgValidInt)
	}
	return n, nil
}

// parseIDsLoose 对应 controller 的裸 `[int(i) for i in ids.split(",") if i.strip()]`：
// 解析失败返回 ok=false（Python 端是未捕获 ValueError）。
func parseIDsLoose(raw string) ([]int64, bool) {
	ids := make([]int64, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, false
		}
		ids = append(ids, n)
	}
	return ids, true
}
