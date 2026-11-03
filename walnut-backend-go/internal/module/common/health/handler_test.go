package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/envelope"
)

func init() { gin.SetMode(gin.TestMode) }

// newTestEngine 构造只含健康探针的引擎，复刻 bootstrap/router.go 的挂载方式。
func newTestEngine(t *testing.T, checkers map[string]DependencyChecker) *gin.Engine {
	t.Helper()
	svc := New("1.0.0", 0) // 0 → 使用 defaultProbeTimeout
	for name, fn := range checkers {
		svc.Register(name, fn)
	}
	e := gin.New()
	g := e.Group("/common/health")
	g.GET("/check", svc.Check)
	g.GET("/live", svc.Live)
	g.GET("/ready", svc.Ready)
	return e
}

func get(t *testing.T, e *gin.Engine, path string) (int, map[string]any, string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	e.ServeHTTP(w, req)

	raw := w.Body.String()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("%s 响应不是合法 JSON: %v\n原始响应: %s", path, err, raw)
	}
	return w.Code, body, raw
}

func okChecker() DependencyChecker {
	return func(context.Context) error { return nil }
}

func failChecker() DependencyChecker {
	return func(context.Context) error { return errors.New("连接被拒绝") }
}

// TestCheckShape 验证 /check 的信封与载荷形状。
func TestCheckShape(t *testing.T) {
	e := newTestEngine(t, nil)
	code, body, raw := get(t, e, "/common/health/check")

	if code != http.StatusOK {
		t.Errorf("HTTP 状态码 = %d，期望 200", code)
	}
	if body["code"] != float64(200) {
		t.Errorf("body.code = %v，期望 200", body["code"])
	}
	if body["msg"] != "系统健康" {
		t.Errorf("body.msg = %v，期望「系统健康」（须与 Python 端逐字一致）", body["msg"])
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data 不是对象: %s", raw)
	}
	// 字段名与取值
	if data["status"] != float64(1) {
		t.Errorf("data.status = %v，期望 1", data["status"])
	}
	if data["version"] != "1.0.0" {
		t.Errorf("data.version = %v，期望 1.0.0", data["version"])
	}
	if _, exists := data["uptime_seconds"]; !exists {
		t.Error("缺少 uptime_seconds 字段（注意是 snake_case，不是 uptimeSeconds）")
	}
	if _, exists := data["timestamp"]; !exists {
		t.Error("缺少 timestamp 字段")
	}
	// 不应出现 check/live 载荷里没有的字段
	for _, forbidden := range []string{"dependencies", "disk_usage", "uptimeSeconds"} {
		if _, exists := data[forbidden]; exists {
			t.Errorf("/check 不应包含 %s 字段", forbidden)
		}
	}
}

// TestTimestampFormat 验证 timestamp 用 isoformat 形态，
// 而**不是**全站通用的 yyyy-MM-dd HH:mm:ss。
//
// Python 端这里直接调 datetime.now().isoformat()，未经过 DATETIME_DISPLAY_FMT。
func TestTimestampFormat(t *testing.T) {
	e := newTestEngine(t, nil)
	_, body, raw := get(t, e, "/common/health/check")

	data := body["data"].(map[string]any)
	ts, _ := data["timestamp"].(string)

	// ISO 8601：2026-11-02T11:53:34 或 2026-11-02T11:53:34.691234
	isoPattern := `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{6})?$`
	matched, err := regexp.MatchString(isoPattern, ts)
	if err != nil {
		t.Fatalf("正则编译失败: %v", err)
	}
	if !matched {
		t.Errorf("timestamp = %q，不符合 isoformat 形态 %s\n原始响应: %s", ts, isoPattern, raw)
	}
	// 反向确认：不能是展示格式（含空格分隔）
	spacePattern := `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`
	if matched, _ := regexp.MatchString(spacePattern, ts); matched {
		t.Errorf("timestamp = %q 误用了展示格式，健康探针应用 isoformat", ts)
	}
}

