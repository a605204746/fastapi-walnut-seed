package user

import (
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
)

type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
	perm *permguard.Guard
	idem *idempotentx.Guard
	deps Deps
}

func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard, deps Deps) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem, deps: deps}
}

func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac, h.deps), true
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

func (h *Handler) ListByDept(c *gin.Context) {
	deptID, err := pathInt(c, "dept_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.ListByDeptID(c.Request.Context(), deptID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) GetByID(c *gin.Context) {
	id, err := pathIntConverter(c, "user_id")
	if err != nil {
		// Python：abc 不匹配 {user_id:int}，但命中 DELETE /{user_ids} 的路径
		// → Starlette 部分匹配返回 405 Method Not Allowed（不是 404）
		if he := errs.As(err); he != nil && he.HTTPStatus == errs.CodeNotFound {
			h.errs.Abort(c, errs.HTTPError(errs.CodeBadMethod, "Method Not Allowed"))
			return
		}
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

func (h *Handler) GetUserInfo(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.GetUserInfoNoID(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) GetInfo(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.GetInfo(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if data == nil {
		envelope.Fail(c, "没有权限访问用户数据!")
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) GetAuthRole(c *gin.Context) {
	id, err := pathIntConverter(c, "user_id")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.GetAuthRole(c.Request.Context(), id)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) OptionSelect(c *gin.Context) {
	var ids []int64
	if raw := c.Query("userIds"); crudx.IsNotBlank(raw) {
		var err error
		ids, err = parseIDListStrict(raw, "用户ID")
		if err != nil {
			h.errs.Abort(c, err)
			return
		}
	}
	var deptID *int64
	if raw := c.Query("deptId"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
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

func (h *Handler) DeptTree(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	// 复用 DeptService.select_dept_tree_list：查询过滤 + 部门数据权限 + 排序
	q, err := dept.BindQueryParamChecked(c)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	tree, err := h.deps.NewDepts(svc.auth).TreeSelectOut(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, tree)
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
	done, err := svc.InsertUser(c.Request.Context(), in)
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
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.UpdateUser(c.Request.Context(), in)
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
	// Python _parse_id_list：非法 → 400「用户ID格式有误」；空串 → [] →
	// service 删 0 行 → 500「删除用户失败!」（不是 400「不能为空」）
	ids, err := parseIDListStrict(c.Param("user_ids"), "用户ID")
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.DeleteUserByIDs(c.Request.Context(), ids)
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

func (h *Handler) ResetPwd(c *gin.Context) {
	in, err := DecodeResetPwd(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.ResetPwd(c.Request.Context(), in)
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
	in, err := DecodeChangeStatus(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.ChangeStatus(c.Request.Context(), in)
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

func (h *Handler) AuthRole(c *gin.Context) {
	userIDStr := strings.TrimSpace(c.Query("userId"))
	if userIDStr == "" {
		h.errs.Abort(c, errs.New("用户ID不能为空"))
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		h.errs.Abort(c, errs.BadRequest(msgValidInt))
		return
	}
	var roleIDs []int64
	if raw := c.Query("roleIds"); crudx.IsNotBlank(raw) {
		roleIDs, err = parseIDListStrict(raw, "角色ID")
		if err != nil {
			h.errs.Abort(c, err)
			return
		}
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.InsertAuthRole(c.Request.Context(), userID, roleIDs)
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

func (h *Handler) Export(c *gin.Context) {
	q, err := BindExportParam(c)
	if err != nil {
		h.errs.Abort(c, err)
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
	cols := make([]excelx.Column, 0, len(exportHeaders))
	keys := ExportColumns()
	for i, hdr := range exportHeaders {
		key := keys[i]
		cols = append(cols, excelx.Column{Header: hdr, Value: pickSnake(key)})
	}
	anyRows := make([]any, 0, len(rows))
	for i := range rows {
		anyRows = append(anyRows, ExportRowToMap(rows[i]))
	}
	if err := excelx.Export(c, anyRows, cols, exportSheetName); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

func (h *Handler) ImportData(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		h.errs.Abort(c, errs.BadRequest("导入文件不能为空"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}

	// Python：form 优先、query 兜底；strip+lower 后属于 true/1/yes/on 即真
	raw := c.PostForm("updateSupport")
	if raw == "" {
		raw = c.Query("updateSupport")
	}
	if raw == "" {
		raw = "false"
	}
	updateSupport := false
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on":
		updateSupport = true
	}

	svc, ok := h.service(c)
	if !ok {
		return
	}
	msg, err := svc.ImportUsers(c.Request.Context(), data, updateSupport)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OKMsg(c, nil, msg)
}

func (h *Handler) ImportTemplate(c *gin.Context) {
	headers := importHeaders()
	selectorHeaders := []string{"用户性别", "账号状态"}
	options := [][]string{
		{"男", "女", "未知"},
		{"正常", "停用"},
	}
	if err := excelx.ExportTemplate(c, headers, selectorHeaders, options, "用户数据"); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
}

func importHeaders() []string {
	return []string{
		"用户序号", "部门编号", "用户账号", "用户昵称", "用户邮箱",
		"手机号码", "用户性别", "账号状态",
	}
}

// ---- Profile ----

func (h *Handler) GetProfile(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	data, err := svc.GetProfile(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if data == nil {
		envelope.Fail(c, "没有权限访问用户数据!")
		return
	}
	envelope.OK(c, data)
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	in, err := DecodeProfileUpdate(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.UpdateProfile(c.Request.Context(), in)
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

func (h *Handler) UpdatePwd(c *gin.Context) {
	in, err := DecodeUpdatePwd(c.Request.Context(), readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	done, err := svc.UpdatePwd(c.Request.Context(), in.OldPassword, in.NewPassword)
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

func (h *Handler) Avatar(c *gin.Context) {
	if h.deps.Storage == nil {
		h.errs.Abort(c, errs.New("对象存储未初始化，头像上传不可用"))
		return
	}
	file, header, err := c.Request.FormFile("avatarfile")
	if err != nil {
		h.errs.Abort(c, errs.New("上传图片异常，请联系管理员"))
		return
	}
	defer file.Close()

	svc, ok := h.service(c)
	if !ok {
		return
	}
	url, err := svc.UpdateAvatar(c.Request.Context(), header.Filename, file)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, map[string]string{"imgUrl": url})
}

// ---- Helpers ----

func pickSnake(field string) func(any) any {
	return func(row any) any {
		m, ok := row.(map[string]any)
		if !ok {
			return nil
		}
		return m[field]
	}
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

// pathIntConverter 对应 Python 的 {user_id:int} 路径转换器：
// 非纯数字路径**不匹配路由** → 框架级 HTTP 404 信封（不是 200+code400）。
func pathIntConverter(c *gin.Context, name string) (int64, error) {
	raw := c.Param(name)
	if !pyIntRE.MatchString(raw) {
		return 0, errs.HTTPError(errs.CodeNotFound, errs.MsgNotFound)
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errs.HTTPError(errs.CodeNotFound, errs.MsgNotFound)
	}
	return n, nil
}
