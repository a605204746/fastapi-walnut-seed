// Package log 对应 sys_logininfor（登录日志）与 sys_oper_log（操作日志）。
//
// P2 只写登录日志；操作日志的异步采集在 P3 落地。
package log

import "time"

// SysLogininfor 系统访问记录 sys_logininfor。
//
// ⚠️ 本表**不含** BaseModel 的审计字段（create_by 等），只有 id + login_time。
// 表结构见 migrations/0001_init_schema.up.sql，嵌入 BaseModel 会多出四列导致插入失败。
type SysLogininfor struct {
	// ID 访问ID（雪花）
	ID int64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	UserName      *string `gorm:"column:user_name"`
	ClientKey     *string `gorm:"column:client_key"`
	DeviceType    *string `gorm:"column:device_type"`
	IPAddr        *string `gorm:"column:ipaddr"`
	LoginLocation *string `gorm:"column:login_location"`
	Browser       *string `gorm:"column:browser"`
	OS            *string `gorm:"column:os"`
	// Status 登录状态：'0' 成功、'1' 失败（varchar）
	Status    *string    `gorm:"column:status"`
	Msg       *string    `gorm:"column:msg"`
	LoginTime *time.Time `gorm:"column:login_time"`
}

func (SysLogininfor) TableName() string { return "sys_logininfor" }

// SysOperLog 操作日志记录 sys_oper_log。P3 使用，此处先定义实体。
type SysOperLog struct {
	// ID 日志主键（雪花）
	ID int64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	Title         *string `gorm:"column:title"`
	BusinessType  *int    `gorm:"column:business_type"`
	Method        *string `gorm:"column:method"`
	RequestMethod *string `gorm:"column:request_method"`
	OperatorType  *int    `gorm:"column:operator_type"`
	OperName      *string `gorm:"column:oper_name"`
	DeptName      *string `gorm:"column:dept_name"`
	// OperURL 请求URL，入库前须截断到 255
	OperURL      *string `gorm:"column:oper_url"`
	OperIP       *string `gorm:"column:oper_ip"`
	OperLocation *string `gorm:"column:oper_location"`
	// OperParam / JsonResult / ErrorMsg 入库前须截断到 4000
	OperParam  *string    `gorm:"column:oper_param"`
	JsonResult *string    `gorm:"column:json_result"`
	Status     *int       `gorm:"column:status"`
	ErrorMsg   *string    `gorm:"column:error_msg"`
	OperTime   *time.Time `gorm:"column:oper_time"`
	CostTime   *int64     `gorm:"column:cost_time"`
}

func (SysOperLog) TableName() string { return "sys_oper_log" }

// 字段截断上限，对应 Python 端 OperationLogRoute 的截断逻辑。
// 超长会被 MySQL 直接报错（strict mode）或静默截断，故必须在应用层先截。
const (
	MaxOperURLLen    = 255
	MaxOperParamLen  = 4000
	MaxJsonResultLen = 4000
	MaxErrorMsgLen   = 4000
)

// 登录状态取值（写入 SysLogininfor.Status），对应 Python 端 Constants。
const (
	StatusSuccess = "0"
	StatusFail    = "1"
)
