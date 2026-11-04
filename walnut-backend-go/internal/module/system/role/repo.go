// 本文件实现角色域的数据访问，对应 Python 端 module_system/role/crud.py。
//
// ⚠️ sys_role 有 del_flag，所有业务查询都加 del_flag='0'。
// ⚠️ sys_role 使用行级数据权限（permx.Default：create_dept 列）。
// ⚠️ sys_role_menu / sys_role_dept / sys_user_role 都是物理操作（无 del_flag）。

package role

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond, scope permx.Scope) ([]SysRole, int64, error) {
	newQuery := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&SysRole{}).Where("del_flag = ?", modelx.Normal)
		q = scope.Apply(q)
		return crudx.Apply(q, conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery().
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "role_sort"}},
			{Column: clause.Column{Name: "create_time"}},
		}})
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysRole, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) ListAll(ctx context.Context, conds []crudx.Cond, scope permx.Scope) ([]SysRole, error) {
	q := r.db.WithContext(ctx).Model(&SysRole{}).Where("del_flag = ?", modelx.Normal)
	q = scope.Apply(q)
	q = crudx.Apply(q, conds...)
	q = q.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Name: "role_sort"}},
		{Column: clause.Column{Name: "create_time"}},
	}})
	rows := make([]SysRole, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) Get(ctx context.Context, id int64, scope permx.Scope) (*SysRole, error) {
	var row SysRole
	q := r.db.WithContext(ctx).Where("id = ? AND del_flag = ?", id, modelx.Normal)
	q = scope.Apply(q)
	if err := q.First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repo) GetByIDNoScope(ctx context.Context, id int64) (*SysRole, error) {
	var row SysRole
	if err := r.db.WithContext(ctx).Where("id = ? AND del_flag = ?", id, modelx.Normal).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repo) ListByIDs(ctx context.Context, ids []int64) ([]SysRole, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows := make([]SysRole, 0)
	err := r.db.WithContext(ctx).Where("id IN ? AND del_flag = ?", ids, modelx.Normal).Find(&rows).Error
	return rows, err
}

func (r *Repo) ListOptionByIDs(ctx context.Context, ids []int64, scope permx.Scope) ([]SysRole, error) {
	q := r.db.WithContext(ctx).Model(&SysRole{}).
		Where("status = ? AND del_flag = ?", modelx.Normal, modelx.Normal)
	q = scope.Apply(q)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	q = q.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Name: "role_sort"}},
		{Column: clause.Column{Name: "create_time"}},
	}})
	rows := make([]SysRole, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) CountVisible(ctx context.Context, ids []int64, scope permx.Scope) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Model(&SysRole{}).
		Where("id IN ? AND del_flag = ?", ids, modelx.Normal)
	q = scope.Apply(q)
	err := q.Count(&n).Error
	return n, err
}

func (r *Repo) ExistsByName(ctx context.Context, name string, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysRole{}).
		Where("role_name = ? AND del_flag = ?", name, modelx.Normal)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	return countGTZero(q)
}

func (r *Repo) ExistsByKey(ctx context.Context, key string, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysRole{}).
		Where("role_key = ? AND del_flag = ?", key, modelx.Normal)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	return countGTZero(q)
}

func (r *Repo) CountUserRoleByRoleID(ctx context.Context, roleID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("sys_user_role").
		Where("role_id = ?", roleID).Count(&n).Error
	return n, err
}

func (r *Repo) SoftDelete(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&SysRole{}).
		Where("id IN ?", ids).Update("del_flag", "1")
	return res.RowsAffected, res.Error
}

func (r *Repo) Create(ctx context.Context, row *SysRole) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *Repo) Update(ctx context.Context, row *SysRole) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// ---- sys_role_menu ----

func (r *Repo) DeleteRoleMenu(ctx context.Context, roleID int64) error {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&SysRoleMenu{}).Error
}

func (r *Repo) DeleteRoleMenuBatch(ctx context.Context, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("role_id IN ?", roleIDs).Delete(&SysRoleMenu{}).Error
}

