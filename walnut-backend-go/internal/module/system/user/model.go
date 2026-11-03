// Package user 对应 sys_user 及其关联表。
//
// P2 只需要**读**用户（登录校验）与**写**最后登录信息；
// 完整的 CRUD 在 P3 补齐（repo.go / service.go / handler.go / dto.go）。
package user

import (
	"time"

	"walnut-seed-go/internal/pkg/modelx"
)

// SysUser 用户信息表 sys_user。
//
// 列定义与 migrations/0001_init_schema.up.sql 逐一对应。
// ⚠️ 可空列一律用指针：Go 的零值无法区分「NULL」与「空串/0」，
// 而 update 时这个区别决定了会不会把 NULL 覆盖成零值。
type SysUser struct {
	modelx.BaseModel

	DeptID      *int64  `gorm:"column:dept_id"`
	UserName    string  `gorm:"column:user_name;not null"`
	NickName    string  `gorm:"column:nick_name;not null"`
	UserType    string  `gorm:"column:user_type;not null"`
	Email       *string `gorm:"column:email"`
	Phonenumber *string `gorm:"column:phonenumber"`
	Sex         *string `gorm:"column:sex"`
	Avatar      *string `gorm:"column:avatar"`
	// Password bcrypt 哈希。
	// ⚠️ 任何响应 DTO 都**不得**包含此字段；日志脱敏名单也已覆盖 password
	Password *string `gorm:"column:password"`
	// Status 帐号状态：'0' 正常、'1' 停用（varchar，不是 int）
	Status    string     `gorm:"column:status;not null"`
	LoginIP   *string    `gorm:"column:login_ip"`
	LoginDate *time.Time `gorm:"column:login_date"`
	Remark    *string    `gorm:"column:remark"`
	// DelFlag 逻辑删除标志：'0' 存在、'1' 已删除。
	// 所有查询都必须带上 del_flag = '0'，否则会捞出已删除用户
	DelFlag string `gorm:"column:del_flag;not null"`
}

// TableName 显式声明表名。
//
// ⚠️ 必须显式声明：GORM 默认把结构体名转成蛇形复数（SysUser → sys_users），
// 而本项目表名是单数 sys_user（已在 gorm.Config 里设了 SingularTable，
// 但显式声明更稳妥，也避免日后有人改掉全局配置时静默出错）。
func (SysUser) TableName() string { return "sys_user" }

// SysUserRole 用户与角色关联表（复合主键，无审计字段）。
type SysUserRole struct {
	UserID int64 `gorm:"column:user_id;primaryKey"`
	RoleID int64 `gorm:"column:role_id;primaryKey"`
}

func (SysUserRole) TableName() string { return "sys_user_role" }

// SysUserPost 用户与岗位关联表（复合主键，无审计字段）。
type SysUserPost struct {
	UserID int64 `gorm:"column:user_id;primaryKey"`
	PostID int64 `gorm:"column:post_id;primaryKey"`
}

func (SysUserPost) TableName() string { return "sys_user_post" }
