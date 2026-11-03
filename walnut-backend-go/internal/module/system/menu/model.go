// Package menu 对应 sys_menu（菜单权限表）。
package menu

import "walnut-seed-go/internal/pkg/modelx"

// SysMenu 菜单权限表 sys_menu。
//
// menu_type 取值：'M' 目录、'C' 菜单、'F' 按钮。
// perms 是权限标识（如 system:user:add），P3 的 getRouters 与权限校验都依赖它。
type SysMenu struct {
	modelx.BaseModel

	MenuName   string  `gorm:"column:menu_name;not null"`
	ParentID   *int64  `gorm:"column:parent_id"`
	OrderNum   *int    `gorm:"column:order_num"`
	Path       *string `gorm:"column:path"`
	Component  *string `gorm:"column:component"`
	QueryParam *string `gorm:"column:query_param"`
	// IsFrame 是否外链：'0' 是、'1' 否（注意与直觉相反）
	IsFrame *string `gorm:"column:is_frame"`
	// IsCache 是否缓存：'0' 缓存、'1' 不缓存
	IsCache  *string `gorm:"column:is_cache"`
	MenuType *string `gorm:"column:menu_type"`
	// Visible 显示状态：'0' 显示、'1' 隐藏
	Visible *string `gorm:"column:visible"`
	// Status 菜单状态：'0' 正常、'1' 停用
	Status *string `gorm:"column:status"`
	Perms  *string `gorm:"column:perms"`
	Icon   *string `gorm:"column:icon"`
	Remark *string `gorm:"column:remark"`
}

func (SysMenu) TableName() string { return "sys_menu" }

// 菜单域常量，对应 Python 端 Constants / SystemConstants 中与菜单相关的部分。
const (
	// TopParentID 顶级菜单的 parent_id
	TopParentID int64 = 0

	// TypeDir 目录、TypeMenu 菜单、TypeButton 按钮
	TypeDir    = "M"
	TypeMenu   = "C"
	TypeButton = "F"

	// YesFrame 是外链、NoFrame 不是外链（⚠️ 与直觉相反）
	YesFrame = "0"
	NoFrame  = "1"

	// Layout / ParentView / InnerLink 是前端约定的组件名
	Layout     = "Layout"
	ParentView = "ParentView"
	InnerLink  = "InnerLink"
)
