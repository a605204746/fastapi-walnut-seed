// 本文件定义参数配置域的入参/出参 DTO，对应 Python 端 module_system/config/schema.py。
//
// ⚠️ 校验器无 validate_default：字段缺省时不跑校验（值留 None），
// 落库缺省值由 service 层按列默认值镜像（""/""/""/"N"/NULL）。

package config

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
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

// ---- 查询入参 ----

// Query 参数配置列表/导出查询参数，对应 ConfigQueryParam。
type Query struct {
	Page       crudx.Page
	ConfigName string
	ConfigKey  string
	ConfigType string
	BeginTime  string
	EndTime    string
}

// BindQuery 从查询串解析。
func BindQuery(q crudx.Querier) (Query, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return Query{}, err
	}
	return Query{
		Page:       page,
		ConfigName: q.Query("configName"),
		ConfigKey:  q.Query("configKey"),
		ConfigType: q.Query("configType"),
		BeginTime:  q.Query("beginTime"),
		EndTime:    q.Query("endTime"),
	}, nil
}

// ---- 写入入参 ----

// CreateIn 新增参数配置入参，对应 ConfigCreateSchema。
type CreateIn struct {
	ConfigName  *string
	ConfigKey   *string
	ConfigValue *string
	ConfigType  *string
	Remark      *string
}

// UpdateIn 按主键修改入参（id 必填，缺省报 Field required）。
type UpdateIn struct {
	CreateIn
	ID *int64
}

// UpdateByKeyIn 按键名修改入参，对应 ConfigUpdateByKeySchema。
//
// ⚠️ id 可选；config_key / config_value 的校验器只对显式提供的键生效，
// 但 service 层对 None 仍会抛 500「不能为空」。
type UpdateByKeyIn struct {
	ID          *int64
	ConfigName  *string
	ConfigKey   *string
	ConfigValue *string
	ConfigType  *string
	Remark      *string
}

// Pydantic v2 类型错误原文（属契约）。
const (
	msgFieldRequired = "Field required"
	msgValidInt      = "Input should be a valid integer, unable to parse string as an integer"
	msgValidIntNum   = "Input should be a valid integer, got a number with a fractional part"
	msgValidIntT     = "Input should be a valid integer"
	msgValidStr      = "Input should be a valid string"
)

// valueErr 校验器错误对外文案（"Value error, " 前缀已被全局剥掉）。
func valueErr(msg string) string { return msg }

// DecodeCreate 解析并校验新增入参。声明序：config_name → config_key → config_value → config_type → remark。
func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	if raw, ok := fields["configName"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigName = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("参数名称不能为空"))
			case utf8.RuneCountInString(*v) > 100:
				msgs = append(msgs, valueErr("参数名称不能超过100个字符"))
			}
		}
	}
	if raw, ok := fields["configKey"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigKey = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("参数键名不能为空"))
			case utf8.RuneCountInString(*v) > 100:
				msgs = append(msgs, valueErr("参数键名长度不能超过100个字符"))
			}
		}
	}
	if raw, ok := fields["configValue"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigValue = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("参数键值不能为空"))
			case utf8.RuneCountInString(*v) > 500:
				msgs = append(msgs, valueErr("参数键值长度不能超过500个字符"))
			}
		}
	}
	if raw, ok := fields["configType"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigType = v
		}
	}
	if raw, ok := fields["remark"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.Remark = v
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeUpdate 解析按主键修改入参（id 追加在父类字段之后）。
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
		msgs = append(msgs, msgFieldRequired)
	} else if v, terr := decodeRequiredInt(raw); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.ID = v
	}
	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeUpdateByKey 解析按键名修改入参。声明序：id → config_name → config_key → config_value → config_type → remark。
func DecodeUpdateByKey(body []byte) (UpdateByKeyIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return UpdateByKeyIn{}, err
	}
	var in UpdateByKeyIn
	var msgs []string

	if raw, ok := fields["id"]; ok {
		v, terr := decodeInt(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ID = v
		}
	}
	if raw, ok := fields["configName"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigName = v
		}
	}
	if raw, ok := fields["configKey"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigKey = v
			if v == nil || strings.TrimSpace(*v) == "" {
				msgs = append(msgs, valueErr("参数键名不能为空"))
			}
		}
	}
	if raw, ok := fields["configValue"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigValue = v
			if v == nil || strings.TrimSpace(*v) == "" {
				msgs = append(msgs, valueErr("参数键值不能为空"))
			}
		}
	}
	if raw, ok := fields["configType"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ConfigType = v
		}
	}
	if raw, ok := fields["remark"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.Remark = v
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// ---- 内部工具（与 dict dto 同构）----

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

func decodeRequiredInt(raw json.RawMessage) (*int64, string) {
	if string(raw) == "null" {
		return nil, msgValidIntT
	}
	n, ok, terr := parseLooseInt(raw)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, msgValidIntT
	}
	return &n, ""
}

func decodeInt(raw json.RawMessage) (*int64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	n, ok, terr := parseLooseInt(raw)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, ""
	}
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

func parseLooseInt(raw json.RawMessage) (int64, bool, string) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, false, ""
	}
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, false, msgValidStr
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return 0, false, msgValidInt
		}
		return n, true, ""
	}
	if text == "true" || text == "false" {
		return 0, false, msgValidIntT
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false, msgValidInt
	}
	if f != float64(int64(f)) {
		return 0, false, msgValidIntNum
	}
	return int64(f), true, ""
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func strPtr(v string) *string { return &v }