func (r *Repo) InsertRoleMenu(ctx context.Context, roleID int64, menuIDs []int64) (int, error) {
	if len(menuIDs) == 0 {
		return 0, nil
	}
	rows := make([]SysRoleMenu, 0, len(menuIDs))
	for _, mid := range menuIDs {
		rows = append(rows, SysRoleMenu{RoleID: roleID, MenuID: mid})
	}
	res := r.db.WithContext(ctx).Create(&rows)
	return int(res.RowsAffected), res.Error
}

// ---- sys_role_dept ----

func (r *Repo) DeleteRoleDept(ctx context.Context, roleID int64) error {
	return r.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&SysRoleDept{}).Error
}

func (r *Repo) DeleteRoleDeptBatch(ctx context.Context, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("role_id IN ?", roleIDs).Delete(&SysRoleDept{}).Error
}

func (r *Repo) InsertRoleDept(ctx context.Context, roleID int64, deptIDs []int64) (int, error) {
	if len(deptIDs) == 0 {
		return 0, nil
	}
	rows := make([]SysRoleDept, 0, len(deptIDs))
	for _, did := range deptIDs {
		rows = append(rows, SysRoleDept{RoleID: roleID, DeptID: did})
	}
	res := r.db.WithContext(ctx).Create(&rows)
	return int(res.RowsAffected), res.Error
}

func (r *Repo) ListDeptIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	ids := make([]int64, 0)
	err := r.db.WithContext(ctx).Table("sys_role_dept").
		Where("role_id = ?", roleID).Pluck("dept_id", &ids).Error
	return ids, err
}

// ---- sys_user_role (authUser) ----

func (r *Repo) DeleteAuthUser(ctx context.Context, roleID, userID int64) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("role_id = ? AND user_id = ?", roleID, userID).
		Delete(&SysUserRole{})
	return res.RowsAffected, res.Error
}

func (r *Repo) DeleteAuthUsers(ctx context.Context, roleID int64, userIDs []int64) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("role_id = ? AND user_id IN ?", roleID, userIDs).
		Delete(&SysUserRole{})
	return res.RowsAffected, res.Error
}

func (r *Repo) InsertAuthUsers(ctx context.Context, roleID int64, userIDs []int64) (int, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	rows := make([]SysUserRole, 0, len(userIDs))
	for _, uid := range userIDs {
		rows = append(rows, SysUserRole{UserID: uid, RoleID: roleID})
	}
	res := r.db.WithContext(ctx).Create(&rows)
	return int(res.RowsAffected), res.Error
}

func (r *Repo) UserIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	ids := make([]int64, 0)
	err := r.db.WithContext(ctx).Table("sys_user_role").
		Where("role_id = ?", roleID).Pluck("user_id", &ids).Error
	return ids, err
}

func (r *Repo) CurrentUserRoleIDs(ctx context.Context, userID int64) ([]int64, error) {
	ids := make([]int64, 0)
	err := r.db.WithContext(ctx).Table("sys_user_role").
		Where("user_id = ?", userID).Pluck("role_id", &ids).Error
	return ids, err
}

type AuthUserRow struct {
	ID          int64      `gorm:"column:id"`
	DeptID      *int64     `gorm:"column:dept_id"`
	UserName    string     `gorm:"column:user_name"`
	NickName    *string    `gorm:"column:nick_name"`
	Email       *string    `gorm:"column:email"`
	PhoneNumber *string    `gorm:"column:phonenumber"`
	Status      string     `gorm:"column:status"`
	CreateTime  *time.Time `gorm:"column:create_time"`
}

