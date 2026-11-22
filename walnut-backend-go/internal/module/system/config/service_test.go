package config

import (
	"context"
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

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// mockRepo 嵌入 configRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	configRepo
	page            func(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysConfig, int64, error)
	listAll         func(ctx context.Context, conds ...crudx.Cond) ([]SysConfig, error)
	get             func(ctx context.Context, id int64) (*SysConfig, error)
	getByKey        func(ctx context.Context, key string) (*SysConfig, error)
	existsByKey     func(ctx context.Context, key string, excludeID *int64) (bool, error)
	create          func(ctx context.Context, row *SysConfig) error
	update          func(ctx context.Context, row *SysConfig) error
	updateByKeyBulk func(ctx context.Context, key string, values map[string]any, uid int64, now time.Time) (int64, error)
	deleteBatch     func(ctx context.Context, ids []int64) (int64, error)
}

func (m *mockRepo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysConfig, int64, error) {
	return m.page(ctx, page, conds)
}

func (m *mockRepo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysConfig, error) {
	return m.listAll(ctx, conds...)
}

func (m *mockRepo) Get(ctx context.Context, id int64) (*SysConfig, error) {
	return m.get(ctx, id)
}

func (m *mockRepo) GetByKey(ctx context.Context, key string) (*SysConfig, error) {
	return m.getByKey(ctx, key)
}

func (m *mockRepo) ExistsByKey(ctx context.Context, key string, excludeID *int64) (bool, error) {
	return m.existsByKey(ctx, key, excludeID)
}

func (m *mockRepo) Create(ctx context.Context, row *SysConfig) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysConfig) error {
	return m.update(ctx, row)
}

func (m *mockRepo) UpdateByKeyBulk(ctx context.Context, key string, values map[string]any, uid int64, now time.Time) (int64, error) {
	return m.updateByKeyBulk(ctx, key, values, uid, now)
}

func (m *mockRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.deleteBatch(ctx, ids)
}

func testAuth(id int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: id, UserName: "tester"}}
}

// pinNow 钉住 nowFunc，便于断言审计时间字段。
func pinNow(t *testing.T, fixed time.Time) {
	t.Helper()
	old := nowFunc
	nowFunc = func() time.Time { return fixed }
	t.Cleanup(func() { nowFunc = old })
}

// ---- 内存 Redis 假件（go-redis hook 拦截命令，无网络依赖） ----
//
// cachex.Client 是具体结构体（内嵌 *redis.Client），无法以接口 mock；
// 这里用 AddHook 在命令层拦截，完整复刻 get/set/del/incr/expire/scan/keys/ttl
// 的语义（含 TTL 记录），让缓存回填/删键契约可离线断言。

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

// value 断言用：读当前值（含惰性过期）。
func (f *fakeRedis) value(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.get(key)
}

// ttl 断言用：最近一次写入请求的 TTL。
func (f *fakeRedis) ttl(key string) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ttls[key]
}

// seed 预置键值（无 TTL）。
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

// ---- 既有守卫用例 ----

func TestCheckKeyUniqueNilGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	err := svc.CheckKeyUnique(context.Background(), nil, nil)
	if se := errs.As(err); se == nil || se.Msg != "参数键名不能为空" {
		t.Errorf("err = %v, want 参数键名不能为空", err)
	}
}

func TestInsertMissingValueGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	err := svc.Insert(context.Background(), CreateIn{ConfigKey: strPtr("k")})
	if se := errs.As(err); se == nil || se.Msg != "参数键值不能为空" {
		t.Errorf("err = %v, want 参数键值不能为空", err)
	}
}

func TestDeleteBuiltinGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	svc.repo = &mockRepo{listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysConfig, error) {
		return []SysConfig{{ConfigType: strPtr("Y"), ConfigKey: strPtr("sys.k")}}, nil
	}}
	err := svc.Delete(context.Background(), []int64{1})
	if se := errs.As(err); se == nil || se.Msg != "内置参数【sys.k】不能删除" {
		t.Errorf("err = %v, want 内置参数不能删除", err)
	}
}

// ---- 新增：Insert 主流程 ----

