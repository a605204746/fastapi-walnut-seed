package log

import (
	"context"
	"strconv"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/idtype"
)

// logRepo 是 service 消费的数据访问能力（consumer-side 接口，定义在消费方）。
// 只列 service 实际调用的方法；*Repo 满足它，单测以 mock 替换。
type logRepo interface {
	OperLogPage(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysOperLog, int64, error)
	OperLogListAll(ctx context.Context, conds []crudx.Cond) ([]SysOperLog, error)
	OperLogDeleteBatch(ctx context.Context, ids []int64) (int64, error)
	OperLogClean(ctx context.Context) error
	LogininforPage(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysLogininfor, int64, error)
	LogininforListAll(ctx context.Context, conds []crudx.Cond) ([]SysLogininfor, error)
	LogininforDeleteBatch(ctx context.Context, ids []int64) (int64, error)
	LogininforClean(ctx context.Context) error
	DictLabelMap(ctx context.Context, dictType string) (map[string]string, error)
}

type Service struct {
	repo logRepo
	auth authctx.Context
}

func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
}

type OperLogPageOut struct {
	Rows  []OperLogOut `json:"rows"`
	Total idtype.JSInt `json:"total"`
}

type OperLogOut struct {
	ID            *idtype.JSInt `json:"id"`
	Title         *string       `json:"title"`
	BusinessType  *idtype.JSInt `json:"businessType"`
	Method        *string       `json:"method"`
	RequestMethod *string       `json:"requestMethod"`
	OperatorType  *idtype.JSInt `json:"operatorType"`
	OperName      *string       `json:"operName"`
	DeptName      *string       `json:"deptName"`
	OperURL       *string       `json:"operUrl"`
	OperIP        *string       `json:"operIp"`
	OperLocation  *string       `json:"operLocation"`
	OperParam     *string       `json:"operParam"`
	JSONResult    *string       `json:"jsonResult"`
	Status        *idtype.JSInt `json:"status"`
	ErrorMsg      *string       `json:"errorMsg"`
	OperTime      *string       `json:"operTime"`
	CostTime      *idtype.JSInt `json:"costTime"`
}

type LogininforPageOut struct {
	Rows  []LogininforOut `json:"rows"`
	Total idtype.JSInt    `json:"total"`
}

type LogininforOut struct {
	ID            *idtype.JSInt `json:"id"`
	UserName      *string       `json:"userName"`
	ClientKey     *string       `json:"clientKey"`
	DeviceType    *string       `json:"deviceType"`
	IPAddr        *string       `json:"ipaddr"`
	LoginLocation *string       `json:"loginLocation"`
	Browser       *string       `json:"browser"`
	OS            *string       `json:"os"`
	Status        *string       `json:"status"`
	Msg           *string       `json:"msg"`
	LoginTime     *string       `json:"loginTime"`
}

func newOperLogOut(r *SysOperLog) OperLogOut {
	o := OperLogOut{
		ID:            jsIntPtr(r.ID),
		Title:         r.Title,
		Method:        r.Method,
		RequestMethod: r.RequestMethod,
		OperName:      r.OperName,
		DeptName:      r.DeptName,
		OperURL:       r.OperURL,
		OperIP:        r.OperIP,
		OperLocation:  r.OperLocation,
		OperParam:     r.OperParam,
		JSONResult:    r.JsonResult,
		ErrorMsg:      r.ErrorMsg,
		OperTime:      timePtrStr(r.OperTime),
	}
	if r.BusinessType != nil {
		o.BusinessType = jsIntPtr(int64(*r.BusinessType))
	}
	if r.OperatorType != nil {
		o.OperatorType = jsIntPtr(int64(*r.OperatorType))
	}
	if r.Status != nil {
		o.Status = jsIntPtr(int64(*r.Status))
	}
	if r.CostTime != nil {
		o.CostTime = jsIntPtr(*r.CostTime)
	}
	return o
}

func newLogininforOut(r *SysLogininfor) LogininforOut {
	return LogininforOut{
		ID:            jsIntPtr(r.ID),
		UserName:      r.UserName,
		ClientKey:     r.ClientKey,
		DeviceType:    r.DeviceType,
		IPAddr:        r.IPAddr,
		LoginLocation: r.LoginLocation,
		Browser:       r.Browser,
		OS:            r.OS,
		Status:        r.Status,
		Msg:           r.Msg,
		LoginTime:     timePtrStr(r.LoginTime),
	}
}

