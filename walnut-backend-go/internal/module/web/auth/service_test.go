package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/module/system/client"
	"walnut-seed-go/internal/module/system/dept"
	logmodel "walnut-seed-go/internal/module/system/log"
	"walnut-seed-go/internal/module/system/menu"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/module/system/user"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/cryptox"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

// mockRepo 嵌入 authRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	authRepo
	countActiveUsersByUsername func(ctx context.Context, username string) (int64, error)
	updateLoginInfo            func(ctx context.Context, userID int64, ip string, at time.Time) error
	insertLogininfor           func(ctx context.Context, entry *logmodel.SysLogininfor) error
}

func (m *mockRepo) CountActiveUsersByUsername(ctx context.Context, username string) (int64, error) {
	return m.countActiveUsersByUsername(ctx, username)
}

func (m *mockRepo) UpdateLoginInfo(ctx context.Context, userID int64, ip string, at time.Time) error {
	return m.updateLoginInfo(ctx, userID, ip, at)
}

func (m *mockRepo) InsertLogininfor(ctx context.Context, entry *logmodel.SysLogininfor) error {
	return m.insertLogininfor(ctx, entry)
}

func testService() *Service {
	return NewService(nil, nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{})
}

func TestRegisterInvalidUserType(t *testing.T) {
	svc := testService()
	svc.repo = &mockRepo{} // 类型守卫先于任何数据访问，触发 repo 调用即 panic
	bad := "bogus"
	err := svc.Register(context.Background(), &RegisterRequest{
		Username: "newuser",
		Password: "Abc12345!",
		UserType: &bad,
	}, RequestMeta{})
	se := errs.As(err)
	if se == nil || se.Code != errs.CodeBadRequest || se.Msg != "'UserType' not found By bogus" {
		t.Errorf("err = %v, want 400 'UserType' not found By bogus", err)
	}
}

func TestRegisterDuplicateUsername(t *testing.T) {
	svc := testService()
	svc.repo = &mockRepo{countActiveUsersByUsername: func(_ context.Context, username string) (int64, error) {
		if username != "newuser" {
			t.Errorf("username = %s, want newuser", username)
		}
		return 1, nil
	}}
	err := svc.Register(context.Background(), &RegisterRequest{
		Username: "newuser",
		Password: "Abc12345!",
	}, RequestMeta{})
	se := errs.As(err)
	if se == nil || se.Code != errs.AuthUserRegisterExists.Code || se.Msg != errs.AuthUserRegisterExists.Fallback {
		t.Errorf("err = %v, want 注册账号已存在（code %d）", err, errs.AuthUserRegisterExists.Code)
	}
}

// ---- 登录主流程测试基建 ----

// testSeedHash 是种子数据里 admin123 的真实 bcrypt 哈希（cost 10，校验快）。
const testSeedHash = "$2a$10$7JB720yubVSZvUI0rEqK/.VqGOZTH.ulu33dHOiBE8ByOhJIrdAu2"

const testSecret = "unit-test-secret-key-0123456789abcdef"

const testUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func intPtr(v int) *int { return &v }

func int64Ptr(v int64) *int64 { return &v }

func loginCfg() *config.Config {
	return &config.Config{
		CaptchaEnable:            false,
		SecretKey:                testSecret,
		PasswordMaxRetryCount:    5,
		PasswordLockTime:         10,
		AccessTokenExpireSeconds: 2592000,
	}
}

func loginClient() *client.SysClient {
	return &client.SysClient{
		ClientID: strPtr("cid-1"), ClientKey: strPtr("pc"), DeviceType: strPtr("pc"),
		GrantType: strPtr("password"), Status: strPtr("0"),
		Timeout: intPtr(3600), ActiveTimeout: intPtr(1800),
	}
}

func loginUser() *user.SysUser {
	u := &user.SysUser{
		UserName: "admin", NickName: "管理员", Status: modelx.Normal,
		Password: strPtr(testSeedHash), DeptID: int64Ptr(100),
	}
	u.ID = 7
	return u
}

func loginMeta() RequestMeta {
	return RequestMeta{ClientIP: "192.168.1.10", UserAgent: testUA}
}

func loginBody(password string) *PasswordLoginRequest {
	return &PasswordLoginRequest{
		LoginRequest: LoginRequest{ClientID: "cid-1", GrantType: "password"},
		Username:     "admin",
		Password:     password,
	}
}

// loginCaptures 收集旁路写入（登录日志 / 最后登录信息），供断言调用序与内容。
type loginCaptures struct {
	entries      []*logmodel.SysLogininfor
	loginInfoN   int
	loginInfoUID int64
	loginInfoIP  string
}

