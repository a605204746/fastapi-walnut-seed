// Package post 对应 sys_post（岗位表）。
//
// ⚠️ sys_post **没有 del_flag 列**，删除是物理删除——与 sys_user / sys_dept /
// sys_role 的逻辑删除不同。查询时不要加 del_flag 条件，否则 SQL 直接报未知列。
package post

import "walnut-seed-go/internal/pkg/modelx"

// SysPost 岗位信息表 sys_post。
type SysPost struct {
	modelx.BaseModel

	// DeptID 部门ID。列是 NOT NULL，故用值类型而非指针
	DeptID int64 `gorm:"column:dept_id;not null"`
	// PostCode 岗位编码，varchar(64) NOT NULL
	PostCode string `gorm:"column:post_code;not null"`
	// PostCategory 岗位类别编码，varchar(100) 可空
	PostCategory *string `gorm:"column:post_category"`
	// PostName 岗位名称，varchar(50) NOT NULL
	PostName string `gorm:"column:post_name;not null"`
	// PostSort 显示顺序，int NOT NULL
	PostSort int `gorm:"column:post_sort;not null"`
	// Status 状态（0正常 1停用），varchar(1) NOT NULL —— 字符串契约，不是整数
	Status string `gorm:"column:status;not null"`
	// Remark 备注，varchar(500) 可空
	Remark *string `gorm:"column:remark"`
}

// TableName 显式声明表名。
func (SysPost) TableName() string { return "sys_post" }