func (s *Service) OperLogPage(ctx context.Context, q OperLogQueryParam) (*OperLogPageOut, error) {
	conds := buildOperLogConditions(q)
	rows, total, err := s.repo.OperLogPage(ctx, q.Page, conds)
	if err != nil {
		return nil, err
	}
	outs := make([]OperLogOut, 0, len(rows))
	for i := range rows {
		outs = append(outs, newOperLogOut(&rows[i]))
	}
	return &OperLogPageOut{Rows: outs, Total: idtype.JSInt(total)}, nil
}

func (s *Service) OperLogExportRows(ctx context.Context, q OperLogFilterParam) ([]map[string]any, error) {
	conds := buildOperLogConditions(q.toQuery())
	rows, err := s.repo.OperLogListAll(ctx, conds)
	if err != nil {
		return nil, err
	}
	businessMap, _ := s.repo.DictLabelMap(ctx, "sys_oper_type")
	statusMap, _ := s.repo.DictLabelMap(ctx, "sys_common_status")
	result := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m := map[string]any{
			"id":            r.ID,
			"title":         r.Title,
			"businessType":  dictLabelInt(businessMap, r.BusinessType),
			"method":        r.Method,
			"requestMethod": r.RequestMethod,
			"operatorType":  operatorTypeLabel(r.OperatorType),
			"operName":      r.OperName,
			"deptName":      r.DeptName,
			"operUrl":       r.OperURL,
			"operIp":        r.OperIP,
			"operLocation":  r.OperLocation,
			"operParam":     r.OperParam,
			"jsonResult":    r.JsonResult,
			"status":        dictLabelInt(statusMap, r.Status),
			"errorMsg":      r.ErrorMsg,
			"operTime":      formatDateTime(r.OperTime),
			"costTime":      r.CostTime,
		}
		result = append(result, m)
	}
	return result, nil
}

func (s *Service) OperLogDelete(ctx context.Context, ids []int64) (int64, error) {
	return s.repo.OperLogDeleteBatch(ctx, ids)
}

func (s *Service) OperLogClean(ctx context.Context) error {
	return s.repo.OperLogClean(ctx)
}

func (s *Service) LogininforPage(ctx context.Context, q LogininforQueryParam) (*LogininforPageOut, error) {
	conds := buildLogininforConditions(q)
	rows, total, err := s.repo.LogininforPage(ctx, q.Page, conds)
	if err != nil {
		return nil, err
	}
	outs := make([]LogininforOut, 0, len(rows))
	for i := range rows {
		outs = append(outs, newLogininforOut(&rows[i]))
	}
	return &LogininforPageOut{Rows: outs, Total: idtype.JSInt(total)}, nil
}

func (s *Service) LogininforExportRows(ctx context.Context, q LogininforFilterParam) ([]map[string]any, error) {
	conds := buildLogininforConditions(q.toQuery())
	rows, err := s.repo.LogininforListAll(ctx, conds)
	if err != nil {
		return nil, err
	}
	deviceMap, _ := s.repo.DictLabelMap(ctx, "sys_device_type")
	statusMap, _ := s.repo.DictLabelMap(ctx, "sys_common_status")
	result := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m := map[string]any{
			"id":            r.ID,
			"userName":      r.UserName,
			"clientKey":     r.ClientKey,
			"deviceType":    dictLabel(deviceMap, r.DeviceType),
			"ipaddr":        r.IPAddr,
			"loginLocation": r.LoginLocation,
			"browser":       r.Browser,
			"os":            r.OS,
			"status":        dictLabel(statusMap, r.Status),
			"msg":           r.Msg,
			"loginTime":     formatDateTime(r.LoginTime),
		}
		result = append(result, m)
	}
	return result, nil
}

func (s *Service) LogininforDelete(ctx context.Context, ids []int64) (int64, error) {
	return s.repo.LogininforDeleteBatch(ctx, ids)
}

func (s *Service) LogininforClean(ctx context.Context) error {
	return s.repo.LogininforClean(ctx)
}

