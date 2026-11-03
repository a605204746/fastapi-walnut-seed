// Package errs 实现业务异常体系，对齐 Python 端 app/core/exceptions.py。
//
// 契约要点：
//   - ServiceError 是唯一业务异常，**默认 HTTP 200**，真实错误码写入 body.code
//   - 401（NotLogin）/403（NotPermission、NotRole）同样走 HTTP 200 + body code，
//     这与「HTTP 语义错误返回真实状态码」并不矛盾——后者只针对框架级 404/405
//     与健康探针 503
//   - detail 信息仅在非生产环境回传，生产不外泄
package errs

import (
	"errors"
	"fmt"
)

// 业务状态码（写入 body.code），与 Python 端 HttpStatus 枚举对齐。
const (
	CodeSuccess      = 200
	CodeBadRequest   = 400
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeNotFound     = 404
	CodeBadMethod    = 405
	CodeConflict     = 409
	CodeError        = 500
	CodeWarn         = 601
)

// AuthErrorCode 认证模块错误码（10000–19999 段）。
//
// Code 与 Key 必须与 Python 端 module_web/exception.py 的 AuthErrorCode 逐条一致，
// 否则前端按 code 分支的逻辑（如 10005 提示重新登录）会错位。
type AuthErrorCode struct {
	Code int
	Key  string
	// Fallback 是 i18n 未就绪或消息键缺失时的兜底文案，
	// 取自 messages_zh_CN.properties，与 Python 端 i18n 解析成功后的结果一致。
	Fallback string
}

var (
	AuthCaptchaExpired        = AuthErrorCode{10001, "user.jcaptcha.expire", "验证码已失效"}
	AuthCaptchaError          = AuthErrorCode{10002, "user.jcaptcha.error", "验证码错误"}
	AuthUserNotExists         = AuthErrorCode{10003, "user.not.exists", "用户不存在"}
	AuthUserBlocked           = AuthErrorCode{10004, "user.blocked", "用户已停用，请联系管理员"}
	AuthUserPasswordNotMatch  = AuthErrorCode{10005, "user.password.not.match", "用户不存在/密码错误"}
	AuthPasswordRetryExceeded = AuthErrorCode{10006, "user.password.retry.limit.exceed", "密码输入错误{0}次，帐户锁定{1}分钟"}
	AuthUserRegisterFailed    = AuthErrorCode{10007, "user.register.error", "注册失败，请联系系统管理人员"}
	AuthUserRegisterExists    = AuthErrorCode{10008, "user.register.save.error", "保存用户 {0} 失败，注册账号已存在"}
)

// 通用异常文案，与 Python 端各 exception_handler 的字面量一致。
const (
	MsgNotFound         = "请求地址不存在"
	MsgDBDuplicate      = "数据库中已存在该记录，请联系管理员确认"
	MsgDBForeignKey     = "存在关联数据，无法删除"
	MsgDBIntegrity      = "数据已存在或违反完整性约束"
	MsgDBFailed         = "数据库操作失败"
	MsgValidationFailed = "请求参数验证失败"
	MsgJSONInvalid      = "请求数据格式错误（JSON 解析失败）"
	MsgResponseInvalid  = "服务器响应格式错误"
	MsgSystemError      = "发生系统异常，请联系管理员"
	MsgNotLogin         = "认证失败，无法访问系统资源"
	MsgNotPermission    = "没有访问权限，请联系管理员授权"
)

// ServiceError 业务异常。
//
// HTTPStatus 为 0 表示沿用 200（业务失败的常规形态）；
// 仅框架级错误（404/405）与健康探针（503）需要显式设置。
type ServiceError struct {
	Code       int    // body.code
	Msg        string // body.msg
	Detail     string // body.data，仅非生产环境回传
	HTTPStatus int    // 传输层状态码，0 → 200
	MessageKey string // i18n 消息键，非空时由 i18nx 覆盖 Msg
	KeyArgs    []any  // i18n 占位符参数
}

// Error 实现 error 接口。
func (e *ServiceError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s (code=%d, detail=%s)", e.Msg, e.Code, e.Detail)
	}
	return fmt.Sprintf("%s (code=%d)", e.Msg, e.Code)
}

