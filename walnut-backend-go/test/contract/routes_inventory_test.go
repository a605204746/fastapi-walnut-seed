// Package contract 承载 Go 与 Python 后端之间的契约一致性测试。
//
// 本文件实现计划 §4.1 的「路由清单 diff」：把 Go 的路由面与
// api/openapi/routes.txt（Python 端导出的 golden 契约）对比。
//
// 两个方向的语义**不对称**，这是刻意设计：
//
//	Go 有、Python 没有  →  硬失败。私自新增接口就是契约漂移，必须挡住。
//	Go 没有、Python 有  →  P0–P2 仅报告覆盖率；P3 起转为硬失败。
//	                       因为迁移是增量的，未完成的路由不该让 CI 一直红。
//
// 响应体级别的逐字节比对在 diff_contract.py（需两端同时运行），
// 本文件只比路由清单，纯静态、无需启动服务，因此可以进每次 CI。
package contract

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"walnut-seed-go/internal/bootstrap"
	"walnut-seed-go/internal/config"
)

// routesFile 是 golden 路由清单，由 scripts/export_contract.py 从 Python 端导出。
const routesFile = "../../api/openapi/routes.txt"

// exemptPythonRoutes 是 Python 有、但 Go **有意不实现**的路由。
//
// 列在这里的必须是有明确理由的缺口，不能为了「让测试变绿」随手加：
//   - /docs、/docs/oauth2-redirect、/redoc：Swagger UI / ReDoc 页面，
//     由 Python 的 register_docs() 提供。Go 端计划在 P5 用 swaggo 或托管
//     同一套静态 Swagger 资源补齐。
//
// ⚠️ 新增豁免项时必须同步更新 api/openapi/README.md 的说明。
var exemptPythonRoutes = map[string]string{
	"GET /docs":                 "P5 补齐 API 文档 UI",
	"GET /docs/oauth2-redirect": "P5 补齐 API 文档 UI",
	"GET /redoc":                "P5 补齐 API 文档 UI",
}

// ---- Python → Go 的路径归一化 ----

var (
	// {name:path} → *name（Gin 的 catch-all 通配符）
	rePathConverter = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*):path\}`)
	// {name:int} / {name:str} 等类型转换器 → :name
	// Gin 没有类型转换器，类型校验由 handler 的绑定层负责
	reTypedParam = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*):[A-Za-z_]+\}`)
	// {name} → :name
	rePlainParam = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// normalizePath 把 FastAPI 的路径模板转成 Gin 的等价形式。
//
// 三类差异（均来自 routes.txt 的实测内容）：
//
//	{user_id}          → :user_id
//	{user_id:int}      → :user_id       Python 用路径转换器限定类型，Gin 无此机制
//	{file_path:path}   → *file_path     Python 的 path 转换器允许含斜杠，对应 Gin 的 catch-all
func normalizePath(p string) string {
	p = rePathConverter.ReplaceAllString(p, "*$1")
	p = reTypedParam.ReplaceAllString(p, ":$1")
	p = rePlainParam.ReplaceAllString(p, ":$1")
	return p
}

// loadPythonRoutes 读取 golden 清单，返回 "METHOD path" 集合（已归一化为 Gin 形式）。
func loadPythonRoutes(t *testing.T) map[string]bool {
	t.Helper()

	f, err := os.Open(routesFile)
	if err != nil {
		t.Fatalf("无法打开 golden 路由清单 %s: %v\n"+
			"请先执行 make contract-export 从 Python 端导出", routesFile, err)
	}
	defer f.Close()

	out := make(map[string]bool)
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		method, path, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("%s 第 %d 行格式非法（应为 \"METHOD path\"）: %q", routesFile, lineNo, line)
		}
		out[strings.ToUpper(method)+" "+normalizePath(path)] = true
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("读取 %s 失败: %v", routesFile, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s 为空，golden 契约未正确导出", routesFile)
	}
	return out
}

