package dict

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

// mockTypeRepo / mockDataRepo 嵌入 consumer-side 接口（nil），仅以函数字段覆盖
// 被测方法；未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockTypeRepo struct {
	dictTypeRepo
	listAll      func(ctx context.Context, conds ...crudx.Cond) ([]SysDictType, error)
	page         func(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysDictType, int64, error)
	get          func(ctx context.Context, id int64) (*SysDictType, error)
	existsByType func(ctx context.Context, dictType string, excludeID *int64) (bool, error)
	create       func(ctx context.Context, row *SysDictType) error
	update       func(ctx context.Context, row *SysDictType) error
	deleteBatch  func(ctx context.Context, ids []int64) (int64, error)
}

func (m *mockTypeRepo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysDictType, error) {
	return m.listAll(ctx, conds...)
}

func (m *mockTypeRepo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysDictType, int64, error) {
	return m.page(ctx, page, conds)
}

func (m *mockTypeRepo) Get(ctx context.Context, id int64) (*SysDictType, error) {
	return m.get(ctx, id)
}

func (m *mockTypeRepo) ExistsByType(ctx context.Context, dictType string, excludeID *int64) (bool, error) {
	return m.existsByType(ctx, dictType, excludeID)
}

func (m *mockTypeRepo) Create(ctx context.Context, row *SysDictType) error {
	return m.create(ctx, row)
}

func (m *mockTypeRepo) Update(ctx context.Context, row *SysDictType) error {
	return m.update(ctx, row)
}

func (m *mockTypeRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.deleteBatch(ctx, ids)
}

type mockDataRepo struct {
	dictDataRepo
	existsByType         func(ctx context.Context, dictType string) (bool, error)
	page                 func(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysDictData, int64, error)
	listAll              func(ctx context.Context, conds ...crudx.Cond) ([]SysDictData, error)
	listByType           func(ctx context.Context, dictType string) ([]SysDictData, error)
	get                  func(ctx context.Context, id int64) (*SysDictData, error)
	existsByTypeAndValue func(ctx context.Context, dictType, dictValue string, excludeID *int64) (bool, error)
	create               func(ctx context.Context, row *SysDictData) error
	update               func(ctx context.Context, row *SysDictData) error
	renameType           func(ctx context.Context, oldType, newType string) (int64, error)
	deleteBatch          func(ctx context.Context, ids []int64) (int64, error)
}

func (m *mockDataRepo) ExistsByType(ctx context.Context, dictType string) (bool, error) {
	return m.existsByType(ctx, dictType)
}

func (m *mockDataRepo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]SysDictData, int64, error) {
	return m.page(ctx, page, conds)
}

func (m *mockDataRepo) ListAll(ctx context.Context, conds ...crudx.Cond) ([]SysDictData, error) {
	return m.listAll(ctx, conds...)
}

func (m *mockDataRepo) ListByType(ctx context.Context, dictType string) ([]SysDictData, error) {
	return m.listByType(ctx, dictType)
}

func (m *mockDataRepo) Get(ctx context.Context, id int64) (*SysDictData, error) {
	return m.get(ctx, id)
}

func (m *mockDataRepo) ExistsByTypeAndValue(ctx context.Context, dictType, dictValue string, excludeID *int64) (bool, error) {
	return m.existsByTypeAndValue(ctx, dictType, dictValue, excludeID)
}

func (m *mockDataRepo) Create(ctx context.Context, row *SysDictData) error {
	return m.create(ctx, row)
}

func (m *mockDataRepo) Update(ctx context.Context, row *SysDictData) error {
	return m.update(ctx, row)
}

func (m *mockDataRepo) RenameType(ctx context.Context, oldType, newType string) (int64, error) {
	return m.renameType(ctx, oldType, newType)
}

func (m *mockDataRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.deleteBatch(ctx, ids)
}

func testAuth(id int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: id, UserName: "tester"}}
}

func TestCheckTypeUniqueNilGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	err := svc.CheckTypeUnique(context.Background(), nil, nil)
	if se := errs.As(err); se == nil || se.Msg != "字典类型不能为空" {
		t.Errorf("err = %v, want 字典类型不能为空", err)
	}
}

func TestCheckDataUniqueNilValueGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	err := svc.CheckDataUnique(context.Background(), strPtr("sys_ok"), nil, nil)
	if se := errs.As(err); se == nil || se.Msg != "字典键值不能为空" {
		t.Errorf("err = %v, want 字典键值不能为空", err)
	}
}

func TestDeleteTypesOccupiedGuard(t *testing.T) {
	svc := NewService(nil, testAuth(1), nil)
	svc.typeRepo = &mockTypeRepo{listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysDictType, error) {
		return []SysDictType{{DictName: strPtr("状态"), DictType: strPtr("sys_status")}}, nil
	}}
	svc.dataRepo = &mockDataRepo{existsByType: func(_ context.Context, dictType string) (bool, error) {
		if dictType != "sys_status" {
			t.Errorf("dictType = %s, want sys_status", dictType)
		}
		return true, nil
	}}
	err := svc.DeleteTypes(context.Background(), []int64{1})
	if se := errs.As(err); se == nil || se.Msg != "状态已分配,不能删除" {
		t.Errorf("err = %v, want 占用守卫文案", err)
	}
}

// ==================== 深度业务流测试 ====================

func i64p(v int64) *int64 { return &v }

// pinNow 钉住 nowFunc，返回固定时间；测试结束自动还原。
func pinNow(t *testing.T) time.Time {
	t.Helper()
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	old := nowFunc
	nowFunc = func() time.Time { return fixed }
	t.Cleanup(func() { nowFunc = old })
	return fixed
}

func wantErr(t *testing.T, err error, msg string) {
	t.Helper()
	se := errs.As(err)
	if se == nil || se.Msg != msg {
		t.Errorf("err = %v, want %q", err, msg)
	}
}

func typeRow(id int64, name, dictType string) SysDictType {
	r := SysDictType{DictName: strPtr(name), DictType: strPtr(dictType)}
	r.ID = id
	return r
}

func dataRow(id int64, dictType, label, value string, sort int) SysDictData {
	r := SysDictData{
		DictType:  strPtr(dictType),
		DictLabel: strPtr(label),
		DictValue: strPtr(value),
		DictSort:  &sort,
	}
	r.ID = id
	return r
}