func TestLiveShape(t *testing.T) {
	e := newTestEngine(t, nil)
	code, body, _ := get(t, e, "/common/health/live")
	if code != http.StatusOK {
		t.Errorf("HTTP 状态码 = %d，期望 200", code)
	}
	if body["msg"] != "进程存活" {
		t.Errorf("body.msg = %v，期望「进程存活」", body["msg"])
	}
}

// TestReadyAllOK 验证就绪探针在依赖全部正常时的形状。
func TestReadyAllOK(t *testing.T) {
	e := newTestEngine(t, map[string]DependencyChecker{
		"database": okChecker(),
		"redis":    okChecker(),
	})
	code, body, raw := get(t, e, "/common/health/ready")

	if code != http.StatusOK {
		t.Errorf("依赖就绪时 HTTP 状态码 = %d，期望 200\n%s", code, raw)
	}
	if body["msg"] != "依赖就绪" {
		t.Errorf("body.msg = %v，期望「依赖就绪」", body["msg"])
	}

	data := body["data"].(map[string]any)
	if data["status"] != float64(1) {
		t.Errorf("data.status = %v，期望 1", data["status"])
	}

	deps, ok := data["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("data.dependencies 不是对象: %s", raw)
	}
	for _, name := range []string{"database", "redis"} {
		d, ok := deps[name].(map[string]any)
		if !ok {
			t.Fatalf("dependencies.%s 缺失或不是对象: %s", name, raw)
		}
		if d["status"] != float64(1) {
			t.Errorf("dependencies.%s.status = %v，期望 1", name, d["status"])
		}
		// latency_ms 必须存在且为数字（不是 null）
		lm, exists := d["latency_ms"]
		if !exists {
			t.Errorf("dependencies.%s 缺少 latency_ms 字段（注意 snake_case）", name)
			continue
		}
		if _, isNum := lm.(float64); !isNum {
			t.Errorf("dependencies.%s.latency_ms = %#v，期望数字", name, lm)
		}
	}

	if _, exists := data["disk_usage"]; !exists {
		t.Error("缺少 disk_usage 字段")
	}
}

// TestReadyFailureReturns503 验证就绪门禁语义：
// 任一依赖不可用 → **HTTP 503** + body.code=503，且载荷仍完整给出各依赖状态。
//
// 这是 compose 编排门禁的基础：后端带着未初始化依赖时不能被标记为 healthy，
// 否则前端容器会立即放流量进来。
func TestReadyFailureReturns503(t *testing.T) {
	cases := []struct {
		name     string
		checkers map[string]DependencyChecker
	}{
		{"数据库不可用", map[string]DependencyChecker{"database": failChecker(), "redis": okChecker()}},
		{"Redis 不可用", map[string]DependencyChecker{"database": okChecker(), "redis": failChecker()}},
		{"两者都不可用", map[string]DependencyChecker{"database": failChecker(), "redis": failChecker()}},
		{"未注册任何探测器", map[string]DependencyChecker{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine(t, tc.checkers)
			code, body, raw := get(t, e, "/common/health/ready")

			if code != http.StatusServiceUnavailable {
				t.Errorf("HTTP 状态码 = %d，期望 503\n%s", code, raw)
			}
			if body["code"] != float64(503) {
				t.Errorf("body.code = %v，期望 503", body["code"])
			}
			if body["msg"] != "依赖未就绪" {
				t.Errorf("body.msg = %v，期望「依赖未就绪」", body["msg"])
			}

			data := body["data"].(map[string]any)
			if data["status"] != float64(0) {
				t.Errorf("data.status = %v，期望 0", data["status"])
			}
		})
	}
}

