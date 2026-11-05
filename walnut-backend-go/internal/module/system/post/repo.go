// 本文件实现岗位域的数据访问，对应 Python 端 module_system/post/crud.py。
//
// ⚠️ sys_post 无 del_flag，所有查询都**不加**逻辑删除条件。
// ⚠️ 岗位域**不使用行级数据权限**（Python 端 PostCrud 未接 Permission），
// 故这里不引入 permx——加了会比 Python 端多过滤，属契约偏差。

package post

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 岗位数据访问层。
type Repo struct{ db *gorm.DB }

// NewRepo 构造数据访问层。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Page 分页查询，对应 PostCrud.page。
//
// ⚠️ 该方法在 Python 端**覆盖**了基类实现，为的是先加一个默认排序
// `ORDER BY post_sort ASC`，再叠加用户指定的排序。故最终排序是
// 「post_sort asc, <用户排序>」——用户排序只能作为次级排序键生效。
// 漏掉这个默认排序会让两端列表顺序不同。
func (r *Repo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysPost, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysPost{}), conds...)
	}

	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := newQuery().
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "post_sort"}},
		}})

	order, err := crudx.BuildOrder(r.db, &SysPost{}, page)
	if err != nil {
		return nil, 0, err
	}
	if len(order) > 0 {
		q = q.Clauses(clause.OrderBy{Columns: order})
	}
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}

	rows := make([]SysPost, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListAll 查询全部（导出用），对应 PostCrud.list_all。
func (r *Repo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysPost, error) {
	return crudx.ListAll[SysPost](ctx, r.db, conds...)
}

// Get 按主键查询，对应 CRUDBase.get。
func (r *Repo) Get(ctx context.Context, id int64) (*SysPost, error) {
	var row SysPost
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ExistsByName 同部门下岗位名称是否已存在，对应 PostCrud.exists_by_name。
func (r *Repo) ExistsByName(ctx context.Context, name string, deptID int64, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysPost{}).
		Where("post_name = ? AND dept_id = ?", name, deptID)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	return countGTZero(q)
}

// ExistsByCode 岗位编码是否已存在，对应 PostCrud.exists_by_code。
func (r *Repo) ExistsByCode(ctx context.Context, code string, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysPost{}).Where("post_code = ?", code)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	return countGTZero(q)
}

// CountUserPost 岗位已分配的用户数量，对应 PostCrud.count_user_post。
func (r *Repo) CountUserPost(ctx context.Context, postID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("sys_user_post").
		Where("post_id = ?", postID).Count(&n).Error
	return n, err
}

// CountByDeptID 部门下的岗位数量，对应 PostCrud.count_by_dept_id。
func (r *Repo) CountByDeptID(ctx context.Context, deptID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&SysPost{}).
		Where("dept_id = ?", deptID).Count(&n).Error
	return n, err
}

// ListOptionByIDs 按 ID 串查询**正常状态**岗位，对应 PostCrud.list_option_by_ids。
//
// ⚠️ ids 为空时 Python 端只施加 status='0' 而不加 IN 条件（`if post_ids:`），
// 即返回全部正常岗位。不要写成恒假条件。
func (r *Repo) ListOptionByIDs(ctx context.Context, ids []int64) ([]SysPost, error) {
	q := r.db.WithContext(ctx).Model(&SysPost{}).Where("status = ?", modelx.Normal)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	rows := make([]SysPost, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListByUserID 用户所属岗位，对应 PostCrud.list_by_user_id。
func (r *Repo) ListByUserID(ctx context.Context, userID int64) ([]SysPost, error) {
	rows := make([]SysPost, 0)
	err := r.db.WithContext(ctx).Model(&SysPost{}).
		Where("id IN (SELECT post_id FROM sys_user_post WHERE user_id = ?)", userID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Create 新增，对应 CRUDBase.create。审计字段由调用方经 crudx.FillCreate 填好。
func (r *Repo) Create(ctx context.Context, row *SysPost) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 更新，对应 CRUDBase.update。
//
// ⚠️ 只更新**非零/显式设置**的字段。Python 端是把 schema 的非 None 字段
// 逐个 setattr 到已加载的实体上再整体 flush，故未提供的字段保持原值。
// GORM 的 Save 会写入所有字段，语义等价；用 Save 而非 Updates，
// 因为 Updates(struct) 会跳过零值字段，导致 status="" 之类的显式清空失效。
func (r *Repo) Update(ctx context.Context, row *SysPost) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// DeleteBatch 批量物理删除，对应 CRUDBase.delete_batch。返回受影响行数。
func (r *Repo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysPost{})
	return res.RowsAffected, res.Error
}

// ---- 部门辅助（对应 service 里的惰性导入 DeptModel）----

// DeptAndChildIDs 查询部门及其所有子孙部门 ID（含自身），对应 _dept_and_child_ids。
//
// ⚠️ 用的是 `CONCAT(',', ancestors, ',') LIKE '%,{id},%'` 而不是 FIND_IN_SET——
// 与 permx 里数据权限用的子查询是**两套不同实现**，Python 端就是如此，
// 各自对齐各自的调用点，不要"统一"。
//
// ancestors 为 NULL 时 CONCAT 结果为 NULL，LIKE 不匹配 → 该行被排除，
// 与 Python 端 `literal(",") + NULL + literal(",")` 的行为一致。
func (r *Repo) DeptAndChildIDs(ctx context.Context, deptID int64) ([]int64, error) {
	ids := make([]int64, 0, 1)
	pattern := "%," + strconv.FormatInt(deptID, 10) + ",%"
	err := r.db.WithContext(ctx).Table("sys_dept").
		Where("del_flag = ? AND CONCAT(',', ancestors, ',') LIKE ?", modelx.Normal, pattern).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	// Python 端在查询结果之后 append(dept_id)，故自身排在最后
	return append(ids, deptID), nil
}

// DeptNameMap 批量查询部门名，对应 _dept_name_map。
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

// DeptNode 是部门树查询的投影行。
type DeptNode struct {
	ID           int64   `gorm:"column:id"`
	ParentID     *int64  `gorm:"column:parent_id"`
	DeptName     *string `gorm:"column:dept_name"`
	DeptCategory *string `gorm:"column:dept_category"`
	OrderNum     *int    `gorm:"column:order_num"`
	Status       *string `gorm:"column:status"`
	Ancestors    *string `gorm:"column:ancestors"`
}

// DeptTree 查询部门列表（建树前的扁平行），对应 PostService.dept_tree_list 的查询部分。
//
// 排序是四级复合排序，顺序不可调换：ancestors, parent_id, order_num, id。
func (r *Repo) DeptTree(ctx context.Context, conds []crudx.Cond) ([]DeptNode, error) {
	rows := make([]DeptNode, 0)
	q := crudx.Apply(r.db.WithContext(ctx).Table("sys_dept"), conds...).
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

// ---- 内部工具 ----

func countGTZero(q *gorm.DB) (bool, error) {
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
