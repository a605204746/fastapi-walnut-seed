package role

import (
	"context"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
	"walnut-seed-go/internal/pkg/timex"
)

var nowFunc = time.Now

var builtinRoleKeys = map[string]bool{
	"superadmin": true,
	"admin":      true,
}

type Service struct {
	repo *Repo
	auth authctx.Context
}

func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

type PageOut struct {
	Rows  []Out        `json:"rows"`
	Total idtype.JSInt `json:"total"`
}

func (s *Service) buildScope(ctx context.Context) (permx.Scope, error) {
	return permx.Build(ctx, s.repo.db, s.auth, permx.Default)
}

func (s *Service) buildUserScope(ctx context.Context) (permx.Scope, error) {
	return permx.Build(ctx, s.repo.db, s.auth, permx.UserList)
}

func (s *Service) buildConditions(q QueryParam) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 5)
	if crudx.IsNotBlank(q.RoleName) {
		conds = append(conds, crudx.Like("role_name", q.RoleName))
	}
	if crudx.IsNotBlank(q.RoleKey) {
		conds = append(conds, crudx.Like("role_key", q.RoleKey))
	}
	if crudx.IsNotBlank(q.Status) {
		conds = append(conds, crudx.Eq("status", q.Status))
	}
	if q.BeginTime != "" && q.EndTime != "" {
		conds = append(conds, crudx.Between("create_time", q.BeginTime, q.EndTime))
	}
	return conds
}

func (s *Service) buildUserConditions(q AuthUserQueryParam) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 3)
	if crudx.IsNotBlank(q.UserName) {
		conds = append(conds, crudx.Like("u.user_name", q.UserName))
	}
	if crudx.IsNotBlank(q.PhoneNumber) {
		conds = append(conds, crudx.Like("u.phonenumber", q.PhoneNumber))
	}
	if crudx.IsNotBlank(q.Status) {
		conds = append(conds, crudx.Eq("u.status", q.Status))
	}
	return conds
}

func (s *Service) PageList(ctx context.Context, q QueryParam) (*PageOut, error) {
	scope, err := s.buildScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	conds := s.buildConditions(q)
	rows, total, err := s.repo.Page(ctx, q.Page, conds, scope)
	if err != nil {
		return nil, err
	}
	return &PageOut{Rows: toOuts(rows), Total: idtype.JSInt(total)}, nil
}

func (s *Service) SelectList(ctx context.Context, q QueryParam) ([]SysRole, error) {
	scope, err := s.buildScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	conds := s.buildConditions(q)
	return s.repo.ListAll(ctx, conds, scope)
}

func (s *Service) GetByID(ctx context.Context, id int64) (*Out, error) {
	if err := s.CheckRoleDataScope(ctx, []int64{id}); err != nil {
		return nil, err
	}
	scope, err := s.buildScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	row, err := s.repo.Get(ctx, id, scope)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	out := NewOut(row)
	return &out, nil
}

func (s *Service) OptionSelect(ctx context.Context, ids []int64) ([]Out, error) {
	scope, err := s.buildScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	rows, err := s.repo.ListOptionByIDs(ctx, ids, scope)
	if err != nil {
		return nil, err
	}
	return toOuts(rows), nil
}