// Status 返回实际使用的传输层状态码。
func (e *ServiceError) Status() int {
	if e.HTTPStatus == 0 {
		return CodeSuccess
	}
	return e.HTTPStatus
}

// WithDetail 附加 detail（生产环境由渲染层过滤）。
func (e *ServiceError) WithDetail(format string, args ...any) *ServiceError {
	e.Detail = fmt.Sprintf(format, args...)
	return e
}

// New 构造 code=500 的业务异常（对齐 ServiceException("msg")，默认 code 为 HttpStatus.ERROR）。
func New(msg string) *ServiceError {
	return &ServiceError{Code: CodeError, Msg: msg}
}

// Newf 是 New 的格式化版本。
func Newf(format string, args ...any) *ServiceError {
	return &ServiceError{Code: CodeError, Msg: fmt.Sprintf(format, args...)}
}

// WithCode 构造指定业务码的异常（对齐 ServiceException("msg", code=xxx)）。
func WithCode(code int, msg string) *ServiceError {
	return &ServiceError{Code: code, Msg: msg}
}

// BadRequest 参数校验类错误：body.code=400，**HTTP 仍为 200**。
func BadRequest(msg string) *ServiceError {
	return &ServiceError{Code: CodeBadRequest, Msg: msg}
}

// NotLogin 认证失败（对齐 NotLoginException）：body.code=401，HTTP 200。
func NotLogin() *ServiceError {
	return &ServiceError{Code: CodeUnauthorized, Msg: MsgNotLogin}
}

// NotLoginMsg 认证失败并指定文案（对齐 NotLoginException("自定义消息")）。
//
// get_current_user 的每条失败分支都有不同的提示（"认证已失效" / "非法凭证" /
// "客户端ID与Token不匹配" / "用户已被停用"），且这些文案会**直接展示给用户**，
// 属契约的一部分，故不能用 NotLogin() 的默认文案兜底。
func NotLoginMsg(msg string) *ServiceError {
	return &ServiceError{Code: CodeUnauthorized, Msg: msg}
}

// NotPermission 权限码校验失败（对齐 NotPermissionException）：body.code=403，HTTP 200。
func NotPermission() *ServiceError {
	return &ServiceError{Code: CodeForbidden, Msg: MsgNotPermission}
}

// NotRole 角色权限校验失败（对齐 NotRoleException）：body.code=403，HTTP 200。
func NotRole() *ServiceError {
	return &ServiceError{Code: CodeForbidden, Msg: MsgNotPermission}
}

// Conflict 数据冲突（唯一键 / 外键），body.code=409，HTTP 200。
func Conflict(msg string) *ServiceError {
	return &ServiceError{Code: CodeConflict, Msg: msg}
}

// FromAuthCode 从认证错误码枚举构造（对齐 ServiceException.of(AuthErrorCode.XXX)）。
func FromAuthCode(ec AuthErrorCode, args ...any) *ServiceError {
	return &ServiceError{
		Code:       ec.Code,
		Msg:        ec.Fallback,
		MessageKey: ec.Key,
		KeyArgs:    args,
	}
}

// HTTPError 构造框架级 HTTP 语义错误：**真实 HTTP 状态码** + 信封体。
// 仅用于路由 404/405 与健康探针 503 等场景，业务错误不要用这个。
func HTTPError(status int, msg string) *ServiceError {
	return &ServiceError{Code: status, Msg: msg, HTTPStatus: status}
}

// As 从 error 链中提取 *ServiceError。
// 非业务异常返回 nil，调用方应转为 500 兜底。
func As(err error) *ServiceError {
	var se *ServiceError
	if errors.As(err, &se) {
		return se
	}
	return nil
}

// Wrap 把任意 error 包装为可渲染的业务异常：
// 已是 ServiceError 则原样返回，否则转为 code=500 且不外泄原始错误内容
// （原始信息只进日志，由渲染层的调用方负责记录）。
func Wrap(err error, publicMsg string) *ServiceError {
	if err == nil {
		return nil
	}
	if se := As(err); se != nil {
		return se
	}
	return &ServiceError{Code: CodeError, Msg: publicMsg, Detail: err.Error()}
}
