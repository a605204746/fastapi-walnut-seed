// 本文件实现客户端域的数据访问，对应 Python 端 module_system/client/crud.py。
//
// ⚠️ sys_client 含 del_flag：所有查询显式过滤 del_flag='0'；
// 删除与改状态都是 UPDATE（逻辑删除带审计字段）。
// ⚠️ 排序：page = id 升序在前、用户排序在后（同 notice）；list_all 不排序。

package client

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 客户端数据访问层。
type Repo struct{ db *gorm.DB }

// NewRepo 构造。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Page 分页查询：del_flag 过滤 + id 升序为主排序键。
func (r *Repo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysClient, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysClient{}),
			append([]crudx.Cond{crudx.Eq("del_flag", modelx.Normal)}, conds...)...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery().Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Name: "id"}},
	}})
	order, err := crudx.BuildOrder(r.db, &SysClient{}, page)
	if err != nil {
		return nil, 0, err
	}
	if len(order) > 0 {
		q = q.Clauses(clause.OrderBy{Columns: order})
	}
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysClient, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListAll 不分页查询（不排序），对应 CRUDBase.list_all。
//
// ⚠️ **不加 del_flag 过滤**：Python 端 ClientCrud 没有覆写 list_all，基类实现
// 不带逻辑删除条件 → 导出会把已逻辑删除的行也带上（2026-11-03 契约 diff 实测）。
// 只有 page/get/exists 等覆写方法才过滤 del_flag。契约优先，别"修正"。
func (r *Repo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysClient, error) {
	return crudx.ListAll[SysClient](ctx, r.db, conds...)
}

// Get 按主键查询（含 del_flag 过滤）。
func (r *Repo) Get(ctx context.Context, id int64) (*SysClient, error) {
	var row SysClient
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

// ExistsByKey 客户端 key 唯一性校验（含 del_flag 过滤）。
func (r *Repo) ExistsByKey(ctx context.Context, key string, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysClient{}).
		Where("client_key = ? AND del_flag = ?", key, modelx.Normal)
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
func (r *Repo) Create(ctx context.Context, row *SysClient) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 整体保存。
func (r *Repo) Update(ctx context.Context, row *SysClient) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// UpdateStatusByClientID 按客户端 id 改状态（带审计字段），对应 update_status_by_client_id。
func (r *Repo) UpdateStatusByClientID(ctx context.Context, clientID, status string, uid int64, now time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Table("sys_client").
		Where("client_id = ? AND del_flag = ?", clientID, modelx.Normal).
		Updates(map[string]any{"status": status, "update_by": uid, "update_time": now})
	return res.RowsAffected, res.Error
}

// LogicalDeleteBatch 批量逻辑删除（del_flag='1' + 审计字段），对应 delete_batch 覆写。
func (r *Repo) LogicalDeleteBatch(ctx context.Context, ids []int64, uid int64, now time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Table("sys_client").
		Where("id IN ? AND del_flag = ?", ids, modelx.Normal).
		Updates(map[string]any{"del_flag": "1", "update_by": uid, "update_time": now})
	return res.RowsAffected, res.Error
}