// TestInsertSuccessDefaultsAndCache 钉住新增成功路径：
// 缺省字段走列默认值（name=""、type="N"、remark=NULL）、审计填充、
// 缓存以**裸值字符串**回填（不是 JSON 信封）。
func TestInsertSuccessDefaultsAndCache(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pinNow(t, fixed)
	cache, f := newFakeCache(t, "")
	svc := NewService(nil, testAuth(9), cache)
	var got *SysConfig
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysConfig) error {
		got = row
		return nil
	}}
	err := svc.Insert(context.Background(), CreateIn{ConfigKey: strPtr("sys.k"), ConfigValue: strPtr("v")})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil {
		t.Fatal("Create 未被调用")
	}
	if got.ID == 0 {
		t.Error("ID 应由 FillCreate 填雪花值")
	}
	if got.ConfigName == nil || *got.ConfigName != "" || got.ConfigType == nil || *got.ConfigType != "N" || got.Remark != nil {
		t.Errorf("缺省字段错误: name=%v type=%v remark=%v", got.ConfigName, got.ConfigType, got.Remark)
	}
	if got.CreateBy == nil || *got.CreateBy != 9 || got.UpdateBy == nil || *got.UpdateBy != 9 {
		t.Errorf("审计用户错误: %v / %v", got.CreateBy, got.UpdateBy)
	}
	if got.CreateTime == nil || !got.CreateTime.Equal(fixed) || got.UpdateTime == nil || !got.UpdateTime.Equal(fixed) {
		t.Errorf("审计时间错误: %v / %v", got.CreateTime, got.UpdateTime)
	}
	if v, ok := f.value("sys_config:sys.k"); !ok || v != "v" {
		t.Errorf("缓存 = %q/%v, want 裸值 v", v, ok)
	}
	if f.ttl("sys_config:sys.k") != 0 {
		t.Errorf("缓存 TTL = %v, want 永不过期", f.ttl("sys_config:sys.k"))
	}
}

// TestInsertExplicitFields 显式字段原样落库（含内置标记 Y 与备注）。
func TestInsertExplicitFields(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	var got *SysConfig
	svc.repo = &mockRepo{create: func(_ context.Context, row *SysConfig) error {
		got = row
		return nil
	}}
	in := CreateIn{
		ConfigName: strPtr("名称"), ConfigKey: strPtr("k"), ConfigValue: strPtr("v"),
		ConfigType: strPtr("Y"), Remark: strPtr("备注"),
	}
	if err := svc.Insert(context.Background(), in); err != nil {
		t.Fatalf("err = %v", err)
	}
	if *got.ConfigName != "名称" || *got.ConfigType != "Y" || *got.Remark != "备注" {
		t.Errorf("显式字段未原样落库: %+v", got)
	}
}

func TestInsertMissingKeyGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	err := svc.Insert(context.Background(), CreateIn{ConfigValue: strPtr("v")})
	if se := errs.As(err); se == nil || se.Msg != "参数键名不能为空" {
		t.Errorf("err = %v, want 参数键名不能为空", err)
	}
}

// TestInsertDBErrorNoCacheWrite 建库失败 → 500「数据库操作失败」，且不回填缓存。
func TestInsertDBErrorNoCacheWrite(t *testing.T) {
	cache, f := newFakeCache(t, "")
	svc := NewService(nil, testAuth(1), cache)
	svc.repo = &mockRepo{create: func(_ context.Context, _ *SysConfig) error {
		return errors.New("boom")
	}}
	err := svc.Insert(context.Background(), CreateIn{ConfigKey: strPtr("k"), ConfigValue: strPtr("v")})
	if se := errs.As(err); se == nil || se.Code != errs.CodeError || se.Msg != errs.MsgDBFailed {
		t.Errorf("err = %v, want 数据库操作失败", err)
	}
	if _, ok := f.value("sys_config:k"); ok {
		t.Error("失败时不应回填缓存")
	}
}

// ---- 新增：唯一性校验 ----

