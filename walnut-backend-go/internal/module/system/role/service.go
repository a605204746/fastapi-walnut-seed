package role

import (
	"context"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/dept"
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

// roleRepo 是 service 消费的数据访问能力（consumer-side 接口，定义在消费方）。
// 只列实际调用的方法（含 DeptTree 编排调用的 GetByIDNoScope/CheckedDeptIDs）；
// *Repo 满足它，单测以 mock 替换。
type roleRepo interface {
	Page(ctx context.Context, page crudx.Page, conds []crudx.Cond, scope permx.Scope) ([]SysRole, int64, error)
	ListAll(ctx context.Context, conds []crudx.Cond, scope permx.Scope) ([]SysRole, error)
	Get(ctx context.Context, id int64, scope permx.Scope) (*SysRole, error)
	GetByIDNoScope(ctx context.Context, id int64) (*SysRole, error)
	ListByIDs(ctx context.Context, ids []int64) ([]SysRole, error)
	ListOptionByIDs(ctx context.Context, ids []int64, scope permx.Scope) ([]SysRole, error)
	CountVisible(ctx context.Context, ids []int64, scope permx.Scope) (int64, error)
	ExistsByName(ctx context.Context, name string, excludeID *int64) (bool, error)
	ExistsByKey(ctx context.Context, key string, excludeID *int64) (bool, error)
	CountUserRoleByRoleID(ctx context.Context, roleID int64) (int64, error)
	SoftDelete(ctx context.Context, ids []int64) (int64, error)
	Create(ctx context.Context, row *SysRole) error
	Update(ctx context.Context, row *SysRole) error
	DeleteRoleMenu(ctx context.Context, roleID int64) error
	DeleteRoleMenuBatch(ctx context.Context, roleIDs []int64) error
	InsertRoleMenu(ctx context.Context, roleID int64, menuIDs []int64) (int, error)
	DeleteRoleDept(ctx context.Context, roleID int64) error
	DeleteRoleDeptBatch(ctx context.Context, roleIDs []int64) error
	InsertRoleDept(ctx context.Context, roleID int64, deptIDs []int64) (int, error)
	DeleteAuthUser(ctx context.Context, roleID, userID int64) (int64, error)
	DeleteAuthUsers(ctx context.Context, roleID int64, userIDs []int64) (int64, error)
	InsertAuthUsers(ctx context.Context, roleID int64, userIDs []int64) (int, error)
	CurrentUserRoleIDs(ctx context.Context, userID int64) ([]int64, error)
	PageAllocatedUsers(ctx context.Context, roleID *int64, page crudx.Page, conds []crudx.Cond, scope permx.Scope) ([]AuthUserRow, int64, error)
	PageUnallocatedUsers(ctx context.Context, roleID *int64, page crudx.Page, conds []crudx.Cond, scope permx.Scope) ([]AuthUserRow, int64, error)
	DeptNameMap(ctx context.Context, deptIDs []int64) (map[int64]*string, error)
	CountUserByUserIDAndVisible(ctx context.Context, userID int64, scope permx.Scope) (int64, error)
	CheckedDeptIDs(ctx context.Context, roleID int64, deptCheckStrictly bool) ([]int64, error)
}

// DeptProvider 是 role 域消费的跨模块能力（consumer-side 窄接口）：
// 部门树查询，具体实现由 bootstrap 注入 dept.Service。
type DeptProvider interface {
	TreeSelectAll(ctx context.Context) ([]*dept.TreeNode, error)
}

// Deps role 模块依赖：跨模块服务工厂（按请求 authctx 构造）。
type Deps struct {
	NewDepts func(authctx.Context) DeptProvider
}

type Service struct {
	db    *gorm.DB
	repo  roleRepo
	auth  authctx.Context
	depts DeptProvider
}

func NewService(db *gorm.DB, ac authctx.Context, deps Deps) *Service {
	s := &Service{db: db, repo: NewRepo(db), auth: ac}
	// 仅 DeptTree 消费 depts；作为 user.RoleProvider 复用时无需注入（Deps{}）。
	if deps.NewDepts != nil {
		s.depts = deps.NewDepts(ac)
	}
	return s
}

func (s *Service) GetByIDNoScope(ctx context.Context, id int64) (*SysRole, error) {
	return s.repo.GetByIDNoScope(ctx, id)
}

func (s *Service) CheckedDeptIDs(ctx context.Context, roleID int64, deptCheckStrictly bool) ([]int64, error) {
	return s.repo.CheckedDeptIDs(ctx, roleID, deptCheckStrictly)
}

// DeptTree 部门树回显编排（自 handler 下沉，行为不变）：
// 角色存在性 → dcs → 勾选部门 → dept 树 → JSInt 转换 → 组装。
func (s *Service) DeptTree(ctx context.Context, roleID int64) (DeptTreeOut, error) {
	row, err := s.GetByIDNoScope(ctx, roleID)
	if err != nil {
		return DeptTreeOut{}, errs.Wrap(err, errs.MsgDBFailed)
	}
	if row == nil {
		return DeptTreeOut{}, errs.New("角色不存在")
	}

	dcs := false
	if row.DeptCheckStrictly != nil {
		dcs = *row.DeptCheckStrictly != 0
	}
	checkedKeys, err := s.CheckedDeptIDs(ctx, roleID, dcs)
	if err != nil {
		return DeptTreeOut{}, errs.Wrap(err, errs.MsgDBFailed)
	}

	// Python 经 DeptService.select_dept_tree_list()：带 dept 自身 id 列数据权限，
	// 根与每层 children 按 (orderNum, id) 排序。复用 dept 模块同款实现。
	tree, err := s.depts.TreeSelectAll(ctx)
	if err != nil {
		return DeptTreeOut{}, errs.Wrap(err, errs.MsgDBFailed)
	}

	cks := make([]idtype.JSInt, 0, len(checkedKeys))
	for _, id := range checkedKeys {
		cks = append(cks, idtype.JSInt(id))
	}

	return DeptTreeOut{CheckedKeys: cks, Depts: tree}, nil
}

type PageOut struct {
	Rows  []Out        `json:"rows"`
	Total idtype.JSInt `json:"total"`
}

func (s *Service) buildScope(ctx context.Context) (permx.Scope, error) {
	return permx.Build(ctx, s.db, s.auth, permx.Default)
}

func (s *Service) buildUserScope(ctx context.Context) (permx.Scope, error) {
	return permx.Build(ctx, s.db, s.auth, permx.UserList)
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

// InsertRole 新增角色。多写操作（角色行 + 菜单关联 + 自定义范围部门关联）
// 包在事务里，任一步失败整体回滚（对齐 Python session.begin，不留孤儿角色）。
func (s *Service) InsertRole(ctx context.Context, in CreateIn) (bool, error) {
	if s.db == nil { // 测试 harness 无真实 DB，直调内部
		return s.insertRoleTx(ctx, in)
	}
	var done bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var e error
		done, e = s.insertRoleTx(crudx.WithTx(ctx, tx), in)
		return e
	})
	return done, err
}

