// 本文件实现岗位域的 HTTP 端点，对应 Python 端 module_system/post/controller.py。
//
// # 一处已修复的两端共有缺陷（2026-11-03）
//
// 导出的 9 列里曾有 6 列恒为空。根因在 Python 端：
//
//	rows = select_list(...)                       # model_dump(by_alias=True) → **驼峰**键
//	_EXPORT_HEADERS = {"dept_id": "部门id", ...}   # 却是**下划线**键
//	build_workbook: item.get("dept_id")           # → None → _fmt_cell → 空串
//
// 只有 "id" / "status" / "remark" 三个键驼峰化后拼写不变，故仅这三列有值。
// 对照证据：config/controller.py 用 {"configName": ...}、dict/controller.py 用
// {"dictName": ...}——都是驼峰，与 model_dump 输出对得上；post 是异类。
//
// 迁移时发现该缺陷后曾按「契约一致性优先」原则镜像，经维护者确认改为**两端同修**：
// Python 端 _EXPORT_HEADERS 与本文件 exportColumns 的键同步改成驼峰。
//
// ⚠️ 排查同类问题时注意：user / role 两个模块的导出行是 service 层**手工构造的
// 下划线键 dict**（role 还做了字典值→标签的翻译），与它们各自的下划线表头本就匹配，
// **不属于此缺陷，不要一起改**。判据是看 rows 的来源：
// 经 model_dump(by_alias=True) 的必须配驼峰表头，手工 dict 则看它自己写的键。
//
// 验证：test/contract/diff_post.py 第 8 组比对导出响应头与解析后的单元格内容。

package post

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
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
	"walnut-seed-go/internal/pkg/timex"
)

// sheetName 是导出工作表名，同时决定默认文件名（{sheetName}_{时间}.xlsx）。
const sheetName = "岗位数据"

// Handler 岗位域端点。
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

// Mount 注册岗位路由。挂载点是 /system/post（由 bootstrap 决定）。
//
// ⚠️ 每个端点都必须挂权限守卫：前端隐藏按钮不是权限控制。
// 权限串与 Python 端 AuthPermission(permissions=[...]) 逐条对应。
//
// 动态段路由（/:post_id）必须注册在全部静态路由之后，避免 gin 路由树冲突。
func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/list", require("system:post:list"), h.List)
	b.POST("/export", require("system:post:export"), h.Export)
	b.GET("/optionselect", require("system:post:query"), h.OptionSelect)
	b.GET("/deptTree", require("system:post:list"), h.DeptTree)
	b.GET("/:post_id", require("system:post:query"), h.GetByID)
	b.POST("", require("system:post:add"), repeat(), h.Add)
	b.PUT("", require("system:post:edit"), repeat(), h.Update)
	b.DELETE("/:post_ids", require("system:post:remove"), h.Delete)
}

// guards 返回「权限守卫」与「防重提交」两个中间件工厂。
//
// 依赖为 nil 时返回放行中间件，让 handler 能脱离 bootstrap 做单元测试。
// ⚠️ 生产装配路径上两者都非 nil（见 bootstrap.mountSystemRoutes）；
// 若这里静默放行掩盖了装配遗漏，契约 diff 的 403 用例会立刻暴露。
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

