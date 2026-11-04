package log

import (
	"context"

	"gorm.io/gorm"

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
	q := newQuery().Order("id DESC")
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
	q := newQuery().Order("id DESC")
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

func (r *Repo) DictLabelMap(ctx context.Context, dictType string) (map[string]string, error) {
	type row struct {
		DictValue string `gorm:"column:dict_value"`
		DictLabel string `gorm:"column:dict_label"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("sys_dict_data").
		Select("dict_value, dict_label").
		Where("dict_type = ? AND status = '0'", dictType).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.DictValue] = r.DictLabel
	}
	return m, nil
}

// ---- 导出列 ----

func OperLogExportColumns() []excelCol {
	return []excelCol{
		{"日志编号", "id"}, {"系统模块", "title"}, {"操作类型", "businessType"},
		{"请求方式", "requestMethod"}, {"操作类别", "operatorType"},
		{"操作人员", "operName"}, {"部门名称", "deptName"}, {"操作地址", "operIp"},
		{"操作地点", "operLocation"}, {"请求URL", "operUrl"},
		{"请求参数", "operParam"}, {"返回参数", "jsonResult"},
		{"操作状态", "status"}, {"错误消息", "errorMsg"}, {"操作时间", "operTime"},
		{"消耗时间", "costTime"},
	}
}

func LogininforExportColumns() []excelCol {
	return []excelCol{
		{"访问编号", "id"}, {"用户账号", "userName"}, {"客户端", "clientKey"},
		{"设备类型", "deviceType"}, {"登录地址", "ipaddr"}, {"登录地点", "loginLocation"},
		{"浏览器", "browser"}, {"操作系统", "os"}, {"登录状态", "status"},
		{"操作信息", "msg"}, {"登录时间", "loginTime"},
	}
}

type excelCol struct {
	Header string
	Key    string
}
