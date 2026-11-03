package bootstrap

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/module/common/health"
	sysconfig "walnut-seed-go/internal/module/system/config"
	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/module/system/dict"
	"walnut-seed-go/internal/module/system/menu"
	"walnut-seed-go/internal/module/system/notice"
	"walnut-seed-go/internal/module/system/post"
	"walnut-seed-go/internal/module/system/user"
	"walnut-seed-go/internal/module/web/auth"
	"walnut-seed-go/internal/pkg/idempotentx"
	"walnut-seed-go/internal/pkg/middleware"
	"walnut-seed-go/internal/pkg/permguard"
	"walnut-seed-go/internal/pkg/ratelimitx"
	"walnut-seed-go/internal/pkg/routex"
)

// RecordedRoute 记录一条路由的认证状态，供启动期审计。
//
// 为什么需要显式登记：FastAPI 能从端点签名里反射出 Depends(get_current_user)，
// Gin 没有等价元数据。故 Go 侧改为「注册时声明」——经 Protected 组注册的路由
// 记为已认证，经 Public 组注册的记为公开，审计时比对白名单。
type RecordedRoute struct {
	Method string
	Path   string
	Authed bool
}

// Builder 是路由的唯一组合根，对应 Python 端 app/api/v1/router.py。
//
// 全部路由的前缀、注册顺序、认证归属都集中在此维护；
// 各业务模块只提供 Routes(rg) 方法，不自行决定挂载位置。
type Builder struct {
	engine *gin.Engine
	cfg    *config.Config

	mu     sync.Mutex
	routes []RecordedRoute

	// authMiddleware 是受保护分组的认证链（P3 起由 auth.NewMiddleware 提供，
	// 对应 Python 端 Depends(get_current_user)）。
	//
	// 为 nil 时 Protected 组不施加任何中间件，但**仍会登记为「已认证」**——
	// 这是 P0 为了让审计逻辑从一开始就完整而刻意保留的行为。
	// ⚠️ 反过来说：审计通过**不等于**认证中间件真的挂上了，
	// 故 buildEngine 必须在 registerRoutes 之前把它注入。
	authMiddleware []gin.HandlerFunc
}

// Group 包装 gin.RouterGroup，在注册路由的同时登记认证状态。
type Group struct {
	rg     *gin.RouterGroup
	b      *Builder
	authed bool
}

// Public 返回公开路由分组（不挂认证中间件）。
// 经此注册的路由**必须**命中 WHITE_API_LIST_PATH，否则启动审计失败。
func (b *Builder) Public(prefix string) *Group {
	return &Group{rg: b.engine.Group(prefix), b: b, authed: false}
}

// Protected 返回受保护路由分组（挂认证中间件）。
func (b *Builder) Protected(prefix string) *Group {
	g := b.engine.Group(prefix)
	if len(b.authMiddleware) > 0 {
		g.Use(b.authMiddleware...)
	}
	return &Group{rg: g, b: b, authed: true}
}

// record 登记一条路由。fullPath 必须是与 gin 内部一致的绝对路径，
// 否则审计与契约 diff 拿到的路径会和真实路由对不上。
func (b *Builder) record(method, fullPath string, authed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.routes = append(b.routes, RecordedRoute{Method: method, Path: fullPath, Authed: authed})
}

// ---- 动词包装：每个都登记路由 ----

func (g *Group) GET(path string, h ...gin.HandlerFunc) *Group {
	g.rg.GET(path, h...)
	g.b.record(http.MethodGet, joinPath(g.rg.BasePath(), path), g.authed)
	return g
}

func (g *Group) POST(path string, h ...gin.HandlerFunc) *Group {
	g.rg.POST(path, h...)
	g.b.record(http.MethodPost, joinPath(g.rg.BasePath(), path), g.authed)
	return g
}

func (g *Group) PUT(path string, h ...gin.HandlerFunc) *Group {
	g.rg.PUT(path, h...)
	g.b.record(http.MethodPut, joinPath(g.rg.BasePath(), path), g.authed)
	return g
}