func TestCheckKeyUniqueBranches(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	var gotKey string
	var gotExclude *int64
	svc.repo = &mockRepo{existsByKey: func(_ context.Context, key string, excludeID *int64) (bool, error) {
		gotKey, gotExclude = key, excludeID
		return false, nil
	}}
	exclude := int64(5)
	if err := svc.CheckKeyUnique(context.Background(), strPtr("k"), &exclude); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotKey != "k" || gotExclude == nil || *gotExclude != 5 {
		t.Errorf("入参透传错误: key=%q exclude=%v", gotKey, gotExclude)
	}

	svc.repo = &mockRepo{existsByKey: func(_ context.Context, _ string, _ *int64) (bool, error) {
		return true, nil
	}}
	err := svc.CheckKeyUnique(context.Background(), strPtr("k"), nil)
	if se := errs.As(err); se == nil || se.Msg != "参数键名已存在" {
		t.Errorf("err = %v, want 参数键名已存在", err)
	}

	svc.repo = &mockRepo{existsByKey: func(_ context.Context, _ string, _ *int64) (bool, error) {
		return false, errors.New("boom")
	}}
	err = svc.CheckKeyUnique(context.Background(), strPtr("k"), nil)
	if se := errs.As(err); se == nil || se.Msg != errs.MsgDBFailed {
		t.Errorf("err = %v, want 数据库操作失败", err)
	}
}

// ---- 新增：Update 主流程 ----

// TestUpdateByIDRenameCache 按主键修改：改键名先删旧键缓存、写后回填新键；
// 仅覆盖非空字段；FillUpdate 无条件覆盖 update_by/update_time。
func TestUpdateByIDRenameCache(t *testing.T) {
	fixed := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	pinNow(t, fixed)
	cache, f := newFakeCache(t, "")
	f.seed("sys_config:old", "stale")
	svc := NewService(nil, testAuth(8), cache)
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	old := &SysConfig{ConfigName: strPtr("旧名"), ConfigKey: strPtr("old"), ConfigValue: strPtr("oldv"), ConfigType: strPtr("N")}
	old.ID = 3
	old.CreateTime = &created
	var updated *SysConfig
	svc.repo = &mockRepo{
		get: func(_ context.Context, id int64) (*SysConfig, error) {
			if id != 3 {
				t.Errorf("id = %d, want 3", id)
			}
			return old, nil
		},
		update: func(_ context.Context, row *SysConfig) error {
			updated = row
			return nil
		},
	}
	id := int64(3)
	err := svc.Update(context.Background(), updateParam{ID: &id, ConfigKey: strPtr("new"), ConfigValue: strPtr("newv")})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := f.value("sys_config:old"); ok {
		t.Error("改键名应删除旧键缓存")
	}
	if v, ok := f.value("sys_config:new"); !ok || v != "newv" {
		t.Errorf("新键缓存 = %q/%v, want newv", v, ok)
	}
	if *updated.ConfigKey != "new" || *updated.ConfigValue != "newv" {
		t.Errorf("键值未更新: %+v", updated)
	}
	// 未提供的字段保持原值
	if *updated.ConfigName != "旧名" || *updated.ConfigType != "N" {
		t.Errorf("未提供字段被误改: %+v", updated)
	}
	if updated.UpdateBy == nil || *updated.UpdateBy != 8 || !updated.UpdateTime.Equal(fixed) {
		t.Errorf("更新审计错误: %v / %v", updated.UpdateBy, updated.UpdateTime)
	}
	if !updated.CreateTime.Equal(created) {
		t.Error("create_time 不应被更新覆盖")
	}
}

func TestUpdateByIDNotFound(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysConfig, error) {
		return nil, nil
	}}
	id := int64(404)
	err := svc.Update(context.Background(), updateParam{ID: &id, ConfigKey: strPtr("k")})
	if se := errs.As(err); se == nil || se.Msg != "操作失败" {
		t.Errorf("err = %v, want 操作失败", err)
	}
}