// ---- 内存版 Redis（fakeRedis）----
//
// service.cache 是具体类型 *cachex.Client（内嵌 *redis.Client），没有可 mock
// 的接口；为了钉住缓存键名与失效时序，这里用 net.Pipe + 自定义 Dialer 起一个
// 极简 RESP 服务端，只实现 dict service 用到的命令子集（GET/SET/DEL/SCAN）。
//
// 握手约定（go-redis v9）：
//   - HELLO 回 -ERR → 客户端按「服务器不支持 HELLO」回退 RESP2；
//   - Options.Protocol=2 → 跳过 CLIENT MAINT_NOTIFICATIONS 与 CLIENT TRACKING；
//   - DisableIdentity=true → 跳过 CLIENT SETINFO；
//   - DB=0 / 无密码 / 无 ClientName → 初始化管道为空。
//
// 键前缀为空（cachex.Client 零值 prefix），故 map 里的键就是业务键原文，
// 可直接断言 "sys_dict_type" **无冒号**这类契约细节。

type fakeRedis struct {
	mu   sync.Mutex
	data map[string]string
	log  []string
}

func newFakeCache(t *testing.T) (*cachex.Client, *fakeRedis) {
	t.Helper()
	f := &fakeRedis{data: make(map[string]string)}
	rc := redis.NewClient(&redis.Options{
		// 用环回 IP 而非主机名：Options.init 的端点类型探测会对主机名做
		// DNS 解析（bounded 2s），IP 则直接分类，构造零延迟。
		Addr: "127.0.0.1:6379",
		Dialer: func(_ context.Context, _, _ string) (net.Conn, error) {
			client, server := net.Pipe()
			go f.serve(server)
			return client, nil
		},
		Protocol:        2,
		DisableIdentity: true,
		MaxRetries:      -1,
		DialTimeout:     2 * time.Second,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
	})
	t.Cleanup(func() { _ = rc.Close() })
	return &cachex.Client{Client: rc}, f
}

func (f *fakeRedis) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	for {
		args, err := readRESPCommand(r)
		if err != nil {
			return
		}
		reply := f.exec(args)
		if _, err := w.WriteString(reply); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func readRESPCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) < 2 || line[0] != '*' {
		return nil, fmt.Errorf("unexpected frame: %q", line)
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("bad array len: %q", line)
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		hdr = strings.TrimRight(hdr, "\r\n")
		if len(hdr) < 2 || hdr[0] != '$' {
			return nil, fmt.Errorf("bad bulk header: %q", hdr)
		}
		l, err := strconv.Atoi(hdr[1:])
		if err != nil || l < 0 {
			return nil, fmt.Errorf("bad bulk len: %q", hdr)
		}
		buf := make([]byte, l+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:l]))
	}
	return args, nil
}

func respBulk(s string) string { return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s) }

func (f *fakeRedis) exec(args []string) string {
	cmd := strings.ToUpper(args[0])
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, strings.Join(args, " "))
	switch cmd {
	case "HELLO":
		return "-ERR unknown command 'HELLO'\r\n"
	case "PING":
		return "+PONG\r\n"
	case "CLIENT":
		return "+OK\r\n"
	case "GET":
		v, ok := f.data[args[1]]
		if !ok {
			return "$-1\r\n"
		}
		return respBulk(v)
	case "SET":
		f.data[args[1]] = args[2]
		return "+OK\r\n"
	case "DEL":
		n := 0
		for _, k := range args[1:] {
			if _, ok := f.data[k]; ok {
				delete(f.data, k)
				n++
			}
		}
		return fmt.Sprintf(":%d\r\n", n)
	case "SCAN":
		pattern := "*"
		for i := 2; i+1 < len(args); i++ {
			if strings.EqualFold(args[i], "MATCH") {
				pattern = args[i+1]
			}
		}
		keys := make([]string, 0)
		for k := range f.data {
			if ok, _ := path.Match(pattern, k); ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("*2\r\n")
		b.WriteString(respBulk("0"))
		fmt.Fprintf(&b, "*%d\r\n", len(keys))
		for _, k := range keys {
			b.WriteString(respBulk(k))
		}
		return b.String()
	default:
		return "-ERR unknown command '" + args[0] + "'\r\n"
	}
}

func (f *fakeRedis) get(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	return v, ok
}

func (f *fakeRedis) seed(kv map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range kv {
		f.data[k] = v
	}
}

func (f *fakeRedis) snapshot() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out
}

// delCommands 返回全部 DEL 命令（动词归一大写，键保持原文），用于断言失效键序。
func (f *fakeRedis) delCommands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.log {
		verb, rest, _ := strings.Cut(c, " ")
		if strings.EqualFold(verb, "DEL") {
			out = append(out, "DEL "+rest)
		}
	}
	return out
}

// ---- 字典类型写操作 ----

// TestInsertTypeCacheAndNulls 类型新增：NULL 语义（不吃列默认值）+ 空集合缓存防穿透。
func TestInsertTypeCacheAndNulls(t *testing.T) {
	t.Run("成功_审计字段与空集合缓存", func(t *testing.T) {
		fixed := pinNow(t)
		cache, f := newFakeCache(t)
		svc := NewService(nil, testAuth(2), cache)
		var got *SysDictType
		svc.typeRepo = &mockTypeRepo{create: func(_ context.Context, row *SysDictType) error {
			got = row
			return nil
		}}
		in := TypeCreateIn{DictName: strPtr("用户性别"), DictType: strPtr("sys_user_sex")}
		if err := svc.InsertType(context.Background(), in); err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.DictName == nil || *got.DictName != "用户性别" || got.DictType == nil || *got.DictType != "sys_user_sex" {
			t.Errorf("字段透传错误: %#v", got)
		}
		// 类型新增：缺省字段插 NULL（与数据新增的列默认值语义相反）
		if got.Remark != nil {
			t.Errorf("Remark 缺省应为 NULL, got %v", *got.Remark)
		}
		if got.ID == 0 || got.CreateBy == nil || *got.CreateBy != 2 ||
			got.CreateTime == nil || !got.CreateTime.Equal(fixed) || !got.UpdateTime.Equal(fixed) {
			t.Errorf("审计字段错误: %#v", got.BaseModel)
		}
		if v, ok := f.get("sys_dict:sys_user_sex"); !ok || v != "[]" {
			t.Errorf("cache = %q/%v, want 空集合 [] 防穿透", v, ok)
		}
	})

	t.Run("缺dictType_500守卫", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{} // create 未覆盖：触达即 panic
		wantErr(t, svc.InsertType(context.Background(), TypeCreateIn{}), "字典类型不能为空")
	})

	t.Run("写入失败_包装500且不写缓存", func(t *testing.T) {
		cache, f := newFakeCache(t)
		svc := NewService(nil, testAuth(2), cache)
		svc.typeRepo = &mockTypeRepo{create: func(_ context.Context, _ *SysDictType) error {
			return errors.New("boom")
		}}
		err := svc.InsertType(context.Background(), TypeCreateIn{DictType: strPtr("sys_x")})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s", err, errs.MsgDBFailed)
		}
		if len(f.snapshot()) != 0 {
			t.Errorf("失败不应写缓存: %v", f.snapshot())
		}
	})

	t.Run("缓存不可用_降级查库", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{create: func(_ context.Context, _ *SysDictType) error { return nil }}
		if err := svc.InsertType(context.Background(), TypeCreateIn{DictType: strPtr("sys_x")}); err != nil {
			t.Errorf("err = %v, want nil（降级）", err)
		}
	})
}

