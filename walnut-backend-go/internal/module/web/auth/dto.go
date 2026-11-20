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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/validatex"
)

// LoginRequest 登录公共入参，对应 Python 端 LoginBodySchema。
//
// ⚠️ json tag 用 **camelCase**（clientId/grantType/tenantId）：
// Pydantic 的 alias 就是驼峰，前端提交的也是驼峰。
// `code` 与 `uuid` 无 alias，保持原样。
type LoginRequest struct {
	ClientID  string `json:"clientId" binding:"required,min=1"`
	GrantType string `json:"grantType" binding:"required,min=1"`
	// TenantID 租户 id（未启用多租户，收下但忽略）
	TenantID *string `json:"tenantId"`
	Code     *string `json:"code"`
	UUID     *string `json:"uuid"`
}

// PasswordLoginRequest 密码登录入参，对应 PasswordLoginBodySchema。
type PasswordLoginRequest struct {
	LoginRequest
	// Username min=2 max=30，与 Pydantic 的 Field(min_length=2, max_length=30) 一致
	Username string `json:"username" binding:"required,min=2,max=30"`
	Password string `json:"password" binding:"required,min=5,max=30"`
}

// RegisterRequest 注册入参，对应 RegisterBodySchema。
type RegisterRequest struct {
	LoginRequest
	Username string  `json:"username" binding:"required,min=2,max=30"`
	Password string  `json:"password" binding:"required,min=5,max=30"`
	UserType *string `json:"userType"`
}

// decodeLoginBody / decodeRegisterBody 镜像 Pydantic 的**全量错误收集**：
// 按字段声明序（clientId → grantType → tenantId → code → uuid → username →
// password [→ userType]）逐字段校验，多条错误用 ", " 拼接。
// ShouldBindJSON 遇到第一个类型错误即返回，无法复刻多错误拼接，故弃用。
func decodeLoginBody(body []byte) (PasswordLoginRequest, error) {
	req, _, err := decodeAuthFields(body, "PasswordLoginBodySchema", false)
	return req, err
}

func decodeRegisterBody(body []byte) (RegisterRequest, error) {
	req, ut, err := decodeAuthFields(body, "RegisterBodySchema", true)
	return RegisterRequest{LoginRequest: req.LoginRequest, Username: req.Username, Password: req.Password, UserType: ut}, err
}

func decodeAuthFields(body []byte, model string, withUserType bool) (PasswordLoginRequest, *string, error) {
	var req PasswordLoginRequest
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		// 空 body：FastAPI 对必填 body 模型报单条 body 级 "Field required"
		return req, nil, errs.BadRequest("Field required")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return req, nil, errs.BadRequest(validatex.MsgJSONInvalid)
		}
		return req, nil, errs.BadRequest("Input should be a valid dictionary or instance of " + model)
	}

	var msgs []string
	// required：缺省 → Field required；null/非字符串 → string_type；长度按 rune 计
	reqStr := func(key string, min, max int, dst *string) {
		raw, present := fields[key]
		if !present {
			msgs = append(msgs, "Field required")
			return
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			msgs = append(msgs, "Input should be a valid string")
			return
		}
		n := utf8.RuneCountInString(s)
		if n < min {
			msgs = append(msgs, fmt.Sprintf("String should have at least %d characters", min))
			return
		}
		if n > max {
			msgs = append(msgs, fmt.Sprintf("String should have at most %d characters", max))
			return
		}
		*dst = s
	}
	optStr := func(key string, dst **string) {
		raw, present := fields[key]
		if !present || string(raw) == "null" {
			return
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			msgs = append(msgs, "Input should be a valid string")
			return
		}
		*dst = &s
	}

	reqStr("clientId", 1, 1<<30, &req.ClientID)
	reqStr("grantType", 1, 1<<30, &req.GrantType)
	optStr("tenantId", &req.TenantID)
	optStr("code", &req.Code)
	optStr("uuid", &req.UUID)
	reqStr("username", 2, 30, &req.Username)
	reqStr("password", 5, 30, &req.Password)
	var userType *string
	if withUserType {
		optStr("userType", &userType)
	}
	if len(msgs) > 0 {
		return req, nil, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return req, userType, nil
}

// LoginResponse 登录成功响应。
//
// ⚠️ 字段名是 **snake_case**，与全站其余接口的 camelCase 不同。
// 原因：Python 端 AuthService.do_login 返回的是**裸 dict**，
// 不经 Pydantic 模型，因此没有 alias_generator 的驼峰转换，键名原样输出。
// Go 侧必须照抄，否则前端取不到 access_token。
//
// 对比：验证码响应用的是 CaptchaOutSchema.model_dump(by_alias=True)，
// 所以那个是 camelCase（captchaEnabled）。两处不一致是 Python 端的既有事实，
// 不是可以"顺手统一"的东西——统一了就破坏契约。
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	// RefreshToken 当前未实现刷新令牌，恒为 null（字段仍须存在）
	RefreshToken *string `json:"refresh_token"`
	// ExpireIn token 固定有效期（秒）
	ExpireIn int `json:"expire_in"`
	// RefreshExpireIn 恒为 null
	RefreshExpireIn *int   `json:"refresh_expire_in"`
	ClientID        string `json:"client_id"`
	// Scope / OpenID 为 OAuth 兼容字段，当前恒为 null
	Scope  *string `json:"scope"`
	OpenID *string `json:"openid"`
}

// CaptchaResponse 验证码响应，对应 CaptchaOutSchema.model_dump(by_alias=True)。
//
// ⚠️ 这里是 **camelCase**（captchaEnabled），与 LoginResponse 的 snake_case 不同，
// 因为它走了 Pydantic 的 by_alias 序列化。
type CaptchaResponse struct {
	// CaptchaEnabled 是否开启验证码。为 false 时前端不展示验证码输入框
	CaptchaEnabled bool `json:"captchaEnabled"`
	// UUID 验证码唯一标识，登录时随 code 一起回传
	UUID *string `json:"uuid"`
	// Img PNG 图片的 base64，**不含** data URI 前缀（前端自行拼接
	// `data:image/png;base64,`）。多带前缀会让图片裂开。
	Img *string `json:"img"`
}

// TenantListResponse 租户下拉响应。
//
// 未启用多租户，固定返回 tenantEnabled=false（camelCase，Python 端也是裸 dict
// 但键名本身就写成了驼峰）。
type TenantListResponse struct {
	TenantEnabled bool `json:"tenantEnabled"`
}
