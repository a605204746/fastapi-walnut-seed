package auth

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
	"walnut-seed-go/internal/pkg/middleware"
	"walnut-seed-go/internal/pkg/ratelimitx"
	"walnut-seed-go/internal/pkg/routex"
	"walnut-seed-go/internal/pkg/validatex"
)

// 限流规则，与 Python 端 controller 的 RateLimiter 依赖逐条对应：
//
//	POST /auth/login        time=60 count=10 limit_type=IP
//	GET  /auth/tenant/list  time=60 count=20 limit_type=IP
//	GET  /auth/code         time=60 count=30 limit_type=IP
//
// ⚠️ 登录限流是**每 IP 10 次/分钟**，不是每账号——它与 AuthService.checkLogin 的
// 「用户名+IP」失败锁定是两个独立机制：前者防暴力扫描，后者防单账号撞库。
var (
	ruleLogin  = ratelimitx.Rule{Window: time.Minute, Count: 10, Type: ratelimitx.LimitIP}
	ruleTenant = ratelimitx.Rule{Window: time.Minute, Count: 20, Type: ratelimitx.LimitIP}
	ruleCode   = ratelimitx.Rule{Window: time.Minute, Count: 30, Type: ratelimitx.LimitIP}
)

// Handler 认证端点，对应 Python 端 module_web/auth/controller.py。
type Handler struct {
	db      *gorm.DB
	cache   *cachex.Client
	cfg     *config.Config
	limiter *ratelimitx.Limiter
	errs    *errs.Handler
	repo    *Repo
}

// NewHandler 构造认证 handler。limiter 可为 nil（Redis 不可用时），
// 此时限流按 fail-closed 处理——见 limit() 的说明。
func NewHandler(db *gorm.DB, cache *cachex.Client, cfg *config.Config, limiter *ratelimitx.Limiter, eh *errs.Handler) *Handler {
	return &Handler{db: db, cache: cache, cfg: cfg, limiter: limiter, errs: eh, repo: NewRepo(db)}
}

// Mount 注册全部 5 条认证路由。
//
// 命名为 Mount 而非 Register：本包的 HTTP handler 已经有一个叫 Register
// （POST /auth/register 的用户注册），同名会冲突。
//
// ⚠️ 全部 5 条都在 WHITE_API_LIST_PATH 白名单内（无需认证）：
// login / code / tenant/list 是登录前置，logout 必须允许 token 失效时调用（幂等）。
// 故 bootstrap 侧要用 **Public** 分组注册，否则启动期路由认证审计会拒绝启动。
func (h *Handler) Mount(b routex.Binder) {
	b.POST("/login", h.Login)
	b.POST("/logout", h.Logout)
	b.POST("/register", h.Register)
	b.GET("/tenant/list", h.TenantList)
	b.GET("/code", h.Code)
}

// service 构造一次请求作用域的 Service（locale 随请求变化，不能缓存在 Handler 上）。
func (h *Handler) service(c *gin.Context) *Service {
	return NewService(h.db, h.cache, h.cfg, nil, middleware.LocaleFrom(c))
}

// meta 从请求中提取 IP / UA / Authorization。
func meta(c *gin.Context) RequestMeta {
	return RequestMeta{
		// ⚠️ 用 c.ClientIP() 而非直接读 X-Forwarded-For：
		// 它遵守 engine.SetTrustedProxies 的配置，非可信来源的伪造头会被忽略
		ClientIP:      c.ClientIP(),
		UserAgent:     c.GetHeader("User-Agent"),
		Authorization: c.GetHeader("Authorization"),
	}
}

// limit 执行限流；被拒时写入响应并返回 false。
//
// ⚠️ limiter 为 nil（Redis 不可用）时按 **fail-closed** 拒绝：
// 限流是安全控制，缓存挂了就放行等于把它形同虚设。
// 这与 Python 端 `except → raise ServiceException("服务器限流异常")` 的语义一致。
func (h *Handler) limit(c *gin.Context, rule ratelimitx.Rule) bool {
	if h.limiter == nil {
		h.errs.Abort(c, errs.New("服务器限流异常，请稍候再试"))
		return false
	}
	res := h.limiter.Allow(c.Request.Context(), rule, c.Request.URL.Path, c.ClientIP())
	if res.Err != nil {
		h.errs.Abort(c, errs.New(res.Message))
		return false
	}
	if !res.Allowed {
		msg := res.Message
		if msg == "" {
			// 与 Python 端一致：缺省文案走 i18n 的 rate.limiter.message
			msg = i18nx.MessageIn(middleware.LocaleFrom(c), "rate.limiter.message")
		}
		h.errs.Abort(c, errs.New(msg))
		return false
	}
	return true
}

