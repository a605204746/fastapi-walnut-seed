// 本文件定义客户端域的出参 DTO 与映射辅助，对应 Python 端 module_system/client/schema.py。
//
// ⚠️ 出参同时含 grantTypeList（由 grant_type 逗号串拆出）与 grantType 原串，键序 = 声明序。

package client

import (
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/idtype"
)

// ---- 出参 ----

// Out 客户端出参，对应 ClientOutSchema（alias_generator=to_camel）。
type Out struct {
	ID            *idtype.JSInt `json:"id"`
	ClientID      *string       `json:"clientId"`
	ClientKey     *string       `json:"clientKey"`
	ClientSecret  *string       `json:"clientSecret"`
	GrantTypeList []string      `json:"grantTypeList"`
	GrantType     *string       `json:"grantType"`
	DeviceType    *string       `json:"deviceType"`
	ActiveTimeout *idtype.JSInt `json:"activeTimeout"`
	Timeout       *idtype.JSInt `json:"timeout"`
	Status        *string       `json:"status"`
}

// NewOut 由实体构造出参。
//
// ⚠️ grantTypeList 恒为数组（default_factory=list）：grant_type 非空时按逗号拆分，
// 否则空数组——不能加 omitempty。
func NewOut(r *SysClient) Out {
	return Out{
		ID:            jsIntPtr(r.ID),
		ClientID:      r.ClientID,
		ClientKey:     r.ClientKey,
		ClientSecret:  r.ClientSecret,
		GrantTypeList: crudx.Str2List(deref(r.GrantType)),
		GrantType:     r.GrantType,
		DeviceType:    r.DeviceType,
		ActiveTimeout: intPtrToJSInt(r.ActiveTimeout),
		Timeout:       intPtrToJSInt(r.Timeout),
		Status:        r.Status,
	}
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

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