func (g *Group) DELETE(path string, h ...gin.HandlerFunc) *Group {
	g.rg.DELETE(path, h...)
	g.b.record(http.MethodDelete, joinPath(g.rg.BasePath(), path), g.authed)
	return g
}

func (g *Group) PATCH(path string, h ...gin.HandlerFunc) *Group {
	g.rg.PATCH(path, h...)
	g.b.record(http.MethodPatch, joinPath(g.rg.BasePath(), path), g.authed)
	return g
}

// Group 派生子分组，继承认证归属。
func (g *Group) Group(prefix string) *Group {
	return &Group{rg: g.rg.Group(prefix), b: g.b, authed: g.authed}
}

// Use 给分组追加中间件（如按模块的操作日志采集）。
func (g *Group) Use(mw ...gin.HandlerFunc) *Group {
	g.rg.Use(mw...)
	return g
}

// Binder 把分组适配成 routex.Binder，供业务模块的 Mount 方法注册端点。
//
// 闭包丢弃了链式返回值——业务模块只看到无返回值的注册函数，
// 既避免了它们反向依赖 bootstrap（导入环），也让 handler 能脱离 bootstrap 单测。
// 详见 routex.Binder 的注释。
func (g *Group) Binder() routex.Binder {
	return routex.Binder{
		GET:    func(path string, hs ...gin.HandlerFunc) { g.GET(path, hs...) },
		POST:   func(path string, hs ...gin.HandlerFunc) { g.POST(path, hs...) },
		PUT:    func(path string, hs ...gin.HandlerFunc) { g.PUT(path, hs...) },
		DELETE: func(path string, hs ...gin.HandlerFunc) { g.DELETE(path, hs...) },
	}
}

// joinPath 复刻 gin 内部的 combinePaths 语义。
//
// ⚠️ 不能简单写 BasePath()+path：根分组的 BasePath() 是 "/"，
// 拼上 "/" 会得到 "//"，导致登记路径与真实路由不一致，
// 审计会把合法的 "/" 判成「不在白名单」而拒绝启动。
func joinPath(base, sub string) string {
	if sub == "" {
		return base
	}
	if !strings.HasPrefix(sub, "/") {
		sub = "/" + sub
	}
	// 根分组特例：base 已经是 "/"，直接返回 sub（sub 本身以 / 开头）
	if base == "/" || base == "" {
		return sub
	}
	return strings.TrimSuffix(base, "/") + sub
}

// Routes 返回已登记的路由快照，供审计与契约 diff 使用。
func (b *Builder) Routes() []RecordedRoute {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]RecordedRoute, len(b.routes))
	copy(out, b.routes)
	return out
}

// HasPath 判断某路径是否已注册（**不限方法**）。
//
// 用于尾斜杠重定向：POST /common/health/check/ 的目标 /common/health/check
// 只注册了 GET，此时仍应重定向过去，由目标路由产生 405——这才是 Starlette 的行为。
// 若按「方法+路径」精确匹配，这里会直接 404，与 Python 端不一致。
//
// 404 不是热路径，线性扫描足够；换来的是登记表永远与真实路由同步，
// 不会因快照过期而在运行期新增路由后失灵。
func (b *Builder) HasPath(path string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, r := range b.routes {
		if r.Path == path {
			return true
		}
	}
	return false
}

