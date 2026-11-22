package role

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
)

type Handler struct {
	db       *gorm.DB
	errs     *errs.Handler
	perm     *permguard.Guard
	idem     *idempotentx.Guard
	NewDepts func(authctx.Context) DeptProvider
}

func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard, newDepts func(authctx.Context) DeptProvider) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem, NewDepts: newDepts}
}

func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac, Deps{NewDepts: h.NewDepts}), true
}

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

func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathInt(c, "role_id")
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
	envelope.OK(c, data)
}

func (h *Handler) OptionSelect(c *gin.Context) {
	var ids []int64
	if raw := c.Query("roleIds"); crudx.IsNotBlank(raw) {
		for _, item := range crudx.Str2List(raw) {
			// Python _parse_ids 语义：int() 失败 → 400「角色ID格式有误」
			// （不再透传 traceback 原文，两端同步修改）
			n, valid := parsePyIntLiteral(item)
			if !valid {
				h.errs.Abort(c, errs.BadRequest("角色ID格式有误"))
				return
			}
			ids = append(ids, n)
		}
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.OptionSelect(c.Request.Context(), ids)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) Add(c *gin.Context) {
	in, err := DecodeCreate(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Python controller 顺序：allowed → 名称唯一 → key 唯一 → insert。
	// 缺省（None）字段跳过 DTO 校验器，由 service 层唯一性检查报 500 首错。
	if err := svc.CheckRoleAllowed(ctx, nil, in.RoleKey); err != nil {
		h.errs.Abort(c, err)
		return
	}
	if in.RoleName == nil {
		h.errs.Abort(c, errs.New(msgRoleNameEmpty))
		return
	}
	exists, err := svc.CheckRoleNameUnique(ctx, *in.RoleName, nil)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if exists {
		h.errs.Abort(c, errs.Newf("新增角色'%s'失败，角色名称已存在", *in.RoleName))
		return
	}
	if in.RoleKey == nil {
		h.errs.Abort(c, errs.New(msgRoleKeyEmpty))
		return
	}
	exists, err = svc.CheckRoleKeyUnique(ctx, *in.RoleKey, nil)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if exists {
		h.errs.Abort(c, errs.Newf("新增角色'%s'失败，角色权限已存在", *in.RoleName))
		return
	}

	done, err := svc.InsertRole(ctx, in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !done {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) Update(c *gin.Context) {
	in, err := DecodeUpdate(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	// id 缺省（非 null）→ controller 报 500「角色ID不能为空」
	if in.ID == nil {
		h.errs.Abort(c, errs.New(msgRoleIDEmpty))
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Python controller 顺序：allowed → dataScope → 名称唯一 → key 唯一。
	// PUT 继承新增校验器：roleName/roleKey 缺省（None）时唯一性检查报 500，
	// 不允许只传部分字段更新。
	if err := svc.CheckRoleAllowed(ctx, in.ID, in.RoleKey); err != nil {
		h.errs.Abort(c, err)
		return
	}
	if err := svc.CheckRoleDataScope(ctx, []int64{*in.ID}); err != nil {
		h.errs.Abort(c, err)
		return
	}
	if in.RoleName == nil {
		h.errs.Abort(c, errs.New(msgRoleNameEmpty))
		return
	}
	exists, err := svc.CheckRoleNameUnique(ctx, *in.RoleName, in.ID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if exists {
		h.errs.Abort(c, errs.Newf("修改角色'%s'失败，角色名称已存在", *in.RoleName))
		return
	}
	if in.RoleKey == nil {
		h.errs.Abort(c, errs.New(msgRoleKeyEmpty))
		return
	}
	exists, err = svc.CheckRoleKeyUnique(ctx, *in.RoleKey, in.ID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if exists {
		h.errs.Abort(c, errs.Newf("修改角色'%s'失败，角色权限已存在", *in.RoleName))
		return
	}

	done, err := svc.UpdateRole(ctx, in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !done {
		h.errs.Abort(c, errs.Newf("修改角色'%s'失败，请联系管理员", derefStr(in.RoleName)))
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) DataScope(c *gin.Context) {
	in, err := DecodeDataScope(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	// id 缺省（非 null）→ controller 报 500「角色ID不能为空」（null 已在 DTO 层 400）
	if in.ID == nil {
		h.errs.Abort(c, errs.New(msgRoleIDEmpty))
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.AuthDataScope(c.Request.Context(), in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !done {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	in, err := DecodeStatus(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	// id 缺省（非 null）→ controller 报 500「角色ID不能为空」（null 已在 DTO 层 400）
	if in.ID == nil {
		h.errs.Abort(c, errs.New(msgRoleIDEmpty))
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.UpdateRoleStatus(c.Request.Context(), in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if !done {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) Delete(c *gin.Context) {
	ids, err := crudx.ParseIDs(c.Param("role_ids"), "角色ID")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.DeleteRoleByIDs(c.Request.Context(), ids)
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

func (h *Handler) Export(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		h.errs.Abort(c, errs.BadRequest(errs.MsgValidationFailed))
		return
	}
	q := QueryParam{}
	q.RoleName = strings.TrimSpace(c.PostForm("roleName"))
	q.RoleKey = strings.TrimSpace(c.PostForm("roleKey"))
	q.Status = strings.TrimSpace(c.PostForm("status"))
	// Python 导出过滤空 form 值后走 RoleQueryParam.model_validate：
	// 非空 beginTime/endTime 非法 → 400（DateStr 解析原文）；空值视为缺省
	var msgs []string
	if raw := c.PostForm("beginTime"); raw != "" {
		v, derr := parseDateQuery(raw)
		if derr != nil {
			if se := errs.As(derr); se != nil {
				msgs = append(msgs, se.Msg)
			}
		} else {
			q.BeginTime = v
		}
	}
	if raw := c.PostForm("endTime"); raw != "" {
		v, derr := parseDateQuery(raw)
		if derr != nil {
			if se := errs.As(derr); se != nil {
				msgs = append(msgs, se.Msg)
			}
		} else {
			q.EndTime = v
		}
	}
	if len(msgs) > 0 {
		h.errs.Abort(c, errs.BadRequest(strings.Join(msgs, ", ")))
		return
	}

	svc, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := svc.SelectList(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	cols := make([]excelx.Column, 0, 10)
	headers := ExportHeaders()
	for _, hdr := range headers {
		key := headerToKey(hdr)
		cols = append(cols, excelx.Column{Header: hdr, Value: pickSnake(key)})
	}
	if err := excelx.Export(c, toAnySlice(ExportRows(rows)), cols, sheetName); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

func (h *Handler) DeptTree(c *gin.Context) {
	roleID, err := pathInt(c, "role_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.DeptTree(c.Request.Context(), roleID)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) AllocatedList(c *gin.Context) {
	q, err := BindAuthUserQueryParam(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.AllocatedList(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) UnallocatedList(c *gin.Context) {
	q, err := BindAuthUserQueryParam(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.UnallocatedList(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) CancelAuthUser(c *gin.Context) {
	in, err := DecodeCancel(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if in.RoleID == nil {
		h.errs.Abort(c, errs.New("角色ID不能为空"))
		return
	}
	if in.UserID == nil {
		h.errs.Abort(c, errs.New("用户ID不能为空"))
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.CancelAuthUser(c.Request.Context(), in)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if n <= 0 {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) CancelAll(c *gin.Context) {
	// roleId 必填查询参数：缺省 → Field required；出现（含空串）非法 → int_parsing 原文
	roleIDRaw, present := c.GetQuery("roleId")
	if !present {
		h.errs.Abort(c, errs.BadRequest(msgFieldRequired))
		return
	}
	roleID, valid := parsePyQueryInt(roleIDRaw)
	if !valid {
		h.errs.Abort(c, errs.BadRequest(msgValidInt))
		return
	}
	// Python 复用 _parse_ids：文案是「角色ID格式有误/不能为空」而非「用户ID…」
	userIDs, err := crudx.ParseIDs(c.Query("userIds"), "角色ID")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.CancelAuthUsers(c.Request.Context(), roleID, userIDs)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if n <= 0 {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

func (h *Handler) SelectAll(c *gin.Context) {
	// roleId 必填查询参数：缺省 → Field required；出现（含空串）非法 → int_parsing 原文
	roleIDRaw, present := c.GetQuery("roleId")
	if !present {
		h.errs.Abort(c, errs.BadRequest(msgFieldRequired))
		return
	}
	roleID, valid := parsePyQueryInt(roleIDRaw)
	if !valid {
		h.errs.Abort(c, errs.BadRequest(msgValidInt))
		return
	}
	// Python 复用 _parse_ids：文案是「角色ID格式有误/不能为空」而非「用户ID…」
	userIDs, err := crudx.ParseIDs(c.Query("userIds"), "角色ID")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.SelectAuthUsers(c.Request.Context(), roleID, userIDs)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if n <= 0 {
		envelope.Fail(c, errs.MsgDBFailed)
		return
	}
	envelope.OK(c, nil)
}

func headerToKey(hdr string) string {
	m := map[string]string{
		"角色序号": "id", "角色名称": "role_name", "角色权限": "role_key",
		"角色排序": "role_sort", "数据范围": "data_scope",
		"菜单树选择项是否关联显示": "menu_check_strictly",
		"部门树选择项是否关联显示": "dept_check_strictly",
		"角色状态":         "status", "备注": "remark", "创建时间": "create_time",
	}
	return m[hdr]
}

func pickSnake(field string) func(any) any {
	return func(row any) any {
		m, ok := row.(map[string]any)
		if !ok {
			return nil
		}
		return m[field]
	}
}

func toAnySlice(rows []map[string]any) []any {
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i])
	}
	return out
}

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
