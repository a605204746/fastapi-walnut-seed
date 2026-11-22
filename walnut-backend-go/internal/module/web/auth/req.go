// Package auth 实现认证端点，对应 Python 端 module_web/auth。
//
// URL 契约：
//
//	POST /auth/login        登录（grant_type=password；请求体加解密由 ApiDecrypt 中间件处理）
//	POST /auth/logout       登出
//	POST /auth/register     注册
//	GET  /auth/tenant/list  租户下拉（未启用多租户，固定返回关闭）
//	GET  /auth/code         图形验证码
package auth

import (
	"context"

	"walnut-seed-go/internal/pkg/bindx"
)

// LoginRequest 登录公共入参，对应 Python 端 LoginBodySchema（service 消费形态）。
//
// ⚠️ json tag 用 **camelCase**（clientId/grantType/tenantId）：
// Pydantic 的 alias 就是驼峰，前端提交的也是驼峰。
// `code` 与 `uuid` 无 alias，保持原样。
type LoginRequest struct {
	ClientID  string `json:"clientId"`
	GrantType string `json:"grantType"`
	// TenantID 租户 id（未启用多租户，收下但忽略）
	TenantID *string `json:"tenantId"`
	Code     *string `json:"code"`
	UUID     *string `json:"uuid"`
}

// PasswordLoginRequest 密码登录入参，对应 PasswordLoginBodySchema（service 消费形态）。
type PasswordLoginRequest struct {
	LoginRequest
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterRequest 注册入参，对应 RegisterBodySchema（service 消费形态）。
type RegisterRequest struct {
	LoginRequest
	Username string  `json:"username"`
	Password string  `json:"password"`
	UserType *string `json:"userType"`
}

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 decodeAuthFields 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - 空 body：旧单条 body 级 "Field required" → 按字段 i18n 必填拼接
//   - 必填/长度/类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - clientId/grantType 补 notblank（纯空白拒绝，对齐业务非空语义）
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集
//
// 字段序镜像旧校验序：clientId → grantType → tenantId → code → uuid →
// username → password [→ userType]（可选字段无校验器，仅类型检查）。

// LoginReq 密码登录请求体，对应 PasswordLoginBodySchema。
type LoginReq struct {
	ClientID  string  `json:"clientId" validate:"required,notblank" label:"客户端id"`
	GrantType string  `json:"grantType" validate:"required,notblank" label:"授权类型"`
	TenantID  *string `json:"tenantId"`
	Code      *string `json:"code"`
	UUID      *string `json:"uuid"`
	// Username min=2 max=30，与 Pydantic 的 Field(min_length=2, max_length=30) 一致
	Username string `json:"username" validate:"required,min=2,max=30" label:"用户名"`
	Password string `json:"password" validate:"required,min=5,max=30" label:"密码"`
}

// RegisterReq 注册请求体，对应 RegisterBodySchema（userType 追加在末尾）。
type RegisterReq struct {
	LoginReq
	UserType *string `json:"userType"`
}

func (r LoginReq) toLoginRequest() LoginRequest {
	return LoginRequest{
		ClientID:  r.ClientID,
		GrantType: r.GrantType,
		TenantID:  r.TenantID,
		Code:      r.Code,
		UUID:      r.UUID,
	}
}

// decodeLoginBody 解析并校验登录入参（service 消费 PasswordLoginRequest，零改动）。
func decodeLoginBody(ctx context.Context, body []byte) (PasswordLoginRequest, error) {
	var req LoginReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return PasswordLoginRequest{}, err
	}
	return PasswordLoginRequest{
		LoginRequest: req.toLoginRequest(),
		Username:     req.Username,
		Password:     req.Password,
	}, nil
}

// decodeRegisterBody 解析并校验注册入参（service 消费 RegisterRequest，零改动）。
func decodeRegisterBody(ctx context.Context, body []byte) (RegisterRequest, error) {
	var req RegisterReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return RegisterRequest{}, err
	}
	return RegisterRequest{
		LoginRequest: req.LoginReq.toLoginRequest(),
		Username:     req.Username,
		Password:     req.Password,
		UserType:     req.UserType,
	}, nil
}
