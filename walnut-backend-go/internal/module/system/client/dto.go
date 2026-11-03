// 本文件定义客户端域的入参/出参 DTO，对应 Python 端 module_system/client/schema.py。
//
// ⚠️ client_key / client_secret / grant_type_list 带 validate_default：缺省也跑校验器；
// update 的 id 同样 validate_default，缺省报「id不能为空」。
// ⚠️ 出参同时含 grantTypeList（由 grant_type 逗号串拆出）与 grantType 原串，键序 = 声明序。

package client

import (
	"encoding/json"
	"strconv"
	"strings"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
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

// ---- 查询入参 ----

// Query 客户端列表查询参数，对应 ClientQueryParam（全字符串等值过滤）。
type Query struct {
	Page         crudx.Page
	ClientID     string
	ClientKey    string
	ClientSecret string
	Status       string
}

// BindQuery 从查询串解析。
func BindQuery(q crudx.Querier) (Query, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return Query{}, err
	}
	return Query{
		Page:         page,
		ClientID:     q.Query("clientId"),
		ClientKey:    q.Query("clientKey"),
		ClientSecret: q.Query("clientSecret"),
		Status:       q.Query("status"),
	}, nil
}

// ---- 写入入参 ----

// CreateIn 新增客户端入参，对应 ClientCreateSchema。
type CreateIn struct {
	ClientKey     *string
	ClientSecret  *string
	GrantTypeList []string
	DeviceType    *string
	ActiveTimeout *int
	Timeout       *int
	Status        *string
}

// UpdateIn 修改客户端入参（id / client_id 均可带）。
type UpdateIn struct {
	CreateIn
	ID       *int64
	ClientID *string
}

// StatusIn 状态修改入参，对应 ClientStatusSchema（两字段均 validate_default）。
type StatusIn struct {
	ClientID *string
	Status   *string
}

// Pydantic v2 类型错误原文（属契约）。
const (
	msgValidInt    = "Input should be a valid integer, unable to parse string as an integer"
	msgValidIntNum = "Input should be a valid integer, got a number with a fractional part"
	msgValidIntT   = "Input should be a valid integer"
	msgValidStr    = "Input should be a valid string"
	msgValidList   = "Input should be a valid list"
)

// valueErr 校验器错误对外文案（"Value error, " 前缀已被全局剥掉）。
func valueErr(msg string) string { return msg }

// DecodeCreate 解析并校验新增入参。
// 声明序：client_key → client_secret → grant_type_list → device_type → active_timeout → timeout → status。
func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	// client_key：validate_default → 缺省也校验
	if raw, ok := fields["clientKey"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ClientKey = v
			if v == nil || strings.TrimSpace(*v) == "" {
				msgs = append(msgs, valueErr("客户端key不能为空"))
			}
		}
	} else {
		msgs = append(msgs, valueErr("客户端key不能为空"))
	}
	// client_secret：validate_default
	if raw, ok := fields["clientSecret"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ClientSecret = v
			if v == nil || strings.TrimSpace(*v) == "" {
				msgs = append(msgs, valueErr("客户端秘钥不能为空"))
			}
		}
	} else {
		msgs = append(msgs, valueErr("客户端秘钥不能为空"))
	}
	// grant_type_list：validate_default；类型是 list[str]
	if raw, ok := fields["grantTypeList"]; ok {
		list, terr := decodeStrList(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.GrantTypeList = list
			if list == nil {
				msgs = append(msgs, valueErr("授权类型不能为空"))
			}
		}
	} else {
		msgs = append(msgs, valueErr("授权类型不能为空"))
	}
	if raw, ok := fields["deviceType"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.DeviceType = v
		}
	}
	if raw, ok := fields["activeTimeout"]; ok {
		v, terr := decodeInt(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ActiveTimeout = intPtr(v)
		}
	}
	if raw, ok := fields["timeout"]; ok {
		v, terr := decodeInt(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.Timeout = intPtr(v)
		}
	}
	if raw, ok := fields["status"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.Status = v
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeUpdate 解析修改入参。声明序：父类字段 → id → client_id。
func DecodeUpdate(body []byte) (UpdateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return UpdateIn{}, err
	}
	base, baseErr := DecodeCreate(body)
	in := UpdateIn{CreateIn: base}

	var msgs []string
	if baseErr != nil {
		if se := errs.As(baseErr); se != nil {
			msgs = append(msgs, se.Msg)
		}
	}
	if raw, ok := fields["id"]; !ok {
		msgs = append(msgs, valueErr("id不能为空"))
	} else if string(raw) == "null" {
		msgs = append(msgs, valueErr("id不能为空"))
	} else if v, terr := decodeInt(raw); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.ID = v
		if v == nil {
			msgs = append(msgs, valueErr("id不能为空"))
		}
	}
	if raw, ok := fields["clientId"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ClientID = v
		}
	}
	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeStatus 解析状态修改入参（两字段均 validate_default）。
func DecodeStatus(body []byte) (StatusIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return StatusIn{}, err
	}
	var in StatusIn
	var msgs []string

	if raw, ok := fields["clientId"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ClientID = v
			if v == nil || strings.TrimSpace(*v) == "" {
				msgs = append(msgs, valueErr("客户端id不能为空"))
			}
		}
	} else {
		msgs = append(msgs, valueErr("客户端id不能为空"))
	}
	if raw, ok := fields["status"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.Status = v
			if v == nil || strings.TrimSpace(*v) == "" {
				msgs = append(msgs, valueErr("状态不能为空"))
			}
		}
	} else {
		msgs = append(msgs, valueErr("状态不能为空"))
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// ---- 内部工具 ----

func rawFields(body []byte) (map[string]json.RawMessage, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, errs.BadRequest(errs.MsgJSONInvalid)
	}
	if fields == nil {
		return map[string]json.RawMessage{}, nil
	}
	return fields, nil
}

// decodeStrList 解析 list[str]；null/缺省由调用方处理；非列表报 list 原文。
func decodeStrList(raw json.RawMessage) ([]string, string) {
	if string(raw) == "null" {
		return nil, ""
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, msgValidList
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		var s string
		if err := json.Unmarshal(item, &s); err != nil {
			return nil, msgValidStr
		}
		out = append(out, s)
	}
	return out, ""
}

func decodeInt(raw json.RawMessage) (*int64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, msgValidStr
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, msgValidInt
		}
		return &n, ""
	}
	if text == "true" || text == "false" {
		return nil, msgValidIntT
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, msgValidInt
	}
	if f != float64(int64(f)) {
		return nil, msgValidIntNum
	}
	n := int64(f)
	return &n, ""
}

func decodeStr(raw json.RawMessage) (*string, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, msgValidStr
	}
	return &s, ""
}

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

func intPtr(v *int64) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