// Login POST /auth/login
//
// 对应 Python 端 controller.login。前置校验顺序必须一致：
//  1. client 存在且 grant_type 被允许
//  2. client 状态正常
//  3. grant_type 必须是 password（当前只实现了密码策略）
//  4. 进入 AuthService.login_password
func (h *Handler) Login(c *gin.Context) {
	if !h.limit(c, ruleLogin) {
		return
	}

	var body PasswordLoginRequest
	if !h.bind(c, &body) {
		return
	}

	ctx := c.Request.Context()
	cli, err := h.repo.FindClientByClientID(ctx, body.ClientID)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}

	// ⚠️ 这里是**字符串子串匹配**，不是列表成员判断——照抄 Python 端
	// `body.grant_type not in (client.grant_type or "")` 的实际语义：
	// client.grant_type 存的是逗号分隔串（如 "password,social"），
	// Python 用 `in` 对字符串做的是子串包含判断。
	// 若"修正"成 split 后精确匹配，行为会与另两端不同
	// （例如 grant_type="pass" 在 Python 端能通过、在 Go 端不能）。
	grantTypes := ""
	if cli != nil {
		grantTypes = strDeref(cli.GrantType)
	}
	if cli == nil || !strings.Contains(grantTypes, body.GrantType) {
		envelope.Fail(c, h.i18n(c, "auth.grant.type.error"))
		return
	}
	if strDeref(cli.Status) != "0" {
		envelope.Fail(c, h.i18n(c, "auth.grant.type.blocked"))
		return
	}
	// 目前仅实现密码认证策略（sms/email/social 按需扩展）
	if body.GrantType != "password" {
		envelope.Fail(c, "授权类型不正确!")
		return
	}

	resp, err := h.service(c).LoginPassword(ctx, &body, meta(c), cli)
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, resp)
}

// Logout POST /auth/logout
func (h *Handler) Logout(c *gin.Context) {
	if err := h.service(c).Logout(c.Request.Context(), meta(c)); err != nil {
		h.errs.Abort(c, err)
		return
	}
	// 文案与 Python 端 SuccessResponse(msg="退出成功") 一致
	envelope.OKMsg(c, nil, "退出成功")
}

// Register POST /auth/register
func (h *Handler) Register(c *gin.Context) {
	var body RegisterRequest
	if !h.bind(c, &body) {
		return
	}

	// 注册开关：读参数配置 sys.account.registerUser，值为 "true" 才放行
	enabled, err := h.registerEnabled(c)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgDBFailed))
		return
	}
	if !enabled {
		envelope.Fail(c, "当前系统没有开启注册功能！")
		return
	}

	if err := h.service(c).Register(c.Request.Context(), &body, meta(c)); err != nil {
		h.errs.Abort(c, err)
		return
	}
	// Python 端是 SuccessResponse()，即 msg="操作成功" data=null
	envelope.OK(c, nil)
}

// TenantList GET /auth/tenant/list
func (h *Handler) TenantList(c *gin.Context) {
	if !h.limit(c, ruleTenant) {
		return
	}
	// 未启用多租户，固定返回关闭
	envelope.OK(c, TenantListResponse{TenantEnabled: false})
}

// Code GET /auth/code
func (h *Handler) Code(c *gin.Context) {
	if !h.limit(c, ruleCode) {
		return
	}
	resp, err := h.service(c).GetCaptcha(c.Request.Context())
	if err != nil {
		h.errs.Abort(c, err)
		return
	}
	envelope.OK(c, resp)
}

// registerEnabled 读取注册开关。
//
// 配置项不存在时返回 false（**默认关闭注册**）——与 Python 端
// `str(value).lower() == "true" if value is not None else False` 一致。
// 默认关闭是安全取向：配置丢失时不应悄悄打开注册入口。
func (h *Handler) registerEnabled(c *gin.Context) (bool, error) {
	value, _, err := h.repo.GetConfigValue(c.Request.Context(), "sys.account.registerUser")
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

// bind 绑定并校验请求体，失败时按契约返回 code=400。
//
// ⚠️ 参数校验错误是 **HTTP 200 + body.code=400**（业务错误形态），
// 不是 HTTP 400。见实施计划 §1 的契约说明。
//
// 错误文案经 validatex 翻译成与 Pydantic v2 逐字一致的消息——
// 前端会直接把 msg 展示给用户，这是契约的一部分，不能自行改写。
func (h *Handler) bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		h.errs.Abort(c, errs.BadRequest(validatex.Format(err)))
		return false
	}
	return true
}

// i18n 按当前请求语言解析文案。
func (h *Handler) i18n(c *gin.Context, key string, args ...any) string {
	return i18nx.MessageIn(middleware.LocaleFrom(c), key, args...)
}