// slashAwareNoRoute 复刻 Starlette 的 redirect_slashes 语义（见实施计划 R16）。
//
// 为什么不用 gin 内置的 RedirectTrailingSlash：它对 GET/HEAD 返回 **301**，
// 而 Starlette 对**所有方法**返回 **307**。301 会被浏览器与中间代理**永久缓存**，
// 日后调整路径时旧重定向会长期滞留且极难排查（契约 diff 已实测出这个偏差）。
//
// 故关闭内置重定向，改由此处精确对齐：路径加/去尾斜杠后若能命中已注册路由，
// 用 307 重定向（保留方法与请求体）；否则按 404 信封返回。
func (a *App) slashAwareNoRoute(b *Builder) gin.HandlerFunc {
	notFound := a.errHandler.NoRoute()

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		var alt string
		if strings.HasSuffix(path, "/") {
			alt = strings.TrimSuffix(path, "/")
			if alt == "" {
				alt = "/"
			}
		} else {
			alt = path + "/"
		}

		if b.HasPath(alt) {
			target := alt
			// 保留查询串，否则重定向后分页/筛选参数会丢
			if raw := c.Request.URL.RawQuery; raw != "" {
				target += "?" + raw
			}
			c.Redirect(http.StatusTemporaryRedirect, target)
			c.Abort()
			return
		}
		notFound(c)
	}
}

// buildEngine 装配 gin 引擎、中间件链与全部路由。
func (a *App) buildEngine() error {
	if a.cfg.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()

	// ⚠️ 安全：Gin 默认信任所有代理（0.0.0.0/0），任何客户端都能伪造
	// X-Forwarded-For，导致限流与审计日志按伪造 IP 计数。必须显式收敛到
	// TRUSTED_PROXY_IPS，语义对齐 Python 端的可信代理判断（见计划 R6）。
	if err := engine.SetTrustedProxies(a.cfg.TrustedProxyIPs); err != nil {
		return fmt.Errorf("设置可信代理失败: %w", err)
	}
	if len(a.cfg.TrustedProxyIPs) == 0 {
		a.log.Warn("⚠️ TRUSTED_PROXY_IPS 为空：将不解析任何 X-Forwarded-For，" +
			"反向代理部署时限流与审计日志会按代理地址计数")
	}

	// ⚠️ 契约：关闭尾斜杠自动重定向（见计划 R16）。
	// Gin 对 GET 缺尾斜杠返回 301（会被浏览器永久缓存），而 Starlette 返回 307。
	// 关闭后改为「需要的形式都显式注册」，两种 URL 均直达 200，未注册路径干净返回 404。
	engine.RedirectTrailingSlash = false
	// 必须开启：否则方法不匹配时 Gin 返回 404，而契约要求 405 + 信封体
	engine.HandleMethodNotAllowed = true
	// 由本项目的 errs.Handler 统一渲染 404/405 信封，不用 gin 默认的纯文本
	engine.RedirectFixedPath = false

	b := &Builder{
		engine: engine,
		cfg:    a.cfg,
		// ⚠️ 必须在 registerRoutes **之前**注入：Protected 组在创建时就把
		// 当时的 authMiddleware 快照进 gin 的 handler 链，之后再改字段不会生效。
		authMiddleware: []gin.HandlerFunc{
			auth.NewMiddleware(a.cfg, a.cache, a.log, a.errHandler),
		},
	}
	a.registerMiddleware(engine)
	a.registerRoutes(b)

	engine.NoRoute(a.slashAwareNoRoute(b))
	engine.NoMethod(a.errHandler.NoMethod())

	a.engine = engine
	a.builder = b
	return nil
}

