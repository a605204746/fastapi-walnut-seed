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

	b.GET("/list", require("system:role:list"), h.List)
	b.GET("/optionselect", require("system:role:query"), h.OptionSelect)
	b.GET("/authUser/allocatedList", require("system:role:list"), h.AllocatedList)
	b.GET("/authUser/unallocatedList", require("system:role:list"), h.UnallocatedList)
	b.GET("/deptTree/:role_id", require("system:role:list"), h.DeptTree)
	b.POST("/export", require("system:role:export"), h.Export)
	b.POST("", require("system:role:add"), repeat(), h.Add)
	b.PUT("", require("system:role:edit"), repeat(), h.Update)
	b.PUT("/dataScope", require("system:role:edit"), repeat(), h.DataScope)
	b.PUT("/changeStatus", require("system:role:edit"), repeat(), h.ChangeStatus)
	b.PUT("/authUser/cancel", require("system:role:edit"), repeat(), h.CancelAuthUser)
	b.PUT("/authUser/cancelAll", require("system:role:edit"), repeat(), h.CancelAll)
	b.PUT("/authUser/selectAll", require("system:role:edit"), repeat(), h.SelectAll)
	b.GET("/:role_id", require("system:role:query"), h.GetByID)
	b.DELETE("/:role_ids", require("system:role:remove"), h.Delete)
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
			n, err := strconv.ParseInt(item, 10, 64)
			if err != nil {
				h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
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
	in, err := DecodeCreate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if in.RoleName == nil || *in.RoleName == "" {
		h.errs.Abort(c, errs.New(msgRoleNameEmpty))
		return
	}
	if in.RoleKey == nil || *in.RoleKey == "" {
		h.errs.Abort(c, errs.New(msgRoleKeyEmpty))
		return
	}
	if in.RoleSort == nil {
		h.errs.Abort(c, errs.New(msgRoleSortEmpty))
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	exists, err := svc.CheckRoleNameUnique(ctx, *in.RoleName, nil)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if exists {
		h.errs.Abort(c, errs.Newf("新增角色'%s'失败，角色名称已存在", *in.RoleName))
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
	in, err := DecodeUpdate(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if in.ID == nil {
		h.errs.Abort(c, errs.New(msgRoleIDEmpty))
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	if in.RoleName != nil {
		exists, err := svc.CheckRoleNameUnique(ctx, *in.RoleName, in.ID)
		if err != nil {
			h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
			return
		}
		if exists {
			h.errs.Abort(c, errs.Newf("修改角色'%s'失败，角色名称已存在", *in.RoleName))
			return
		}
	}
	if in.RoleKey != nil {
		exists, err := svc.CheckRoleKeyUnique(ctx, *in.RoleKey, in.ID)
		if err != nil {
			h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
			return
		}
		if exists {
			h.errs.Abort(c, errs.Newf("修改角色'%s'失败，角色权限已存在", *in.RoleName))
			return
		}
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
	in, err := DecodeDataScope(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if in.ID == nil {
		h.errs.Abort(c, errs.BadRequest(msgFieldRequired))
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
	in, err := DecodeStatus(readBody(c))
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	if in.ID == nil {
		h.errs.Abort(c, errs.BadRequest(msgFieldRequired))
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
	q.BeginTime = strings.TrimSpace(c.PostForm("beginTime"))
	q.EndTime = strings.TrimSpace(c.PostForm("endTime"))

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

type DeptTreeOut struct {
	CheckedKeys []idtype.JSInt `json:"checkedKeys"`
	Depts       []*TreeNode    `json:"depts"`
}

type TreeNode struct {
	ID       idtype.JSInt  `json:"id"`
	ParentID *idtype.JSInt `json:"parentId"`
	Label    *string       `json:"label"`
	Weight   *idtype.JSInt `json:"weight"`
	Disabled bool          `json:"disabled"`
	Children []*TreeNode   `json:"children,omitempty"`
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
	ctx := c.Request.Context()

	row, err := svc.repo.GetByIDNoScope(ctx, roleID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if row == nil {
		h.errs.Abort(c, errs.New("角色不存在"))
		return
	}

	dcs := false
	if row.DeptCheckStrictly != nil {
		dcs = *row.DeptCheckStrictly != 0
	}
	checkedKeys, err := svc.repo.CheckedDeptIDs(ctx, roleID, dcs)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}

	nodes, err := svc.repo.DeptTreeNodes(ctx)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}

	tree := buildDeptTree(nodes)

	cks := make([]idtype.JSInt, 0, len(checkedKeys))
	for _, id := range checkedKeys {
		cks = append(cks, idtype.JSInt(id))
	}

	envelope.OK(c, DeptTreeOut{CheckedKeys: cks, Depts: tree})
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
	in, err := DecodeCancel(readBody(c))
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
	roleIDStr := strings.TrimSpace(c.Query("roleId"))
	if roleIDStr == "" {
		h.errs.Abort(c, errs.BadRequest(msgFieldRequired))
		return
	}
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		h.errs.Abort(c, errs.BadRequest(msgValidInt))
		return
	}
	userIDs, err := crudx.ParseIDs(c.Query("userIds"), "用户ID")
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
	roleIDStr := strings.TrimSpace(c.Query("roleId"))
	if roleIDStr == "" {
		h.errs.Abort(c, errs.BadRequest(msgFieldRequired))
		return
	}
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		h.errs.Abort(c, errs.BadRequest(msgValidInt))
		return
	}
	userIDs, err := crudx.ParseIDs(c.Query("userIds"), "用户ID")
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

func buildDeptTree(nodes []DeptTreeNode) []*TreeNode {
	nodeMap := make(map[int64]*TreeNode, len(nodes))
	idSet := make(map[int64]bool, len(nodes))
	for _, n := range nodes {
		idSet[n.ID] = true
		tn := &TreeNode{
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

	roots := make([]*TreeNode, 0)
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
