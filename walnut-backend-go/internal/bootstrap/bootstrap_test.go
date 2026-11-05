package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
)

func init() { gin.SetMode(gin.TestMode) }

// newTestApp 构造一个不依赖真实 DB/Redis 的 App。
//
// 连接指向 127.0.0.1 的未占用端口：连接会被立即拒绝（而非等超时），
// 因此测试很快；同时这正好验证了「DB/Redis 不可用时应用仍能启动」的降级语义。
func newTestApp(t *testing.T) *App {
	t.Helper()
	logDir := t.TempDir()

	t.Setenv("ENVIRONMENT", "dev")
	t.Setenv("LOG_DIR", logDir)
	// 关掉 gin 的调试输出：dev 默认 DEBUG=true 会打开 DebugMode，
	// 每条路由注册都打印一行，测试输出会被淹没
	t.Setenv("DEBUG", "false")
	t.Setenv("LOGGER_LEVEL", "ERROR")
	t.Setenv("DATABASE_HOST", "127.0.0.1")
	t.Setenv("DATABASE_PORT", "1") // 未占用端口，连接立即被拒
	t.Setenv("REDIS_HOST", "127.0.0.1")
	t.Setenv("REDIS_PORT", "1")
	// 清掉可能从真实 shell 继承的配置，保证测试可重复
	t.Setenv("SNOWFLAKE_WORKER_ID", "")

	cfg, err := config.Load("dev")
	if err != nil {
		t.Fatalf("配置装载失败: %v", err)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatalf("App 装配失败: %v", err)
	}
	// 必须关闭：lumberjack 持有日志文件句柄，不关会导致 t.TempDir 清理失败
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// TestAppAssembles 验证装配成功且关键路由都已注册。
func TestAppAssembles(t *testing.T) {
	app := newTestApp(t)
	if app.Engine() == nil {
		t.Fatal("gin 引擎未初始化")
	}

	routes := app.Engine().Routes()
	want := map[string]bool{
		"GET /":                    false,
		"GET /common/health/check": false,
		"GET /common/health/live":  false,
		"GET /common/health/ready": false,
	}
	for _, r := range routes {
		key := r.Method + " " + r.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("缺少路由 %s（已注册: %v）", key, routeKeys(routes))
		}
	}
}