// registerMiddleware 装配中间件链。
//
// 目标顺序（从外到内，与 Python 端 MIDDLEWARE_LIST 一致）：
//
//	HTTPSRedirect → TrustedHost → CORS → Locale → ApiDecrypt → XSS
//	  → RequestLog → GZip → CorrelationId → 业务路由
//
// ⚠️ Gin 的 Use() 调用顺序**就是**执行顺序，与 FastAPI 的「逆序叠加」相反，
// 移植时不要照抄 Python 的 [::-1]。
//
// 已落地：Recovery、CORS、Locale、ApiDecrypt、RequestLog、CorrelationId。
// 待补（P2）：HTTPSRedirect、TrustedHost、XSS、GZip。
//
// 顺序里两处有实质影响，不可调换：
//   - ApiDecrypt 必须在 RequestLog **之前**：否则请求日志记的是密文，无法排障
//   - ApiDecrypt 必须在 XSS **之前**：在密文上做 XSS 过滤等于没过滤
func (a *App) registerMiddleware(engine *gin.Engine) {
	// Recovery 必须在最外层，否则后续中间件的 panic 无人兜底，
	// 客户端会收到裸 500 空响应而非信封体
	engine.Use(a.errHandler.Recovery())

	// TODO(P2): HTTPSRedirect（仅 prod 且 HTTPS_REDIRECT=true）
	// TODO(P2): TrustedHost（仅 prod，校验 ALLOWED_HOSTS）

	engine.Use(middleware.CORS(middleware.CORSConfig{
		AllowOrigins:     a.cfg.AllowedHosts,
		AllowAllOrigins:  containsStar(a.cfg.AllowedHosts) || !a.cfg.Environment.IsProd(),
		AllowMethods:     a.cfg.AllowMethods,
		AllowHeaders:     a.cfg.AllowHeaders,
		ExposeHeaders:    a.cfg.CORSExposeHeaders,
		AllowCredentials: a.cfg.AllowCredentials,
		MaxAge:           a.cfg.CORSMaxAge,
	}))

	// 语言环境：Accept-Language 优先，Content-Language 兜底
	engine.Use(middleware.Locale())

	// 接口加解密。密钥无效时 config.ValidateSecurity 已把 APIDecryptEnabled 置 false
	// 并记了告警，故这里直接按开关决定是否注册——**绝不带着无效密钥注册中间件**，
	// 否则每个加密请求都会失败，且错误信息会指向错误的方向。
	if a.cfg.APIDecryptEnabled {
		engine.Use(middleware.ApiDecrypt(middleware.ApiDecryptConfig{
			HeaderFlag:    a.cfg.APIDecryptHeaderFlag,
			PrivateKeyB64: a.cfg.APIDecryptPrivateKey, // 解请求
			PublicKeyB64:  a.cfg.APIDecryptPublicKey,  // 加响应
			Logger:        a.log,
		}))
	} else {
		a.log.Warn("接口加解密未启用，请求/响应按明文处理",
			slog.String("reason", a.cfg.APIDecryptDisabledReason))
	}

	// TODO(P2): XSS —— 注意 XSS_EXCLUDE_URLS（/system/notice 富文本不能被转义）

	engine.Use(middleware.RequestLog(middleware.RequestLogConfig{
		Logger: a.log,
		// 健康探针跳过：compose 每 2-10s 探一次，记下来会淹没真实流量
		SkipPaths: []string{"/common/health"},
	}))

	// TODO(P2): GZip —— GZIP_MIN_SIZE / GZIP_COMPRESS_LEVEL

	// CorrelationId 在链的**最内层**（紧邻业务路由），与 Python 端一致。
	// 因此外层中间件记日志时可能还没有关联 ID——这是两端一致的既有行为，
	// 不要为了"让每条日志都有 request_id"而把它挪到外层，那会改变契约。
	engine.Use(middleware.CorrelationID())
}