func (s *Service) InsertRole(ctx context.Context, in CreateIn) (bool, error) {
	if err := s.CheckRoleAllowed(ctx, nil, in.RoleKey); err != nil {
		return false, err
	}
	// 默认值与 Python 端 SQLAlchemy 列默认对齐：
	// data_scope='1' (全部), menu_check_strictly=1, dept_check_strictly=1, status='0'
	defaultScope := "1"
	defaultStrictly := 1
	row := SysRole{
		Status:            modelx.Normal,
		DelFlag:           modelx.Normal,
		DataScope:         &defaultScope,
		MenuCheckStrictly: &defaultStrictly,
		DeptCheckStrictly: &defaultStrictly,
	}
	if in.RoleName != nil {
		row.RoleName = *in.RoleName
	}
	if in.RoleKey != nil {
		row.RoleKey = *in.RoleKey
	}
	if in.RoleSort != nil {
		row.RoleSort = *in.RoleSort
	}
	if in.DataScope != nil {
		row.DataScope = in.DataScope
	}
	if in.MenuCheckStrictly != nil {
		row.MenuCheckStrictly = boolToInt(in.MenuCheckStrictly)
	}
	if in.DeptCheckStrictly != nil {
		row.DeptCheckStrictly = boolToInt(in.DeptCheckStrictly)
	}
	if in.Status != nil {
		row.Status = *in.Status
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	}
	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return false, err
	}
	_, err := s.repo.InsertRoleMenu(ctx, row.ID, in.MenuIDs)
	if err != nil {
		return false, err
	}
	if in.DataScope != nil && *in.DataScope == permx.ScopeCustom {
		_, err = s.repo.InsertRoleDept(ctx, row.ID, in.DeptIDs)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Service) UpdateRole(ctx context.Context, in UpdateIn) (bool, error) {
	if in.ID == nil {
		return false, nil
	}
	row, err := s.repo.GetByIDNoScope(ctx, *in.ID)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	if err := s.CheckRoleAllowed(ctx, in.ID, in.RoleKey); err != nil {
		return false, err
	}
	if row.RoleKey != derefStr(in.RoleKey) {
		if builtinRoleKeys[row.RoleKey] {
			return false, errs.New("不允许修改系统内置管理员角色标识符!")
		}
		if in.RoleKey != nil && builtinRoleKeys[*in.RoleKey] {
			return false, errs.New("不允许使用系统内置管理员角色标识符!")
		}
	}
	if err := s.CheckRoleDataScope(ctx, []int64{*in.ID}); err != nil {
		return false, err
	}
	if err := s.CheckRoleSelfEdit(ctx, *in.ID); err != nil {
		return false, err
	}
	if in.Status != nil && *in.Status == modelx.Disabled {
		n, err := s.repo.CountUserRoleByRoleID(ctx, *in.ID)
		if err != nil {
			return false, err
		}
		if n > 0 {
			return false, errs.New("角色已分配，不能禁用!")
		}
	}
	scope, err := s.buildScope(ctx)
	if err != nil {
		return false, errs.Wrap(err, errs.MsgDBFailed)
	}
	loaded, err := s.repo.Get(ctx, *in.ID, scope)
	if err != nil {
		return false, err
	}
	if loaded == nil {
		return false, nil
	}
	if in.RoleName != nil {
		loaded.RoleName = *in.RoleName
	}
	if in.RoleKey != nil {
		loaded.RoleKey = *in.RoleKey
	}
	if in.RoleSort != nil {
		loaded.RoleSort = *in.RoleSort
	}
	if in.DataScope != nil {
		loaded.DataScope = in.DataScope
	}
	if in.MenuCheckStrictly != nil {
		loaded.MenuCheckStrictly = boolToInt(in.MenuCheckStrictly)
	}
	if in.DeptCheckStrictly != nil {
		loaded.DeptCheckStrictly = boolToInt(in.DeptCheckStrictly)
	}
	if in.Status != nil {
		loaded.Status = *in.Status
	}
	if in.Remark != nil {
		loaded.Remark = in.Remark
	}
	crudx.FillUpdate(&loaded.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, loaded); err != nil {
		return false, err
	}
	if err := s.repo.DeleteRoleMenu(ctx, *in.ID); err != nil {
		return false, err
	}
	if _, err := s.repo.InsertRoleMenu(ctx, *in.ID, in.MenuIDs); err != nil {
		return false, err
	}
	if in.DataScope != nil && *in.DataScope == permx.ScopeCustom {
		if err := s.repo.DeleteRoleDept(ctx, *in.ID); err != nil {
			return false, err
		}
		if _, err := s.repo.InsertRoleDept(ctx, *in.ID, in.DeptIDs); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Service) AuthDataScope(ctx context.Context, in DataScopeIn) (bool, error) {
	if in.ID == nil {
		return false, nil
	}
	if err := s.CheckRoleAllowed(ctx, in.ID, nil); err != nil {
		return false, err
	}
	if err := s.CheckRoleDataScope(ctx, []int64{*in.ID}); err != nil {
		return false, err
	}
	if err := s.CheckRoleSelfEdit(ctx, *in.ID); err != nil {
		return false, err
	}
	scope, err := s.buildScope(ctx)
	if err != nil {
		return false, errs.Wrap(err, errs.MsgDBFailed)
	}
	row, err := s.repo.Get(ctx, *in.ID, scope)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	if in.DataScope != nil {
		row.DataScope = in.DataScope
	}
	if in.DeptCheckStrictly != nil {
		row.DeptCheckStrictly = boolToInt(in.DeptCheckStrictly)
	}
	crudx.FillUpdate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, row); err != nil {
		return false, err
	}
	if err := s.repo.DeleteRoleDept(ctx, *in.ID); err != nil {
		return false, err
	}
	if _, err := s.repo.InsertRoleDept(ctx, *in.ID, in.DeptIDs); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) UpdateRoleStatus(ctx context.Context, in StatusIn) (bool, error) {
	if in.ID == nil {
		return false, nil
	}
	if in.Status == nil {
		return false, errs.New("角色状态不能为空")
	}
	if err := s.CheckRoleAllowed(ctx, in.ID, nil); err != nil {
		return false, err
	}
	if err := s.CheckRoleDataScope(ctx, []int64{*in.ID}); err != nil {
		return false, err
	}
	if *in.Status == modelx.Disabled {
		n, err := s.repo.CountUserRoleByRoleID(ctx, *in.ID)
		if err != nil {
			return false, err
		}
		if n > 0 {
			return false, errs.New("角色已分配，不能禁用!")
		}
	}
	scope, err := s.buildScope(ctx)
	if err != nil {
		return false, errs.Wrap(err, errs.MsgDBFailed)
	}
	row, err := s.repo.Get(ctx, *in.ID, scope)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	row.Status = *in.Status
	crudx.FillUpdate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, row); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) DeleteRoleByIDs(ctx context.Context, ids []int64) (int64, error) {
	if err := s.CheckRoleDataScope(ctx, ids); err != nil {
		return 0, err
	}
	roles, err := s.repo.ListByIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	for _, role := range roles {
		rid := role.ID
		rkey := role.RoleKey
		if err := s.CheckRoleAllowed(ctx, &rid, &rkey); err != nil {
			return 0, err
		}
		n, err := s.repo.CountUserRoleByRoleID(ctx, rid)
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return 0, errs.Newf("%s已分配，不能删除!", role.RoleName)
		}
	}
	if err := s.repo.DeleteRoleMenuBatch(ctx, ids); err != nil {
		return 0, err
	}
	if err := s.repo.DeleteRoleDeptBatch(ctx, ids); err != nil {
		return 0, err
	}
	return s.repo.SoftDelete(ctx, ids)
}