// TestUpdateByKeyBulk 无 id 走批量：先删键缓存 → 仅非空字段进 values →
// uid/now 透传 → 命中后回填缓存；未命中（n=0）报「操作失败」且不回填。
func TestUpdateByKeyBulk(t *testing.T) {
	fixed := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	pinNow(t, fixed)
	cache, f := newFakeCache(t, "")
	f.seed("sys_config:k", "stale")
	svc := NewService(nil, testAuth(6), cache)
	var gotKey string
	var gotValues map[string]any
	var gotUID int64
	var gotNow time.Time
	svc.repo = &mockRepo{updateByKeyBulk: func(_ context.Context, key string, values map[string]any, uid int64, now time.Time) (int64, error) {
		gotKey, gotValues, gotUID, gotNow = key, values, uid, now
		return 2, nil
	}}
	err := svc.Update(context.Background(), updateParam{ConfigKey: strPtr("k"), ConfigValue: strPtr("v"), Remark: strPtr("r")})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotKey != "k" || gotUID != 6 || !gotNow.Equal(fixed) {
		t.Errorf("批量入参错误: key=%q uid=%d now=%v", gotKey, gotUID, gotNow)
	}
	if len(gotValues) != 3 || gotValues["config_key"] != "k" || gotValues["config_value"] != "v" || gotValues["remark"] != "r" {
		t.Errorf("values 应只含非空字段: %v", gotValues)
	}
	if v, ok := f.value("sys_config:k"); !ok || v != "v" {
		t.Errorf("缓存 = %q/%v, want 回填 v", v, ok)
	}

	// 未命中 → 操作失败，缓存保持已删状态（不回填）
	svc.repo = &mockRepo{updateByKeyBulk: func(_ context.Context, _ string, _ map[string]any, _ int64, _ time.Time) (int64, error) {
		return 0, nil
	}}
	err = svc.Update(context.Background(), updateParam{ConfigKey: strPtr("k"), ConfigValue: strPtr("v2")})
	if se := errs.As(err); se == nil || se.Msg != "操作失败" {
		t.Errorf("err = %v, want 操作失败", err)
	}
	if _, ok := f.value("sys_config:k"); ok {
		t.Error("n=0 时不应回填缓存")
	}
}

// TestUpdateByKeyBulkDefaultUID 未登录（uid=0）时批量更新用 DefaultUserID(-1)。
func TestUpdateByKeyBulkDefaultUID(t *testing.T) {
	svc := NewService(nil, authctx.Context{}, nil)
	var gotUID int64
	svc.repo = &mockRepo{updateByKeyBulk: func(_ context.Context, _ string, _ map[string]any, uid int64, _ time.Time) (int64, error) {
		gotUID = uid
		return 1, nil
	}}
	err := svc.Update(context.Background(), updateParam{ConfigKey: strPtr("k"), ConfigValue: strPtr("v")})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotUID != crudx.DefaultUserID {
		t.Errorf("uid = %d, want %d", gotUID, crudx.DefaultUserID)
	}
}

// TestUpdateNilValueSkipsCacheSet 值为 None 时 Python redis-py 拒绝写入被静默吞掉
// → Go 侧等价跳过：affected>0 但 ConfigValue=nil 不回填缓存。
func TestUpdateNilValueSkipsCacheSet(t *testing.T) {
	cache, f := newFakeCache(t, "")
	svc := NewService(nil, testAuth(1), cache)
	svc.repo = &mockRepo{updateByKeyBulk: func(_ context.Context, _ string, _ map[string]any, _ int64, _ time.Time) (int64, error) {
		return 1, nil
	}}
	err := svc.Update(context.Background(), updateParam{ConfigKey: strPtr("k"), ConfigName: strPtr("n")})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := f.value("sys_config:k"); ok {
		t.Error("ConfigValue=nil 时不应写缓存")
	}
}

func TestUpdateMissingKeyGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	err := svc.Update(context.Background(), updateParam{ConfigValue: strPtr("v")})
	if se := errs.As(err); se == nil || se.Msg != "参数键名不能为空" {
		t.Errorf("err = %v, want 参数键名不能为空", err)
	}
}

// ---- 新增：Delete 主流程 ----

