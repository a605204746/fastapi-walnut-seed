// 本文件定义字典域的出参 DTO 与映射辅助，对应 Python 端 module_system/dict/schema.py。

package dict

import (
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// ---- 出参 ----

// TypeOut 字典类型出参，对应 DictTypeOutSchema（键序 = 声明序）。
type TypeOut struct {
	ID         *idtype.JSInt   `json:"id"`
	DictName   *string         `json:"dictName"`
	DictType   *string         `json:"dictType"`
	Remark     *string         `json:"remark"`
	CreateTime *timex.JSONTime `json:"createTime"`
}

// NewTypeOut 由实体构造出参。
func NewTypeOut(r *SysDictType) TypeOut {
	out := TypeOut{ID: jsIntPtr(r.ID), DictName: r.DictName, DictType: r.DictType, Remark: r.Remark}
	if r.CreateTime != nil {
		jt := timex.FromTime(*r.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// DataOut 字典数据出参，对应 DictDataOutSchema（键序 = 声明序）。
type DataOut struct {
	ID         *idtype.JSInt   `json:"id"`
	DictSort   *idtype.JSInt   `json:"dictSort"`
	DictLabel  *string         `json:"dictLabel"`
	DictValue  *string         `json:"dictValue"`
	DictType   *string         `json:"dictType"`
	CssClass   *string         `json:"cssClass"`
	ListClass  *string         `json:"listClass"`
	IsDefault  *string         `json:"isDefault"`
	Remark     *string         `json:"remark"`
	CreateTime *timex.JSONTime `json:"createTime"`
}

// NewDataOut 由实体构造出参。
func NewDataOut(r *SysDictData) DataOut {
	out := DataOut{
		ID:        jsIntPtr(r.ID),
		DictSort:  intPtrToJSInt(r.DictSort),
		DictLabel: r.DictLabel,
		DictValue: r.DictValue,
		DictType:  r.DictType,
		CssClass:  r.CssClass,
		ListClass: r.ListClass,
		IsDefault: r.IsDefault,
		Remark:    r.Remark,
	}
	if r.CreateTime != nil {
		jt := timex.FromTime(*r.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// ---- 映射辅助 ----

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func intPtrToJSInt(v *int) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(int64(*v))
	return &j
}

func strPtr(v string) *string { return &v }