// newLoginFixture 组装完整登录依赖：provider（用户/部门/角色/菜单）+ repo + 内存缓存。
// u 为 nil 时模拟「用户不存在」。svc.db 置非 nil 以启用 recordLogininfor
// （它只做 nil 检查，实际写入走 mock repo，不触达真实数据库）。
func newLoginFixture(t *testing.T, cfg *config.Config, u *user.SysUser) (*Service, *fakeRedis, *loginCaptures) {
	t.Helper()
	cache, f := newFakeCache(t, "")
	cap := &loginCaptures{}
	svc := NewService(nil, cache, cfg, nil, "", Deps{
		Users: &mockUserProvider{findByUsername: func(_ context.Context, username string) (*user.SysUser, error) {
			if u != nil && username == u.UserName {
				return u, nil
			}
			return nil, nil
		}},
		Depts: &mockDeptProvider{findByID: func(_ context.Context, deptID int64) (*dept.SysDept, error) {
			if deptID != 100 {
				t.Errorf("deptID = %d, want 100", deptID)
			}
			return &dept.SysDept{DeptName: strPtr("研发部"), DeptCategory: strPtr("tech")}, nil
		}},
		Roles: &mockRoleProvider{findActiveByUserID: func(_ context.Context, userID int64) ([]role.SysRole, error) {
			if userID != 7 {
				t.Errorf("userID = %d, want 7", userID)
			}
			r := role.SysRole{RoleKey: "admin"}
			r.ID = 2
			return []role.SysRole{r}, nil
		}},
		Menus: &mockMenuProvider{findByRoleIDs: func(_ context.Context, roleIDs []int64) ([]menu.SysMenu, error) {
			if len(roleIDs) != 1 || roleIDs[0] != 2 {
				t.Errorf("roleIDs = %v, want [2]", roleIDs)
			}
			m1 := menu.SysMenu{Perms: strPtr("system:user:list")}
			m1.ID = 9
			m2 := menu.SysMenu{Perms: strPtr("system:role:list")}
			m2.ID = 3
			m3 := menu.SysMenu{Perms: strPtr("system:user:list")} // 多角色重复菜单
			m3.ID = 9
			return []menu.SysMenu{m1, m2, m3}, nil
		}},
	})
	svc.db = &gorm.DB{}
	svc.repo = &mockRepo{
		updateLoginInfo: func(_ context.Context, userID int64, ip string, _ time.Time) error {
			cap.loginInfoN++
			cap.loginInfoUID, cap.loginInfoIP = userID, ip
			return nil
		},
		insertLogininfor: func(_ context.Context, entry *logmodel.SysLogininfor) error {
			cap.entries = append(cap.entries, entry)
			return nil
		},
	}
	return svc, f, cap
}

// ---- 内存 Redis 假件（go-redis hook 拦截命令，无网络依赖） ----
//
// cachex.Client 是具体结构体（内嵌 *redis.Client），无法以接口 mock；
// 这里用 AddHook 在命令层拦截，复刻 get/set/del/incr/expire/scan/keys/ttl
// 语义并记录 TTL，让会话/在线缓存/失败计数的键值契约可离线断言。

type fakeRedis struct {
	mu   sync.Mutex
	vals map[string]string
	ttls map[string]time.Duration // 最近一次请求的 TTL（0 = 永不过期）
	exps map[string]time.Time
}

func newFakeCache(t *testing.T, prefix string) (*cachex.Client, *fakeRedis) {
	t.Helper()
	f := &fakeRedis{vals: map[string]string{}, ttls: map[string]time.Duration{}, exps: map[string]time.Time{}}
	// 端口 0 不可达：hook 拦截后不会真正联网；MinIdleConns 的后台拨号立即失败退出
	c := cachex.New(cachex.Params{Host: "127.0.0.1", Port: 0, KeyPrefix: prefix})
	c.AddHook(f)
	c.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = c.Close() })
	return c, f
}

func (f *fakeRedis) DialHook(next redis.DialHook) redis.DialHook { return next }

func (f *fakeRedis) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.run(cmd)
	}
}

func (f *fakeRedis) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(_ context.Context, cmds []redis.Cmder) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, cmd := range cmds {
			if cmd.Name() == "multi" || cmd.Name() == "exec" {
				continue
			}
			if err := f.run(cmd); err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
		}
		return nil
	}
}

