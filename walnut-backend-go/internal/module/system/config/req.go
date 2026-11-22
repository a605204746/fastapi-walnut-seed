// 本文件定义参数配置域的入参/查询 DTO，对应 Python 端 module_system/config/schema.py。
//
// ⚠️ 校验器无 validate_default：字段缺省时不跑校验（值留 None），
// 落库缺省值由 service 层按列默认值镜像（""/""/""/"N"/NULL）。
// 但**显式 null 会触发校验器**（Pydantic 语义）——见 nullToEmpty 的说明。
//
// JSON body 走阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案。

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

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
//
// beginTime/endTime 是 DateStr|None：出现（含空串）即走 Pydantic 解析，
// 非法 → 400 date 解析原文（bindx.ParseDateQuery）。多个错误按声明序拼接：
// pageNum/pageSize（crudx）→ beginTime → endTime。
func BindQuery(q crudx.Querier) (Query, error) {
	page, pageErr := crudx.BindPageQuery(q)
	out := Query{
		Page:       page,
		ConfigName: q.Query("configName"),
		ConfigKey:  q.Query("configKey"),
		ConfigType: q.Query("configType"),
	}
	var msgs []string
	if pageErr != nil {
		se := errs.As(pageErr)
		if se == nil {
			return Query{}, pageErr
		}
		msgs = append(msgs, se.Msg)
	}
	dateField := func(key string, dst *string) {
		if raw, ok := q.GetQuery(key); ok {
			v, derr := bindx.ParseDateQuery(raw)
			if derr != nil {
				if se := errs.As(derr); se != nil {
					msgs = append(msgs, se.Msg)
				}
				return
			}
			*dst = v
		}
	}
	dateField("beginTime", &out.BeginTime)
	dateField("endTime", &out.EndTime)
	if len(msgs) > 0 {
		return Query{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return out, nil
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

// Pydantic v2 类型错误原文（handler 的 ID 串解析仍在用，单点托管于 bindx）。
const msgValidInt = bindx.MsgValidInt

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - lax 强转退役（"3.0"/"1_0" 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - 长度文案新格式（"参数名称长度不能超过100个字符"）
//   - update 缺省 id 的 "Field required" → i18n "id不能为空"
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集
//
// ⚠️ 本域校验器无 validate_default：**缺省不校验、显式 null 触发校验**。
// encoding/json 对 *string 的缺省与 null 都落 nil，无法区分——故绑定前经
// nullToEmpty 把受校验键的显式 null 归一为 ""，配 omitempty tag 复刻：
// 缺省 → 指针留 nil → 跳过；null/空串/空白 → notblank 拒绝。

// nullToEmpty 把指定键的显式 null 归一为 ""（其余键值原样保留）。
// 非法 JSON / 非对象体原样返回，交给 BindJSON 报格式错误。
// ⚠️ 用 Decoder 按原文键序重建（不是 map 重序列化）：保持 encoding/json
// 「首个类型错误按文档序短路」的确定性。
func nullToEmpty(body []byte, keys ...string) []byte {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil || probe == nil {
		return body
	}
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	changed := false
	for k, raw := range probe {
		if set[k] && string(raw) == "null" {
			changed = true
		}
	}
	if !changed {
		return body
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	if _, err := dec.Token(); err != nil { // 消费 '{'
		return body
	}
	var b strings.Builder
	b.WriteByte('{')
	for first := true; dec.More(); first = false {
		keyTok, err := dec.Token()
		if err != nil {
			return body
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return body
		}
		if !first {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(keyTok)
		b.Write(kb)
		b.WriteByte(':')
		if key, _ := keyTok.(string); set[key] && string(raw) == "null" {
			b.WriteString(`""`)
		} else {
			b.Write(raw)
		}
	}
	b.WriteByte('}')
	return []byte(b.String())
}

// CreateReq 新增参数配置请求体，对应 ConfigCreateSchema。
// 声明序：config_name → config_key → config_value → config_type → remark。
type CreateReq struct {
	ConfigName  *string `json:"configName" validate:"omitempty,notblank,max=100" label:"参数名称"`
	ConfigKey   *string `json:"configKey" validate:"omitempty,notblank,max=100" label:"参数键名"`
	ConfigValue *string `json:"configValue" validate:"omitempty,notblank,max=500" label:"参数键值"`
	ConfigType  *string `json:"configType"`
	Remark      *string `json:"remark"`
}

// UpdateReq 按主键修改请求体（id 必填，追加在父类字段之后）。
type UpdateReq struct {
	CreateReq
	ID *int64 `json:"id" validate:"required" label:"id"`
}

// UpdateByKeyReq 按键名修改请求体，对应 ConfigUpdateByKeySchema。
// 声明序：id → config_name → config_key → config_value → config_type → remark。
// ⚠️ id/config_name 无校验器；config_key/config_value 仅非空校验（无长度上限）。
type UpdateByKeyReq struct {
	ID          *int64  `json:"id"`
	ConfigName  *string `json:"configName"`
	ConfigKey   *string `json:"configKey" validate:"omitempty,notblank" label:"参数键名"`
	ConfigValue *string `json:"configValue" validate:"omitempty,notblank" label:"参数键值"`
	ConfigType  *string `json:"configType"`
	Remark      *string `json:"remark"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		ConfigName:  r.ConfigName,
		ConfigKey:   r.ConfigKey,
		ConfigValue: r.ConfigValue,
		ConfigType:  r.ConfigType,
		Remark:      r.Remark,
	}
}

// DecodeCreate 解析并校验新增入参（service 消费 CreateIn，零改动）。
func DecodeCreate(ctx context.Context, body []byte) (CreateIn, error) {
	var req CreateReq
	body = nullToEmpty(body, "configName", "configKey", "configValue")
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return CreateIn{}, err
	}
	return req.toIn(), nil
}

// DecodeUpdate 解析按主键修改入参。
func DecodeUpdate(ctx context.Context, body []byte) (UpdateIn, error) {
	var req UpdateReq
	body = nullToEmpty(body, "configName", "configKey", "configValue")
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return UpdateIn{}, err
	}
	return UpdateIn{CreateIn: req.CreateReq.toIn(), ID: req.ID}, nil
}

// DecodeUpdateByKey 解析按键名修改入参。
func DecodeUpdateByKey(ctx context.Context, body []byte) (UpdateByKeyIn, error) {
	var req UpdateByKeyReq
	body = nullToEmpty(body, "configKey", "configValue")
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return UpdateByKeyIn{}, err
	}
	return UpdateByKeyIn{
		ID:          req.ID,
		ConfigName:  req.ConfigName,
		ConfigKey:   req.ConfigKey,
		ConfigValue: req.ConfigValue,
		ConfigType:  req.ConfigType,
		Remark:      req.Remark,
	}, nil
}
