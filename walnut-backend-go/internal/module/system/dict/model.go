// Package dict 对应 sys_dict_type / sys_dict_data（字典类型与字典数据）。
package dict

import "walnut-seed-go/internal/pkg/modelx"

// SysDictType 字典类型表 sys_dict_type。
//
// ⚠️ 本表**无 del_flag**：删除是物理删除（CRUDBase.delete_batch），
// 所有查询都不加逻辑删除条件。
type SysDictType struct {
	modelx.BaseModel

	DictName *string `gorm:"column:dict_name"`
	DictType *string `gorm:"column:dict_type"`
	Remark   *string `gorm:"column:remark"`
}

func (SysDictType) TableName() string { return "sys_dict_type" }

// SysDictData 字典数据表 sys_dict_data（同样无 del_flag）。
type SysDictData struct {
	modelx.BaseModel

	DictSort  *int    `gorm:"column:dict_sort"`
	DictLabel *string `gorm:"column:dict_label"`
	DictValue *string `gorm:"column:dict_value"`
	DictType  *string `gorm:"column:dict_type"`
	CssClass  *string `gorm:"column:css_class"`
	ListClass *string `gorm:"column:list_class"`
	IsDefault *string `gorm:"column:is_default"`
	Remark    *string `gorm:"column:remark"`
}

func (SysDictData) TableName() string { return "sys_dict_data" }