func (f *fakeRedis) run(cmd redis.Cmder) error {
	args := cmd.Args()
	switch cmd.Name() {
	case "get":
		v, ok := f.get(fakeArgStr(args[1]))
		if !ok {
			return redis.Nil
		}
		cmd.(*redis.StringCmd).SetVal(v)
		return nil
	case "set":
		key := fakeArgStr(args[1])
		f.vals[key] = fmt.Sprint(args[2])
		f.setTTL(key, 0)
		for i := 3; i+1 < len(args); i++ {
			switch strings.ToLower(fakeArgStr(args[i])) {
			case "ex":
				f.setTTL(key, time.Duration(fakeArgInt(args[i+1]))*time.Second)
				i++
			case "px":
				f.setTTL(key, time.Duration(fakeArgInt(args[i+1]))*time.Millisecond)
				i++
			}
		}
		cmd.(*redis.StatusCmd).SetVal("OK")
		return nil
	case "del":
		var n int64
		for _, a := range args[1:] {
			key := fakeArgStr(a)
			if _, ok := f.vals[key]; ok {
				delete(f.vals, key)
				delete(f.ttls, key)
				delete(f.exps, key)
				n++
			}
		}
		cmd.(*redis.IntCmd).SetVal(n)
		return nil
	case "incr":
		key := fakeArgStr(args[1])
		var n int64
		if v, ok := f.get(key); ok {
			n, _ = strconv.ParseInt(v, 10, 64)
		}
		n++
		f.vals[key] = strconv.FormatInt(n, 10)
		cmd.(*redis.IntCmd).SetVal(n)
		return nil
	case "expire":
		key := fakeArgStr(args[1])
		sec := fakeArgInt(args[2])
		if _, ok := f.get(key); !ok {
			cmd.(*redis.BoolCmd).SetVal(false)
			return nil
		}
		if sec <= 0 {
			delete(f.vals, key)
			delete(f.ttls, key)
			delete(f.exps, key)
		} else {
			f.setTTL(key, time.Duration(sec)*time.Second)
		}
		cmd.(*redis.BoolCmd).SetVal(true)
		return nil
	case "scan":
		pattern := ""
		for i := 2; i+1 < len(args); i++ {
			if strings.EqualFold(fakeArgStr(args[i]), "match") {
				pattern = fakeArgStr(args[i+1])
			}
		}
		cmd.(*redis.ScanCmd).SetVal(f.matchKeys(pattern), 0)
		return nil
	case "keys":
		cmd.(*redis.StringSliceCmd).SetVal(f.matchKeys(fakeArgStr(args[1])))
		return nil
	case "ttl":
		key := fakeArgStr(args[1])
		if _, ok := f.get(key); !ok {
			cmd.(*redis.DurationCmd).SetVal(-2)
			return nil
		}
		exp, ok := f.exps[key]
		if !ok {
			cmd.(*redis.DurationCmd).SetVal(-1)
			return nil
		}
		cmd.(*redis.DurationCmd).SetVal(time.Until(exp).Truncate(time.Second))
		return nil
	case "ping":
		cmd.(*redis.StatusCmd).SetVal("PONG")
		return nil
	default:
		return fmt.Errorf("fakeRedis: 未实现的命令 %q", cmd.Name())
	}
}

func (f *fakeRedis) setTTL(key string, d time.Duration) {
	f.ttls[key] = d
	if d > 0 {
		f.exps[key] = time.Now().Add(d)
	} else {
		delete(f.exps, key)
	}
}

func (f *fakeRedis) get(key string) (string, bool) {
	v, ok := f.vals[key]
	if !ok {
		return "", false
	}
	if exp, has := f.exps[key]; has && time.Now().After(exp) {
		delete(f.vals, key)
		delete(f.exps, key)
		delete(f.ttls, key)
		return "", false
	}
	return v, true
}

func (f *fakeRedis) matchKeys(pattern string) []string {
	keys := make([]string, 0)
	for k := range f.vals {
		if _, ok := f.get(k); !ok {
			continue
		}
		if ok, _ := path.Match(pattern, k); ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func (f *fakeRedis) value(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.get(key)
}

func (f *fakeRedis) ttl(key string) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ttls[key]
}

func (f *fakeRedis) seed(key, val string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vals[key] = val
}

func (f *fakeRedis) allKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.matchKeys("*")
}

func fakeArgStr(a any) string { s, _ := a.(string); return s }