func routeKeys(routes []gin.RouteInfo) []string {
	out := make([]string, 0, len(routes))
	for _, r := range routes {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

// TestAppStartsWithoutDependencies 验证降级语义：
// DB 与 Redis 都连不上时，应用仍必须启动成功（由 /ready 返回 503 反映状态），
// 而不是让容器反复重启。对齐 Python 端 lifespan 的容错策略。
func TestAppStartsWithoutDependencies(t *testing.T) {
	app := newTestApp(t)
	if app.db != nil {
		t.Error("测试环境下数据库不应连接成功")
	}

	// /check 与 /live 不依赖任何外部服务，必须仍然 200
	for _, path := range []string{"/common/health/check", "/common/health/live"} {
		w := do(t, app, http.MethodGet, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s 在无依赖时应返回 200，实际 %d（body=%s）", path, w.Code, w.Body.String())
		}
	}

	// /ready 必须返回 503，把真实状态暴露给编排门禁
	w := do(t, app, http.MethodGet, "/common/health/ready")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("/ready 在依赖不可用时应返回 503，实际 %d（body=%s）", w.Code, w.Body.String())
	}
}

// TestRouteAuditPasses 验证 P0 注册的全部路由都命中白名单，审计通过。
func TestRouteAuditPasses(t *testing.T) {
	app := newTestApp(t)

	recorded := app.builder.Routes()
	if len(recorded) == 0 {
		t.Fatal("路由登记表为空——Builder 未记录任何路由，审计会静默失效")
	}
	for _, r := range recorded {
		if r.Authed {
			continue
		}
		if !app.cfg.IsWhiteListed(r.Path) {
			t.Errorf("路由 %s %s 登记为公开但不在白名单内", r.Method, r.Path)
		}
	}

	// 审计本身应当通过
	if err := app.AuditRoutes(); err != nil {
		t.Errorf("路由认证审计失败: %v", err)
	}
}

// TestAuditCatchesUnregisteredRoute 验证审计能抓到绕过 Builder 注册的路由。
//
// 这是比 Python 端更强的一层：日后新增模块若忘记走 Builder，
// 审计必须报错，否则「白名单外路由必须带认证」的约束会静默失效。
func TestAuditCatchesUnregisteredRoute(t *testing.T) {
	app := newTestApp(t)

	// 故意绕过 Builder 直接往引擎上挂一个非白名单路由
	app.Engine().GET("/system/secret", func(c *gin.Context) { c.String(200, "leaked") })

	err := app.AuditRoutes()
	if err == nil {
		t.Fatal("审计应当失败：存在未经 Builder 登记且不在白名单的路由")
	}
	if !strings.Contains(err.Error(), "/system/secret") {
		t.Errorf("错误信息应指明违规路由，实际为: %v", err)
	}
}

// TestAuditCatchesNonWhitelistedPublicRoute 验证经 Builder 登记为公开、
// 但不在白名单的路由同样会被拒绝。
func TestAuditCatchesNonWhitelistedPublicRoute(t *testing.T) {
	app := newTestApp(t)

	app.builder.Public("/system").GET("/leaked", func(c *gin.Context) { c.String(200, "x") })

	err := app.AuditRoutes()
	if err == nil {
		t.Fatal("审计应当失败：/system/leaked 登记为公开但不在白名单")
	}
	if !strings.Contains(err.Error(), "/system/leaked") {
		t.Errorf("错误信息应指明违规路由，实际为: %v", err)
	}
}

// TestTrailingSlashRedirects307 验证 R16 的处理：尾斜杠差异用 **307** 重定向对齐 Starlette，
// 而不是 gin 内置的 301。
//
// 契约 diff 跑分器实测出过这个偏差：Python 端 GET /common/health/check/ 经 307 重定向
// 后返回 200，而关闭 RedirectTrailingSlash 却不补重定向时 Go 端直接 404。
//
// 为什么必须是 307 而不是 301：
//   - 307 保留请求方法与请求体，POST/PUT 重定向后语义不变
//   - 301 会被浏览器与中间代理**永久缓存**，日后调整路径时旧重定向长期滞留且极难排查
func TestTrailingSlashRedirects307(t *testing.T) {
	app := newTestApp(t)

	cases := []struct {
		method   string
		path     string
		wantLoc  string
		wantCode int
	}{
		{http.MethodGet, "/common/health/check/", "/common/health/check", http.StatusTemporaryRedirect},
		{http.MethodGet, "/common/health/ready/", "/common/health/ready", http.StatusTemporaryRedirect},
		{http.MethodPost, "/common/health/check/", "/common/health/check", http.StatusTemporaryRedirect},
	}
	for _, tc := range cases {
		w := do(t, app, tc.method, tc.path)
		if w.Code != tc.wantCode {
			t.Errorf("%s %s -> HTTP %d，期望 %d（301 会被浏览器永久缓存，必须用 307）",
				tc.method, tc.path, w.Code, tc.wantCode)
			continue
		}
		if loc := w.Header().Get("Location"); loc != tc.wantLoc {
			t.Errorf("%s %s -> Location=%q，期望 %q", tc.method, tc.path, loc, tc.wantLoc)
		}
	}

	// 查询串必须在重定向中保留，否则分页/筛选参数会丢
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/common/health/check/?pageNum=1&pageSize=10", nil)
	app.Engine().ServeHTTP(w, req)
	if loc := w.Header().Get("Location"); loc != "/common/health/check?pageNum=1&pageSize=10" {
		t.Errorf("重定向丢失了查询串：Location=%q", loc)
	}
}

// TestTrailingSlashStillRedirectsAfterFollowing 验证跟随重定向后能拿到与无斜杠一致的响应。
// 这是契约 diff 跑分器实际观察到的端到端行为（Python 端正是如此）。
func TestTrailingSlashStillRedirectsAfterFollowing(t *testing.T) {
	app := newTestApp(t)

	direct := do(t, app, http.MethodGet, "/common/health/check")
	if direct.Code != http.StatusOK {
		t.Fatalf("无尾斜杠请求应直达 200，实际 %d", direct.Code)
	}

	// 手动跟随一次重定向
	w := do(t, app, http.MethodGet, "/common/health/check/")
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("尾斜杠请求应返回 307，实际 %d", w.Code)
	}
	followed := do(t, app, http.MethodGet, w.Header().Get("Location"))
	if followed.Code != http.StatusOK {
		t.Errorf("跟随重定向后应得到 200，实际 %d", followed.Code)
	}
	if followed.Body.String() != direct.Body.String() {
		// uptime_seconds / timestamp 会随时间变化，只比对信封结构
		var a, b map[string]any
		if err := json.Unmarshal(followed.Body.Bytes(), &a); err != nil {
			t.Fatalf("响应不是合法 JSON: %v", err)
		}
		if err := json.Unmarshal(direct.Body.Bytes(), &b); err != nil {
			t.Fatalf("响应不是合法 JSON: %v", err)
		}
		if a["code"] != b["code"] || a["msg"] != b["msg"] {
			t.Errorf("跟随重定向后的信封与直达不一致: %v vs %v", a, b)
		}
	}
}

