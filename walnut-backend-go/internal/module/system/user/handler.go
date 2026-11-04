package user

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
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/routex"
)

type Handler struct {
	db   *gorm.DB
	errs *errs.Handler
	perm *permguard.Guard
	idem *idempotentx.Guard
}

func NewHandler(db *gorm.DB, eh *errs.Handler, perm *permguard.Guard, idem *idempotentx.Guard) *Handler {
	return &Handler{db: db, errs: eh, perm: perm, idem: idem}
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
	id, err := pathInt(c, "user_id")
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
	id, err := pathInt(c, "user_id")
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
		ids = parseIDList(raw)
	}
	var deptID *int64
	if raw := c.Query("deptId"); crudx.IsNotBlank(raw) {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			deptID = &n
		}
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

type DeptTreeNodeOut struct {
	ID       idtype.JSInt       `json:"id"`
	ParentID *idtype.JSInt      `json:"parentId"`
	Label    *string            `json:"label"`
	Weight   *idtype.JSInt      `json:"weight"`
	Disabled bool               `json:"disabled"`
	Children []*DeptTreeNodeOut `json:"children,omitempty"`
}

func (h *Handler) DeptTree(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	nodes, err := svc.repo.DeptTreeNodes(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	tree := buildDeptTree(nodes)
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
	if in.ID == nil {
		h.errs.Abort(c, errs.New("用户ID不能为空"))
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
	ids, err := crudx.ParseIDs(c.Param("user_ids"), "用户ID")
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
	if in.ID == nil {
		h.errs.Abort(c, errs.New("用户ID不能为空"))
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
	if in.ID == nil {
		h.errs.Abort(c, errs.New("用户ID不能为空"))
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
		roleIDs = parseIDList(raw)
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
	q := QueryParam{}
	q.UserName = strings.TrimSpace(c.PostForm("userName"))
	q.NickName = strings.TrimSpace(c.PostForm("nickName"))
	q.Status = strings.TrimSpace(c.PostForm("status"))
	q.PhoneNumber = strings.TrimSpace(c.PostForm("phonenumber"))
	q.BeginTime = strings.TrimSpace(c.PostForm("beginTime"))
	q.EndTime = strings.TrimSpace(c.PostForm("endTime"))
	if raw := c.PostForm("deptId"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			q.DeptID = &n
		}
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
	h.errs.Abort(c, errs.New("头像上传功能待 P4 storagex 实现"))
}

// ---- Helpers ----

func buildDeptTree(nodes []DeptTreeNode) []*DeptTreeNodeOut {
	nodeMap := make(map[int64]*DeptTreeNodeOut, len(nodes))
	idSet := make(map[int64]bool, len(nodes))
	for _, n := range nodes {
		idSet[n.ID] = true
		tn := &DeptTreeNodeOut{
			ID:       idtype.JSInt(n.ID),
			Label:    n.DeptName,
			Disabled: n.Status != nil && *n.Status == "1",
		}
		if n.ParentID != nil {
			pid := idtype.JSInt(*n.ParentID)
			tn.ParentID = &pid
		}
		if n.OrderNum != nil {
			w := idtype.JSInt(*n.OrderNum)
			tn.Weight = &w
		}
		nodeMap[n.ID] = tn
	}
	roots := make([]*DeptTreeNodeOut, 0)
	for _, n := range nodes {
		tn := nodeMap[n.ID]
		if n.ParentID == nil || !idSet[*n.ParentID] {
			roots = append(roots, tn)
			continue
		}
		parent := nodeMap[*n.ParentID]
		parent.Children = append(parent.Children, tn)
	}
	return roots
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