// ---- Query params ----

type OperLogFilterParam struct {
	Title         string
	BusinessType  *int64
	BusinessTypes []int64
	Status        *int64
	OperName      string
	OperIP        string
	BeginTime     *time.Time
	EndTime       *time.Time
}

func (p OperLogFilterParam) toQuery() OperLogQueryParam {
	return OperLogQueryParam{Filter: p}
}

type OperLogQueryParam struct {
	Page   crudx.Page
	Filter OperLogFilterParam
}

type LogininforFilterParam struct {
	UserName  string
	IPAddr    string
	Status    string
	BeginTime *time.Time
	EndTime   *time.Time
}

func (p LogininforFilterParam) toQuery() LogininforQueryParam {
	return LogininforQueryParam{Filter: p}
}

type LogininforQueryParam struct {
	Page   crudx.Page
	Filter LogininforFilterParam
}

func buildOperLogConditions(q OperLogQueryParam) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 8)
	f := q.Filter
	if crudx.IsNotBlank(f.Title) {
		conds = append(conds, crudx.Like("title", f.Title))
	}
	if crudx.IsNotBlank(f.OperName) {
		conds = append(conds, crudx.Like("oper_name", f.OperName))
	}
	if crudx.IsNotBlank(f.OperIP) {
		conds = append(conds, crudx.Like("oper_ip", f.OperIP))
	}
	if f.BusinessType != nil && *f.BusinessType > 0 {
		conds = append(conds, crudx.Eq("business_type", *f.BusinessType))
	}
	if len(f.BusinessTypes) > 0 {
		conds = append(conds, crudx.In("business_type", f.BusinessTypes))
	}
	if f.Status != nil {
		conds = append(conds, crudx.Eq("status", *f.Status))
	}
	if begin, end, ok := dateRange(f.BeginTime, f.EndTime); ok {
		conds = append(conds, crudx.Between("oper_time", begin, end))
	}
	return conds
}

func buildLogininforConditions(q LogininforQueryParam) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 5)
	f := q.Filter
	if crudx.IsNotBlank(f.IPAddr) {
		conds = append(conds, crudx.Like("ipaddr", f.IPAddr))
	}
	if crudx.IsNotBlank(f.Status) {
		conds = append(conds, crudx.Eq("status", f.Status))
	}
	if crudx.IsNotBlank(f.UserName) {
		conds = append(conds, crudx.Like("user_name", f.UserName))
	}
	if begin, end, ok := dateRange(f.BeginTime, f.EndTime); ok {
		conds = append(conds, crudx.Between("login_time", begin, end))
	}
	return conds
}

// ---- Helpers ----

// dictLabel 对应 Python `if row.status:` 真值判断后才查字典：nil/空串保持原样
// （单元格为空），未命中回退原值。
func dictLabel(m map[string]string, key *string) string {
	if key == nil || *key == "" {
		return ""
	}
	if v, ok := m[*key]; ok {
		return v
	}
	return *key
}

// dictLabelInt 对应 Python labels.get(str(v), v)：未命中回退**数字本身**
// （Excel 单元格保留数值类型），nil 保持空单元格。
func dictLabelInt(m map[string]string, key *int) any {
	if key == nil {
		return nil
	}
	if v, ok := m[strconv.Itoa(*key)]; ok {
		return v
	}
	return *key
}

// operatorTypeLabel 对应 OPERATOR_TYPE_LABELS.get(str(v), v)：
// nil → 空单元格；未知值 → 原数字（不是「其它」）。
func operatorTypeLabel(v *int) any {
	if v == nil {
		return nil
	}
	switch *v {
	case 0:
		return "其它"
	case 1:
		return "后台用户"
	case 2:
		return "手机端用户"
	}
	return *v
}

func timePtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02 15:04:05")
	return &s
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func pickSnake(field string) func(any) any {
	return func(row any) any {
		m, ok := row.(map[string]any)
		if !ok {
			return nil
		}
		return m[field]
	}
}

func dateRange(beginTime, endTime *time.Time) (time.Time, time.Time, bool) {
	if beginTime == nil || endTime == nil {
		return time.Time{}, time.Time{}, false
	}
	return *beginTime, endTime.Add(24*time.Hour - time.Second), true
}

func formatDateTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
