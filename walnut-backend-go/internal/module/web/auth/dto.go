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