// registerRoutes 注册全部业务路由。
//
// 对应 Python 端 router.py 的组合根职责。P0 只有 index 与健康探针；
// P2/P3 的 auth、system、monitor、resource、upload 路由在此按序追加。
func (a *App) registerRoutes(b *Builder) {
	// ---- 根路径：欢迎文案（白名单内，返回 JSON 字符串而非信封）----
	root := b.Public("")
	root.GET("/", func(c *gin.Context) {
		// Python 端 @web_router.get("/") 的返回类型注解是 str，
		// FastAPI 用 JSONResponse 序列化 → body 是带引号的 JSON 字符串。
		// 这里必须保持同样形状，不能改成信封。
		c.JSON(http.StatusOK, "欢迎使用 WalnutSeed 后台管理框架，请通过前端地址访问。")
	})

	// ---- /common/health/{check,live,ready} ----
	// 经 Group 包装注册（而非把 *gin.RouterGroup 直接交给 health 模块），
	// 否则这三条路由不会被登记，启动期认证审计就会漏掉它们。
	a.health = health.New(a.cfg.Version, time.Duration(a.cfg.HealthProbeTimeout)*time.Second)
	a.health.Register("database", a.checkDatabase)
	a.health.Register("redis", a.checkRedis)

	healthGroup := b.Public("/common").Group("/health")
	healthGroup.GET("/check", a.health.Check)
	healthGroup.GET("/live", a.health.Live)
	healthGroup.GET("/ready", a.health.Ready)

	// ---- /auth/*：login / logout / register / tenant/list / code ----
	// 全部在白名单内（登录前置 + 幂等登出），故用 Public 分组注册。
	// 限流器在 Redis 不可用时为 nil，handler 内部按 fail-closed 处理。
	a.mountAuthRoutes(b)

	// ---- /system/*：P3 窄切片（getInfo + getRouters）----
	// 这两条是前端登录后立刻调用的接口：没有它们，登录能拿到 token 却进不了主页。
	// 其余 111 条 /system 路由按交接文档 §4.2 的顺序逐个补齐。
	a.mountSystemRoutes(b)

	// TODO(P3): /system/*   —— profile role dept post dict config notice client social
	//                        以及 user/menu 的其余端点
	// TODO(P3): /monitor/*  —— operlog / logininfor
	// TODO(P4): /common/file/*、/upload/{key}、/resource/sse、/resource/websocket
}

// mountAuthRoutes 装配认证模块。
//
// 用 routex.Binder 的闭包适配把 bootstrap.Group 的链式方法转成无返回值函数，
// 避免 auth 包反向依赖 bootstrap 形成导入环（详见 routex.Binder 的注释）。
func (a *App) mountAuthRoutes(b *Builder) {
	var limiter *ratelimitx.Limiter
	if a.cache != nil {
		limiter = ratelimitx.New(a.cache.Client, a.cache.Prefix(), a.log)
	}

	h := auth.NewHandler(a.db, a.cache, a.cfg, limiter, a.errHandler)
	h.Mount(b.Public("/auth").Binder())
}

// mountSystemRoutes 装配 /system 下的受保护端点。
//
// ⚠️ 必须用 b.Protected：这些路径都不在 WHITE_API_LIST_PATH 内，
// 用 Public 注册会被启动期路由认证审计直接拒绝启动（fail-fast）。
//
// ⚠️ 认证中间件只保证「已登录」，**不保证有权限**。每个业务端点还必须挂
// permguard 守卫（对应 Python 端 Depends(AuthPermission(...))）。
// 启动期审计检查不到这一层，只能靠契约 diff 的 403 用例兜底。
func (a *App) mountSystemRoutes(b *Builder) {
	system := b.Protected("/system")

	// 权限守卫与防重提交是全部 system 模块共用的两个中间件工厂
	perm := permguard.New(a.errHandler)
	var idem *idempotentx.Guard
	if a.cache != nil {
		idem = idempotentx.New(a.cache, a.errHandler, middleware.LocaleFrom)
	}

	user.NewHandler(a.db, a.errHandler).Mount(system.Group("/user").Binder())
	menu.NewHandler(a.db, a.errHandler).Mount(system.Group("/menu").Binder())
	post.NewHandler(a.db, a.errHandler, perm, idem).Mount(system.Group("/post").Binder())
	dept.NewHandler(a.db, a.errHandler, perm, idem).Mount(system.Group("/dept").Binder())
	dict.NewHandler(a.db, a.errHandler, perm, a.cache).Mount(system.Group("/dict").Binder())
	sysconfig.NewHandler(a.db, a.errHandler, perm, a.cache).Mount(system.Group("/config").Binder())
	notice.NewHandler(a.db, a.errHandler, perm, idem).Mount(system.Group("/notice").Binder())
}

func containsStar(list []string) bool {
	for _, v := range list {
		if v == "*" {
			return true
		}
	}
	return false
}