// List GET /system/post/list
func (h *Handler) List(c *gin.Context) {
	q, err := BindQueryParam(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.PageList(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

// GetByID GET /system/post/{post_id}
//
// ⚠️ FastAPI 的路径参数是 `post_id: int`，非数字会被 Pydantic 拦下并返回
// "Input should be a valid integer, unable to parse string as an integer"（code=400）。
// gin 的路径参数恒为字符串，故这里必须自己解析并给出同样文案。
func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathInt(c, "post_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.GetByID(c.Request.Context(), id)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	// ⚠️ 记录不存在时是 **code=200 + data=null**，不是错误。
	// Python 端是 `return SuccessResponse(data=await service.get_by_id(post_id))`，
	// get_by_id 返回 None 就原样塞进 data。
	// 别照搬 user.getInfo 的「没有权限访问…数据!」——那是另一个端点的契约，
	// 契约比对实测抓到过这个偏差。
	envelope.OK(c, data)
}

// OptionSelect GET /system/post/optionselect
//
// 查询参数用的是 **别名**：postIds（逗号分隔串）、deptId。
// ⚠️ Python 端对 postIds 做的是 `[int(item) for item in str2list(post_ids)]`，
// **没有** try/except —— 非数字会抛 ValueError 变成 500，而不是 400。
// 这里保持一致：解析失败按系统异常处理，不要"改进"成 400。
func (h *Handler) OptionSelect(c *gin.Context) {
	var ids []int64
	if raw := c.Query("postIds"); crudx.IsNotBlank(raw) {
		for _, item := range crudx.Str2List(raw) {
			n, err := strconv.ParseInt(item, 10, 64)
			if err != nil {
				h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
				return
			}
			ids = append(ids, n)
		}
	}
	var deptID *int64
	if raw := c.Query("deptId"); crudx.IsNotBlank(raw) {
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			// deptId 是 `int | None` 的查询参数，由 FastAPI/Pydantic 校验 → 400
			h.errs.Abort(c, errs.BadRequest(msgValidInt))
			return
		}
		deptID = &n
	}

	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.OptionSelect(c.Request.Context(), ids, deptID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

// DeptTree GET /system/post/deptTree
func (h *Handler) DeptTree(c *gin.Context) {
	q := BindDeptTreeQuery(c)
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.DeptTreeList(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

// Add POST /system/post
//
// 两条唯一性校验的**文案里都用 req.post_name**（包括编码重复那条），
// 这是 Python 端的既有写法，照抄不"修正"。
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

	uniq, err := svc.CheckPostNameUnique(ctx, in, nil)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !uniq {
		h.errs.Abort(c, errs.Newf("新增岗位'%s'失败，岗位名称已存在", deref(in.PostName)))
		return
	}
	uniq, err = svc.CheckPostCodeUnique(ctx, in, nil)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !uniq {
		h.errs.Abort(c, errs.Newf("新增岗位'%s'失败，岗位编码已存在", deref(in.PostName)))
		return
	}

	done, err := svc.InsertPost(ctx, in)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !done {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

// Update PUT /system/post
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

	uniq, err := svc.CheckPostNameUnique(ctx, in.CreateIn, in.ID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !uniq {
		h.errs.Abort(c, errs.Newf("修改岗位'%s'失败，岗位名称已存在", deref(in.PostName)))
		return
	}
	uniq, err = svc.CheckPostCodeUnique(ctx, in.CreateIn, in.ID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !uniq {
		h.errs.Abort(c, errs.Newf("修改岗位'%s'失败，岗位编码已存在", deref(in.PostName)))
		return
	}

	// 停用前校验是否仍有用户占用该岗位
	if in.Status != nil && *in.Status == "1" {
		n, err := svc.CountUserPostByID(ctx, *in.ID)
		if err != nil {
			h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
			return
		}
		if n > 0 {
			h.errs.Abort(c, errs.New("该岗位下存在已分配用户，不能禁用!"))
			return
		}
	}

	done, err := svc.UpdatePost(ctx, in)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !done {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

// Delete DELETE /system/post/{post_ids}
//
// ⚠️ post_ids 是**逗号分隔的 ID 串**，解析失败返回 code=400 + "岗位ID格式有误"，
// 解析后为空返回 "岗位ID不能为空"（见 crudx.ParseIDs）。
func (h *Handler) Delete(c *gin.Context) {
	ids, err := crudx.ParseIDs(c.Param("post_ids"), "岗位ID")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.DeletePostByIDs(c.Request.Context(), ids)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if n <= 0 {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

// Export POST /system/post/export
//
// ⚠️ 请求体是 **form**（不是 JSON）：Python 端 `await request.form()`，
// 且会 `data.pop("pageSize")` —— 导出不分页，查全部。
func (h *Handler) Export(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		h.errs.Abort(c, errs.BadRequest(errs.MsgValidationFailed))
		return
	}
	// 只取非空的 form 字段（Python: `if value not in (None, "")`）
	q := QueryParam{}
	q.DeptID = formInt(c, "deptId")
	q.BelongDeptID = formInt(c, "belongDeptId")
	q.PostCode = c.PostForm("postCode")
	q.PostCategory = c.PostForm("postCategory")
	q.PostName = c.PostForm("postName")
	q.Status = c.PostForm("status")
	q.BeginTime = c.PostForm("beginTime")
	q.EndTime = c.PostForm("endTime")
	// 刻意不设 q.Page.PageSize：导出走 select_list → list_all，本就不分页

	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.SelectList(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if err := excelx.Export(c, toAnySlice(rows), exportColumns(), sheetName); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

// exportColumns 对应 Python 端 _EXPORT_HEADERS（**顺序即列序**）。
//
// ⚠️ Key 必须是**驼峰**：toCamelMap 产出的是 model_dump(by_alias=True) 的键形态，
// pick 用它取值，键名对不上就得到 nil → 单元格为空。
//
// 这里曾有过一个**两端共有**的缺陷：Python 端 _EXPORT_HEADERS 用的是下划线键，
// 导致 9 列里 6 列导出恒为空（只有 id/status/remark 三列因驼峰化后拼写不变而有值）。
// 2026-11-03 已在两端同步修为驼峰（Python 侧见 post/controller.py 的同名注释）。
// 注意 user/role 两个模块的导出行是 service 手工构造的下划线键 dict，
// 与它们各自的下划线表头本就匹配，**不受影响、不要一起改**。
func exportColumns() []excelx.Column {
	return []excelx.Column{
		{Header: "岗位序号", Value: pick("id")},
		{Header: "部门id", Value: pick("deptId")},
		{Header: "岗位编码", Value: pick("postCode")},
		{Header: "岗位名称", Value: pick("postName")},
		{Header: "类别编码", Value: pick("postCategory")},
		{Header: "岗位排序", Value: pick("postSort")},
		{Header: "状态", Value: pick("status")},
		{Header: "备注", Value: pick("remark")},
		{Header: "创建时间", Value: pick("createTime")},
	}
}

// toAnySlice 把出参切片转成 excelx 需要的 []any。
//
// 先序列化成驼峰键的 map，**复刻 Python 端 model_dump(by_alias=True) 的结果**，
// 这样「用下划线键去取驼峰字典」的缺陷才能被同样地复现出来。
func toAnySlice(rows []Out) []any {
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, toCamelMap(rows[i]))
	}
	return out
}

// pick 返回一个按下划线键取值的取值函数（对应 item.get(field)）。
func pick(field string) func(any) any {
	return func(row any) any {
		m, ok := row.(map[string]any)
		if !ok {
			return nil
		}
		return m[field]
	}
}

// toCamelMap 把 Out 转成驼峰键的 map，等价于 model_dump(by_alias=True, mode="json")。
func toCamelMap(o Out) map[string]any {
	m := map[string]any{
		"id":           nil,
		"deptId":       nil,
		"postCode":     nil,
		"postName":     nil,
		"postCategory": nil,
		"postSort":     nil,
		"status":       nil,
		"remark":       nil,
		"createTime":   nil,
		"deptName":     nil,
	}
	if o.ID != nil {
		m["id"] = o.ID.Int64()
	}
	if o.DeptID != nil {
		m["deptId"] = o.DeptID.Int64()
	}
	if o.PostCode != nil {
		m["postCode"] = *o.PostCode
	}
	if o.PostName != nil {
		m["postName"] = *o.PostName
	}
	if o.PostCategory != nil {
		m["postCategory"] = *o.PostCategory
	}
	if o.PostSort != nil {
		m["postSort"] = o.PostSort.Int64()
	}
	if o.Status != nil {
		m["status"] = *o.Status
	}
	if o.Remark != nil {
		m["remark"] = *o.Remark
	}
	if o.CreateTime != nil {
		m["createTime"] = timex.JSONTime(*o.CreateTime).Time().Format("2006-01-02 15:04:05")
	}
	if o.DeptName != nil {
		m["deptName"] = *o.DeptName
	}
	return m
}

// ---- 内部工具 ----

// readBody 读取请求体（可能为空）。
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

// formInt 读取可选的 form 整型字段（空值视为未提供）。
func formInt(c *gin.Context, key string) *int64 {
	raw := strings.TrimSpace(c.PostForm(key))
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
