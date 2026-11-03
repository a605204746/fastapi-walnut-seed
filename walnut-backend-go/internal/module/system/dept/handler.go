// 本文件实现部门域的 HTTP 端点，对应 Python 端 module_system/dept/controller.py。
//
// ⚠️ 删除链的四道前置校验返回的是 **WarnResponse（code=601）** 而不是 500，
// 顺序为：默认部门 → 有下级 → 有用户 → 有岗位，最后才是数据权限校验。
// 顺序调换会让两端的报错文案错位（契约 diff 会抓到）。

package dept

import (
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
)

// defaultDeptID 默认部门，不允许删除，对应 SystemConstants.DEFAULT_DEPT_ID。
const defaultDeptID int64 = 100

// Handler 部门域端点。
type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
	perm *permguard.Guard
	idem *idempotentx.Guard
}

// NewHandler 构造 handler。perm / idem 可为 nil（此时不施加守卫，仅用于单测）。
func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem}
}

// Mount 注册部门路由。挂载点是 /system/dept（由 bootstrap 决定）。
//
// ⚠️ 权限串与 Python 端 AuthPermission(permissions=[...]) 逐条对应。
// ⚠️ 动态段路由（/:dept_id、/list/exclude/:dept_id）必须注册在静态路由之后。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:dept:list"), h.List)
	b.GET("/list/exclude/:dept_id", require("system:dept:list"), h.ListExclude)
	b.GET("/treeselect", require("system:dept:query"), h.TreeSelect)
	b.GET("/optionselect", require("system:dept:query"), h.OptionSelect)
	b.GET("/:dept_id", require("system:dept:query"), h.GetByID)
	b.POST("", require("system:dept:add"), repeat(), h.Add)
	b.PUT("", require("system:dept:edit"), repeat(), h.Update)
	b.DELETE("/:dept_id", require("system:dept:remove"), h.Delete)
}

// guards 返回「权限守卫」与「防重提交」两个中间件工厂（同 post/handler.go）。
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

// service 取出当前请求的业务层。ok=false 表示路由未挂认证中间件（编程错误）。
func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac), true
}

// List GET /system/dept/list
//
// ⚠️ 响应 data 是**数组**而不是 {"rows","total"}——部门列表不分页。
func (h *Handler) List(c *gin.Context) {
	q := BindQueryParam(c)
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.ListOut(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// ListExclude GET /system/dept/list/exclude/{dept_id}
func (h *Handler) ListExclude(c *gin.Context) {
	id, err := pathInt(c, "dept_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.ListOutExclude(c.Request.Context(), id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// TreeSelect GET /system/dept/treeselect
func (h *Handler) TreeSelect(c *gin.Context) {
	q := BindQueryParam(c)
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.TreeSelectOut(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// OptionSelect GET /system/dept/optionselect
//
// ⚠️ deptIds 解析失败是 **400「部门ID参数格式不正确」**——Python 端这里
// 有 try/except ValueError（与 post/optionselect 的裸 int() 500 不同），照抄。
func (h *Handler) OptionSelect(c *gin.Context) {
	var ids []int64
	if raw := c.Query("deptIds"); crudx.IsNotBlank(raw) {
		for _, item := range crudx.Str2List(raw) {
			n, err := strconv.ParseInt(item, 10, 64)
			if err != nil {
				h.errs.Abort(c, errs.BadRequest("部门ID参数格式不正确"))
				return
			}
			ids = append(ids, n)
		}
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.OptionSelectOut(c.Request.Context(), ids)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// GetByID GET /system/dept/{dept_id}
//
// ⚠️ 先做数据权限校验再查详情：越权与不存在**同文案**「没有权限访问部门数据！」
// （超管跳过校验，不存在时 data=null）。
func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathInt(c, "dept_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := svc.CheckDataScope(ctx, &id); err != nil {
		h.errs.Abort(c, err)
		return
	}
	data, err := svc.GetOut(ctx, id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

// Add POST /system/dept
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

	uniq, err := svc.CheckNameUnique(ctx, deref(in.DeptName), in.ParentID, nil)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !uniq {
		h.errs.Abort(c, errs.Newf("新增部门'%s'失败，部门名称已存在", deref(in.DeptName)))
		return
	}
	if err := svc.Insert(ctx, in); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// Update PUT /system/dept
//
// ⚠️ 校验顺序照抄 Python：数据权限 → 名称唯一 → 上级是自己 → 停用前置两道。
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

	if err := svc.CheckDataScope(ctx, in.ID); err != nil {
		h.errs.Abort(c, err)
		return
	}
	uniq, err := svc.CheckNameUnique(ctx, deref(in.DeptName), in.ParentID, in.ID)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !uniq {
		h.errs.Abort(c, errs.Newf("修改部门'%s'失败，部门名称已存在", deref(in.DeptName)))
		return
	}
	if in.ParentID != nil && in.ID != nil && *in.ParentID == *in.ID {
		h.errs.Abort(c, errs.Newf("修改部门'%s'失败，上级部门不能是自己", deref(in.DeptName)))
		return
	}
	if deref(in.Status) == "1" {
		n, err := svc.CountNormalChildren(ctx, *in.ID)
		if err != nil {
			h.errs.Abort(c, err)
			return
		}
		if n > 0 {
			h.errs.Abort(c, errs.New("该部门包含未停用的子部门!"))
			return
		}
		exists, err := svc.ExistsUser(ctx, *in.ID)
		if err != nil {
			h.errs.Abort(c, err)
			return
		}
		if exists {
			h.errs.Abort(c, errs.New("该部门下存在已分配用户，不能禁用!"))
			return
		}
	}
	if err := svc.Update(ctx, in); err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, nil)
}

// Delete DELETE /system/dept/{dept_id}
func (h *Handler) Delete(c *gin.Context) {
	id, err := pathInt(c, "dept_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	if id == defaultDeptID {
		envelope.Warn(c, nil, "默认部门,不允许删除")
		return
	}
	hasChild, err := svc.HasChild(ctx, id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if hasChild {
		envelope.Warn(c, nil, "存在下级部门,不允许删除")
		return
	}
	existsUser, err := svc.ExistsUser(ctx, id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if existsUser {
		envelope.Warn(c, nil, "部门存在用户,不允许删除")
		return
	}
	postCount, err := svc.CountPost(ctx, id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if postCount > 0 {
		envelope.Warn(c, nil, "部门存在岗位,不允许删除")
		return
	}
	if err := svc.CheckDataScope(ctx, &id); err != nil {
		h.errs.Abort(c, err)
		return
	}
	done, err := svc.Delete(ctx, id)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !done {
		envelope.Fail(c, envelope.MsgFail)
		return
	}
	envelope.OK(c, nil)
}

// ---- 内部工具（与 post/handler.go 同构）----

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

// pathInt 解析整型路径参数，失败时给出 Pydantic 同款文案。
func pathInt(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Param(name))
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errs.BadRequest(msgValidInt)
	}
	return n, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