// TestDeleteSuccessCachePerRow 非内置批量删除：逐条删缓存后 DeleteBatch 原样透传 ids。
func TestDeleteSuccessCachePerRow(t *testing.T) {
	cache, f := newFakeCache(t, "")
	f.seed("sys_config:a", "1")
	f.seed("sys_config:b", "2")
	f.seed("other:k", "3")
	svc := NewService(nil, testAuth(1), cache)
	var gotIDs []int64
	svc.repo = &mockRepo{
		listAll: func(_ context.Context, conds ...crudx.Cond) ([]SysConfig, error) {
			if len(conds) != 1 || conds[0].SQL != "id IN ?" {
				t.Errorf("conds = %+v, want id IN ?", conds)
			}
			return []SysConfig{
				{ConfigType: strPtr("N"), ConfigKey: strPtr("a")},
				{ConfigType: strPtr("N"), ConfigKey: strPtr("b")},
			}, nil
		},
		deleteBatch: func(_ context.Context, ids []int64) (int64, error) {
			gotIDs = ids
			return 2, nil
		},
	}
	if err := svc.Delete(context.Background(), []int64{1, 2}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(gotIDs) != 2 || gotIDs[0] != 1 || gotIDs[1] != 2 {
		t.Errorf("ids = %v, want [1 2]", gotIDs)
	}
	if _, ok := f.value("sys_config:a"); ok {
		t.Error("a 键缓存应被删除")
	}
	if _, ok := f.value("sys_config:b"); ok {
		t.Error("b 键缓存应被删除")
	}
	if _, ok := f.value("other:k"); !ok {
		t.Error("无关键不应被误删")
	}
}

// TestDeleteBuiltinNoRollback 内置参数拒绝删除时，抛错前已删的缓存**不回滚**
// （Python 端同样如此），且 DeleteBatch 不被触达（未覆盖即 panic）。
func TestDeleteBuiltinNoRollback(t *testing.T) {
	cache, f := newFakeCache(t, "")
	f.seed("sys_config:free", "1")
	f.seed("sys_config:builtin", "2")
	svc := NewService(nil, testAuth(1), cache)
	svc.repo = &mockRepo{listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysConfig, error) {
		return []SysConfig{
			{ConfigType: strPtr("N"), ConfigKey: strPtr("free")},
			{ConfigType: strPtr("Y"), ConfigKey: strPtr("builtin")},
		}, nil
	}}
	err := svc.Delete(context.Background(), []int64{1, 2})
	if se := errs.As(err); se == nil || se.Msg != "内置参数【builtin】不能删除" {
		t.Errorf("err = %v, want 内置参数【builtin】不能删除", err)
	}
	if _, ok := f.value("sys_config:free"); ok {
		t.Error("抛错前已删的 free 键不应回滚（照抄 Python）")
	}
	if _, ok := f.value("sys_config:builtin"); !ok {
		t.Error("内置键缓存尚未轮到删除，应保留")
	}
}

// ---- 新增：GetByKey 缓存语义 ----

func TestGetByKeyBranches(t *testing.T) {
	cache, f := newFakeCache(t, "")
	svc := NewService(nil, testAuth(1), cache)

	// 命中裸值：直接返回，不查库（getByKey 未覆盖，触达即 panic）
	f.seed("sys_config:raw", "plain")
	if v, err := svc.GetByKey(context.Background(), "raw"); err != nil || v != "plain" {
		t.Errorf("raw = %q, err = %v; want plain", v, err)
	}

	// 命中带引号 JSON 字符串（历史写入形态）：解包
	f.seed("sys_config:quoted", `"hello"`)
	if v, err := svc.GetByKey(context.Background(), "quoted"); err != nil || v != "hello" {
		t.Errorf("quoted = %q, err = %v; want hello（解包）", v, err)
	}

	// 未命中 → 查库 → 回填缓存
	svc.repo = &mockRepo{getByKey: func(_ context.Context, key string) (*SysConfig, error) {
		if key != "miss" {
			t.Errorf("key = %s, want miss", key)
		}
		return &SysConfig{ConfigValue: strPtr("dbv")}, nil
	}}
	if v, err := svc.GetByKey(context.Background(), "miss"); err != nil || v != "dbv" {
		t.Errorf("miss = %q, err = %v; want dbv", v, err)
	}
	if v, ok := f.value("sys_config:miss"); !ok || v != "dbv" {
		t.Errorf("回填缓存 = %q/%v, want dbv", v, ok)
	}

	// 行不存在 / 值为 NULL → 空串（不是 null），同样回填
	svc.repo = &mockRepo{getByKey: func(_ context.Context, _ string) (*SysConfig, error) {
		return nil, nil
	}}
	if v, err := svc.GetByKey(context.Background(), "gone"); err != nil || v != "" {
		t.Errorf("gone = %q, err = %v; want 空串", v, err)
	}
	if v, ok := f.value("sys_config:gone"); !ok || v != "" {
		t.Errorf("空串也应回填: %q/%v", v, ok)
	}

	// 查库出错 → 500 数据库操作失败
	svc.repo = &mockRepo{getByKey: func(_ context.Context, _ string) (*SysConfig, error) {
		return nil, errors.New("boom")
	}}
	if _, err := svc.GetByKey(context.Background(), "err"); errs.As(err) == nil || errs.As(err).Msg != errs.MsgDBFailed {
		t.Errorf("err = %v, want 数据库操作失败", err)
	}
}

