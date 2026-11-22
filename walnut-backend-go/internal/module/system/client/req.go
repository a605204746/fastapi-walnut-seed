// 本文件定义客户端域的入参/查询 DTO，对应 Python 端 module_system/client/schema.py。
//
// ⚠️ client_key / client_secret / grant_type_list 带 validate_default：缺省也跑校验器；
// update 的 id 同样 validate_default，缺省报「id不能为空」。

package client

import (
	"context"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
)

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

// Pydantic v2 类型错误原文（handler 的 ID 串解析仍在用，单点托管于 bindx）。
const msgValidInt = bindx.MsgValidInt

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - lax 强转退役（"3.0"/"1_0" 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - 空数组 grantTypeList 拒绝（对齐 Python falsy 语义；旧实现 nil-only 检查是移植偏差）
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集

// CreateReq 新增客户端请求体，对应 ClientCreateSchema。
type CreateReq struct {
	ClientKey     string   `json:"clientKey" validate:"required,notblank" label:"客户端key"`
	ClientSecret  string   `json:"clientSecret" validate:"required,notblank" label:"客户端秘钥"`
	GrantTypeList []string `json:"grantTypeList" validate:"required,min=1" label:"授权类型"`
	DeviceType    *string  `json:"deviceType"`
	ActiveTimeout *int     `json:"activeTimeout" label:"活跃超时时间"`
	Timeout       *int     `json:"timeout" label:"超时时间"`
	Status        *string  `json:"status"`
}

// UpdateReq 修改客户端请求体（id 必填，clientId 可选）。
type UpdateReq struct {
	CreateReq
	ID       *int64  `json:"id" validate:"required" label:"id"`
	ClientID *string `json:"clientId" label:"客户端id"`
}

// StatusReq 状态修改请求体，对应 ClientStatusSchema。
type StatusReq struct {
	ClientID string `json:"clientId" validate:"required,notblank" label:"客户端id"`
	Status   string `json:"status" validate:"required,notblank" label:"状态"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		ClientKey:     &r.ClientKey,
		ClientSecret:  &r.ClientSecret,
		GrantTypeList: r.GrantTypeList,
		DeviceType:    r.DeviceType,
		ActiveTimeout: r.ActiveTimeout,
		Timeout:       r.Timeout,
		Status:        r.Status,
	}
}

// DecodeCreate 解析并校验新增入参（service 消费 CreateIn，零改动）。
func DecodeCreate(ctx context.Context, body []byte) (CreateIn, error) {
	var req CreateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return CreateIn{}, err
	}
	return req.toIn(), nil
}

// DecodeUpdate 解析修改入参。
func DecodeUpdate(ctx context.Context, body []byte) (UpdateIn, error) {
	var req UpdateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return UpdateIn{}, err
	}
	return UpdateIn{CreateIn: req.CreateReq.toIn(), ID: req.ID, ClientID: req.ClientID}, nil
}

// DecodeStatus 解析状态修改入参。
func DecodeStatus(ctx context.Context, body []byte) (StatusIn, error) {
	var req StatusReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return StatusIn{}, err
	}
	return StatusIn{ClientID: &req.ClientID, Status: &req.Status}, nil
}