// newTestApp 构造一个不依赖真实 DB/Redis 的 App（同 bootstrap 内部测试的做法）。
func newTestApp(t *testing.T) *bootstrap.App {
	t.Helper()
	t.Setenv("ENVIRONMENT", "dev")
	t.Setenv("LOG_DIR", t.TempDir())
	t.Setenv("DEBUG", "false")
	t.Setenv("LOGGER_LEVEL", "ERROR")
	t.Setenv("DATABASE_HOST", "127.0.0.1")
	t.Setenv("DATABASE_PORT", "1")
	t.Setenv("REDIS_HOST", "127.0.0.1")
	t.Setenv("REDIS_PORT", "1")
	t.Setenv("SNOWFLAKE_WORKER_ID", "")

	cfg, err := config.Load("dev")
	if err != nil {
		t.Fatalf("配置装载失败: %v", err)
	}
	app, err := bootstrap.New(cfg)
	if err != nil {
		t.Fatalf("App 装配失败: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// goRouteSet 返回 Go 端已登记路由的 "METHOD path" 集合。
func goRouteSet(t *testing.T) map[string]bool {
	t.Helper()
	app := newTestApp(t)
	recorded := app.RegisteredRoutes()
	if len(recorded) == 0 {
		t.Fatal("Go 端路由登记表为空")
	}
	out := make(map[string]bool, len(recorded))
	for _, r := range recorded {
		out[r.Method+" "+r.Path] = true
	}
	return out
}

// TestNoInventedRoutes 是**硬门禁**：Go 不得出现 Python 契约里没有的路由。
//
// 这是防漂移最关键的一条。三份实现最容易出问题的不是「少实现」，而是
// 某一端擅自加了接口或改了路径——那样前端就要开始区分后端，同契约的前提就崩了。
func TestNoInventedRoutes(t *testing.T) {
	python := loadPythonRoutes(t)
	golang := goRouteSet(t)

	var invented []string
	for route := range golang {
		if !python[route] {
			invented = append(invented, route)
		}
	}
	sort.Strings(invented)

	if len(invented) > 0 {
		t.Errorf("Go 端存在 Python 契约中不存在的路由（%d 条），属于契约漂移：\n  %s\n"+
			"若确实是新增需求，必须**先在 Python 端落地**并重新 make contract-export，"+
			"再在 Go 端实现——不能单端先行。",
			len(invented), strings.Join(invented, "\n  "))
	}
}

// TestRouteCoverage 报告迁移覆盖率。
//
// P0–P2 阶段仅输出信息（缺失路由是预期的，迁移尚未完成）；
// P3 完成后应把 missing 断言改为硬失败，作为「功能对齐」里程碑 M4 的门禁。
func TestRouteCoverage(t *testing.T) {
	python := loadPythonRoutes(t)
	golang := goRouteSet(t)

	var missing, implemented, exempt []string
	for route := range python {
		switch {
		case golang[route]:
			implemented = append(implemented, route)
		case exemptPythonRoutes[route] != "":
			exempt = append(exempt, route+"  ← "+exemptPythonRoutes[route])
		default:
			missing = append(missing, route)
		}
	}
	sort.Strings(implemented)
	sort.Strings(missing)
	sort.Strings(exempt)

	total := len(python)
	want := total - len(exempt) // 扣除有意豁免后的应实现总数
	pct := 0.0
	if want > 0 {
		pct = float64(len(implemented)) / float64(want) * 100
	}

	t.Logf("路由覆盖率：%d/%d = %.1f%%（golden 共 %d 条，豁免 %d 条）",
		len(implemented), want, pct, total, len(exempt))
	if len(exempt) > 0 {
		t.Logf("有意豁免（%d 条）:\n  %s", len(exempt), strings.Join(exempt, "\n  "))
	}

	// 按模块前缀汇总缺失情况，便于判断下一步该做哪个模块
	byPrefix := make(map[string]int)
	for _, route := range missing {
		_, path, _ := strings.Cut(route, " ")
		prefix := modulePrefix(path)
		byPrefix[prefix]++
	}
	prefixes := make([]string, 0, len(byPrefix))
	for p := range byPrefix {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	t.Logf("尚未实现（%d 条），按模块汇总:", len(missing))
	for _, p := range prefixes {
		t.Logf("  %-28s %3d 条", p, byPrefix[p])
	}

	// P0 阶段的最小断言：健康探针与根路径必须已实现（这是 P0 的出口标准）
	for _, required := range []string{
		"GET /",
		"GET /common/health/check",
		"GET /common/health/live",
		"GET /common/health/ready",
	} {
		if !golang[required] {
			t.Errorf("P0 出口标准未达成：缺少必需路由 %s", required)
		}
	}

	// P3 完成后取消下面这行的注释，把覆盖率变成硬门禁：
	// if len(missing) > 0 { t.Errorf("仍有 %d 条路由未实现", len(missing)) }
}

// TestNormalization 用 routes.txt 里真实出现的形态验证归一化规则。
//
// 这些形态是实测得来的，不是推测：Python 端确实用了 {user_id:int} 类型转换器
// 和 {file_path:path} 通配，Go 侧必须正确映射，否则清单 diff 会产生大量假差异。
func TestNormalization(t *testing.T) {
	cases := []struct {
		python string
		want   string
	}{
		{"/system/user/{user_id:int}", "/system/user/:user_id"},
		{"/system/user/authRole/{user_id:int}", "/system/user/authRole/:user_id"},
		{"/upload/{file_path:path}", "/upload/*file_path"},
		{"/monitor/logininfor/unlock/{user_name}", "/monitor/logininfor/unlock/:user_name"},
		{"/system/dict/data/type/{dict_type}", "/system/dict/data/type/:dict_type"},
		{"/system/menu/cascade/{menu_ids}", "/system/menu/cascade/:menu_ids"},
		{"/common/health/check", "/common/health/check"},
		{"/", "/"},
		// 尾斜杠必须原样保留：Python 端 GET /system/user/ 带斜杠而 POST /system/user 不带，
		// 这个不对称是真实存在的，归一化不能擅自抹平
		{"/system/user/", "/system/user/"},
	}
	for _, tc := range cases {
		if got := normalizePath(tc.python); got != tc.want {
			t.Errorf("normalizePath(%q) = %q，期望 %q", tc.python, got, tc.want)
		}
	}
}

// TestGoldenFileIsFresh 提示 golden 契约可能过期。
//
// 无法真正判断新鲜度（那需要访问 Python 端），但可以校验文件形态是否合法，
// 避免有人手工编辑出一个格式错误的清单导致 diff 全线失真。
func TestGoldenFileIsFresh(t *testing.T) {
	python := loadPythonRoutes(t)

	// routes.txt 当前应为 125 条（2026-11-02 导出）。数量大幅偏离说明导出有问题
	// 或 Python 端路由面发生了变更——两种情况都需要人工确认，而不是让测试静默通过。
	const expectedAtExport = 125
	if len(python) != expectedAtExport {
		t.Logf("⚠️ golden 路由数为 %d，导出时为 %d。若 Python 端确有增删，"+
			"请重新 make contract-export 并同步更新本常量与 api/openapi/README.md",
			len(python), expectedAtExport)
	}

	// 形态校验：每条都必须是 "METHOD /path"
	methods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true,
	}
	for route := range python {
		method, path, ok := strings.Cut(route, " ")
		if !ok || !methods[method] {
			t.Errorf("非法路由条目: %q", route)
			continue
		}
		if !strings.HasPrefix(path, "/") {
			t.Errorf("路径必须以 / 开头: %q", route)
		}
		// 归一化后不应残留 Python 的花括号参数
		if strings.Contains(path, "{") || strings.Contains(path, "}") {
			t.Errorf("归一化后仍残留 Python 参数语法: %q", route)
		}
	}
}

// modulePrefix 从路径提取模块前缀，用于按模块汇总缺失路由。
func modulePrefix(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case len(parts) == 0 || parts[0] == "":
		return "(根路径)"
	case len(parts) == 1:
		return "/" + parts[0]
	default:
		return "/" + parts[0] + "/" + parts[1]
	}
}

// TestGoldenFileLocation 确保测试能从自身位置找到 golden 文件。
// go test 的工作目录是包目录，故用相对路径 ../../api/openapi/routes.txt。
func TestGoldenFileLocation(t *testing.T) {
	abs, err := filepath.Abs(routesFile)
	if err != nil {
		t.Fatalf("解析路径失败: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("golden 契约文件不存在: %s (%v)", abs, err)
	}
}