func (s *Service) AllocatedList(ctx context.Context, q AuthUserQueryParam) (*AuthUserPageOut, error) {
	scope, err := s.buildUserScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	conds := s.buildUserConditions(q)
	rows, total, err := s.repo.PageAllocatedUsers(ctx, q.RoleID, q.Page, conds, scope)
	if err != nil {
		return nil, err
	}
	outs := s.toAuthUserOuts(ctx, rows)
	return &AuthUserPageOut{Rows: outs, Total: idtype.JSInt(total)}, nil
}

func (s *Service) UnallocatedList(ctx context.Context, q AuthUserQueryParam) (*AuthUserPageOut, error) {
	scope, err := s.buildUserScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	conds := s.buildUserConditions(q)
	rows, total, err := s.repo.PageUnallocatedUsers(ctx, q.RoleID, q.Page, conds, scope)
	if err != nil {
		return nil, err
	}
	outs := s.toAuthUserOuts(ctx, rows)
	return &AuthUserPageOut{Rows: outs, Total: idtype.JSInt(total)}, nil
}

func (s *Service) CancelAuthUser(ctx context.Context, in CancelIn) (int64, error) {
	if in.RoleID == nil || in.UserID == nil {
		return 0, nil
	}
	if *in.UserID == s.auth.User.ID {
		return 0, errs.New("不允许修改当前用户角色!")
	}
	if err := s.CheckAuthUserAllowed(ctx, []int64{*in.UserID}); err != nil {
		return 0, err
	}
	return s.repo.DeleteAuthUser(ctx, *in.RoleID, *in.UserID)
}

func (s *Service) CancelAuthUsers(ctx context.Context, roleID int64, userIDs []int64) (int64, error) {
	for _, uid := range userIDs {
		if uid == s.auth.User.ID {
			return 0, errs.New("不允许修改当前用户角色!")
		}
	}
	if err := s.CheckAuthUserAllowed(ctx, userIDs); err != nil {
		return 0, err
	}
	return s.repo.DeleteAuthUsers(ctx, roleID, userIDs)
}

