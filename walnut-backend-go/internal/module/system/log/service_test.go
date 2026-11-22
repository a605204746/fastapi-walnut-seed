package log

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// mockRepo 嵌入 logRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	logRepo
	operLogPage           func(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysOperLog, int64, error)
	operLogListAll        func(ctx context.Context, conds []crudx.Cond) ([]SysOperLog, error)
	operLogDeleteBatch    func(ctx context.Context, ids []int64) (int64, error)
	operLogClean          func(ctx context.Context) error
	logininforPage        func(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysLogininfor, int64, error)
	logininforListAll     func(ctx context.Context, conds []crudx.Cond) ([]SysLogininfor, error)
	logininforDeleteBatch func(ctx context.Context, ids []int64) (int64, error)
	logininforClean       func(ctx context.Context) error
	dictLabelMap          func(ctx context.Context, dictType string) (map[string]string, error)
}

func (m *mockRepo) OperLogPage(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysOperLog, int64, error) {
	return m.operLogPage(ctx, page, conds)
}

func (m *mockRepo) OperLogListAll(ctx context.Context, conds []crudx.Cond) ([]SysOperLog, error) {
	return m.operLogListAll(ctx, conds)
}

func (m *mockRepo) OperLogDeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.operLogDeleteBatch(ctx, ids)
}

func (m *mockRepo) OperLogClean(ctx context.Context) error {
	return m.operLogClean(ctx)
}

func (m *mockRepo) LogininforPage(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysLogininfor, int64, error) {
	return m.logininforPage(ctx, page, conds)
}

func (m *mockRepo) LogininforListAll(ctx context.Context, conds []crudx.Cond) ([]SysLogininfor, error) {
	return m.logininforListAll(ctx, conds)
}

func (m *mockRepo) LogininforDeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.logininforDeleteBatch(ctx, ids)
}

func (m *mockRepo) LogininforClean(ctx context.Context) error {
	return m.logininforClean(ctx)
}

func (m *mockRepo) DictLabelMap(ctx context.Context, dictType string) (map[string]string, error) {
	return m.dictLabelMap(ctx, dictType)
}

func ptrOf(s string) *string { return &s }

func ptrInt(v int) *int { return &v }

func ptrInt64(v int64) *int64 { return &v }