// TestUpdateTypeRenameAndCacheEvict 类型修改：数据表联动改名 + 四键缓存失效。
func TestUpdateTypeRenameAndCacheEvict(t *testing.T) {
	seedKeys := map[string]string{
		"sys_dict:sys_old":     `[{"id":1}]`,
		"sys_dict_typesys_old": "old-type-cache",
		"sys_dict:sys_new":     "[]",
		"sys_dict_typesys_new": "new-type-cache",
		"sys_dict:sys_other":   "keep",
	}

	t.Run("成功_改名联动与四键失效", func(t *testing.T) {
		fixed := pinNow(t)
		cache, f := newFakeCache(t)
		f.seed(seedKeys)
		svc := NewService(nil, testAuth(2), cache)
		old := typeRow(1, "旧名", "sys_old")
		old.Remark = strPtr("备注")
		var seq []string
		var got *SysDictType
		svc.typeRepo = &mockTypeRepo{
			get: func(_ context.Context, id int64) (*SysDictType, error) {
				seq = append(seq, "get")
				if id != 1 {
					t.Errorf("id = %d, want 1", id)
				}
				return &old, nil
			},
			update: func(_ context.Context, row *SysDictType) error {
				seq = append(seq, "update")
				got = row
				return nil
			},
		}
		svc.dataRepo = &mockDataRepo{renameType: func(_ context.Context, oldType, newType string) (int64, error) {
			seq = append(seq, "rename")
			if oldType != "sys_old" || newType != "sys_new" {
				t.Errorf("renameType(%s → %s), want sys_old → sys_new", oldType, newType)
			}
			return 2, nil
		}}
		in := TypeUpdateIn{TypeCreateIn: TypeCreateIn{DictName: strPtr("新名"), DictType: strPtr("sys_new")}, ID: i64p(1)}
		if err := svc.UpdateType(context.Background(), in); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(seq) != 3 || seq[0] != "get" || seq[1] != "rename" || seq[2] != "update" {
			t.Errorf("调用序 = %v, want [get rename update]（先联动数据表再保存类型）", seq)
		}
		if got.DictType == nil || *got.DictType != "sys_new" || got.DictName == nil || *got.DictName != "新名" ||
			got.Remark == nil || *got.Remark != "备注" {
			t.Errorf("仅覆盖非空字段, Remark 应保留: %#v", got)
		}
		if got.UpdateBy == nil || *got.UpdateBy != 2 || !got.UpdateTime.Equal(fixed) {
			t.Errorf("审计字段错误: %#v", got.BaseModel)
		}
		// 新旧类型的数据缓存 + 类型缓存共四键失效；无关键保留
		snap := f.snapshot()
		if len(snap) != 1 || snap["sys_dict:sys_other"] != "keep" {
			t.Errorf("缓存失效后 = %v, want 仅留 sys_dict:sys_other", snap)
		}
		dels := f.delCommands()
		if len(dels) != 1 || dels[0] != "DEL sys_dict:sys_old sys_dict_typesys_old sys_dict:sys_new sys_dict_typesys_new" {
			t.Errorf("DEL 命令 = %v, want 单命令四键（注意 sys_dict_type 无冒号）", dels)
		}
	})

	t.Run("缺dictType_500守卫", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{}
		wantErr(t, svc.UpdateType(context.Background(), TypeUpdateIn{ID: i64p(1)}), "字典类型不能为空")
	})

	t.Run("类型不存在_操作失败", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{get: func(_ context.Context, _ int64) (*SysDictType, error) { return nil, nil }}
		svc.dataRepo = &mockDataRepo{} // renameType 未覆盖：触达即 panic
		wantErr(t, svc.UpdateType(context.Background(), TypeUpdateIn{
			TypeCreateIn: TypeCreateIn{DictType: strPtr("sys_new")}, ID: i64p(1),
		}), "操作失败")
	})

	t.Run("查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{get: func(_ context.Context, _ int64) (*SysDictType, error) {
			return nil, errors.New("boom")
		}}
		err := svc.UpdateType(context.Background(), TypeUpdateIn{
			TypeCreateIn: TypeCreateIn{DictType: strPtr("sys_new")}, ID: i64p(1),
		})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s", err, errs.MsgDBFailed)
		}
	})

	t.Run("改名失败_短路保存与缓存", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(seedKeys)
		svc := NewService(nil, testAuth(2), cache)
		old := typeRow(1, "旧名", "sys_old")
		svc.typeRepo = &mockTypeRepo{
			get:    func(_ context.Context, _ int64) (*SysDictType, error) { return &old, nil },
			update: func(_ context.Context, _ *SysDictType) error { t.Error("改名失败不应保存"); return nil },
		}
		svc.dataRepo = &mockDataRepo{renameType: func(_ context.Context, _, _ string) (int64, error) {
			return 0, errors.New("boom")
		}}
		err := svc.UpdateType(context.Background(), TypeUpdateIn{
			TypeCreateIn: TypeCreateIn{DictType: strPtr("sys_new")}, ID: i64p(1),
		})
		wantErr(t, err, errs.MsgDBFailed)
		if len(f.snapshot()) != len(seedKeys) {
			t.Errorf("失败不应动缓存: %v", f.snapshot())
		}
	})

	t.Run("保存失败_缓存不失效", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(seedKeys)
		svc := NewService(nil, testAuth(2), cache)
		old := typeRow(1, "旧名", "sys_old")
		svc.typeRepo = &mockTypeRepo{
			get:    func(_ context.Context, _ int64) (*SysDictType, error) { return &old, nil },
			update: func(_ context.Context, _ *SysDictType) error { return errors.New("boom") },
		}
		svc.dataRepo = &mockDataRepo{renameType: func(_ context.Context, _, _ string) (int64, error) { return 0, nil }}
		err := svc.UpdateType(context.Background(), TypeUpdateIn{
			TypeCreateIn: TypeCreateIn{DictType: strPtr("sys_new")}, ID: i64p(1),
		})
		wantErr(t, err, errs.MsgDBFailed)
		if len(f.snapshot()) != len(seedKeys) {
			t.Errorf("保存失败缓存应原样: %v", f.snapshot())
		}
	})
}

