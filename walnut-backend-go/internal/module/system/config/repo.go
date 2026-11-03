// 本文件实现参数配置域的数据访问，对应 Python 端 module_system/config/crud.py。
//
// 排序语义：page = 用户排序 + id 兜底；list_all 不排序。sys_config 无 del_flag。

package config

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
)

// Repo 参数配置数据访问层。
type Repo struct{ db *gorm.DB }

// NewRepo 构造。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Page 分页查询：用户排序 + id 兜底排序。
func (r *Repo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysConfig, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysConfig{}), conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery()
	order, err := crudx.BuildOrder(r.db, &SysConfig{}, page)
	if err != nil {
		return nil, 0, err
	}
	order = append(order, clause.OrderByColumn{Column: clause.Column{Name: "id"}})
	q = q.Clauses(clause.OrderBy{Columns: order})
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysConfig, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListAll 不分页查询（不排序），对应 CRUDBase.list_all。
func (r *Repo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysConfig, error) {
	return crudx.ListAll[SysConfig](ctx, r.db, conds...)
}

// Get 按主键查询。
func (r *Repo) Get(ctx context.Context, id int64) (*SysConfig, error) {
	var row SysConfig
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// GetByKey 按键名查询。
func (r *Repo) GetByKey(ctx context.Context, key string) (*SysConfig, error) {
	var row SysConfig
	err := r.db.WithContext(ctx).Where("config_key = ?", key).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ExistsByKey 键名唯一性校验。
func (r *Repo) ExistsByKey(ctx context.Context, key string, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysConfig{}).Where("config_key = ?", key)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// Create 新增（审计字段由调用方 FillCreate）。
func (r *Repo) Create(ctx context.Context, row *SysConfig) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 整体保存（按主键修改路径）。
func (r *Repo) Update(ctx context.Context, row *SysConfig) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// UpdateByKeyBulk 按键名批量更新（updateByKey 无 id 路径）。
//
// ⚠️ 只更新 values 里给出的列 + update_by/update_time（Python 端 sa_update 语义，
// 不碰其余审计字段）。返回受影响行数。
func (r *Repo) UpdateByKeyBulk(ctx context.Context, key string, values map[string]any, uid int64, now time.Time) (int64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	columns := make(map[string]any, len(values)+2)
	for k, v := range values {
		columns[k] = v
	}
	columns["update_by"] = uid
	columns["update_time"] = now
	res := r.db.WithContext(ctx).Table("sys_config").
		Where("config_key = ?", key).
		Updates(columns)
	return res.RowsAffected, res.Error
}

// DeleteBatch 物理删除。
func (r *Repo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysConfig{})
	return res.RowsAffected, res.Error
}