func (s *Service) SelectAuthUsers(ctx context.Context, roleID int64, userIDs []int64) (int, error) {
	if err := s.CheckRoleDataScope(ctx, []int64{roleID}); err != nil {
		return 0, err
	}
	for _, uid := range userIDs {
		if uid == s.auth.User.ID {
			return 0, errs.New("不允许修改当前用户角色!")
		}
	}
	if err := s.CheckAuthUserAllowed(ctx, userIDs); err != nil {
		return 0, err
	}
	return s.repo.InsertAuthUsers(ctx, roleID, userIDs)
}

// ---- 校验方法 ----

func (s *Service) CheckRoleAllowed(_ context.Context, roleID *int64, roleKey *string) error {
	if roleID != nil && *roleID == modelx.SuperAdminID {
		return errs.New("不允许操作超级管理员角色")
	}
	if roleKey != nil && builtinRoleKeys[*roleKey] {
		return errs.New("不允许使用系统内置管理员角色标识符!")
	}
	return nil
}

func (s *Service) CheckRoleDataScope(ctx context.Context, ids []int64) error {
	if s.auth.User.IsSuperAdmin() {
		return nil
	}
	scope, err := s.buildScope(ctx)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	n, err := s.repo.CountVisible(ctx, ids, scope)
	if err != nil {
		return err
	}
	if n != int64(len(ids)) {
		return errs.New("没有权限访问部分角色数据！")
	}
	return nil
}

func (s *Service) CheckRoleNameUnique(ctx context.Context, name string, excludeID *int64) (bool, error) {
	return s.repo.ExistsByName(ctx, name, excludeID)
}

func (s *Service) CheckRoleKeyUnique(ctx context.Context, key string, excludeID *int64) (bool, error) {
	return s.repo.ExistsByKey(ctx, key, excludeID)
}

func (s *Service) CheckRoleSelfEdit(ctx context.Context, roleID int64) error {
	if s.auth.User.IsSuperAdmin() {
		return nil
	}
	ids, err := s.repo.CurrentUserRoleIDs(ctx, s.auth.User.ID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == roleID {
			return errs.New("不允许修改自身所属角色，防止自我提权")
		}
	}
	return nil
}

func (s *Service) CheckAuthUserAllowed(ctx context.Context, userIDs []int64) error {
	if s.auth.User.IsSuperAdmin() {
		return nil
	}
	scope, err := s.buildUserScope(ctx)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	for _, uid := range userIDs {
		if uid == modelx.SuperAdminID {
			return errs.New("不允许操作超级管理员用户")
		}
		n, err := s.repo.CountUserByUserIDAndVisible(ctx, uid, scope)
		if err != nil {
			return err
		}
		if n == 0 {
			return errs.New("没有权限访问用户数据！")
		}
	}
	return nil
}

// ---- AuthUser 出参转换 ----

func (s *Service) toAuthUserOuts(ctx context.Context, rows []AuthUserRow) []AuthUserOut {
	outs := make([]AuthUserOut, 0, len(rows))
	deptIDs := make([]int64, 0)
	for _, r := range rows {
		if r.DeptID != nil {
			deptIDs = append(deptIDs, *r.DeptID)
		}
	}
	deptMap, _ := s.repo.DeptNameMap(ctx, deptIDs)

	for _, r := range rows {
		out := AuthUserOut{
			ID:          jsIntPtr(r.ID),
			UserName:    strPtr(r.UserName),
			NickName:    r.NickName,
			Email:       r.Email,
			PhoneNumber: r.PhoneNumber,
			Status:      strPtr(r.Status),
		}
		if r.DeptID != nil {
			out.DeptID = jsIntPtr(*r.DeptID)
			out.DeptName = deptMap[*r.DeptID]
		}
		if r.CreateTime != nil {
			jt := timex.FromTime(*r.CreateTime)
			out.CreateTime = &jt
		}
		outs = append(outs, out)
	}
	return outs
}

// ---- 导出 map 转换 ----

func ExportRows(roles []SysRole) []map[string]any {
	out := make([]map[string]any, 0, len(roles))
	for i := range roles {
		out = append(out, ExportRow(roles[i]))
	}
	return out
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
