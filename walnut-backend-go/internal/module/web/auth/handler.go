package auth

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/module/system/client"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
	"walnut-seed-go/internal/pkg/middleware"
	"walnut-seed-go/internal/pkg/ratelimitx"
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

// handlerRepo 是 handler 消费的数据访问能力（consumer-side 小接口，定义在消费方）。
// 只列 handler 实际直调的两个前置查询；由 providerRepo 适配跨域 provider 实现，
// handler 不再穿透到具体 *Repo（客户端/配置查询已归还 client/config 域）。
type handlerRepo interface {
	FindClientByClientID(ctx context.Context, clientID string) (*client.SysClient, error)
	GetConfigValue(ctx context.Context, key string) (string, bool, error)
}

// providerRepo 把 ClientProvider/ConfigProvider 适配成 handlerRepo，
// 仅做方法名映射，无任何业务逻辑——调用点语义与迁移前逐字一致。
type providerRepo struct {
	clients ClientProvider
	configs ConfigProvider
}

func (p providerRepo) FindClientByClientID(ctx context.Context, clientID string) (*client.SysClient, error) {
	return p.clients.FindByClientID(ctx, clientID)
}

func (p providerRepo) GetConfigValue(ctx context.Context, key string) (string, bool, error) {
	return p.configs.GetValue(ctx, key)
}

// Handler 认证端点，对应 Python 端 module_web/auth/controller.py。
type Handler struct {
	db      *gorm.DB
	cache   *cachex.Client
	cfg     *config.Config
	limiter *ratelimitx.Limiter
	errs    *errs.Handler
	repo    handlerRepo
	deps    Deps
}

// NewHandler 构造认证 handler。limiter 可为 nil（Redis 不可用时），
// 此时限流按 fail-closed 处理——见 limit() 的说明。
// deps 提供跨域只读能力：handler 直调的客户端/配置查询经 providerRepo 适配，
// 请求作用域 Service 经同一份 deps 注入用户/部门/角色/菜单 provider。
func NewHandler(db *gorm.DB, cache *cachex.Client, cfg *config.Config, limiter *ratelimitx.Limiter, eh *errs.Handler, deps Deps) *Handler {
	return &Handler{
		db:      db,
		cache:   cache,
		cfg:     cfg,
		limiter: limiter,
		errs:    eh,
		repo:    providerRepo{clients: deps.Clients, configs: deps.Configs},
		deps:    deps,
	}
}

// service 构造一次请求作用域的 Service（locale 随请求变化，不能缓存在 Handler 上）。
func (h *Handler) service(c *gin.Context) *Service {
	return NewService(h.db, h.cache, h.cfg, nil, middleware.LocaleFrom(c), h.deps)
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
	raw, _ := io.ReadAll(c.Request.Body)
	body, err := decodeLoginBody(c.Request.Context(), raw)
	if err != nil {
		h.errs.Abort(c, err)
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
	raw, _ := io.ReadAll(c.Request.Body)
	body, err := decodeRegisterBody(c.Request.Context(), raw)
	if err != nil {
		h.errs.Abort(c, err)
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
// ⚠️ **不 trim**：Python 只 lower() 后精确比较，" true" 不算开启（曾误加
// TrimSpace+EqualFold，契约比对抓出）。默认关闭是安全取向：配置丢失时不应悄悄打开注册入口。
func (h *Handler) registerEnabled(c *gin.Context) (bool, error) {
	value, _, err := h.repo.GetConfigValue(c.Request.Context(), "sys.account.registerUser")
	if err != nil {
		return false, err
	}
	return strings.ToLower(value) == "true", nil
}

// i18n 按当前请求语言解析文案。
func (h *Handler) i18n(c *gin.Context, key string, args ...any) string {
	return i18nx.MessageIn(middleware.LocaleFrom(c), key, args...)
}