// TestCheckTypeUniqueBranches dictType 唯一性哨兵与错误包装。
func TestCheckTypeUniqueBranches(t *testing.T) {
	t.Run("已存在_哨兵错误", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{existsByType: func(_ context.Context, dictType string, excludeID *int64) (bool, error) {
			if dictType != "sys_user_sex" || excludeID == nil || *excludeID != 1 {
				t.Errorf("参数透传错误: %s %v", dictType, excludeID)
			}
			return true, nil
		}}
		err := svc.CheckTypeUnique(context.Background(), strPtr("sys_user_sex"), i64p(1))
		if !errors.Is(err, errDuplicateType) {
			t.Fatalf("err = %v, want errDuplicateType 哨兵", err)
		}
		wantErr(t, err, "字典类型已存在")
	})

	t.Run("不存在_放行", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{existsByType: func(_ context.Context, _ string, _ *int64) (bool, error) {
			return false, nil
		}}
		if err := svc.CheckTypeUnique(context.Background(), strPtr("sys_new"), nil); err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{existsByType: func(_ context.Context, _ string, _ *int64) (bool, error) {
			return false, errors.New("boom")
		}}
		err := svc.CheckTypeUnique(context.Background(), strPtr("sys_x"), nil)
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s", err, errs.MsgDBFailed)
		}
	})
}

// TestDeleteTypesGuardAtomicity 批量删除：占用检查全量前置，任一占用一条不删。
func TestDeleteTypesGuardAtomicity(t *testing.T) {
	t.Run("第二个类型占用_整体拒绝", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(map[string]string{"sys_dict:sys_status": "[]", "sys_dict:sys_sex": "[]"})
		svc := NewService(nil, testAuth(2), cache)
		var checked []string
		svc.typeRepo = &mockTypeRepo{
			listAll: func(_ context.Context, conds ...crudx.Cond) ([]SysDictType, error) {
				if len(conds) != 1 || conds[0].SQL != "id IN ?" {
					t.Errorf("conds = %#v, want id IN ?", conds)
				}
				return []SysDictType{typeRow(1, "状态", "sys_status"), typeRow(2, "性别", "sys_sex")}, nil
			},
			deleteBatch: func(_ context.Context, _ []int64) (int64, error) {
				t.Error("占用时不应删除任何行")
				return 0, nil
			},
		}
		svc.dataRepo = &mockDataRepo{existsByType: func(_ context.Context, dictType string) (bool, error) {
			checked = append(checked, dictType)
			return dictType == "sys_sex", nil
		}}
		err := svc.DeleteTypes(context.Background(), []int64{1, 2})
		wantErr(t, err, "性别已分配,不能删除")
		if len(checked) != 2 || checked[0] != "sys_status" || checked[1] != "sys_sex" {
			t.Errorf("占用检查序 = %v, want 全量前置 [sys_status sys_sex]", checked)
		}
		if len(f.snapshot()) != 2 {
			t.Errorf("拒绝时缓存应原样: %v", f.snapshot())
		}
	})

	t.Run("成功_批量删除与逐类型缓存失效", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(map[string]string{
			"sys_dict:sys_status":     "[]",
			"sys_dict_typesys_status": "x",
			"sys_dict:sys_sex":        "[]",
			"sys_dict_typesys_sex":    "y",
			"sys_dict:other":          "keep",
		})
		svc := NewService(nil, testAuth(2), cache)
		var gotIDs []int64
		svc.typeRepo = &mockTypeRepo{
			listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysDictType, error) {
				return []SysDictType{typeRow(1, "状态", "sys_status"), typeRow(2, "性别", "sys_sex")}, nil
			},
			deleteBatch: func(_ context.Context, ids []int64) (int64, error) {
				gotIDs = ids
				return 2, nil
			},
		}
		svc.dataRepo = &mockDataRepo{existsByType: func(_ context.Context, _ string) (bool, error) { return false, nil }}
		if err := svc.DeleteTypes(context.Background(), []int64{1, 2}); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(gotIDs) != 2 || gotIDs[0] != 1 || gotIDs[1] != 2 {
			t.Errorf("deleteBatch ids = %v, want [1 2]", gotIDs)
		}
		snap := f.snapshot()
		if len(snap) != 1 || snap["sys_dict:other"] != "keep" {
			t.Errorf("缓存 = %v, want 仅留无关键", snap)
		}
		dels := f.delCommands()
		if len(dels) != 2 || dels[0] != "DEL sys_dict:sys_status sys_dict_typesys_status" ||
			dels[1] != "DEL sys_dict:sys_sex sys_dict_typesys_sex" {
			t.Errorf("DEL 命令 = %v, want 逐类型两键", dels)
		}
	})

	t.Run("查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysDictType, error) {
			return nil, errors.New("boom")
		}}
		wantErr(t, svc.DeleteTypes(context.Background(), []int64{1}), errs.MsgDBFailed)
	})

	t.Run("占用检查失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.typeRepo = &mockTypeRepo{listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysDictType, error) {
			return []SysDictType{typeRow(1, "状态", "sys_status")}, nil
		}}
		svc.dataRepo = &mockDataRepo{existsByType: func(_ context.Context, _ string) (bool, error) {
			return false, errors.New("boom")
		}}
		wantErr(t, svc.DeleteTypes(context.Background(), []int64{1}), errs.MsgDBFailed)
	})

	t.Run("删除失败_包装500且缓存不动", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(map[string]string{"sys_dict:sys_status": "[]"})
		svc := NewService(nil, testAuth(2), cache)
		svc.typeRepo = &mockTypeRepo{
			listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysDictType, error) {
				return []SysDictType{typeRow(1, "状态", "sys_status")}, nil
			},
			deleteBatch: func(_ context.Context, _ []int64) (int64, error) { return 0, errors.New("boom") },
		}
		svc.dataRepo = &mockDataRepo{existsByType: func(_ context.Context, _ string) (bool, error) { return false, nil }}
		wantErr(t, svc.DeleteTypes(context.Background(), []int64{1}), errs.MsgDBFailed)
		if _, ok := f.get("sys_dict:sys_status"); !ok {
			t.Error("删除失败缓存应原样")
		}
	})
}

// ---- 字典数据写操作 ----

