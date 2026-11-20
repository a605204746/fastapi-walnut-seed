package user

import (
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/module/system/post"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
	"walnut-seed-go/internal/pkg/storagex"
)

type Handler struct {
	db      *gorm.DB
	errs    *errs.Handler
	perm    *permguard.Guard
	idem    *idempotentx.Guard
	storage storagex.Service
}

func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard, storage storagex.Service) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem, storage: storage}
}

func (h *Handler) Mount(b routex.Binder) {
	require, repeat := h.guards()

	b.GET("/getInfo", h.GetInfo)
	b.GET("/profile", h.GetProfile)
	b.PUT("/profile", repeat(), h.UpdateProfile)
	b.PUT("/profile/updatePwd", repeat(), h.UpdatePwd)
	b.POST("/profile/avatar", repeat(), h.Avatar)
	b.GET("/list", require("system:user:list"), h.List)
	b.GET("/list/dept/:dept_id", require("system:user:list"), h.ListByDept)
	b.POST("/export", require("system:user:export"), h.Export)
	b.POST("/importData", require("system:user:import"), h.ImportData)
	b.POST("/importTemplate", h.ImportTemplate)
	b.GET("/optionselect", require("system:user:query"), h.OptionSelect)
	b.GET("/deptTree", require("system:user:list"), h.DeptTree)
	b.GET("/authRole/:user_id", require("system:user:query"), h.GetAuthRole)
	b.PUT("/resetPwd", require("system:user:resetPwd"), repeat(), h.ResetPwd)
	b.PUT("/changeStatus", require("system:user:edit"), repeat(), h.ChangeStatus)
	b.PUT("/authRole", require("system:user:edit"), repeat(), h.AuthRole)
	b.POST("", require("system:user:add"), repeat(), h.Add)
	b.PUT("", require("system:user:edit"), repeat(), h.Update)
	b.DELETE("/:user_ids", require("system:user:remove"), h.Delete)
	b.GET("/", require("system:user:query"), h.GetUserInfo)
	b.GET("/:user_id", require("system:user:query"), h.GetByID)
}

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

func (h *Handler) service(c *gin.Context) (*Service, bool) {
	ac, ok := authctx.From(c)
	if !ok {
		h.errs.Abort(c, errs.NotLogin())
		return nil, false
	}
	return NewService(h.db, ac), true
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
	tree, err := dept.NewService(h.db, svc.auth).TreeSelectOut(c.Request.Context(), q)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	envelope.OK(c, tree)
}

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
	in, err := DecodeUpdate(readBody(c))
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
	in, err := DecodeResetPwd(readBody(c))
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
	in, err := DecodeChangeStatus(readBody(c))
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
	if err := c.Request.ParseForm(); err != nil {
		h.errs.Abort(c, errs.BadRequest(errs.MsgValidationFailed))
		return
	}
	// Python：query 打底，非空 form 值覆盖；pageNum/pageSize 丢弃；
	// id/userIds/excludeUserIds 参与类型校验但不进导出条件
	merged := make(map[string]string)
	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			merged[k] = v[0]
		}
	}
	for k, v := range c.Request.PostForm {
		if len(v) > 0 && v[0] != "" {
			merged[k] = v[0]
		}
	}
	delete(merged, "pageNum")
	delete(merged, "pageSize")

	q := QueryParam{}
	q.UserName = strings.TrimSpace(merged["userName"])
	q.NickName = strings.TrimSpace(merged["nickName"])
	q.Status = strings.TrimSpace(merged["status"])
	q.PhoneNumber = strings.TrimSpace(merged["phonenumber"])
	// UserQueryParam.model_validate 抛 pydantic ValidationError（⊂ValueError）
	// → 全局 ValueError 处理器透传 __str__ 原文（与 post/export 同款排版）
	var ferrs []post.FormFieldError
	for _, key := range []string{"id", "deptId"} {
		if raw, ok := merged[key]; ok {
			n, valid := dept.ParsePyQueryInt(raw)
			if !valid {
				ferrs = append(ferrs, post.FormFieldError{Loc: key, Msg: msgValidInt, Typ: "int_parsing", Input: raw})
				continue
			}
			if key == "deptId" {
				q.DeptID = &n
			}
		}
	}
	for _, key := range []string{"beginTime", "endTime"} {
		if raw, ok := merged[key]; ok {
			v, derr := dept.ParseDateQuery(raw)
			if derr != nil {
				if se := errs.As(derr); se != nil {
					ferrs = append(ferrs, post.FormFieldError{Loc: key, Msg: se.Msg, Typ: post.DateErrType(se.Msg), Input: raw})
				}
				continue
			}
			if key == "beginTime" {
				q.BeginTime = v
			} else {
				q.EndTime = v
			}
		}
	}
	if len(ferrs) > 0 {
		h.errs.Abort(c, errs.BadRequest(post.ValidationErrorText("UserQueryParam", ferrs)))
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
	in, err := DecodeProfileUpdate(readBody(c))
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
	in, err := DecodeUpdatePwd(readBody(c))
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
	if h.storage == nil {
		h.errs.Abort(c, errs.New("对象存储未初始化，头像上传不可用"))
		return
	}
	file, header, err := c.Request.FormFile("avatarfile")
	if err != nil {
		h.errs.Abort(c, errs.New("上传图片异常，请联系管理员"))
		return
	}
	defer file.Close()

	// 扩展名白名单（仅图片），文案与 Python _IMAGE_EXTENSIONS 拼接一致
	ext := strings.ToLower(filepath.Ext(header.Filename))
	ext = strings.TrimPrefix(ext, ".")
	imageExts := map[string]bool{"bmp": true, "gif": true, "jpg": true, "jpeg": true, "png": true}
	if !imageExts[ext] {
		h.errs.Abort(c, errs.New("文件格式不正确，请上传[bmp, gif, jpg, jpeg, png]格式"))
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}

	result, err := h.storage.Upload(c.Request.Context(), header.Filename, data)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}

	svc, ok := h.service(c)
	if !ok {
		return
	}
	n, err := svc.repo.UpdateAvatar(c.Request.Context(), svc.auth.User.ID, result.URL)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if n == 0 {
		h.errs.Abort(c, errs.New("上传图片异常，请联系管理员"))
		return
	}
	envelope.OK(c, map[string]string{"imgUrl": result.URL})
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