// TestGetByKeyNoCacheDegrade cache=nil 时降级直查库，不 panic。
func TestGetByKeyNoCacheDegrade(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	svc.repo = &mockRepo{getByKey: func(_ context.Context, _ string) (*SysConfig, error) {
		return &SysConfig{ConfigValue: strPtr("v")}, nil
	}}
	if v, err := svc.GetByKey(context.Background(), "k"); err != nil || v != "v" {
		t.Errorf("v = %q, err = %v; want v", v, err)
	}
}

// ---- 新增：ResetCache / 查询条件 ----

func TestResetCachePattern(t *testing.T) {
	cache, f := newFakeCache(t, "")
	f.seed("sys_config:a", "1")
	f.seed("sys_config:b", "2")
	f.seed("sys_configX:c", "3") // 前缀相似但不同段，不应误删
	f.seed("other:d", "4")
	svc := NewService(nil, testAuth(1), cache)
	svc.ResetCache(context.Background())
	if _, ok := f.value("sys_config:a"); ok {
		t.Error("a 应被清除")
	}
	if _, ok := f.value("sys_config:b"); ok {
		t.Error("b 应被清除")
	}
	if _, ok := f.value("sys_configX:c"); !ok {
		t.Error("sys_configX 不属于 sys_config: 前缀，不应误删")
	}
	if _, ok := f.value("other:d"); !ok {
		t.Error("无关键不应误删")
	}
}

// TestPageAndListAllConditions 钉住 buildConditions：
// config_type 等值、name/key 模糊、时间区间需两端齐备；出参转换与 total 透传。
func TestPageAndListAllConditions(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	var gotConds []crudx.Cond
	row := SysConfig{ConfigName: strPtr("n"), ConfigKey: strPtr("k"), ConfigValue: strPtr("v"), ConfigType: strPtr("N")}
	row.ID = 11
	svc.repo = &mockRepo{
		page: func(_ context.Context, _ crudx.Page, conds []crudx.Cond) ([]SysConfig, int64, error) {
			gotConds = conds
			return []SysConfig{row}, 42, nil
		},
		listAll: func(_ context.Context, conds ...crudx.Cond) ([]SysConfig, error) {
			gotConds = conds
			return []SysConfig{row}, nil
		},
	}
	q := Query{ConfigName: "名", ConfigType: "Y", ConfigKey: "键", BeginTime: "2026-01-01", EndTime: "2026-01-31"}
	outs, total, err := svc.Page(context.Background(), q)
	if err != nil || total != 42 || len(outs) != 1 || outs[0].ID == nil || int64(*outs[0].ID) != 11 {
		t.Fatalf("outs = %+v, total = %d, err = %v", outs, total, err)
	}
	wantSQL := []string{"config_name LIKE ?", "config_type = ?", "config_key LIKE ?", "create_time BETWEEN ? AND ?"}
	if len(gotConds) != len(wantSQL) {
		t.Fatalf("conds = %+v, want %d 条", gotConds, len(wantSQL))
	}
	for i, w := range wantSQL {
		if gotConds[i].SQL != w {
			t.Errorf("cond[%d].SQL = %q, want %q", i, gotConds[i].SQL, w)
		}
	}
	if gotConds[0].Args[0] != "%名%" || gotConds[1].Args[0] != "Y" {
		t.Errorf("like 应包 %%，type 应等值: %+v", gotConds)
	}

	// 只给 beginTime 不施加 BETWEEN
	if _, err := svc.ListAll(context.Background(), Query{BeginTime: "2026-01-01"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(gotConds) != 0 {
		t.Errorf("单端时间不应产生条件: %+v", gotConds)
	}
}