// TestInsertDataDefaultsAndCacheRefresh 数据新增：列默认值兜底 + 按类型刷新缓存。
func TestInsertDataDefaultsAndCacheRefresh(t *testing.T) {
	t.Run("缺省走列默认值_刷新缓存字节级一致", func(t *testing.T) {
		fixed := pinNow(t)
		cache, f := newFakeCache(t)
		svc := NewService(nil, testAuth(2), cache)
		var got *SysDictData
		var seq []string
		svc.dataRepo = &mockDataRepo{
			create: func(_ context.Context, row *SysDictData) error {
				seq = append(seq, "create")
				got = row
				return nil
			},
			listByType: func(_ context.Context, dictType string) ([]SysDictData, error) {
				seq = append(seq, "refresh")
				if dictType != "sys_user_sex" {
					t.Errorf("dictType = %s, want sys_user_sex", dictType)
				}
				return []SysDictData{dataRow(1, "sys_user_sex", "男", "0", 1), dataRow(2, "sys_user_sex", "女", "1", 2)}, nil
			},
		}
		in := DataCreateIn{DictType: strPtr("sys_user_sex"), DictLabel: strPtr("男"), DictValue: strPtr("0")}
		if err := svc.InsertData(context.Background(), in); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(seq) != 2 || seq[0] != "create" || seq[1] != "refresh" {
			t.Errorf("调用序 = %v, want [create refresh]", seq)
		}
		if got.DictSort == nil || *got.DictSort != 0 || dstr(got.DictLabel) != "男" || dstr(got.DictValue) != "0" ||
			dstr(got.IsDefault) != "N" {
			t.Errorf("列默认值错误: %#v", got)
		}
		if got.CssClass != nil || got.ListClass != nil || got.Remark != nil {
			t.Errorf("cssClass/listClass/remark 缺省应保持 NULL: %#v", got)
		}
		if got.ID == 0 || got.CreateBy == nil || *got.CreateBy != 2 || !got.CreateTime.Equal(fixed) {
			t.Errorf("审计字段错误: %#v", got.BaseModel)
		}
		// 缓存内容 = 该类型全量出参的 JSON（键序 = struct 声明序）
		rows := []SysDictData{dataRow(1, "sys_user_sex", "男", "0", 1), dataRow(2, "sys_user_sex", "女", "1", 2)}
		want, err := json.Marshal(dataOuts(rows))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if v, ok := f.get("sys_dict:sys_user_sex"); !ok || v != string(want) {
			t.Errorf("cache = %q, want %q", v, want)
		}
	})

	t.Run("显式值覆盖默认值", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		var got *SysDictData
		svc.dataRepo = &mockDataRepo{create: func(_ context.Context, row *SysDictData) error {
			got = row
			return nil
		}}
		sort := 5
		in := DataCreateIn{
			DictType: strPtr("sys_x"), DictSort: &sort, DictLabel: strPtr("l"), DictValue: strPtr("v"),
			CssClass: strPtr("c"), ListClass: strPtr("lc"), IsDefault: strPtr("Y"), Remark: strPtr("r"),
		}
		if err := svc.InsertData(context.Background(), in); err != nil {
			t.Fatalf("err = %v", err)
		}
		if *got.DictSort != 5 || dstr(got.CssClass) != "c" || dstr(got.ListClass) != "lc" ||
			dstr(got.IsDefault) != "Y" || dstr(got.Remark) != "r" {
			t.Errorf("显式值透传错误: %#v", got)
		}
	})

	t.Run("缺dictType_500守卫", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{}
		wantErr(t, svc.InsertData(context.Background(), DataCreateIn{}), "字典类型不能为空")
	})

	t.Run("写入失败_包装500且不刷新", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{create: func(_ context.Context, _ *SysDictData) error {
			return errors.New("boom")
		}}
		err := svc.InsertData(context.Background(), DataCreateIn{DictType: strPtr("sys_x")})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s", err, errs.MsgDBFailed)
		}
	})

	t.Run("刷新失败_包装500", func(t *testing.T) {
		cache, _ := newFakeCache(t)
		svc := NewService(nil, testAuth(2), cache)
		svc.dataRepo = &mockDataRepo{
			create: func(_ context.Context, _ *SysDictData) error { return nil },
			listByType: func(_ context.Context, _ string) ([]SysDictData, error) {
				return nil, errors.New("boom")
			},
		}
		err := svc.InsertData(context.Background(), DataCreateIn{DictType: strPtr("sys_x")})
		wantErr(t, err, errs.MsgDBFailed)
	})

	t.Run("缓存不可用_不触发刷新查询", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{create: func(_ context.Context, _ *SysDictData) error { return nil }}
		// listByType 未覆盖：cache=nil 时 refreshDataCache 必须直接返回
		if err := svc.InsertData(context.Background(), DataCreateIn{DictType: strPtr("sys_x")}); err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})
}