func fakeArgInt(a any) int64 {
	switch v := a.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case uint64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

// ---- 登录成功主流程 ----

// TestLoginPasswordSuccessSession 钉住成功登录的完整副作用链：
// JWT claims（硬过期）+ 会话快照（snake_case）+ 在线用户（camelCase）+
// 三键 TTL + 失败计数清零 + 成功登录日志 + 最后登录信息更新。
func TestLoginPasswordSuccessSession(t *testing.T) {
	svc, f, cap := newLoginFixture(t, loginCfg(), loginUser())
	errKey := "pwd_err_cnt:admin:192.168.1.10"
	f.seed(errKey, "2") // 历史失败计数，成功登录后应清零

	resp, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), loginClient())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resp.AccessToken == "" || resp.ExpireIn != 3600 || resp.ClientID != "cid-1" {
		t.Fatalf("resp = %+v, want token/3600/cid-1", resp)
	}
	if resp.RefreshToken != nil || resp.RefreshExpireIn != nil || resp.Scope != nil || resp.OpenID != nil {
		t.Errorf("未实现字段应恒为 null: %+v", resp)
	}

	// JWT claims：exp 是固定硬过期（timeout），不受滑动过期影响
	claims, err := cryptox.ParseToken(resp.AccessToken, testSecret, true)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Sub == "" || claims.UserID == nil || *claims.UserID != 7 || claims.UserName == nil || *claims.UserName != "admin" {
		t.Errorf("claims 身份错误: %+v", claims)
	}
	if claims.DeptID == nil || *claims.DeptID != 100 || claims.DeptName == nil || *claims.DeptName != "研发部" ||
		claims.DeptCategory == nil || *claims.DeptCategory != "tech" {
		t.Errorf("claims 部门错误: %+v", claims)
	}
	if claims.ClientID == nil || *claims.ClientID != "cid-1" || claims.IsRefresh {
		t.Errorf("claims 客户端错误: %+v", claims)
	}
	if delta := claims.Exp - time.Now().Unix(); delta < 3590 || delta > 3600 {
		t.Errorf("exp 距现在 %ds, want ≈3600（硬过期）", delta)
	}

	// 会话快照：snake_case 键 + 权限去重排序 + _active_timeout
	raw, ok := f.value("user_session:" + claims.Sub)
	if !ok {
		t.Fatal("user_session 键未写入")
	}
	var sess map[string]any
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		t.Fatalf("会话 JSON 非法: %v", err)
	}
	if sess["user_id"] != float64(7) || sess["user_name"] != "admin" || sess["nickname"] != "管理员" ||
		sess["dept_id"] != float64(100) || sess["dept_name"] != "研发部" || sess["dept_category"] != "tech" || sess["user_status"] != "0" {
		t.Errorf("会话基本字段错误: %v", sess)
	}
	if sess["_active_timeout"] != float64(1800) {
		t.Errorf("_active_timeout = %v, want 1800", sess["_active_timeout"])
	}
	wantPerms := []any{"system:role:list", "system:user:list"}
	if got, _ := json.Marshal(sess["menu_permission"]); string(got) != mustJSON(wantPerms) {
		t.Errorf("menu_permission = %v, want 去重排序 %v", sess["menu_permission"], wantPerms)
	}
	if got, _ := json.Marshal(sess["role_permission"]); string(got) != `["admin"]` {
		t.Errorf("role_permission = %v, want [admin]", sess["role_permission"])
	}
	if got, _ := json.Marshal(sess["menu_ids"]); string(got) != `[3,9]` {
		t.Errorf("menu_ids = %v, want [3,9]（去重升序）", sess["menu_ids"])
	}

	// access_token 键存 token 本身；在线用户键以 token 为后缀、camelCase 键
	if v, ok := f.value("access_token:" + claims.Sub); !ok || v != resp.AccessToken {
		t.Errorf("access_token 键 = %q/%v, want token 本身", v, ok)
	}
	rawOnline, ok := f.value("online_tokens:" + resp.AccessToken)
	if !ok {
		t.Fatal("online_tokens 键未写入")
	}
	var online map[string]any
	if err := json.Unmarshal([]byte(rawOnline), &online); err != nil {
		t.Fatalf("在线用户 JSON 非法: %v", err)
	}
	if online["tokenId"] != resp.AccessToken || online["userName"] != "admin" || online["deptName"] != "研发部" ||
		online["ipaddr"] != "192.168.1.10" || online["loginLocation"] != "内网IP" ||
		online["browser"] != "Chrome" || online["os"] != "Windows" ||
		online["clientKey"] != "pc" || online["deviceType"] != "pc" {
		t.Errorf("在线用户字段错误: %v", online)
	}
	if lt, _ := online["loginTime"].(float64); lt <= 0 {
		t.Errorf("loginTime = %v, want 毫秒时间戳", online["loginTime"])
	}

	// 非滑动过期：三键 TTL 都是固定 timeout
	for _, k := range []string{"user_session:" + claims.Sub, "access_token:" + claims.Sub, "online_tokens:" + resp.AccessToken} {
		if f.ttl(k) != 3600*time.Second {
			t.Errorf("%s TTL = %v, want 3600s", k, f.ttl(k))
		}
	}

	// 成功 → 失败计数清零
	if _, ok := f.value(errKey); ok {
		t.Error("登录成功应删除失败计数键")
	}

	// 成功登录日志（旁路）
	if len(cap.entries) != 1 {
		t.Fatalf("登录日志条数 = %d, want 1", len(cap.entries))
	}
	e := cap.entries[0]
	if e.Status == nil || *e.Status != logmodel.StatusSuccess || e.Msg == nil || *e.Msg != "登录成功" {
		t.Errorf("登录日志状态/文案错误: %v / %v", e.Status, e.Msg)
	}
	if e.UserName == nil || *e.UserName != "admin" || e.IPAddr == nil || *e.IPAddr != "192.168.1.10" ||
		e.ClientKey == nil || *e.ClientKey != "pc" || e.DeviceType == nil || *e.DeviceType != "pc" ||
		e.LoginLocation == nil || *e.LoginLocation != "内网IP" || e.Browser == nil || *e.Browser != "Chrome" || e.OS == nil || *e.OS != "Windows" {
		t.Errorf("登录日志字段错误: %+v", e)
	}
	if e.ID == 0 || e.LoginTime == nil {
		t.Errorf("登录日志 ID/LoginTime 未填充: %+v", e)
	}

	// 最后登录信息更新
	if cap.loginInfoN != 1 || cap.loginInfoUID != 7 || cap.loginInfoIP != "192.168.1.10" {
		t.Errorf("UpdateLoginInfo 调用错误: n=%d uid=%d ip=%s", cap.loginInfoN, cap.loginInfoUID, cap.loginInfoIP)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestLoginPasswordSlidingAndClamp 滑动过期只影响 Redis TTL（JWT exp 仍硬过期）；
// activeTimeout 超过 timeout 时钳到 timeout。
func TestLoginPasswordSlidingAndClamp(t *testing.T) {
	t.Run("滑动过期TTL取活跃超时", func(t *testing.T) {
		cfg := loginCfg()
		cfg.TokenSlidingExpire = true
		svc, f, _ := newLoginFixture(t, cfg, loginUser())
		resp, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), loginClient())
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		claims, err := cryptox.ParseToken(resp.AccessToken, testSecret, true)
		if err != nil {
			t.Fatalf("ParseToken: %v", err)
		}
		if f.ttl("user_session:"+claims.Sub) != 1800*time.Second {
			t.Errorf("会话 TTL = %v, want 1800s（活跃超时）", f.ttl("user_session:"+claims.Sub))
		}
		if delta := claims.Exp - time.Now().Unix(); delta < 3590 || delta > 3600 {
			t.Errorf("JWT exp 距现在 %ds, want ≈3600（硬过期不滑动）", delta)
		}
	})
	t.Run("活跃超时钳到固定超时", func(t *testing.T) {
		cfg := loginCfg()
		cfg.TokenSlidingExpire = true
		cli := loginClient()
		cli.ActiveTimeout = intPtr(7200) // > timeout 3600
		svc, f, _ := newLoginFixture(t, cfg, loginUser())
		resp, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), cli)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		claims, _ := cryptox.ParseToken(resp.AccessToken, testSecret, true)
		raw, _ := f.value("user_session:" + claims.Sub)
		var sess map[string]any
		_ = json.Unmarshal([]byte(raw), &sess)
		if sess["_active_timeout"] != float64(3600) {
			t.Errorf("_active_timeout = %v, want 钳到 3600", sess["_active_timeout"])
		}
		if f.ttl("user_session:"+claims.Sub) != 3600*time.Second {
			t.Errorf("会话 TTL = %v, want 3600s", f.ttl("user_session:"+claims.Sub))
		}
	})
}

