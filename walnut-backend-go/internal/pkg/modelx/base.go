// Package modelx 提供 GORM 实体的共享基类，对应 Python 端 app/core/base_model.py。
//
// 设计要点：
//   - 主键是**雪花 ID**（bigint），不使用数据库自增，故必须显式关闭 autoIncrement
//   - 审计字段（create_dept/create_by/create_time/update_by/update_time）由应用层填充，
//     不用 GORM 的自动时间戳钩子——那会与另两端的写入时机不一致
//   - 模型层用 int64 / time.Time 等原生类型；**JSON 序列化用的 JSInt / JSONTime
//     只出现在 dto 层**。这样 DB 映射与契约表达互不干扰
package modelx

import "time"

// BaseModel 是所有业务表的公共审计字段，对应 Python 端 BaseMixin。
//
// 嵌入方式用**匿名结构体嵌入**：GORM 会把嵌入结构的字段平铺到同一张表，
// 与 Python 端 SQLAlchemy 的 declarative mixin 行为一致。
type BaseModel struct {
	// ID 主键（雪花）。
	// ⚠️ autoIncrement:false 是必须的：GORM 默认把名为 ID 的整型主键当自增列，
	// 那样插入时会忽略应用层赋的雪花值、让数据库自己生成 1/2/3…，
	// 与另两端的 ID 体系完全脱节。
	ID int64 `gorm:"column:id;primaryKey;autoIncrement:false" json:"-"`

	CreateDept *int64     `gorm:"column:create_dept" json:"-"`
	CreateBy   *int64     `gorm:"column:create_by" json:"-"`
	CreateTime *time.Time `gorm:"column:create_time" json:"-"`
	UpdateBy   *int64     `gorm:"column:update_by" json:"-"`
	UpdateTime *time.Time `gorm:"column:update_time" json:"-"`
}

// 删除标志与状态的取值，对应 Python 端 SystemConstants。
//
// ⚠️ 这些是**字符串** '0'/'1' 而非整数——表结构里 del_flag / status 都是 varchar(1)。
// 用整数比较会静默不匹配（GORM 会把 int 转成 '0'/'1' 字符串，看似能work，
// 但一旦有人写成 status == 0 就变成 SQL 里的 status = '0'，恰好对；
// 而写成 status == 1 对 '1' 也对——真正的坑是有人以为是 int 后做数值运算）。
const (
	// Normal 正常 / 未删除
	Normal = "0"
	// Disabled 停用 / 已删除
	Disabled = "1"
)

// 超级管理员约定，对应 Python 端 SystemConstants.SUPER_ADMIN_ID / SUPER_ADMIN_ROLE_KEY。
const (
	// SuperAdminID 超管用户 ID。该用户拥有全部菜单权限（*:*:*）
	SuperAdminID int64 = 1
	// SuperAdminRoleKey 超管角色标识
	SuperAdminRoleKey = "superadmin"
	// AllPermission 超管的菜单权限通配符
	AllPermission = "*:*:*"
)

// 登录日志状态取值，对应 Python 端 Constants。
const (
	LoginSuccess = "Success"
	LoginFail    = "Error"
	Logout       = "Logout"
	Register     = "Register"
)