func dstr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// TestUpdateDataPartialAndCacheScope 数据修改：部分更新 + 只刷新 req.dict_type。
func TestUpdateDataPartialAndCacheScope(t *testing.T) {
	t.Run("改类型_旧类型缓存不刷新_Python既有语义", func(t *testing.T) {
		fixed := pinNow(t)
		cache, f := newFakeCache(t)
		f.seed(map[string]string{"sys_dict:sys_old_type": "stale-but-kept"})
		svc := NewService(nil, testAuth(2), cache)
		old := dataRow(1, "sys_old_type", "旧标签", "0", 1)
		var got *SysDictData
		var refreshTypes []string
		svc.dataRepo = &mockDataRepo{
			get: func(_ context.Context, id int64) (*SysDictData, error) {
				if id != 1 {
					t.Errorf("id = %d, want 1", id)
				}
				return &old, nil
			},
			update: func(_ context.Context, row *SysDictData) error {
				got = row
				return nil
			},
			listByType: func(_ context.Context, dictType string) ([]SysDictData, error) {
				refreshTypes = append(refreshTypes, dictType)
				return []SysDictData{dataRow(1, "sys_new_type", "新标签", "0", 1)}, nil
			},
		}
		in := DataUpdateIn{DataCreateIn: DataCreateIn{DictType: strPtr("sys_new_type"), DictLabel: strPtr("新标签")}, ID: i64p(1)}
		if err := svc.UpdateData(context.Background(), in); err != nil {
			t.Fatalf("err = %v", err)
		}
		if dstr(got.DictType) != "sys_new_type" || dstr(got.DictLabel) != "新标签" ||
			dstr(got.DictValue) != "0" || got.DictSort == nil || *got.DictSort != 1 {
			t.Errorf("仅覆盖非空字段: %#v", got)
		}
		if got.UpdateBy == nil || *got.UpdateBy != 2 || !got.UpdateTime.Equal(fixed) {
			t.Errorf("审计字段错误: %#v", got.BaseModel)
		}
		if len(refreshTypes) != 1 || refreshTypes[0] != "sys_new_type" {
			t.Errorf("刷新类型 = %v, want 仅 [sys_new_type]（旧类型缓存不刷新，照抄）", refreshTypes)
		}
		if v, ok := f.get("sys_dict:sys_old_type"); !ok || v != "stale-but-kept" {
			t.Errorf("旧类型缓存应原样: %q/%v", v, ok)
		}
		if _, ok := f.get("sys_dict:sys_new_type"); !ok {
			t.Error("新类型缓存应被刷新写入")
		}
	})

	t.Run("缺dictType_500守卫", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{}
		wantErr(t, svc.UpdateData(context.Background(), DataUpdateIn{ID: i64p(1)}), "字典类型不能为空")
	})

	t.Run("数据不存在_操作失败", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{get: func(_ context.Context, _ int64) (*SysDictData, error) { return nil, nil }}
		wantErr(t, svc.UpdateData(context.Background(), DataUpdateIn{
			DataCreateIn: DataCreateIn{DictType: strPtr("sys_x")}, ID: i64p(1),
		}), "操作失败")
	})

	t.Run("查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{get: func(_ context.Context, _ int64) (*SysDictData, error) {
			return nil, errors.New("boom")
		}}
		err := svc.UpdateData(context.Background(), DataUpdateIn{
			DataCreateIn: DataCreateIn{DictType: strPtr("sys_x")}, ID: i64p(1),
		})
		wantErr(t, err, errs.MsgDBFailed)
	})

	t.Run("保存失败_包装500且不刷新", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		old := dataRow(1, "sys_old", "旧", "0", 1)
		svc.dataRepo = &mockDataRepo{
			get:    func(_ context.Context, _ int64) (*SysDictData, error) { return &old, nil },
			update: func(_ context.Context, _ *SysDictData) error { return errors.New("boom") },
		}
		err := svc.UpdateData(context.Background(), DataUpdateIn{
			DataCreateIn: DataCreateIn{DictType: strPtr("sys_x")}, ID: i64p(1),
		})
		wantErr(t, err, errs.MsgDBFailed)
	})
}

// TestCheckDataUniqueBranches 键值唯一性（按类型关联）与守卫。
func TestCheckDataUniqueBranches(t *testing.T) {
	t.Run("同类型同键值_哨兵错误", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{existsByTypeAndValue: func(_ context.Context, dictType, dictValue string, excludeID *int64) (bool, error) {
			if dictType != "sys_user_sex" || dictValue != "0" || excludeID == nil || *excludeID != 1 {
				t.Errorf("参数透传错误: %s %s %v", dictType, dictValue, excludeID)
			}
			return true, nil
		}}
		err := svc.CheckDataUnique(context.Background(), strPtr("sys_user_sex"), strPtr("0"), i64p(1))
		if !errors.Is(err, errDuplicateValue) {
			t.Fatalf("err = %v, want errDuplicateValue 哨兵", err)
		}
		wantErr(t, err, "字典键值已存在")
	})

	t.Run("不存在_放行", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{existsByTypeAndValue: func(_ context.Context, _, _ string, _ *int64) (bool, error) {
			return false, nil
		}}
		if err := svc.CheckDataUnique(context.Background(), strPtr("sys_x"), strPtr("v"), nil); err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("缺dictType_500守卫", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{}
		wantErr(t, svc.CheckDataUnique(context.Background(), nil, strPtr("0"), nil), "字典类型不能为空")
	})

	t.Run("查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{existsByTypeAndValue: func(_ context.Context, _, _ string, _ *int64) (bool, error) {
			return false, errors.New("boom")
		}}
		err := svc.CheckDataUnique(context.Background(), strPtr("sys_x"), strPtr("v"), nil)
		wantErr(t, err, errs.MsgDBFailed)
	})
}

// TestDeleteDataCacheEvict 数据批量删除：逐行按类型失效数据缓存。
func TestDeleteDataCacheEvict(t *testing.T) {
	t.Run("成功_逐行类型失效", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(map[string]string{"sys_dict:t1": "a", "sys_dict:t2": "b", "sys_dict:other": "keep"})
		svc := NewService(nil, testAuth(2), cache)
		var gotIDs []int64
		svc.dataRepo = &mockDataRepo{
			listAll: func(_ context.Context, conds ...crudx.Cond) ([]SysDictData, error) {
				if len(conds) != 1 || conds[0].SQL != "id IN ?" {
					t.Errorf("conds = %#v, want id IN ?", conds)
				}
				return []SysDictData{
					dataRow(1, "t1", "a", "0", 1),
					dataRow(2, "t2", "b", "1", 2),
					dataRow(3, "t1", "c", "2", 3),
				}, nil
			},
			deleteBatch: func(_ context.Context, ids []int64) (int64, error) {
				gotIDs = ids
				return 3, nil
			},
		}
		if err := svc.DeleteData(context.Background(), []int64{1, 2, 3}); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(gotIDs) != 3 {
			t.Errorf("ids = %v, want [1 2 3]", gotIDs)
		}
		snap := f.snapshot()
		if len(snap) != 1 || snap["sys_dict:other"] != "keep" {
			t.Errorf("缓存 = %v, want 仅留无关键", snap)
		}
		dels := f.delCommands()
		if len(dels) != 3 || dels[0] != "DEL sys_dict:t1" || dels[1] != "DEL sys_dict:t2" || dels[2] != "DEL sys_dict:t1" {
			t.Errorf("DEL 命令 = %v, want 逐行（含重复类型）", dels)
		}
	})

	t.Run("查询失败_不删除", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysDictData, error) {
			return nil, errors.New("boom")
		}}
		wantErr(t, svc.DeleteData(context.Background(), []int64{1}), errs.MsgDBFailed)
	})

	t.Run("删除失败_缓存不动", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(map[string]string{"sys_dict:t1": "a"})
		svc := NewService(nil, testAuth(2), cache)
		svc.dataRepo = &mockDataRepo{
			listAll: func(_ context.Context, _ ...crudx.Cond) ([]SysDictData, error) {
				return []SysDictData{dataRow(1, "t1", "a", "0", 1)}, nil
			},
			deleteBatch: func(_ context.Context, _ []int64) (int64, error) { return 0, errors.New("boom") },
		}
		wantErr(t, svc.DeleteData(context.Background(), []int64{1}), errs.MsgDBFailed)
		if _, ok := f.get("sys_dict:t1"); !ok {
			t.Error("删除失败缓存应原样")
		}
	})
}