// TestLoginPasswordNoDeptClaimsEmptyString 无部门用户：claims 与会话的
// dept_name/dept_category 是**空串**而非 null（跨端契约，曾被比对抓出）。
func TestLoginPasswordNoDeptClaimsEmptyString(t *testing.T) {
	u := loginUser()
	u.DeptID = nil
	svc, f, _ := newLoginFixture(t, loginCfg(), u)
	resp, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), loginClient())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	claims, err := cryptox.ParseToken(resp.AccessToken, testSecret, true)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.DeptName == nil || *claims.DeptName != "" || claims.DeptCategory == nil || *claims.DeptCategory != "" {
		t.Errorf("claims 部门应为空串非 null: %+v", claims)
	}
	raw, _ := f.value("user_session:" + claims.Sub)
	if !strings.Contains(raw, `"dept_name":""`) || !strings.Contains(raw, `"dept_category":""`) {
		t.Errorf("会话部门字段应为空串: %s", raw)
	}
}

// ---- 登录失败分支 ----

// TestLoginPasswordWrongPassword 密码错误：统一 10005 文案、失败计数 +1
// 且 TTL=锁定分钟数（滑动窗口）、失败日志、不发 token、不更新登录信息。
func TestLoginPasswordWrongPassword(t *testing.T) {
	svc, f, cap := newLoginFixture(t, loginCfg(), loginUser())
	resp, err := svc.LoginPassword(context.Background(), loginBody("wrong-password"), loginMeta(), loginClient())
	if resp != nil {
		t.Errorf("失败不应发放 token: %+v", resp)
	}
	se := errs.As(err)
	if se == nil || se.Code != errs.AuthUserPasswordNotMatch.Code || se.Msg != "用户不存在/密码错误" {
		t.Fatalf("err = %v, want 10005 用户不存在/密码错误", err)
	}
	errKey := "pwd_err_cnt:admin:192.168.1.10"
	if v, ok := f.value(errKey); !ok || v != "1" {
		t.Errorf("失败计数 = %q/%v, want 1", v, ok)
	}
	if f.ttl(errKey) != 10*time.Minute {
		t.Errorf("计数 TTL = %v, want 10min（PasswordLockTime）", f.ttl(errKey))
	}
	// 除计数键外不应有任何会话/在线键
	if keys := f.allKeys(); len(keys) != 1 || keys[0] != errKey {
		t.Errorf("缓存键 = %v, want 仅失败计数键", keys)
	}
	if len(cap.entries) != 1 || cap.entries[0].Status == nil || *cap.entries[0].Status != logmodel.StatusFail ||
		cap.entries[0].Msg == nil || *cap.entries[0].Msg != "用户不存在/密码错误" {
		t.Errorf("失败日志错误: %+v", cap.entries)
	}
	if cap.loginInfoN != 0 {
		t.Error("失败不应更新最后登录信息")
	}
}

