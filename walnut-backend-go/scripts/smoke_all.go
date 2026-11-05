// walnut-backend-go 全端点冒烟验收（对应 Python 端 scripts/smoke_all.py）。
//
// 覆盖：认证全流程 + system/monitor 全部核心只读端点 + 行级数据权限 + 登出失效。
// 与契约 diff 脚本（test/contract/diff_*.py）的分工：diff 需要 Python/Go 双端在线做
// 逐字节比对；本脚本只打 Go 单端，验证「种子数据 + 真实 HTTP 链路」能走通，
// 是可独立运行的最小验收集。
//
// 用法（walnut-backend-go 目录）：
//
//	go run ./scripts/smoke_all.go                          # 自动构建并起服务（默认 18091 端口）
//	go run ./scripts/smoke_all.go -base http://127.0.0.1:8011   # 打已在线的服务，不自起
//
// 前置：MySQL/Redis 中间件在线（docker compose -f docker-compose.middleware.yml up -d）。
// 注意：连跑多个登录类脚本会撞 ruleLogin 限流（10 次/分钟/IP），批量回归时间隔 ≥20 秒。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	flagBase      = flag.String("base", "", "已在线服务的 base URL（留空则自动构建并起服务）")
	flagPort      = flag.Int("port", 18091, "自动起服务时监听的端口（避开 8011 常驻服务）")
	flagRedisAddr = flag.String("redis-addr", "localhost:6380", "Redis 地址（中间件 compose 映射到宿主机 6380）")
	flagRedisDB   = flag.Int("redis-db", 0, "Redis DB")
	flagRedisPfx  = flag.String("redis-prefix", "walnut-seed-go", "Redis 键前缀（三端隔离硬约束）")
	flagClientID  = flag.String("clientid", "e5cd7e4891bf95d1d19206ce24a7b32e", "种子数据里的 pc 客户端 ID")
	flagReadyWait = flag.Duration("ready-wait", 120*time.Second, "等待服务就绪的超时")
	flagSkipBuild = flag.Bool("skip-build", false, "自动起服务时复用 bin/smoke-server 已有二进制，跳过 go build")
	adminUser     = "admin"
	adminPwd      = "admin123"
	normalUser    = "test"
	normalPwd     = "666666"
	results       []result
	httpClient    = &http.Client{Timeout: 30 * time.Second}
	redisClient   *redis.Client
	baseURL       string
	captchaKeyPfx = "global:captcha_codes:"
)

type result struct {
	name   string
	ok     bool
	detail string
}

