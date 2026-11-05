// 本文件实现通知公告域的数据访问，对应 Python 端 module_system/notice/crud.py。
//
// ⚠️ 排序语义与 config/dict **相反**：先 id 升序、再叠加用户排序
// （Python 端 stmt.order_by(asc(id)) 在前，_apply_order 在后）。
// sys_notice 无 del_flag，删除为物理删除。

package notice

import (
	"context"
	"errors"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 通知公告数据访问层。
type Repo struct{ db *gorm.DB }

// NewRepo 构造。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Page 分页查询：id 升序为主排序键，用户排序为次级键。
func (r *Repo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysNotice, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysNotice{}), conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery().Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Name: "id"}},
	}})
	order, err := crudx.BuildOrder(r.db, &SysNotice{}, page)
	if err != nil {
		return nil, 0, err
	}
	if len(order) > 0 {
		q = q.Clauses(clause.OrderBy{Columns: order})
	}
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysNotice, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Get 按主键查询。
func (r *Repo) Get(ctx context.Context, id int64) (*SysNotice, error) {
	var row SysNotice
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// Create 新增（审计字段由调用方 FillCreate）。
func (r *Repo) Create(ctx context.Context, row *SysNotice) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 整体保存。
func (r *Repo) Update(ctx context.Context, row *SysNotice) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// DeleteBatch 物理删除，返回受影响行数。
func (r *Repo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysNotice{})
	return res.RowsAffected, res.Error
}

// FindUserIDByName 按用户账号查用户ID（创建人过滤用），不存在返回 nil。
//
// ⚠️ 用 Pluck 而不是 First(&int64)：GORM 的 First 要求 struct 目的地，
// 标量指针会报 "model value required"（契约 diff 实测抓到）。
func (r *Repo) FindUserIDByName(ctx context.Context, name string) (*int64, error) {
	ids := make([]int64, 0, 1)
	err := r.db.WithContext(ctx).Table("sys_user").
		Select("id").
		Where("user_name = ? AND del_flag = ?", name, modelx.Normal).
		Limit(1).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return &ids[0], nil
}

// UserNameMap 批量取用户账号（回填 createByName），对应 _user_name_map。
func (r *Repo) UserNameMap(ctx context.Context, ids []int64) (map[int64]*string, error) {
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
		Where("id IN ? AND del_flag = ?", ids, modelx.Normal).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].ID] = rows[i].UserName
	}
	return out, nil
}

// DictLabelMap 批量取字典标签（dict_type + dict_values → labels），
// 对应 _dict_label_map。
func (r *Repo) DictLabelMap(ctx context.Context, dictType string, values []string) (map[string]string, error) {
	out := make(map[string]string, len(values))
	if len(values) == 0 {
		return out, nil
	}
	type row struct {
		DictValue string `gorm:"column:dict_value"`
		DictLabel string `gorm:"column:dict_label"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("sys_dict_data").
		Select("dict_value, dict_label").
		Where("dict_type = ? AND status = '0' AND dict_value IN ?", dictType, values).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].DictValue] = rows[i].DictLabel
	}
	return out, nil
}

// sortedIDs 去重升序（Python 端 sorted({...})），让 SQL 参数顺序稳定。
func sortedIDs(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