// TestLoginPasswordUserNotFoundAntiEnumeration 用户不存在与密码错误
// 外部完全不可区分（同码同文案、同样计数）；空 IP 归一为 "unknown"。
func TestLoginPasswordUserNotFoundAntiEnumeration(t *testing.T) {
	t.Run("不存在与密码错误同路径", func(t *testing.T) {
		svc, f, cap := newLoginFixture(t, loginCfg(), nil)
		body := loginBody("whatever")
		body.Username = "ghost"
		_, err := svc.LoginPassword(context.Background(), body, loginMeta(), loginClient())
		se := errs.As(err)
		if se == nil || se.Code != errs.AuthUserPasswordNotMatch.Code || se.Msg != "用户不存在/密码错误" {
			t.Fatalf("err = %v, want 与密码错误同码同文案", err)
		}
		if v, ok := f.value("pwd_err_cnt:ghost:192.168.1.10"); !ok || v != "1" {
			t.Errorf("不存在也应计数: %q/%v", v, ok)
		}
		if len(cap.entries) != 1 || cap.entries[0].Msg == nil || *cap.entries[0].Msg != "用户不存在/密码错误" {
			t.Errorf("失败日志错误: %+v", cap.entries)
		}
	})
	t.Run("空IP归一unknown", func(t *testing.T) {
		svc, f, _ := newLoginFixture(t, loginCfg(), nil)
		body := loginBody("whatever")
		body.Username = "ghost"
		_, err := svc.LoginPassword(context.Background(), body, RequestMeta{}, loginClient())
		if errs.As(err) == nil || errs.As(err).Code != errs.AuthUserPasswordNotMatch.Code {
			t.Fatalf("err = %v", err)
		}
		if _, ok := f.value("pwd_err_cnt:ghost:unknown"); !ok {
			t.Error("空 IP 的计数键应为 pwd_err_cnt:ghost:unknown")
		}
	})
}

