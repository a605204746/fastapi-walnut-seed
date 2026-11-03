// 本文件实现字典域的数据访问，对应 Python 端 module_system/dict/crud.py。
//
// ⚠️ 排序语义照抄 DictTypeCrud / DictDataCrud 的 _apply_order：
//   - page：用户排序在前，**默认排序兜底在后**（type: id；data: dict_sort, id）
//   - list_all（导出/optionselect/按类型查）：**完全不排序**（base 实现不加 order）
// 两表都**无 del_flag**，删除是物理删除。

package dict

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
)

// ---- 字典类型 ----

// TypeRepo 字典类型数据访问层。
type TypeRepo struct{ db *gorm.DB }

// NewTypeRepo 构造。
func NewTypeRepo(db *gorm.DB) *TypeRepo { return &TypeRepo{db: db} }

// Page 分页查询：用户排序 + id 兜底排序。
func (r *TypeRepo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysDictType, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysDictType{}), conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery()
	order, err := crudx.BuildOrder(r.db, &SysDictType{}, page)
	if err != nil {
		return nil, 0, err
	}
	order = append(order, clause.OrderByColumn{Column: clause.Column{Name: "id"}})
	q = q.Clauses(clause.OrderBy{Columns: order})
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysDictType, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListAll 不分页查询（**不排序**），对应 CRUDBase.list_all。
func (r *TypeRepo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysDictType, error) {
	return crudx.ListAll[SysDictType](ctx, r.db, conds...)
}

// Get 按主键查询。
func (r *TypeRepo) Get(ctx context.Context, id int64) (*SysDictType, error) {
	var row SysDictType
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ExistsByType 字典类型是否已存在（唯一性校验）。
func (r *TypeRepo) ExistsByType(ctx context.Context, dictType string, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysDictType{}).Where("dict_type = ?", dictType)
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
func (r *TypeRepo) Create(ctx context.Context, row *SysDictType) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 整体保存（Python 端 setattr 后 merge+flush）。
func (r *TypeRepo) Update(ctx context.Context, row *SysDictType) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// DeleteBatch 物理删除，对应 CRUDBase.delete_batch。
func (r *TypeRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysDictType{})
	return res.RowsAffected, res.Error
}

// ---- 字典数据 ----

// DataRepo 字典数据数据访问层。
type DataRepo struct{ db *gorm.DB }

// NewDataRepo 构造。
func NewDataRepo(db *gorm.DB) *DataRepo { return &DataRepo{db: db} }

// Page 分页查询：用户排序 + dict_sort, id 兜底排序。
func (r *DataRepo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysDictData, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysDictData{}), conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery()
	order, err := crudx.BuildOrder(r.db, &SysDictData{}, page)
	if err != nil {
		return nil, 0, err
	}
	order = append(order,
		clause.OrderByColumn{Column: clause.Column{Name: "dict_sort"}},
		clause.OrderByColumn{Column: clause.Column{Name: "id"}})
	q = q.Clauses(clause.OrderBy{Columns: order})
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysDictData, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListAll 不分页查询（**不排序**）。
func (r *DataRepo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysDictData, error) {
	return crudx.ListAll[SysDictData](ctx, r.db, conds...)
}

// ListByType 按字典类型查全量数据（缓存加载用，**不排序**，同 list_all）。
func (r *DataRepo) ListByType(ctx context.Context, dictType string) ([]SysDictData, error) {
	return r.ListAll(ctx, crudx.Eq("dict_type", dictType))
}

// Get 按主键查询。
func (r *DataRepo) Get(ctx context.Context, id int64) (*SysDictData, error) {
	var row SysDictData
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ExistsByType 类型下是否存在字典数据（删除类型前的占用检查）。
func (r *DataRepo) ExistsByType(ctx context.Context, dictType string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&SysDictData{}).
		Where("dict_type = ?", dictType).Count(&n).Error
	return n > 0, err
}

// ExistsByTypeAndValue 同类型下键值唯一性校验。
func (r *DataRepo) ExistsByTypeAndValue(ctx context.Context, dictType, dictValue string, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysDictData{}).
		Where("dict_type = ? AND dict_value = ?", dictType, dictValue)
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
func (r *DataRepo) Create(ctx context.Context, row *SysDictData) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 整体保存。
func (r *DataRepo) Update(ctx context.Context, row *SysDictData) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// RenameType 批量改字典数据的类型列（修改类型时同步，**不填审计字段**）。
func (r *DataRepo) RenameType(ctx context.Context, oldType, newType string) (int64, error) {
	res := r.db.WithContext(ctx).Table("sys_dict_data").
		Where("dict_type = ?", oldType).
		Update("dict_type", newType)
	return res.RowsAffected, res.Error
}

// DeleteBatch 物理删除。
func (r *DataRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysDictData{})
	return res.RowsAffected, res.Error
}
