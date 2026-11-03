// 本文件实现部门域的数据访问，对应 Python 端 module_system/dept/crud.py。
//
// ⚠️ sys_dept 带 del_flag：除逻辑删除/批量启用两条 UPDATE 外，所有查询都显式
// 追加 del_flag='0'（Python 端每条 stmt 都写了，漏一条就会查出已删除部门）。
// ⚠️ 部门域的行级数据权限条件由 service 构建后作为 Cond 传入（与 post 不同，
// post 不接数据权限，dept 接且语义是「按自身 id 列过滤」，见 service.go 注释）。

package dept

import (
	"context"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

// Repo 部门数据访问层。
type Repo struct{ db *gorm.DB }

// NewRepo 构造数据访问层。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// List 部门列表，对应 DeptCrud.list_depts。
//
// ⚠️ 排序是四级复合排序，顺序不可调换：ancestors, parent_id, order_num, id。
// 列表响应与建树都依赖这个顺序。
func (r *Repo) List(ctx context.Context, conds ...crudx.Cond) ([]SysDept, error) {
	rows := make([]SysDept, 0)
	q := crudx.Apply(r.db.WithContext(ctx).Model(&SysDept{}),
		append([]crudx.Cond{crudx.Eq("del_flag", modelx.Normal)}, conds...)...).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "ancestors"}},
			{Column: clause.Column{Name: "parent_id"}},
			{Column: clause.Column{Name: "order_num"}},
			{Column: clause.Column{Name: "id"}},
		}})
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Count 按条件计数（含 del_flag 过滤），对应 count_dept / exists_dept 的底子。
func (r *Repo) Count(ctx context.Context, conds ...crudx.Cond) (int64, error) {
	var n int64
	err := crudx.Apply(r.db.WithContext(ctx).Model(&SysDept{}),
		append([]crudx.Cond{crudx.Eq("del_flag", modelx.Normal)}, conds...)...).
		Count(&n).Error
	return n, err
}

// Get 按主键查询（含逻辑删除过滤），对应 get_by_id。
func (r *Repo) Get(ctx context.Context, id int64) (*SysDept, error) {
	var row SysDept
	err := r.db.WithContext(ctx).
		Where("id = ? AND del_flag = ?", id, modelx.Normal).
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// findInSetCond 对应 DeptCrud.find_in_set：FIND_IN_SET(str(int(v)), ancestors) <> 0。
//
// ⚠️ 第一个参数在 Python 端是 **str(int(value))**，即字符串形态绑定，保持一致。
func findInSetCond(deptID int64) crudx.Cond {
	return crudx.Cond{
		SQL:  "FIND_IN_SET(?, ancestors) <> 0",
		Args: []any{strconv.FormatInt(deptID, 10)},
	}
}

// ListDescendants 所有子孙部门（ancestors 含 deptID，不含自身），对应 list_descendants。
func (r *Repo) ListDescendants(ctx context.Context, deptID int64) ([]SysDept, error) {
	rows := make([]SysDept, 0)
	err := crudx.Apply(r.db.WithContext(ctx).Model(&SysDept{}),
		crudx.Eq("del_flag", modelx.Normal), findInSetCond(deptID)).
		Find(&rows).Error
	return rows, err
}

// DescendantIDs 所有子孙部门 ID，对应 list_descendant_ids。
func (r *Repo) DescendantIDs(ctx context.Context, deptID int64) ([]int64, error) {
	ids := make([]int64, 0)
	err := crudx.Apply(r.db.WithContext(ctx).Table("sys_dept"),
		crudx.Eq("del_flag", modelx.Normal), findInSetCond(deptID)).
		Pluck("id", &ids).Error
	return ids, err
}

// CountNormalChildren 正常状态的子孙部门数，对应 count_normal_children。
func (r *Repo) CountNormalChildren(ctx context.Context, deptID int64) (int64, error) {
	var n int64
	err := crudx.Apply(r.db.WithContext(ctx).Model(&SysDept{}),
		crudx.Eq("del_flag", modelx.Normal),
		crudx.Eq("status", modelx.Normal),
		findInSetCond(deptID)).
		Count(&n).Error
	return n, err
}

// CountUserInDept 部门下用户数，对应 count_user_in_dept。
func (r *Repo) CountUserInDept(ctx context.Context, deptID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("sys_user").
		Where("del_flag = ? AND dept_id = ?", modelx.Normal, deptID).
		Count(&n).Error
	return n, err
}

// CountPostInDept 部门下岗位数，对应 count_post_in_dept。
//
// ⚠️ sys_post 无 del_flag，这里**不加**逻辑删除条件（Python 端也没加）。
func (r *Repo) CountPostInDept(ctx context.Context, deptID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("sys_post").
		Where("dept_id = ?", deptID).
		Count(&n).Error
	return n, err
}

// DeptNameMap 批量取部门名称（回填 parentName），对应 map_dept_names。
func (r *Repo) DeptNameMap(ctx context.Context, ids []int64) (map[int64]*string, error) {
	out := make(map[int64]*string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		ID       int64   `gorm:"column:id"`
		DeptName *string `gorm:"column:dept_name"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("sys_dept").
		Select("id, dept_name").
		Where("id IN ? AND del_flag = ?", ids, modelx.Normal).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].ID] = rows[i].DeptName
	}
	return out, nil
}

// LeaderNameMap 批量取负责人账号（回填 leaderName），对应 map_leader_names。
func (r *Repo) LeaderNameMap(ctx context.Context, ids []int64) (map[int64]*string, error) {
	out := make(map[int64]*string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		ID       int64   `gorm:"column:id"`
		UserName *string `gorm:"column:user_name"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("sys_user").
		Select("id, user_name").
		Where("del_flag = ? AND id IN ?", modelx.Normal, ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].ID] = rows[i].UserName
	}
	return out, nil
}

// RoleScopes 当前用户角色的数据范围，复用 permx 的查询（SQL 逐字一致）。
func (r *Repo) RoleScopes(ctx context.Context, userID int64) ([]permx.RoleScope, error) {
	return permx.RoleScopes(ctx, r.db, userID)
}

// UpdateStatusNormal 批量启用部门（不填审计字段），对应 update_status_normal。
func (r *Repo) UpdateStatusNormal(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Table("sys_dept").
		Where("del_flag = ? AND id IN ?", modelx.Normal, ids).
		Update("status", modelx.Normal)
	return res.RowsAffected, res.Error
}

// LogicalDelete 逻辑删除（置 del_flag='1'，不填审计字段），对应 logical_delete。
func (r *Repo) LogicalDelete(ctx context.Context, id int64) (int64, error) {
	res := r.db.WithContext(ctx).Table("sys_dept").
		Where("id = ? AND del_flag = ?", id, modelx.Normal).
		Update("del_flag", "1")
	return res.RowsAffected, res.Error
}

// Create 新增，审计字段由调用方经 crudx.FillCreate 填好。
func (r *Repo) Create(ctx context.Context, row *SysDept) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 整体保存，对应 CRUDBase.update（Python 端 setattr 后整体 flush）。
//
// ⚠️ 用 Save 而非 Updates(struct)：Updates 会跳过零值字段，
// 导致 status="" 之类的显式清空失效（同 post/repo.go 注释）。
func (r *Repo) Update(ctx context.Context, row *SysDept) error {
	return r.db.WithContext(ctx).Save(row).Error
}