// TestLoginPasswordRetryExceeded 达到上限即锁定（正确密码也拒绝）；
// 第 maxRetry 次失败触发锁定（newCount >= maxRetry 边界）。
func TestLoginPasswordRetryExceeded(t *testing.T) {
	t.Run("已达上限正确密码也锁定", func(t *testing.T) {
		svc, f, cap := newLoginFixture(t, loginCfg(), loginUser())
		errKey := "pwd_err_cnt:admin:192.168.1.10"
		f.seed(errKey, "5")
		_, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), loginClient())
		se := errs.As(err)
		if se == nil || se.Code != errs.AuthPasswordRetryExceeded.Code {
			t.Fatalf("err = %v, want 10006 锁定", err)
		}
		if se.MessageKey != "user.password.retry.limit.exceed" || len(se.KeyArgs) != 2 ||
			se.KeyArgs[0] != int64(5) || se.KeyArgs[1] != 10 {
			t.Errorf("i18n 键/参数错误: %q %v", se.MessageKey, se.KeyArgs)
		}
		if v, _ := f.value(errKey); v != "5" {
			t.Errorf("锁定分支不应再自增: %q", v)
		}
		if len(cap.entries) != 1 || cap.entries[0].Msg == nil || *cap.entries[0].Msg != "密码输入错误5次，帐户锁定10分钟" {
			t.Errorf("锁定日志文案错误: %+v", cap.entries)
		}
	})
	t.Run("最后一次失败触发锁定", func(t *testing.T) {
		svc, f, _ := newLoginFixture(t, loginCfg(), loginUser())
		errKey := "pwd_err_cnt:admin:192.168.1.10"
		f.seed(errKey, "4")
		_, err := svc.LoginPassword(context.Background(), loginBody("wrong-password"), loginMeta(), loginClient())
		se := errs.As(err)
		if se == nil || se.Code != errs.AuthPasswordRetryExceeded.Code {
			t.Fatalf("err = %v, want 第 5 次失败即锁定（不是 10005）", err)
		}
		if v, _ := f.value(errKey); v != "5" {
			t.Errorf("计数 = %q, want 5", v)
		}
	})
}

// TestLoginPasswordDisabledNoSideEffects 停用账号在密码校验前拒绝：
// 不计数、不写登录日志、不触达会话构建（provider 未覆盖即 panic 兜底）。
func TestLoginPasswordDisabledNoSideEffects(t *testing.T) {
	u := loginUser()
	u.Status = modelx.Disabled
	svc, f, cap := newLoginFixture(t, loginCfg(), u)
	_, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), loginClient())
	se := errs.As(err)
	if se == nil || se.Code != errs.AuthUserBlocked.Code {
		t.Fatalf("err = %v, want 10004 用户已停用", err)
	}
	if keys := f.allKeys(); len(keys) != 0 {
		t.Errorf("停用分支不应写任何缓存键: %v", keys)
	}
	if len(cap.entries) != 0 || cap.loginInfoN != 0 {
		t.Errorf("停用分支不应写日志/更新登录信息: %+v", cap)
	}
}

// TestLoginPasswordCorruptErrCount 计数值损坏 → 400 Python int() 原文
// （不是重置 0，契约比对抓出过的坑）。
func TestLoginPasswordCorruptErrCount(t *testing.T) {
	svc, f, _ := newLoginFixture(t, loginCfg(), loginUser())
	f.seed("pwd_err_cnt:admin:192.168.1.10", "abc")
	_, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), loginClient())
	se := errs.As(err)
	if se == nil || se.Code != errs.CodeBadRequest || se.Msg != "invalid literal for int() with base 10: 'abc'" {
		t.Errorf("err = %v, want 400 Python int() 原文", err)
	}
}

// ---- 验证码分支 ----