func (r *Repo) PageAllocatedUsers(ctx context.Context, roleID int64, page crudx.Page, conds []crudx.Cond, scope permx.Scope) ([]AuthUserRow, int64, error) {
	newQuery := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("sys_user u").
			Select("u.id, u.dept_id, u.user_name, u.nick_name, u.email, u.phonenumber, u.status, u.create_time").
			Joins("INNER JOIN sys_user_role ur ON ur.user_id = u.id").
			Where("u.del_flag = ? AND ur.role_id = ?", modelx.Normal, roleID)
		q = scope.Apply(q)
		return crudx.Apply(q, conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery().Order("u.id ASC")
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]AuthUserRow, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) PageUnallocatedUsers(ctx context.Context, roleID int64, page crudx.Page, conds []crudx.Cond, scope permx.Scope) ([]AuthUserRow, int64, error) {
	newQuery := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("sys_user u").
			Select("u.id, u.dept_id, u.user_name, u.nick_name, u.email, u.phonenumber, u.status, u.create_time").
			Where("u.del_flag = ? AND u.id NOT IN (SELECT ur.user_id FROM sys_user_role ur WHERE ur.role_id = ?)", modelx.Normal, roleID)
		q = scope.Apply(q)
		return crudx.Apply(q, conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery().Order("u.id ASC")
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]AuthUserRow, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ---- 部门辅助 ----

func (r *Repo) DeptNameMap(ctx context.Context, deptIDs []int64) (map[int64]*string, error) {
	out := make(map[int64]*string, len(deptIDs))
	if len(deptIDs) == 0 {
		return out, nil
	}
	type row struct {
		ID       int64   `gorm:"column:id"`
		DeptName *string `gorm:"column:dept_name"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("sys_dept").
		Select("id, dept_name").
		Where("id IN ? AND del_flag = ?", deptIDs, modelx.Normal).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].ID] = rows[i].DeptName
	}
	return out, nil
}

func (r *Repo) CountUserByUserIDAndVisible(ctx context.Context, userID int64, scope permx.Scope) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Table("sys_user").
		Where("id = ? AND del_flag = ?", userID, modelx.Normal)
	q = scope.Apply(q)
	err := q.Count(&n).Error
	return n, err
}

type SysUserRole struct {
	UserID int64 `gorm:"column:user_id;primaryKey"`
	RoleID int64 `gorm:"column:role_id;primaryKey"`
}

func (SysUserRole) TableName() string { return "sys_user_role" }

type DeptTreeNode struct {
	ID       int64   `gorm:"column:id"`
	ParentID *int64  `gorm:"column:parent_id"`
	DeptName *string `gorm:"column:dept_name"`
	OrderNum *int    `gorm:"column:order_num"`
	Status   *string `gorm:"column:status"`
}

func (r *Repo) CheckedDeptIDs(ctx context.Context, roleID int64, deptCheckStrictly bool) ([]int64, error) {
	grantSub := r.db.WithContext(ctx).Table("sys_role_dept rd").
		Select("rd.dept_id").
		Joins("INNER JOIN sys_role sr ON sr.id = rd.role_id").
		Where("rd.role_id = ? AND sr.status = ? AND sr.del_flag = ?", roleID, modelx.Normal, modelx.Normal)

	q := r.db.WithContext(ctx).Table("sys_dept").
		Select("id").
		Where("del_flag = ? AND id IN (?)", modelx.Normal, grantSub).
		Order("parent_id, order_num")

	ids := make([]int64, 0)
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}

	if deptCheckStrictly {
		parentIDs := make([]int64, 0)
		if len(ids) > 0 {
			err := r.db.WithContext(ctx).Table("sys_dept").
				Where("id IN ? AND del_flag = ?", ids, modelx.Normal).
				Where("parent_id IS NOT NULL").
				Pluck("parent_id", &parentIDs).Error
			if err != nil {
				return nil, err
			}
		}
		if len(parentIDs) > 0 {
			parentSet := make(map[int64]bool, len(parentIDs))
			for _, pid := range parentIDs {
				parentSet[pid] = true
			}
			filtered := make([]int64, 0, len(ids))
			for _, id := range ids {
				if !parentSet[id] {
					filtered = append(filtered, id)
				}
			}
			ids = filtered
		}
	}
	return ids, nil
}

func (r *Repo) DeptTreeNodes(ctx context.Context) ([]DeptTreeNode, error) {
	rows := make([]DeptTreeNode, 0)
	err := r.db.WithContext(ctx).Table("sys_dept").
		Select("id, parent_id, dept_name, order_num, status").
		Where("del_flag = ?", modelx.Normal).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "parent_id"}},
			{Column: clause.Column{Name: "order_num"}},
			{Column: clause.Column{Name: "id"}},
		}}).
		Find(&rows).Error
	return rows, err
}

func countGTZero(q *gorm.DB) (bool, error) {
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}