// ---- 缓存读取与重置 ----

// TestDataTypeCacheHitMiss DataByType 的缓存优先语义（含空集合与异构缓存）。
func TestDataTypeCacheHitMiss(t *testing.T) {
	rows := []SysDictData{dataRow(1, "sys_user_sex", "男", "0", 1)}
	payload, err := json.Marshal(dataOuts(rows))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	t.Run("命中合法缓存_不查库", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(map[string]string{"sys_dict:sys_user_sex": string(payload)})
		svc := NewService(nil, testAuth(2), cache)
		svc.dataRepo = &mockDataRepo{} // listByType 未覆盖：命中即不应查库
		outs, err := svc.DataByType(context.Background(), "sys_user_sex")
		if err != nil || len(outs) != 1 || outs[0].ID.Int64() != 1 || dstr(outs[0].DictLabel) != "男" {
			t.Fatalf("outs = %#v, err = %v", outs, err)
		}
	})

	t.Run("命中空集合_返回nil由handler兜底", func(t *testing.T) {
		cache, f := newFakeCache(t)
		f.seed(map[string]string{"sys_dict:sys_user_sex": "[]"})
		svc := NewService(nil, testAuth(2), cache)
		svc.dataRepo = &mockDataRepo{}
		outs, err := svc.DataByType(context.Background(), "sys_user_sex")
		if err != nil || outs != nil {
			t.Fatalf("outs = %#v, err = %v; want nil, nil", outs, err)
		}
	})

	t.Run("异构缓存_视为未命中并重写", func(t *testing.T) {
		for _, raw := range []string{"not-json", `{"a":1}`} {
			cache, f := newFakeCache(t)
			f.seed(map[string]string{"sys_dict:sys_user_sex": raw})
			svc := NewService(nil, testAuth(2), cache)
			svc.dataRepo = &mockDataRepo{listByType: func(_ context.Context, dictType string) ([]SysDictData, error) {
				if dictType != "sys_user_sex" {
					t.Errorf("dictType = %s", dictType)
				}
				return rows, nil
			}}
			outs, err := svc.DataByType(context.Background(), "sys_user_sex")
			if err != nil || len(outs) != 1 {
				t.Fatalf("raw=%q outs = %#v, err = %v", raw, outs, err)
			}
			if v, _ := f.get("sys_dict:sys_user_sex"); v != string(payload) {
				t.Errorf("raw=%q 缓存应重写为 %q, got %q", raw, payload, v)
			}
		}
	})

	t.Run("未命中_查库并写缓存", func(t *testing.T) {
		cache, f := newFakeCache(t)
		svc := NewService(nil, testAuth(2), cache)
		svc.dataRepo = &mockDataRepo{listByType: func(_ context.Context, _ string) ([]SysDictData, error) {
			return rows, nil
		}}
		outs, err := svc.DataByType(context.Background(), "sys_user_sex")
		if err != nil || len(outs) != 1 {
			t.Fatalf("outs = %#v, err = %v", outs, err)
		}
		if v, ok := f.get("sys_dict:sys_user_sex"); !ok || v != string(payload) {
			t.Errorf("cache = %q/%v, want %q", v, ok, payload)
		}
	})

	t.Run("查库为空_缓存空集合防穿透", func(t *testing.T) {
		cache, f := newFakeCache(t)
		svc := NewService(nil, testAuth(2), cache)
		svc.dataRepo = &mockDataRepo{listByType: func(_ context.Context, _ string) ([]SysDictData, error) {
			return nil, nil
		}}
		outs, err := svc.DataByType(context.Background(), "sys_none")
		if err != nil || outs != nil {
			t.Fatalf("outs = %#v, err = %v; want nil, nil", outs, err)
		}
		if v, ok := f.get("sys_dict:sys_none"); !ok || v != "[]" {
			t.Errorf("cache = %q/%v, want []", v, ok)
		}
	})

	t.Run("查库失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		cache, _ := newFakeCache(t)
		svc := NewService(nil, testAuth(2), cache)
		svc.dataRepo = &mockDataRepo{listByType: func(_ context.Context, _ string) ([]SysDictData, error) {
			return nil, boom
		}}
		if _, err := svc.DataByType(context.Background(), "sys_x"); !errors.Is(err, boom) {
			t.Errorf("err = %v, want 原始错误", err)
		}
	})

	t.Run("缓存不可用_直接查库", func(t *testing.T) {
		svc := NewService(nil, testAuth(2), nil)
		svc.dataRepo = &mockDataRepo{listByType: func(_ context.Context, _ string) ([]SysDictData, error) {
			return rows, nil
		}}
		outs, err := svc.DataByType(context.Background(), "sys_user_sex")
		if err != nil || len(outs) != 1 {
			t.Fatalf("outs = %#v, err = %v", outs, err)
		}
	})
}

// TestResetCachePatterns 重置缓存：两个前缀模式扫描，不误删无关键。
func TestResetCachePatterns(t *testing.T) {
	cache, f := newFakeCache(t)
	f.seed(map[string]string{
		"sys_dict:a":     "1",
		"sys_dict:b":     "2",
		"sys_dict_typex": "3",
		"other:key":      "keep",
	})
	svc := NewService(nil, testAuth(1), cache)
	svc.ResetCache(context.Background())
	snap := f.snapshot()
	if len(snap) != 1 || snap["other:key"] != "keep" {
		t.Errorf("重置后 = %v, want 仅留 other:key", snap)
	}

	t.Run("缓存不可用_安全返回", func(t *testing.T) {
		svc := NewService(nil, testAuth(1), nil)
		svc.ResetCache(context.Background()) // 不 panic 即通过
	})
}

// ---- 查询条件拼装 ----

