// Package role 对应 sys_role 及其关联表。
package role

import "walnut-seed-go/internal/pkg/modelx"

// SysRole 角色信息表 sys_role。
type SysRole struct {
	modelx.BaseModel

	RoleName string `gorm:"column:role_name;not null"`
	// RoleKey 角色权限字符串（如 admin / common）
	RoleKey   string  `gorm:"column:role_key;not null"`
	RoleSort  int     `gorm:"column:role_sort;not null"`
	DataScope *string `gorm:"column:data_scope"`
	// MenuCheckStrictly / DeptCheckStrictly 树选择项是否关联显示
	MenuCheckStrictly *int    `gorm:"column:menu_check_strictly"`
	DeptCheckStrictly *int    `gorm:"column:dept_check_strictly"`
	Status            string  `gorm:"column:status;not null"`
	Remark            *string `gorm:"column:remark"`
	DelFlag           string  `gorm:"column:del_flag;not null"`
}

func (SysRole) TableName() string { return "sys_role" }

// SysRoleMenu 角色和菜单关联表（复合主键）。
type SysRoleMenu struct {
	RoleID int64 `gorm:"column:role_id;primaryKey"`
	MenuID int64 `gorm:"column:menu_id;primaryKey"`
}

func (SysRoleMenu) TableName() string { return "sys_role_menu" }

// SysRoleDept 角色和部门关联表（复合主键，用于 data_scope=2 自定义数据权限）。
type SysRoleDept struct {
	RoleID int64 `gorm:"column:role_id;primaryKey"`
	DeptID int64 `gorm:"column:dept_id;primaryKey"`
}

func (SysRoleDept) TableName() string { return "sys_role_dept" }