func (s *Service) insertRoleTx(ctx context.Context, in CreateIn) (bool, error) {
	// Python: status / role_sort 列 nullable=False 且**无默认值**，缺省落 NULL →
	// IntegrityError → 全局处理器转 409「数据已存在或违反完整性约束」（实测钉住）。
	// check_role_allowed 由 handler 在唯一性校验之前调用（controller 顺序）。
	if in.Status == nil || in.RoleSort == nil {
		return false, errs.Conflict(errs.MsgDBIntegrity)
	}
	// 其余列默认与 Python 端 SQLAlchemy 列默认对齐（显式 None 也会触发默认值，实测）：
	// data_scope='1' (全部), menu_check_strictly=1, dept_check_strictly=1
	defaultScope := "1"
	defaultStrictly := 1
	row := SysRole{
		Status:            *in.Status,
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
	row.RoleSort = *in.RoleSort
	if in.DataScope != nil {
		row.DataScope = in.DataScope
	}
	if in.MenuCheckStrictly != nil {
		row.MenuCheckStrictly = boolToInt(in.MenuCheckStrictly)
	}
	if in.DeptCheckStrictly != nil {
		row.DeptCheckStrictly = boolToInt(in.DeptCheckStrictly)
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

// UpdateRole 修改角色。多写操作（角色行更新 + 菜单关联重建 + 部门关联重建）
// 包在事务里，任一步失败整体回滚（对齐 Python session.begin）。
func (s *Service) UpdateRole(ctx context.Context, in UpdateIn) (bool, error) {
	if s.db == nil { // 测试 harness 无真实 DB，直调内部
		return s.updateRoleTx(ctx, in)
	}
	var done bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var e error
		done, e = s.updateRoleTx(crudx.WithTx(ctx, tx), in)
		return e
	})
	return done, err
}

func (s *Service) updateRoleTx(ctx context.Context, in UpdateIn) (bool, error) {
	if in.ID == nil {
		return false, nil
	}
	// check_role_allowed / check_role_data_scope / 名称与 key 唯一性由 handler
	// 按 Python controller 顺序先行调用；此处对应 service.update_role 本体。
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

// AuthDataScope 修改角色数据权限。多写操作（角色行更新 + 部门关联先删后插）
// 包在事务里，任一步失败整体回滚（对齐 Python session.begin）。
func (s *Service) AuthDataScope(ctx context.Context, in DataScopeIn) (bool, error) {
	if s.db == nil { // 测试 harness 无真实 DB，直调内部
		return s.authDataScopeTx(ctx, in)
	}
	var done bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var e error
		done, e = s.authDataScopeTx(crudx.WithTx(ctx, tx), in)
		return e
	})
	return done, err
}