// TestOperLogExportRowsDictLabels 钉住导出的字典翻译纯逻辑：
// 命中 → 标签；未命中 → 回退数字本身；nil → 空单元格；operatorType 走内置标签。
func TestOperLogExportRowsDictLabels(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	bt1, op1, st0 := 1, 1, 0
	bt9 := 9
	hit := SysOperLog{BusinessType: &bt1, OperatorType: &op1, Status: &st0}
	hit.ID = 1
	miss := SysOperLog{BusinessType: &bt9}
	miss.ID = 2
	svc.repo = &mockRepo{
		operLogListAll: func(_ context.Context, _ []crudx.Cond) ([]SysOperLog, error) {
			return []SysOperLog{hit, miss}, nil
		},
		dictLabelMap: func(_ context.Context, dictType string) (map[string]string, error) {
			switch dictType {
			case "sys_oper_type":
				return map[string]string{"1": "新增"}, nil
			case "sys_common_status":
				return map[string]string{"0": "成功"}, nil
			}
			t.Errorf("意外的字典类型: %s", dictType)
			return nil, nil
		},
	}
	rows, err := svc.OperLogExportRows(context.Background(), OperLogFilterParam{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	if rows[0]["businessType"] != "新增" || rows[0]["status"] != "成功" || rows[0]["operatorType"] != "后台用户" {
		t.Errorf("字典命中行翻译错误: %v", rows[0])
	}
	if rows[1]["businessType"] != 9 {
		t.Errorf("未命中应回退数字本身, got %v (%T)", rows[1]["businessType"], rows[1]["businessType"])
	}
	if rows[1]["status"] != nil || rows[1]["operatorType"] != nil {
		t.Errorf("nil 列应保持空单元格: %v", rows[1])
	}
}

// ---- 新增：导出字典翻译补充 ----

// TestOperLogExportRowsDictErrorSwallowed 字典查询出错被静默吞掉（Python try/except），
// 翻译回退数字本身；operTime 格式化、costTime 原样、operatorType 内置标签全覆盖。
func TestOperLogExportRowsDictErrorSwallowed(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	opTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	bt2, op2, op9, st1 := 2, 2, 9, 1
	cost := int64(123)
	row := SysOperLog{
		Title: ptrOf("用户管理"), BusinessType: &bt2, OperatorType: &op2, Status: &st1,
		OperTime: &opTime, CostTime: &cost,
	}
	row.ID = 1
	unknown := SysOperLog{OperatorType: &op9}
	unknown.ID = 2
	svc.repo = &mockRepo{
		operLogListAll: func(_ context.Context, _ []crudx.Cond) ([]SysOperLog, error) {
			return []SysOperLog{row, unknown}, nil
		},
		dictLabelMap: func(_ context.Context, _ string) (map[string]string, error) {
			return nil, errors.New("dict down")
		},
	}
	rows, err := svc.OperLogExportRows(context.Background(), OperLogFilterParam{})
	if err != nil {
		t.Fatalf("字典出错不应让导出失败: %v", err)
	}
	if rows[0]["businessType"] != 2 || rows[0]["status"] != 1 {
		t.Errorf("字典不可用应回退数字: %v", rows[0])
	}
	if rows[0]["operatorType"] != "手机端用户" || rows[1]["operatorType"] != 9 {
		t.Errorf("operatorType 内置标签错误: %v / %v", rows[0]["operatorType"], rows[1]["operatorType"])
	}
	if rows[0]["operTime"] != "2026-01-02 03:04:05" || rows[0]["costTime"] != &cost {
		t.Errorf("operTime/costTime 组装错误: %v / %v", rows[0]["operTime"], rows[0]["costTime"])
	}
	if rows[1]["operTime"] != "" {
		t.Errorf("nil 时间应导出空串: %v", rows[1]["operTime"])
	}
}

// TestLogininforExportRowsDictLabels 登录日志导出：device/status 字典翻译
// （命中→标签、未命中→原值、nil/空串→空单元格），loginTime 格式化。
func TestLogininforExportRowsDictLabels(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	loginTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	hit := SysLogininfor{UserName: ptrOf("admin"), DeviceType: ptrOf("pc"), Status: ptrOf("0"), LoginTime: &loginTime}
	hit.ID = 1
	miss := SysLogininfor{DeviceType: ptrOf("tv"), Status: ptrOf("9")}
	miss.ID = 2
	empty := SysLogininfor{DeviceType: ptrOf(""), Status: nil}
	empty.ID = 3
	svc.repo = &mockRepo{
		logininforListAll: func(_ context.Context, _ []crudx.Cond) ([]SysLogininfor, error) {
			return []SysLogininfor{hit, miss, empty}, nil
		},
		dictLabelMap: func(_ context.Context, dictType string) (map[string]string, error) {
			switch dictType {
			case "sys_device_type":
				return map[string]string{"pc": "电脑"}, nil
			case "sys_common_status":
				return map[string]string{"0": "成功"}, nil
			}
			t.Errorf("意外的字典类型: %s", dictType)
			return nil, nil
		},
	}
	rows, err := svc.LogininforExportRows(context.Background(), LogininforFilterParam{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rows[0]["deviceType"] != "电脑" || rows[0]["status"] != "成功" || rows[0]["loginTime"] != "2026-01-02 03:04:05" {
		t.Errorf("命中行翻译错误: %v", rows[0])
	}
	if rows[1]["deviceType"] != "tv" || rows[1]["status"] != "9" {
		t.Errorf("未命中应回退原值: %v", rows[1])
	}
	if rows[2]["deviceType"] != "" || rows[2]["status"] != "" || rows[2]["loginTime"] != "" {
		t.Errorf("nil/空串应保持空单元格: %v", rows[2])
	}
}

// ---- 新增：分页转换与条件 ----

// TestOperLogPageConversion 分页出参：int 列转 JSInt、operTime 转字符串、total 透传。
func TestOperLogPageConversion(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	opTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	bt1, op0, st0 := 1, 0, 0
	cost := int64(88)
	row := SysOperLog{Title: ptrOf("t"), BusinessType: &bt1, OperatorType: &op0, Status: &st0, OperTime: &opTime, CostTime: &cost}
	row.ID = 9
	var gotPage crudx.Page
	svc.repo = &mockRepo{operLogPage: func(_ context.Context, page crudx.Page, _ []crudx.Cond) ([]SysOperLog, int64, error) {
		gotPage = page
		return []SysOperLog{row}, 55, nil
	}}
	size := 10
	out, err := svc.OperLogPage(context.Background(), OperLogQueryParam{Page: crudx.Page{PageNum: 1, PageSize: &size}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if int64(out.Total) != 55 || len(out.Rows) != 1 {
		t.Fatalf("out = %+v", out)
	}
	r := out.Rows[0]
	if r.ID == nil || int64(*r.ID) != 9 || r.BusinessType == nil || int64(*r.BusinessType) != 1 ||
		r.OperatorType == nil || int64(*r.OperatorType) != 0 || r.Status == nil || int64(*r.Status) != 0 ||
		r.CostTime == nil || int64(*r.CostTime) != 88 {
		t.Errorf("数值列转换错误: %+v", r)
	}
	if r.OperTime == nil || *r.OperTime != "2026-01-02 03:04:05" {
		t.Errorf("operTime = %v, want 格式化字符串", r.OperTime)
	}
	if gotPage.PageSize == nil || *gotPage.PageSize != 10 {
		t.Errorf("page = %+v, want 原样透传", gotPage)
	}
}

// TestLogininforPageConversion 登录日志分页：status 保持字符串、loginTime 格式化。
func TestLogininforPageConversion(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	loginTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	row := SysLogininfor{UserName: ptrOf("admin"), Status: ptrOf("1"), Msg: ptrOf("用户不存在/密码错误"), LoginTime: &loginTime}
	row.ID = 3
	svc.repo = &mockRepo{logininforPage: func(_ context.Context, _ crudx.Page, _ []crudx.Cond) ([]SysLogininfor, int64, error) {
		return []SysLogininfor{row}, 1, nil
	}}
	out, err := svc.LogininforPage(context.Background(), LogininforQueryParam{})
	if err != nil || int64(out.Total) != 1 || len(out.Rows) != 1 {
		t.Fatalf("out = %+v, err = %v", out, err)
	}
	r := out.Rows[0]
	if r.Status == nil || *r.Status != "1" || r.LoginTime == nil || *r.LoginTime != "2026-01-02 03:04:05" {
		t.Errorf("转换错误: %+v", r)
	}
	if r.Msg == nil || *r.Msg != "用户不存在/密码错误" {
		t.Errorf("msg = %v", r.Msg)
	}
}

// TestBuildOperLogConditions 条件序与语义：title/operName/operIp 模糊、
// businessType>0 才等值、businessTypes IN、status=0 也施加、时间区间右端 +24h-1s。
func TestBuildOperLogConditions(t *testing.T) {
	begin := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	bt0 := int64(0)
	st0 := int64(0)
	q := OperLogQueryParam{Filter: OperLogFilterParam{
		Title: "用户", OperName: "admin", OperIP: "127.", BusinessType: &bt0,
		BusinessTypes: []int64{1, 2}, Status: &st0, BeginTime: &begin, EndTime: &end,
	}}
	conds := buildOperLogConditions(q)
	wantSQL := []string{"title LIKE ?", "oper_name LIKE ?", "oper_ip LIKE ?", "business_type IN ?", "status = ?", "oper_time BETWEEN ? AND ?"}
	if len(conds) != len(wantSQL) {
		t.Fatalf("conds = %+v, want %d 条（businessType=0 不施加）", conds, len(wantSQL))
	}
	for i, w := range wantSQL {
		if conds[i].SQL != w {
			t.Errorf("cond[%d].SQL = %q, want %q", i, conds[i].SQL, w)
		}
	}
	if conds[4].Args[0] != int64(0) {
		t.Errorf("status=0 应施加等值: %+v", conds[4])
	}
	wantEnd := end.Add(24*time.Hour - time.Second)
	if conds[5].Args[1] != wantEnd {
		t.Errorf("区间右端 = %v, want %v（+24h-1s）", conds[5].Args[1], wantEnd)
	}

	// 单端时间不施加 BETWEEN；businessType>0 施加等值
	bt2 := int64(2)
	conds = buildOperLogConditions(OperLogQueryParam{Filter: OperLogFilterParam{BusinessType: &bt2, BeginTime: &begin}})
	if len(conds) != 1 || conds[0].SQL != "business_type = ?" || conds[0].Args[0] != int64(2) {
		t.Errorf("conds = %+v, want 仅 business_type = 2", conds)
	}
}

// TestBuildLogininforConditions 登录日志条件：ipaddr 模糊、status 等值、userName 模糊、区间。
func TestBuildLogininforConditions(t *testing.T) {
	begin := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	conds := buildLogininforConditions(LogininforQueryParam{Filter: LogininforFilterParam{
		IPAddr: "192.", Status: "0", UserName: "adm", BeginTime: &begin, EndTime: &end,
	}})
	wantSQL := []string{"ipaddr LIKE ?", "status = ?", "user_name LIKE ?", "login_time BETWEEN ? AND ?"}
	if len(conds) != len(wantSQL) {
		t.Fatalf("conds = %+v", conds)
	}
	for i, w := range wantSQL {
		if conds[i].SQL != w {
			t.Errorf("cond[%d].SQL = %q, want %q", i, conds[i].SQL, w)
		}
	}
	if conds[0].Args[0] != "%192.%" || conds[2].Args[0] != "%adm%" {
		t.Errorf("like 应包 %%: %+v", conds)
	}
	if len(buildLogininforConditions(LogininforQueryParam{})) != 0 {
		t.Error("空过滤不应产生条件")
	}
}

// ---- 新增：删除/清空透传 ----

func TestLogDeleteAndCleanPassthrough(t *testing.T) {
	svc := NewService(nil, authctx.Context{})
	var gotIDs []int64
	boom := errors.New("boom")
	svc.repo = &mockRepo{
		operLogDeleteBatch: func(_ context.Context, ids []int64) (int64, error) {
			gotIDs = ids
			return 2, nil
		},
		operLogClean: func(_ context.Context) error { return boom },
		logininforDeleteBatch: func(_ context.Context, ids []int64) (int64, error) {
			gotIDs = ids
			return 3, nil
		},
		logininforClean: func(_ context.Context) error { return nil },
	}
	if n, err := svc.OperLogDelete(context.Background(), []int64{1, 2}); err != nil || n != 2 || len(gotIDs) != 2 {
		t.Errorf("OperLogDelete: n=%d err=%v ids=%v", n, err, gotIDs)
	}
	if err := svc.OperLogClean(context.Background()); !errors.Is(err, boom) {
		t.Errorf("OperLogClean err = %v, want 原错误", err)
	}
	if n, err := svc.LogininforDelete(context.Background(), []int64{4, 5, 6}); err != nil || n != 3 || len(gotIDs) != 3 {
		t.Errorf("LogininforDelete: n=%d err=%v ids=%v", n, err, gotIDs)
	}
	if err := svc.LogininforClean(context.Background()); err != nil {
		t.Errorf("LogininforClean err = %v", err)
	}
}

// ---- 内存 Redis 假件（go-redis hook 拦截命令，无网络依赖） ----
//
// 解锁端点（handler 层）直接操作缓存键，这里以 hook 假件钉住
// 「pwd_err_cnt:{userName}:* 模式匹配 + 逐键删除」的键前缀契约。

type fakeRedis struct {
	mu   sync.Mutex
	vals map[string]string
	ttls map[string]time.Duration
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

func (f *fakeRedis) seed(key, val string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vals[key] = val
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

// ---- 新增：登录解锁（handler 层缓存契约，经 service_test 覆盖） ----

// TestLogininforUnlock 解锁按「pwd_err_cnt:{userName}:*」模式删键：
// 键前缀必须与 auth 写入侧一致（无 global: 段），只清目标用户、不误伤他人。
func TestLogininforUnlock(t *testing.T) {
	cache, f := newFakeCache(t, "walnut-seed-go")
	ctx := context.Background()
	// 经 cachex.Set 写入（自动套根前缀），模拟 auth 登录失败计数
	cache.Set(ctx, "pwd_err_cnt:admin:1.1.1.1", "3", 10*time.Minute)
	cache.Set(ctx, "pwd_err_cnt:admin:2.2.2.2", "5", 10*time.Minute)
	cache.Set(ctx, "pwd_err_cnt:bob:1.1.1.1", "1", 10*time.Minute)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/monitor/logininfor/unlock/admin", nil)
	c.Params = gin.Params{{Key: "user_name", Value: "admin"}}
	h := &Handler{cache: cache, errs: &errs.Handler{}}
	h.LogininforUnlock(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":200`) {
		t.Errorf("响应 = %d %s, want 200 成功信封", w.Code, w.Body.String())
	}
	if _, ok := f.value("walnut-seed-go:pwd_err_cnt:admin:1.1.1.1"); ok {
		t.Error("admin 的失败计数键应被删除")
	}
	if _, ok := f.value("walnut-seed-go:pwd_err_cnt:admin:2.2.2.2"); ok {
		t.Error("admin 的所有 IP 组合键都应被删除")
	}
	if _, ok := f.value("walnut-seed-go:pwd_err_cnt:bob:1.1.1.1"); !ok {
		t.Error("其他用户的计数键不应被误删")
	}
}

// TestLogininforUnlockEmptyUsername 用户名为空 → 400「用户名不能为空」，不触达缓存。
func TestLogininforUnlockEmptyUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/monitor/logininfor/unlock/", nil)
	c.Params = gin.Params{{Key: "user_name", Value: ""}}
	h := &Handler{errs: &errs.Handler{}} // cache=nil：触达即 panic
	h.LogininforUnlock(c)
	if !strings.Contains(w.Body.String(), "用户名不能为空") || !strings.Contains(w.Body.String(), `"code":400`) {
		t.Errorf("响应 = %s, want 400 用户名不能为空", w.Body.String())
	}
}
