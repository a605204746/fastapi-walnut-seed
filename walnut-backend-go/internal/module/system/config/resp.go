// 本文件定义参数配置域的出参 DTO 与映射辅助，对应 Python 端 module_system/config/schema.py。

package config

import (
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// ---- 出参 ----

// Out 参数配置出参，对应 ConfigOutSchema（键序 = 声明序）。
type Out struct {
	ID          *idtype.JSInt   `json:"id"`
	ConfigName  *string         `json:"configName"`
	ConfigKey   *string         `json:"configKey"`
	ConfigValue *string         `json:"configValue"`
	ConfigType  *string         `json:"configType"`
	Remark      *string         `json:"remark"`
	CreateTime  *timex.JSONTime `json:"createTime"`
}

// NewOut 由实体构造出参。
func NewOut(r *SysConfig) Out {
	out := Out{
		ID:          jsIntPtr(r.ID),
		ConfigName:  r.ConfigName,
		ConfigKey:   r.ConfigKey,
		ConfigValue: r.ConfigValue,
		ConfigType:  r.ConfigType,
		Remark:      r.Remark,
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

func strPtr(v string) *string { return &v }