func (s *Service) authDataScopeTx(ctx context.Context, in DataScopeIn) (bool, error) {
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
	// Python controller 先 check_role_allowed + check_role_data_scope，
	// 之后 service.update_role_status 才判 status 为空（顺序不可调换）
	if err := s.CheckRoleAllowed(ctx, in.ID, nil); err != nil {
		return false, err
	}
	if err := s.CheckRoleDataScope(ctx, []int64{*in.ID}); err != nil {
		return false, err
	}
	if in.Status == nil {
		return false, errs.New("角色状态不能为空")
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

// DeleteRoleByIDs 批量删除角色。多写操作（菜单/部门关联批删 + 软删角色）
// 包在事务里，任一步失败整体回滚（对齐 Python session.begin）。
func (s *Service) DeleteRoleByIDs(ctx context.Context, ids []int64) (int64, error) {
	if s.db == nil { // 测试 harness 无真实 DB，直调内部
		return s.deleteRoleByIDsTx(ctx, ids)
	}
	var n int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var e error
		n, e = s.deleteRoleByIDsTx(crudx.WithTx(ctx, tx), ids)
		return e
	})
	return n, err
}

func (s *Service) deleteRoleByIDsTx(ctx context.Context, ids []int64) (int64, error) {
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

// CheckRoleAllowed 对应 Python check_role_allowed：
//   - id==1 → 一律拒绝；
//   - 新增（roleID==nil）→ 仅拒绝「使用」内置标识符；
//   - 修改（roleID!=nil）→ 仅当 key **变化**时才校验：现 key 是内置 → 拒「修改」，
//     新 key 是内置 → 拒「使用」。保持原 key（含 DELETE 传自身 key、
//     dataScope/changeStatus 传 None 之外的相等 key）放行。
//
// ⚠️ dataScope/changeStatus 传 role_key=None：DB key 非空 → 恒视为「变化」，
// 内置角色因此被拒「不允许修改系统内置管理员角色标识符!」（Python 实测分支）。
func (s *Service) CheckRoleAllowed(ctx context.Context, roleID *int64, roleKey *string) error {
	if roleID != nil && *roleID == modelx.SuperAdminID {
		return errs.New("不允许操作超级管理员角色")
	}
	if roleID == nil {
		if roleKey != nil && builtinRoleKeys[*roleKey] {
			return errs.New("不允许使用系统内置管理员角色标识符!")
		}
		return nil
	}
	roles, err := s.repo.ListByIDs(ctx, []int64{*roleID})
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if len(roles) == 0 {
		return nil
	}
	sysRole := roles[0]
	// Python: sys_role.role_key != role_key（role_key 可为 None，DB key 非空 → 恒不等）
	keyChanged := roleKey == nil || sysRole.RoleKey != *roleKey
	if keyChanged {
		if builtinRoleKeys[sysRole.RoleKey] {
			return errs.New("不允许修改系统内置管理员角色标识符!")
		}
		if roleKey != nil && builtinRoleKeys[*roleKey] {
			return errs.New("不允许使用系统内置管理员角色标识符!")
		}
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