// TestTypeAndDataQueryConditions 类型 like / 数据等值的条件差异 + 分页透传。
func TestTypeAndDataQueryConditions(t *testing.T) {
	t.Run("类型分页_like与时间范围", func(t *testing.T) {
		svc := NewService(nil, testAuth(1), nil)
		size := 10
		var gotPage crudx.Page
		var gotConds []crudx.Cond
		svc.typeRepo = &mockTypeRepo{page: func(_ context.Context, page crudx.Page, conds []crudx.Cond) ([]SysDictType, int64, error) {
			gotPage, gotConds = page, conds
			return []SysDictType{typeRow(1, "用户性别", "sys_user_sex")}, 1, nil
		}}
		q := TypeQuery{
			Page:     crudx.Page{PageNum: 2, PageSize: &size},
			DictName: "用户", DictType: "sys",
			BeginTime: "2026-01-01", EndTime: "2026-01-31",
		}
		outs, total, err := svc.TypePage(context.Background(), q)
		if err != nil || total != 1 || len(outs) != 1 || outs[0].ID.Int64() != 1 {
			t.Fatalf("outs = %d, total = %d, err = %v", len(outs), total, err)
		}
		if gotPage.PageNum != 2 || gotPage.PageSize == nil || *gotPage.PageSize != 10 {
			t.Errorf("分页透传错误: %#v", gotPage)
		}
		want := []string{"dict_name LIKE ?", "dict_type LIKE ?", "create_time BETWEEN ? AND ?"}
		if len(gotConds) != 3 {
			t.Fatalf("conds = %#v, want %v", gotConds, want)
		}
		for i, w := range want {
			if gotConds[i].SQL != w {
				t.Errorf("conds[%d] = %s, want %s", i, gotConds[i].SQL, w)
			}
		}
		if gotConds[0].Args[0] != "%用户%" {
			t.Errorf("Like 参数 = %v", gotConds[0].Args[0])
		}
	})

	t.Run("数据分页_dictType等值而非like", func(t *testing.T) {
		svc := NewService(nil, testAuth(1), nil)
		sort := 1
		var gotConds []crudx.Cond
		svc.dataRepo = &mockDataRepo{page: func(_ context.Context, _ crudx.Page, conds []crudx.Cond) ([]SysDictData, int64, error) {
			gotConds = conds
			return []SysDictData{dataRow(1, "sys_user_sex", "男", "0", 1)}, 1, nil
		}}
		q := DataQuery{DictSort: &sort, DictLabel: "男", DictType: "sys_user_sex"}
		outs, total, err := svc.DataPage(context.Background(), q)
		if err != nil || total != 1 || len(outs) != 1 {
			t.Fatalf("outs = %d, total = %d, err = %v", len(outs), total, err)
		}
		want := []string{"dict_sort = ?", "dict_label LIKE ?", "dict_type = ?"}
		if len(gotConds) != 3 {
			t.Fatalf("conds = %#v, want %v", gotConds, want)
		}
		for i, w := range want {
			if gotConds[i].SQL != w {
				t.Errorf("conds[%d] = %s, want %s（dict_type 必须等值）", i, gotConds[i].SQL, w)
			}
		}
		if gotConds[2].Args[0] != "sys_user_sex" {
			t.Errorf("dict_type 参数 = %v, want 原值（非 %%..%%）", gotConds[2].Args[0])
		}
	})

	t.Run("分页失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(1), nil)
		svc.typeRepo = &mockTypeRepo{page: func(_ context.Context, _ crudx.Page, _ []crudx.Cond) ([]SysDictType, int64, error) {
			return nil, 0, boom
		}}
		svc.dataRepo = &mockDataRepo{page: func(_ context.Context, _ crudx.Page, _ []crudx.Cond) ([]SysDictData, int64, error) {
			return nil, 0, boom
		}}
		if _, _, err := svc.TypePage(context.Background(), TypeQuery{}); !errors.Is(err, boom) {
			t.Errorf("TypePage err = %v", err)
		}
		if _, _, err := svc.DataPage(context.Background(), DataQuery{}); !errors.Is(err, boom) {
			t.Errorf("DataPage err = %v", err)
		}
	})

	t.Run("详情_存在与不存在", func(t *testing.T) {
		svc := NewService(nil, testAuth(1), nil)
		tr := typeRow(1, "用户性别", "sys_user_sex")
		dr := dataRow(2, "sys_user_sex", "男", "0", 1)
		svc.typeRepo = &mockTypeRepo{get: func(_ context.Context, _ int64) (*SysDictType, error) { return &tr, nil }}
		svc.dataRepo = &mockDataRepo{get: func(_ context.Context, _ int64) (*SysDictData, error) { return &dr, nil }}
		if out, err := svc.TypeGet(context.Background(), 1); err != nil || out == nil || out.ID.Int64() != 1 {
			t.Fatalf("TypeGet = %#v, err = %v", out, err)
		}
		if out, err := svc.DataGet(context.Background(), 2); err != nil || out == nil || out.ID.Int64() != 2 {
			t.Fatalf("DataGet = %#v, err = %v", out, err)
		}
		svc.typeRepo = &mockTypeRepo{get: func(_ context.Context, _ int64) (*SysDictType, error) { return nil, nil }}
		svc.dataRepo = &mockDataRepo{get: func(_ context.Context, _ int64) (*SysDictData, error) { return nil, nil }}
		if out, err := svc.TypeGet(context.Background(), 9); err != nil || out != nil {
			t.Fatalf("TypeGet = %#v, err = %v; want nil, nil", out, err)
		}
		if out, err := svc.DataGet(context.Background(), 9); err != nil || out != nil {
			t.Fatalf("DataGet = %#v, err = %v; want nil, nil", out, err)
		}
	})

	t.Run("optionselect_无条件全量", func(t *testing.T) {
		svc := NewService(nil, testAuth(1), nil)
		svc.typeRepo = &mockTypeRepo{listAll: func(_ context.Context, conds ...crudx.Cond) ([]SysDictType, error) {
			if len(conds) != 0 {
				t.Errorf("conds = %#v, want 空", conds)
			}
			return []SysDictType{typeRow(1, "用户性别", "sys_user_sex")}, nil
		}}
		outs, err := svc.TypeOptionSelect(context.Background())
		if err != nil || len(outs) != 1 {
			t.Fatalf("outs = %d, err = %v", len(outs), err)
		}
	})
}

// TestAuditFillUpdateByFallback 未登录（ID=0）时审计人兜底 -1。
func TestAuditFillUpdateByFallback(t *testing.T) {
	pinNow(t)
	svc := NewService(nil, authctx.Context{}, nil)
	var got *SysDictType
	svc.typeRepo = &mockTypeRepo{create: func(_ context.Context, row *SysDictType) error {
		got = row
		return nil
	}}
	if err := svc.InsertType(context.Background(), TypeCreateIn{DictType: strPtr("sys_x")}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.CreateBy == nil || *got.CreateBy != -1 || got.UpdateBy == nil || *got.UpdateBy != -1 {
		t.Errorf("未登录审计人 = %v/%v, want -1/-1", got.CreateBy, got.UpdateBy)
	}
	if got.CreateDept != nil {
		t.Errorf("未登录 createDept 应为 NULL")
	}
}