// TestUnknownPathIs404NotRedirect 验证真正不存在的路径干净地 404，
// 不会因为尾斜杠逻辑被误重定向。
func TestUnknownPathIs404NotRedirect(t *testing.T) {
	app := newTestApp(t)

	for _, path := range []string{"/nonexistent", "/nonexistent/", "/common/health/nope/"} {
		w := do(t, app, http.MethodGet, path)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s -> HTTP %d，期望 404", path, w.Code)
			continue
		}
		assertEnvelope(t, w.Body.String(), 404, "请求地址不存在")
	}
}

// TestNoRouteEnvelope 验证未匹配路由返回真实 404 + 信封体。
func TestNoRouteEnvelope(t *testing.T) {
	app := newTestApp(t)
	w := do(t, app, http.MethodGet, "/nonexistent/path")

	if w.Code != http.StatusNotFound {
		t.Errorf("HTTP 状态码 = %d，期望 404", w.Code)
	}
	assertEnvelope(t, w.Body.String(), 404, "请求地址不存在")
}

// TestNoMethodEnvelope 验证方法不匹配返回真实 405 + 信封体。
//
// ⚠️ 必须显式开启 engine.HandleMethodNotAllowed，否则 gin 默认返回 404，
// 与契约要求的 405 不符。
func TestNoMethodEnvelope(t *testing.T) {
	app := newTestApp(t)
	w := do(t, app, http.MethodPost, "/common/health/check")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("HTTP 状态码 = %d，期望 405（若为 404 说明 HandleMethodNotAllowed 未开启）", w.Code)
	}
	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是合法 JSON: %v (%s)", err, w.Body.String())
	}
	if env["code"] != float64(405) {
		t.Errorf("body.code = %v，期望 405", env["code"])
	}
}

// TestIndexRouteReturnsJSONString 验证根路径返回 JSON 字符串而非信封。
//
// Python 端 @web_router.get("/") 的返回类型注解是 str，
// FastAPI 用 JSONResponse 序列化 → body 是带引号的 JSON 字符串。
func TestIndexRouteReturnsJSONString(t *testing.T) {
	app := newTestApp(t)
	w := do(t, app, http.MethodGet, "/")

	if w.Code != http.StatusOK {
		t.Errorf("HTTP 状态码 = %d，期望 200", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, `"`) || !strings.HasSuffix(body, `"`) {
		t.Errorf("根路径应返回 JSON 字符串（带引号），实际为: %s", body)
	}
	var s string
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("不是合法的 JSON 字符串: %v (%s)", err, body)
	}
	if !strings.Contains(s, "WalnutSeed") {
		t.Errorf("欢迎文案不符: %s", s)
	}
}