// TestFailedDependencyLatencyIsNull 验证失败依赖的 latency_ms 是 null 而非 0。
//
// 对应 Python 端 DependencyStatus.latency_ms: float | None = None ——
// 必须用指针才能输出 null；若用 float64 零值会输出 0，与 Python 不一致。
func TestFailedDependencyLatencyIsNull(t *testing.T) {
	e := newTestEngine(t, map[string]DependencyChecker{
		"database": failChecker(),
		"redis":    okChecker(),
	})
	_, body, raw := get(t, e, "/common/health/ready")

	deps := body["data"].(map[string]any)["dependencies"].(map[string]any)
	db := deps["database"].(map[string]any)

	lm, exists := db["latency_ms"]
	if !exists {
		t.Fatalf("缺少 latency_ms 字段: %s", raw)
	}
	if lm != nil {
		t.Errorf("失败依赖的 latency_ms = %#v，期望 null（Python 端是 float | None = None）", lm)
	}
	if db["status"] != float64(0) {
		t.Errorf("失败依赖的 status = %v，期望 0", db["status"])
	}
}

// TestReadyPayloadIsCompleteOnFailure 验证失败时载荷仍然完整，
// 便于排障时看出「是哪个依赖挂了」。
func TestReadyPayloadIsCompleteOnFailure(t *testing.T) {
	e := newTestEngine(t, map[string]DependencyChecker{
		"database": failChecker(),
		"redis":    okChecker(),
	})
	_, body, raw := get(t, e, "/common/health/ready")

	data := body["data"].(map[string]any)
	for _, field := range []string{"status", "timestamp", "version", "uptime_seconds", "dependencies", "disk_usage"} {
		if _, exists := data[field]; !exists {
			t.Errorf("失败响应缺少 %s 字段\n%s", field, raw)
		}
	}
	deps := data["dependencies"].(map[string]any)
	if len(deps) != 2 {
		t.Errorf("dependencies 应包含全部 2 个依赖，实际 %d 个: %s", len(deps), raw)
	}
}

// TestNoTrailingNewline 验证响应体末尾没有换行。
// 契约 diff 是逐字节比对，json.Encoder.Encode 追加的 '\n' 会造成假差异。
func TestNoTrailingNewline(t *testing.T) {
	for _, path := range []string{"/common/health/check", "/common/health/live"} {
		e := newTestEngine(t, nil)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		raw := w.Body.String()
		if len(raw) > 0 && raw[len(raw)-1] == '\n' {
			t.Errorf("%s 响应体末尾有换行符，Python 端 JSONResponse 没有", path)
		}
	}
}

// TestEnvelopeIsUsed 确保本模块走的是统一信封渲染器而非 gin 默认的 c.JSON。
func TestEnvelopeIsUsed(t *testing.T) {
	e := newTestEngine(t, nil)
	_, _, raw := get(t, e, "/common/health/check")

	// 信封的三个顶层键必须齐全且顺序为 code/msg/data
	re := regexp.MustCompile(`^\{"code":\d+,"msg":"[^"]*","data":\{`)
	if !re.MatchString(raw) {
		t.Errorf("响应不符合信封形状 {\"code\",\"msg\",\"data\"}: %s", raw)
	}
}

// 编译期确认 envelope 常量与 Python 端 HttpStatus 对齐。
func TestEnvelopeConstants(t *testing.T) {
	if envelope.CodeSuccess != 200 || envelope.CodeFail != 500 || envelope.CodeWarn != 601 {
		t.Errorf("信封业务码与契约不符: success=%d fail=%d warn=%d",
			envelope.CodeSuccess, envelope.CodeFail, envelope.CodeWarn)
	}
}

func TestRoundTo(t *testing.T) {
	cases := []struct {
		in     float64
		digits int
		want   float64
	}{
		{1.234567, 2, 1.23},
		{1.235, 2, 1.24}, // 非精确 .5，正常进位
		{1.005, 2, 1.0},  // 浮点表示下 1.005 实际略小于，与 Python round 同为 1.0
		{99.99, 1, 100.0},
		{-1.0, 1, -1.0},
		{0, 2, 0},
	}
	for _, tc := range cases {
		if got := roundTo(tc.in, tc.digits); got != tc.want {
			t.Errorf("roundTo(%v, %d) = %v，期望 %v", tc.in, tc.digits, got, tc.want)
		}
	}
}