// TestLoginPasswordCaptchaOneShot 验证码一次性：无论成败先删键；
// 过期/错误各有专属码与日志文案；大小写不敏感匹配成功后继续登录。
func TestLoginPasswordCaptchaOneShot(t *testing.T) {
	newBody := func(code, uuid string) *PasswordLoginRequest {
		b := loginBody("admin123")
		b.Code = strPtr(code)
		b.UUID = strPtr(uuid)
		return b
	}
	t.Run("过期", func(t *testing.T) {
		cfg := loginCfg()
		cfg.CaptchaEnable = true
		svc, f, cap := newLoginFixture(t, cfg, loginUser())
		_, err := svc.LoginPassword(context.Background(), newBody("abc", "u1"), loginMeta(), loginClient())
		se := errs.As(err)
		if se == nil || se.Code != errs.AuthCaptchaExpired.Code {
			t.Fatalf("err = %v, want 10001 验证码已失效", err)
		}
		if _, ok := f.value("global:captcha_codes:u1"); ok {
			t.Error("验证码键应被删除")
		}
		if len(cap.entries) != 1 || cap.entries[0].Msg == nil || *cap.entries[0].Msg != "验证码已失效" {
			t.Errorf("过期日志错误: %+v", cap.entries)
		}
	})
	t.Run("错误且一次性作废", func(t *testing.T) {
		cfg := loginCfg()
		cfg.CaptchaEnable = true
		svc, f, cap := newLoginFixture(t, cfg, loginUser())
		f.seed("global:captcha_codes:u1", "AbC")
		_, err := svc.LoginPassword(context.Background(), newBody("xyz", "u1"), loginMeta(), loginClient())
		se := errs.As(err)
		if se == nil || se.Code != errs.AuthCaptchaError.Code {
			t.Fatalf("err = %v, want 10002 验证码错误", err)
		}
		if _, ok := f.value("global:captcha_codes:u1"); ok {
			t.Error("校验失败也应删除验证码（一次性凭证）")
		}
		if len(cap.entries) != 1 || cap.entries[0].Msg == nil || *cap.entries[0].Msg != "验证码错误" {
			t.Errorf("错误日志错误: %+v", cap.entries)
		}
	})
	t.Run("大小写不敏感成功后继续登录", func(t *testing.T) {
		cfg := loginCfg()
		cfg.CaptchaEnable = true
		svc, f, _ := newLoginFixture(t, cfg, loginUser())
		f.seed("global:captcha_codes:u1", "AbC")
		resp, err := svc.LoginPassword(context.Background(), newBody("abc", "u1"), loginMeta(), loginClient())
		if err != nil || resp == nil || resp.AccessToken == "" {
			t.Fatalf("resp = %+v, err = %v; want 登录成功", resp, err)
		}
		if _, ok := f.value("global:captcha_codes:u1"); ok {
			t.Error("校验成功也应删除验证码（防复用）")
		}
	})
}

// ---- 客户端超时配置分支 ----

// TestLoginPasswordClientTimeout 超时取值链：cli.Timeout → cfg 默认；
// 0/nil 都回退；两者皆无 → 500「客户端 token 超时时间配置无效」。
func TestLoginPasswordClientTimeout(t *testing.T) {
	t.Run("全缺省报配置无效", func(t *testing.T) {
		cfg := loginCfg()
		cfg.AccessTokenExpireSeconds = 0
		cli := loginClient()
		cli.Timeout, cli.ActiveTimeout = nil, nil
		svc, f, _ := newLoginFixture(t, cfg, loginUser())
		_, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), cli)
		se := errs.As(err)
		if se == nil || se.Msg != "客户端 token 超时时间配置无效" {
			t.Fatalf("err = %v, want 客户端 token 超时时间配置无效", err)
		}
		for _, k := range f.allKeys() {
			if strings.HasPrefix(k, "user_session:") || strings.HasPrefix(k, "online_tokens:") {
				t.Errorf("配置无效不应写会话键: %s", k)
			}
		}
	})
	t.Run("零值回退全局默认", func(t *testing.T) {
		cfg := loginCfg()
		cfg.AccessTokenExpireSeconds = 1234
		cli := loginClient()
		cli.Timeout = intPtr(0) // derefInt 把 0 视同缺省
		cli.ActiveTimeout = nil
		svc, _, _ := newLoginFixture(t, cfg, loginUser())
		resp, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), cli)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if resp.ExpireIn != 1234 {
			t.Errorf("ExpireIn = %d, want 回退全局 1234", resp.ExpireIn)
		}
	})
}

// TestLoginPasswordUpdateLoginInfoErrorSwallowed 更新最后登录信息失败
// 不影响登录本身（token 已发放，回滚反而更糟）。
func TestLoginPasswordUpdateLoginInfoErrorSwallowed(t *testing.T) {
	svc, _, cap := newLoginFixture(t, loginCfg(), loginUser())
	svc.repo = &mockRepo{
		updateLoginInfo: func(_ context.Context, _ int64, _ string, _ time.Time) error {
			return errors.New("db down")
		},
		insertLogininfor: func(_ context.Context, entry *logmodel.SysLogininfor) error {
			cap.entries = append(cap.entries, entry)
			return nil
		},
	}
	resp, err := svc.LoginPassword(context.Background(), loginBody("admin123"), loginMeta(), loginClient())
	if err != nil || resp == nil || resp.AccessToken == "" {
		t.Errorf("resp = %+v, err = %v; want 登录仍成功", resp, err)
	}
}
