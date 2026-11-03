// Package dept 对应 sys_dept（部门表）。
package dept

import "walnut-seed-go/internal/pkg/modelx"

// SysDept 部门表 sys_dept。
//
// Ancestors 是祖级列表（如 "0,100,101"），树形查询与数据权限
// 「本部门及以下」都靠它做前缀匹配，避免递归 CTE。
type SysDept struct {
	modelx.BaseModel

	ParentID     *int64  `gorm:"column:parent_id"`
	Ancestors    *string `gorm:"column:ancestors"`
	DeptName     *string `gorm:"column:dept_name"`
	DeptCategory *string `gorm:"column:dept_category"`
	OrderNum     *int    `gorm:"column:order_num"`
	Leader       *int64  `gorm:"column:leader"`
	Phone        *string `gorm:"column:phone"`
	Email        *string `gorm:"column:email"`
	Status       *string `gorm:"column:status"`
	DelFlag      string  `gorm:"column:del_flag;not null"`
}

func (SysDept) TableName() string { return "sys_dept" }