func record(name string, ok bool, detail string) {
	results = append(results, result{name, ok, detail})
	tag := "PASS"
	if !ok {
		tag = "FAIL"
	}
	if ok || detail == "" {
		fmt.Printf("%s %s\n", tag, name)
	} else {
		fmt.Printf("%s %s\n       %s\n", tag, name, truncate(detail, 300))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// envelope 是三端统一的响应信封 {code, msg, data}。
type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// do 发一个请求，返回 HTTP 状态码与解析后的信封。body 为 nil 时不带请求体。
func do(method, path string, body any, headers map[string]string) (int, *envelope, string) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Sprintf("<序列化失败: %v>", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, rdr)
	if err != nil {
		return 0, nil, fmt.Sprintf("<构造请求失败: %v>", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("clientid", *flagClientID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Sprintf("<连接失败: %v>", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return resp.StatusCode, nil, string(raw)
	}
	return resp.StatusCode, &env, string(raw)
}

// dataMap / dataList 解出信封 data 字段的两种常见形态。
func dataMap(env *envelope) map[string]any {
	if env == nil || len(env.Data) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(env.Data, &m) != nil {
		return nil
	}
	return m
}

func dataList(env *envelope) []any {
	if env == nil || len(env.Data) == 0 {
		return nil
	}
	var l []any
	if json.Unmarshal(env.Data, &l) != nil {
		return nil
	}
	return l
}

func dataString(env *envelope) string {
	if env == nil || len(env.Data) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(env.Data, &s) != nil {
		return ""
	}
	return s
}

// pageNum 取分页信封 data.total。
func total(env *envelope) float64 {
	m := dataMap(env)
	if m == nil {
		return -1
	}
	t, _ := m["total"].(float64)
	return t
}

// redisGet 读验证码答案（键体系 {prefix}:global:captcha_codes:{uuid}）。
func redisGet(key string) string {
	v, err := redisClient.Get(context.Background(), key).Result()
	if err != nil {
		return ""
	}
	return v
}

// login 走完整登录链路：取验证码 → 从 Redis 捞答案 → 登录，返回 access_token。
func login(username, password string) (string, string) {
	_, env, raw := do("GET", "/auth/code", nil, nil)
	d := dataMap(env)
	if env == nil || env.Code != 200 || d == nil {
		return "", fmt.Sprintf("auth/code 失败: %s", raw)
	}
	body := map[string]any{
		"clientId":  *flagClientID,
		"grantType": "password",
		"username":  username,
		"password":  password,
	}
	if uuid, _ := d["uuid"].(string); uuid != "" {
		ans := redisGet(*flagRedisPfx + ":" + captchaKeyPfx + uuid)
		if ans == "" {
			return "", fmt.Sprintf("Redis 里取不到验证码答案（uuid=%s，prefix=%s）", uuid, *flagRedisPfx)
		}
		body["code"] = ans
		body["uuid"] = uuid
	} else if enabled, _ := d["captchaEnabled"].(bool); enabled {
		return "", "captchaEnabled=true 但 uuid 为空"
	}
	_, lenv, lraw := do("POST", "/auth/login", body, nil)
	if lenv == nil || lenv.Code != 200 {
		return "", fmt.Sprintf("登录失败（user=%s）: %s", username, truncate(lraw, 200))
	}
	tok, _ := dataMap(lenv)["access_token"].(string)
	if tok == "" {
		return "", fmt.Sprintf("登录 200 但无 access_token: %s", truncate(lraw, 200))
	}
	return tok, ""
}

func auth(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// startServer 构建并拉起 Go 服务，等待就绪探针通过后返回停止函数。
func startServer() (stop func(), err error) {
	// 端口预检：若上一轮 smoke 异常退出留下孤儿进程仍占着端口，新服务会绑定失败
	// 退出，而就绪轮询会打到孤儿进程上——拿旧代码的结果继续跑，造成假象。
	if ln, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *flagPort)); e != nil {
		return nil, fmt.Errorf("端口 %d 已被占用（可能有上一轮 smoke 的孤儿进程，Windows 下 taskkill /F /IM smoke-server.exe 清理）: %w", *flagPort, e)
	} else {
		ln.Close()
	}
	binPath := "bin/smoke-server"
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if !*flagSkipBuild {
		fmt.Printf("构建服务二进制 → %s …\n", binPath)
		build := exec.Command("go", "build", "-o", binPath, ".")
		build.Stdout, build.Stderr = os.Stdout, os.Stderr
		if e := build.Run(); e != nil {
			return nil, fmt.Errorf("go build 失败: %w", e)
		}
	}
	var logBuf bytes.Buffer
	cmd := exec.Command(binPath, "run", "--env", "dev")
	cmd.Env = append(os.Environ(), fmt.Sprintf("SERVER_PORT=%d", *flagPort))
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	if e := cmd.Start(); e != nil {
		return nil, fmt.Errorf("启动 %s 失败: %w", binPath, e)
	}
	baseURL = fmt.Sprintf("http://127.0.0.1:%d", *flagPort)
	deadline := time.Now().Add(*flagReadyWait)
	for time.Now().Before(deadline) {
		if resp, e := httpClient.Get(baseURL + "/common/health/ready"); e == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Printf("服务就绪（%s，PID %d）\n", baseURL, cmd.Process.Pid)
				return func() {
					cmd.Process.Kill()
					cmd.Wait()
				}, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	cmd.Process.Kill()
	cmd.Wait()
	return nil, fmt.Errorf("等待就绪超时（%s），服务日志尾部:\n%s", *flagReadyWait, tail(logBuf.String(), 2000))
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func main() {
	flag.Parse()
	redisClient = redis.NewClient(&redis.Options{
		Addr: *flagRedisAddr,
		DB:   *flagRedisDB,
	})
	defer redisClient.Close()

	var stop func()
	if *flagBase != "" {
		baseURL = strings.TrimRight(*flagBase, "/")
		fmt.Printf("目标服务（在线）：%s\n", baseURL)
	} else {
		s, err := startServer()
		if err != nil {
			fmt.Fprintln(os.Stderr, "起服务失败:", err)
			os.Exit(2)
		}
		stop = s
	}

	smoke()

	// 先停服务再汇总退出：os.Exit 不执行 defer，失败路径上会留孤儿进程占着端口
	if stop != nil {
		stop()
	}

	failed := 0
	for _, r := range results {
		if !r.ok {
			failed++
		}
	}
	fmt.Printf("\n==== %d/%d passed ====\n", len(results)-failed, len(results))
	if failed > 0 {
		names := []string{}
		for _, r := range results {
			if !r.ok {
				names = append(names, r.name)
			}
		}
		fmt.Println("FAILED:", strings.Join(names, ", "))
		os.Exit(1)
	}
}

func smoke() {
	// ---------- 认证 ----------
	_, env, raw := do("GET", "/auth/code", nil, nil)
	d := dataMap(env)
	uuid, _ := d["uuid"].(string)
	record("auth/code", env != nil && env.Code == 200 && uuid != "", raw)

	// 错误密码 → 10005（先造一个新鲜验证码）
	_, env, raw = do("GET", "/auth/code", nil, nil)
	d = dataMap(env)
	uuid, _ = d["uuid"].(string)
	wrongBody := map[string]any{
		"clientId": *flagClientID, "grantType": "password",
		"username": adminUser, "password": "WRONG1",
		"code": redisGet(*flagRedisPfx + ":" + captchaKeyPfx + uuid), "uuid": uuid,
	}
	_, wenv, _ := do("POST", "/auth/login", wrongBody, nil)
	record("login 错误密码 → 10005", wenv != nil && wenv.Code == 10005, fmt.Sprintf("got=%v", wenv))

	token, detail := login(adminUser, adminPwd)
	record("login admin", token != "", detail)
	if token == "" {
		fmt.Println("admin 登录失败，后续用例全部跳过")
		return
	}
	AH := auth(token)

	// ---------- getInfo / 路由 ----------
	_, env, raw = do("GET", "/system/user/getInfo", nil, AH)
	d = dataMap(env)
	user, _ := d["user"].(map[string]any)
	perms, _ := d["permissions"].([]any)
	hasAll := false
	for _, p := range perms {
		if p == "*:*:*" {
			hasAll = true
		}
	}
	record("user/getInfo", env != nil && env.Code == 200 && user["userName"] == adminUser && hasAll, raw)

	_, env, raw = do("GET", "/system/menu/getRouters", nil, AH)
	routers := dataList(env)
	record("menu/getRouters", env != nil && env.Code == 200 && len(routers) > 0, raw)

	// ---------- 部门 ----------
	_, env, raw = do("GET", "/system/dept/list", nil, AH)
	record("dept/list", env != nil && env.Code == 200 && len(dataList(env)) >= 10, raw)

	_, env, raw = do("GET", "/system/dept/treeselect", nil, AH)
	ts := dataList(env)
	ok := env != nil && env.Code == 200 && len(ts) > 0
	if ok {
		first, _ := ts[0].(map[string]any)
		ok = first["label"] != nil && first["label"] != ""
	}
	record("dept/treeselect", ok, raw)

	_, env, raw = do("GET", "/system/dept/103", nil, AH)
	d = dataMap(env)
	record("dept/103", env != nil && env.Code == 200 && d["deptName"] == "研发部门", raw)

	// ---------- 岗位 ----------
	// 总量用 >=：契约 diff 脚本的写入闭环可能留下未清理的历史数据（如 test_go_001）
	_, env, raw = do("GET", "/system/post/list?pageNum=1&pageSize=10", nil, AH)
	rows, _ := dataMap(env)["rows"].([]any)
	firstHasCode := len(rows) > 0
	if firstHasCode {
		r0, _ := rows[0].(map[string]any)
		firstHasCode = r0["postCode"] != nil && r0["postCode"] != ""
	}
	record("post/list", env != nil && env.Code == 200 && total(env) >= 4 && firstHasCode, raw)

	// ---------- 字典 ----------
	_, env, raw = do("GET", "/system/dict/type/list?pageNum=1&pageSize=20", nil, AH)
	record("dict/type/list", env != nil && env.Code == 200 && total(env) >= 10, raw)

	_, env, raw = do("GET", "/system/dict/data/type/sys_user_sex", nil, AH)
	sex := dataList(env)
	sexOK := env != nil && env.Code == 200 && len(sex) > 0
	if sexOK {
		s0, _ := sex[0].(map[string]any)
		l, _ := s0["dictLabel"].(string)
		sexOK = l == "男" || l == "女" || l == "未知"
	}
	record("dict/data/type", sexOK, raw)

	// ---------- 参数 ----------
	_, env, raw = do("GET", "/system/config/configKey/sys.user.initPassword", nil, AH)
	record("config/configKey", env != nil && env.Code == 200 && dataString(env) == "123456", raw)

	// ---------- 通知公告 / 客户端 / 社交 ----------
	// 总量用 >=：契约 diff 脚本的写入闭环可能留下已清理外的历史数据
	_, env, raw = do("GET", "/system/notice/list?pageNum=1&pageSize=10", nil, AH)
	record("notice/list", env != nil && env.Code == 200 && total(env) >= 2, raw)

	_, env, raw = do("GET", "/system/client/list?pageNum=1&pageSize=10", nil, AH)
	record("client/list", env != nil && env.Code == 200 && total(env) >= 2, raw)

	_, env, raw = do("GET", "/system/social/list", nil, AH)
	record("social/list", env != nil && env.Code == 200, raw)

	// ---------- 角色 ----------
	_, env, raw = do("GET", "/system/role/list?pageNum=1&pageSize=10", nil, AH)
	record("role/list", env != nil && env.Code == 200 && total(env) >= 3, raw)

	_, env, raw = do("GET", "/system/role/deptTree/3", nil, AH)
	d = dataMap(env)
	_, hasChecked := d["checkedKeys"]
	_, hasDepts := d["depts"]
	record("role/deptTree/3", env != nil && env.Code == 200 && hasChecked && hasDepts, raw)

	// ---------- 用户 ----------
	_, env, raw = do("GET", "/system/user/list?pageNum=1&pageSize=10", nil, AH)
	record("user/list", env != nil && env.Code == 200 && total(env) >= 3, raw)

	_, env, raw = do("GET", "/system/user/", nil, AH)
	d = dataMap(env)
	_, hasRoles := d["roles"].([]any)
	record("user/ (新增前置)", env != nil && env.Code == 200 && hasRoles, raw)

	_, env, raw = do("GET", "/system/user/3", nil, AH)
	d = dataMap(env)
	u3, _ := d["user"].(map[string]any)
	record("user/3", env != nil && env.Code == 200 && u3["userName"] == normalUser, raw)

	_, env, raw = do("GET", "/system/user/authRole/3", nil, AH)
	d = dataMap(env)
	record("user/authRole/3", env != nil && env.Code == 200 && d["user"] != nil, raw)

	_, env, raw = do("GET", "/system/user/deptTree", nil, AH)
	record("user/deptTree", env != nil && env.Code == 200 && dataList(env) != nil, raw)

	_, env, raw = do("GET", "/system/user/list/dept/103", nil, AH)
	record("user/list/dept/103", env != nil && env.Code == 200 && dataList(env) != nil, raw)

	// ---------- profile ----------
	_, env, raw = do("GET", "/system/user/profile", nil, AH)
	d = dataMap(env)
	pu, _ := d["user"].(map[string]any)
	record("profile GET", env != nil && env.Code == 200 && pu["userName"] == adminUser, raw)

	// ---------- monitor ----------
	_, env, raw = do("GET", "/monitor/logininfor/list?pageNum=1&pageSize=10", nil, AH)
	record("logininfor/list", env != nil && env.Code == 200 && total(env) >= 1, raw)

	_, env, raw = do("GET", "/monitor/operlog/list?pageNum=1&pageSize=10", nil, AH)
	record("operlog/list", env != nil && env.Code == 200, raw)

	// ---------- 行级数据权限（test 用户：有 list 权限，但数据范围只允许看到本人） ----------
	tok2, detail := login(normalUser, normalPwd)
	if tok2 == "" {
		record("test 用户登录", false, detail)
	} else {
		TH := auth(tok2)
		_, env, raw = do("GET", "/system/user/list?pageNum=1&pageSize=10", nil, TH)
		rows, _ := dataMap(env)["rows"].([]any)
		onlySelf := env != nil && env.Code == 200 && len(rows) == 1
		if onlySelf {
			r0, _ := rows[0].(map[string]any)
			onlySelf = r0["userName"] == normalUser
		}
		record("test 用户数据权限过滤（只见本人）", onlySelf, raw)

		// menu/list 需 superadmin 角色，test 用户无此角色 → 403
		_, env, raw = do("GET", "/system/menu/list", nil, TH)
		record("test 用户访问 menu/list → 403", env != nil && env.Code == 403, raw)
	}

	// ---------- 登出 ----------
	_, env, raw = do("POST", "/auth/logout", nil, AH)
	record("logout", env != nil && env.Code == 200, raw)

	_, env, raw = do("GET", "/system/user/getInfo", nil, AH)
	record("登出后 getInfo → 401", env != nil && env.Code == 401, raw)
}
