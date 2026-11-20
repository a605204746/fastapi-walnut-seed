package log

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/crudx"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ---- sys_oper_log ----

func (r *Repo) OperLogPage(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysOperLog, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysOperLog{}), conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// 用户排序在前；仅未指定 orderByColumn 时兜底 id DESC（log/crud._apply_order）。
	order, err := crudx.BuildOrder(r.db, &SysOperLog{}, page)
	if err != nil {
		return nil, 0, err
	}
	if crudx.IsBlank(page.OrderByColumn) {
		order = append(order, clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true})
	}
	q := newQuery()
	if len(order) > 0 {
		q = q.Clauses(clause.OrderBy{Columns: order})
	}
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysOperLog, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) OperLogListAll(ctx context.Context, conds []crudx.Cond) ([]SysOperLog, error) {
	q := crudx.Apply(r.db.WithContext(ctx).Model(&SysOperLog{}), conds...)
	q = q.Order("id DESC")
	rows := make([]SysOperLog, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) OperLogDeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysOperLog{})
	return res.RowsAffected, res.Error
}

func (r *Repo) OperLogClean(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("1 = 1").
		Delete(&SysOperLog{}).Error
}

// ---- sys_logininfor ----

func (r *Repo) LogininforPage(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysLogininfor, int64, error) {
	newQuery := func() *gorm.DB {
		return crudx.Apply(r.db.WithContext(ctx).Model(&SysLogininfor{}), conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order, err := crudx.BuildOrder(r.db, &SysLogininfor{}, page)
	if err != nil {
		return nil, 0, err
	}
	if crudx.IsBlank(page.OrderByColumn) {
		order = append(order, clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true})
	}
	q := newQuery()
	if len(order) > 0 {
		q = q.Clauses(clause.OrderBy{Columns: order})
	}
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysLogininfor, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) LogininforListAll(ctx context.Context, conds []crudx.Cond) ([]SysLogininfor, error) {
	q := crudx.Apply(r.db.WithContext(ctx).Model(&SysLogininfor{}), conds...)
	q = q.Order("id DESC")
	rows := make([]SysLogininfor, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) LogininforDeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysLogininfor{})
	return res.RowsAffected, res.Error
}

func (r *Repo) LogininforClean(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("1 = 1").
		Delete(&SysLogininfor{}).Error
}

// ---- 字典辅助 ----

// DictLabelMap 对应 service._dict_label_map：不过滤 status，跳过 value/label 为 NULL 的行。
func (r *Repo) DictLabelMap(ctx context.Context, dictType string) (map[string]string, error) {
	type row struct {
		DictValue *string `gorm:"column:dict_value"`
		DictLabel *string `gorm:"column:dict_label"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("sys_dict_data").
		Select("dict_value, dict_label").
		Where("dict_type = ?", dictType).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.DictValue == nil || r.DictLabel == nil {
			continue
		}
		m[*r.DictValue] = *r.DictLabel
	}
	return m, nil
}

// ---- 导出列 ----

// 列序/列头逐字对应 Python log/schema.py 的 OPER_LOG_EXPORT_HEADERS / LOGININFOR_EXPORT_HEADERS。

func OperLogExportColumns() []excelCol {
	return []excelCol{
		{"日志主键", "id"}, {"操作模块", "title"}, {"业务类型", "businessType"},
		{"请求方法", "method"}, {"请求方式", "requestMethod"}, {"操作类别", "operatorType"},
		{"操作人员", "operName"}, {"部门名称", "deptName"}, {"请求地址", "operUrl"},
		{"操作地址", "operIp"}, {"操作地点", "operLocation"}, {"请求参数", "operParam"},
		{"返回参数", "jsonResult"}, {"状态", "status"}, {"错误消息", "errorMsg"},
		{"操作时间", "operTime"}, {"消耗时间", "costTime"},
	}
}

func LogininforExportColumns() []excelCol {
	return []excelCol{
		{"序号", "id"}, {"用户账号", "userName"}, {"客户端", "clientKey"},
		{"设备类型", "deviceType"}, {"登录状态", "status"}, {"登录地址", "ipaddr"},
		{"登录地点", "loginLocation"}, {"浏览器", "browser"}, {"操作系统", "os"},
		{"提示消息", "msg"}, {"访问时间", "loginTime"},
	}
}

type excelCol struct {
	Header string
	Key    string
}