// TestPanicRecoveryReturnsEnvelope 验证 panic 被兜住并渲染成信封，
// 而不是让客户端收到裸 500 空响应（那会被前端拦截器当成网络错误）。
func TestPanicRecoveryReturnsEnvelope(t *testing.T) {
	app := newTestApp(t)
	app.Engine().GET("/boom", func(c *gin.Context) { panic("测试用 panic") })

	w := do(t, app, http.MethodGet, "/boom")
	if w.Code != http.StatusOK {
		t.Errorf("panic 应按业务失败处理（HTTP 200 + body.code=500），实际 HTTP %d", w.Code)
	}
	assertEnvelope(t, w.Body.String(), 500, "发生系统异常，请联系管理员")
}

// TestTrustedProxiesConfigured 验证 R6：必须显式设置可信代理，
// 不能让 gin 沿用「信任所有代理」的默认值（否则 X-Forwarded-For 可被伪造，
// 限流与审计日志的 IP 全部失真）。
func TestTrustedProxiesConfigured(t *testing.T) {
	app := newTestApp(t)

	// 默认 TRUSTED_PROXY_IPS 为空 → SetTrustedProxies(nil) → 不信任任何代理
	var observed string
	app.Engine().GET("/ip-probe", func(c *gin.Context) {
		observed = c.ClientIP()
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ip-probe", nil)
	// 伪造转发头：来源不可信时必须被完全忽略
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	req.Header.Set("X-Real-IP", "8.8.4.4")
	app.Engine().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("探测路由未命中，HTTP %d", w.Code)
	}
	if observed == "8.8.8.8" || observed == "8.8.4.4" {
		t.Errorf("伪造的转发头被采信（ClientIP=%s）——TRUSTED_PROXY_IPS 为空时"+
			"不应解析任何 X-Forwarded-For / X-Real-IP（见计划 R6）", observed)
	}
	if observed == "" {
		t.Error("ClientIP 解析结果为空")
	}
	// httptest 的默认来源地址是 192.0.2.1:1234
	if !strings.HasPrefix(observed, "192.0.2.1") {
		t.Errorf("ClientIP = %s，期望回落到直连地址 192.0.2.1", observed)
	}
}

// TestTrustedProxiesHonoredWhenConfigured 验证配置了可信代理后转发头才被采信。
func TestTrustedProxiesHonoredWhenConfigured(t *testing.T) {
	t.Setenv("ENVIRONMENT", "dev")
	t.Setenv("LOG_DIR", t.TempDir())
	t.Setenv("DEBUG", "false")
	t.Setenv("LOGGER_LEVEL", "ERROR")
	t.Setenv("DATABASE_PORT", "1")
	t.Setenv("REDIS_PORT", "1")
	t.Setenv("SNOWFLAKE_WORKER_ID", "")
	// httptest 的直连来源是 192.0.2.1，把它列为可信代理
	t.Setenv("TRUSTED_PROXY_IPS", "192.0.2.1")

	cfg, err := config.Load("dev")
	if err != nil {
		t.Fatalf("配置装载失败: %v", err)
	}
	if len(cfg.TrustedProxyIPs) != 1 || cfg.TrustedProxyIPs[0] != "192.0.2.1" {
		t.Fatalf("TRUSTED_PROXY_IPS 解析结果 = %v", cfg.TrustedProxyIPs)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatalf("App 装配失败: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	var observed string
	app.Engine().GET("/ip-probe2", func(c *gin.Context) {
		observed = c.ClientIP()
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ip-probe2", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	app.Engine().ServeHTTP(w, req)

	if observed != "203.0.113.7" {
		t.Errorf("可信代理已配置，ClientIP = %s，期望解析出 203.0.113.7", observed)
	}
}

// TestShutdownIsIdempotent 验证重复调用 Shutdown 不会二次关闭资源。
//
// 这是容器内实测抓到的真实缺陷：SIGTERM 时 cmd/run.go 与 app.Run 内部的 select
// 都监听了同一个 ctx.Done()，导致 Shutdown 被调用两次——停机日志打印两遍，
// 并对已关闭的 Redis client 二次 Close 报 "redis: client is closed"。
// 修法：Run 不再自行停机（只返回），Shutdown 用 sync.Once 保护，closeResources 关闭后置 nil。
func TestShutdownIsIdempotent(t *testing.T) {
	app := newTestApp(t)

	ctx := context.Background()
	if err := app.Shutdown(ctx); err != nil {
		t.Errorf("第一次 Shutdown 失败: %v", err)
	}
	// 第二次必须静默成功，不得 panic、不得重复关闭
	if err := app.Shutdown(ctx); err != nil {
		t.Errorf("第二次 Shutdown 失败（应幂等）: %v", err)
	}
	// Close 与 Shutdown 混用也必须安全
	if err := app.Close(); err != nil {
		t.Errorf("Shutdown 之后再 Close 失败: %v", err)
	}

	// 资源应已被释放（置 nil），二次释放不会触及已关闭的 client
	if app.db != nil {
		t.Error("Shutdown 后 db 应已置 nil")
	}
	if app.cache != nil {
		t.Error("Shutdown 后 cache 应已置 nil")
	}
}

// TestRunReturnsOnContextCancel 验证 Run 在 ctx 取消时返回而**不自行停机**。
//
// 职责划分：Run 只负责提供 HTTP 服务，收尾由调用方（cmd/run.go）掌握超时窗口后显式调用。
//
// ⚠️ 本用例会真实监听端口（Run 内部调 ListenAndServe），故用 39011 避开默认 8011，
// 防止与其它测试或本机正在运行的服务冲突。
func TestRunReturnsOnContextCancel(t *testing.T) {
	t.Setenv("SERVER_PORT", "39011")
	app := newTestApp(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	// 给 ListenAndServe 一点时间绑定端口；若绑定失败 Run 会立刻返回错误
	select {
	case err := <-done:
		t.Fatalf("Run 在 ctx 取消前就退出了（端口 39011 可能被占用）: %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("ctx 取消后 Run 应返回 nil，却返回 %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ctx 取消后 Run 未在 10s 内返回")
	}

	// Run 返回后资源应仍由调用方负责关闭（Run 不得自行停机）
	if app.cache == nil {
		t.Error("Run 不应自行调用 closeResources——停机是调用方的职责")
	}
	_ = app.Shutdown(context.Background())
}

// TestProdRejectsWeakSecret 验证 fail-fast：生产环境弱密钥必须拒绝启动。
func TestProdRejectsWeakSecret(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("LOG_DIR", t.TempDir())
	t.Setenv("DATABASE_PORT", "1")
	t.Setenv("REDIS_PORT", "1")
	t.Setenv("SECRET_KEY", "") // 空 → 弱

	cfg, err := config.Load("prod")
	if err != nil {
		t.Fatalf("配置装载失败: %v", err)
	}
	if _, err := New(cfg); err == nil {
		t.Fatal("prod 环境下弱 SECRET_KEY 必须拒绝启动，但 New() 成功了")
	} else if !strings.Contains(err.Error(), "SECRET_KEY") {
		t.Errorf("错误信息应指明 SECRET_KEY，实际为: %v", err)
	}
}

// ---- 辅助 ----

func do(t *testing.T, app *App, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	app.Engine().ServeHTTP(w, req)
	return w
}

// assertEnvelope 校验响应体是 {"code","msg","data"} 信封且字段值符合预期。
func assertEnvelope(t *testing.T, body string, wantCode int, wantMsg string) {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("响应不是合法 JSON: %v\n原始: %s", err, body)
	}
	// data 字段必须存在（即使为 null），否则前端解构会抛异常
	if _, exists := env["data"]; !exists {
		t.Errorf("信封缺少 data 字段: %s", body)
	}
	if env["code"] != float64(wantCode) {
		t.Errorf("body.code = %v，期望 %d", env["code"], wantCode)
	}
	if env["msg"] != wantMsg {
		t.Errorf("body.msg = %v，期望 %q", env["msg"], wantMsg)
	}
}
