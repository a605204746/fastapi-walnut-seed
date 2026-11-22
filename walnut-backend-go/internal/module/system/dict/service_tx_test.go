package dict

// 本文件测试 UpdateType 的事务包裹（多写：数据表联动改名 + 类型保存）。
//
// 假 DB 只承担事务边界（BEGIN/COMMIT/ROLLBACK 经 database/sql 假驱动记录），
// repo 仍是 mock——二者组合即可离线断言「失败整体回滚 + 缓存提交后失效」，
// 不触达真实数据库。既有深度测试经 nil-db 守卫直调，不受影响。

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// ---- 假事务驱动：只记录 BEGIN/COMMIT/ROLLBACK ----

type txRecorder struct {
	mu        sync.Mutex
	events    []string
	commitErr error
	onCommit  func() // 提交时刻钩子（用于断言彼时缓存尚未失效）
}

func (r *txRecorder) record(ev string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *txRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type txConn struct{ rec *txRecorder }

func (c *txConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected Prepare") }
func (c *txConn) Close() error                        { return nil }
func (c *txConn) Begin() (driver.Tx, error) {
	c.rec.record("BEGIN")
	return &txHandle{rec: c.rec}, nil
}

type txHandle struct{ rec *txRecorder }

func (h *txHandle) Commit() error {
	if h.rec.onCommit != nil {
		h.rec.onCommit()
	}
	h.rec.record("COMMIT")
	return h.rec.commitErr
}

func (h *txHandle) Rollback() error {
	h.rec.record("ROLLBACK")
	return nil
}

type txConnector struct{ rec *txRecorder }

func (c txConnector) Connect(context.Context) (driver.Conn, error) { return &txConn{rec: c.rec}, nil }
func (c txConnector) Driver() driver.Driver                        { return txDriver{} }

type txDriver struct{}

func (txDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use sql.OpenDB") }

func newTxDB(t *testing.T, rec *txRecorder) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sql.OpenDB(txConnector{rec: rec}),
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		t.Fatalf("构造假事务 DB 失败: %v", err)
	}
	return db
}

func wantTxEvents(t *testing.T, rec *txRecorder, want ...string) {
	t.Helper()
	got := rec.snapshot()
	if len(got) != len(want) {
		t.Fatalf("事务事件 = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("事务事件[%d] = %q, want %q（完整: %v）", i, got[i], want[i], got)
		}
	}
}

// ---- UpdateType 事务测试 ----

// TestUpdateTypeTxSecondWriteFailureRollsBack 第二步（类型保存）失败：
// 错误透传（500 包装）、整体 ROLLBACK、缓存一个键都不动。
func TestUpdateTypeTxSecondWriteFailureRollsBack(t *testing.T) {
	pinNow(t)
	cache, f := newFakeCache(t)
	seed := map[string]string{
		"sys_dict:sys_old":     `[{"id":1}]`,
		"sys_dict_typesys_old": "old-type-cache",
		"sys_dict:sys_new":     "[]",
		"sys_dict_typesys_new": "new-type-cache",
		"sys_dict:sys_other":   "keep",
	}
	f.seed(seed)
	rec := &txRecorder{}
	svc := NewService(newTxDB(t, rec), testAuth(2), cache)
	old := typeRow(1, "旧名", "sys_old")
	svc.typeRepo = &mockTypeRepo{
		get: func(_ context.Context, _ int64) (*SysDictType, error) { return &old, nil },
		update: func(ctx context.Context, _ *SysDictType) error {
			if crudx.DB(ctx, nil) == nil {
				t.Error("类型保存应在事务内（ctx 应携带 tx 句柄）")
			}
			return errors.New("boom")
		},
	}
	svc.dataRepo = &mockDataRepo{renameType: func(ctx context.Context, _, _ string) (int64, error) {
		if crudx.DB(ctx, nil) == nil {
			t.Error("联动改名应在事务内（ctx 应携带 tx 句柄）")
		}
		return 2, nil
	}}
	err := svc.UpdateType(context.Background(), TypeUpdateIn{
		TypeCreateIn: TypeCreateIn{DictType: strPtr("sys_new")}, ID: i64p(1),
	})
	se := errs.As(err)
	if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
		t.Fatalf("err = %v, want 500 %s（detail boom）", err, errs.MsgDBFailed)
	}
	wantTxEvents(t, rec, "BEGIN", "ROLLBACK")
	if len(f.snapshot()) != len(seed) {
		t.Errorf("回滚不应失效缓存: %v", f.snapshot())
	}
	if dels := f.delCommands(); len(dels) != 0 {
		t.Errorf("回滚不应发 DEL: %v", dels)
	}
}

// TestUpdateTypeTxFirstWriteFailureRollsBack 第一步（联动改名）失败：
// 短路保存（update 未覆盖即 panic 兜底）、整体 ROLLBACK、缓存不动。
func TestUpdateTypeTxFirstWriteFailureRollsBack(t *testing.T) {
	pinNow(t)
	cache, f := newFakeCache(t)
	seed := map[string]string{"sys_dict:sys_old": `[{"id":1}]`, "sys_dict:sys_other": "keep"}
	f.seed(seed)
	rec := &txRecorder{}
	svc := NewService(newTxDB(t, rec), testAuth(2), cache)
	old := typeRow(1, "旧名", "sys_old")
	svc.typeRepo = &mockTypeRepo{
		get: func(_ context.Context, _ int64) (*SysDictType, error) { return &old, nil },
	}
	svc.dataRepo = &mockDataRepo{renameType: func(_ context.Context, _, _ string) (int64, error) {
		return 0, errors.New("boom")
	}}
	err := svc.UpdateType(context.Background(), TypeUpdateIn{
		TypeCreateIn: TypeCreateIn{DictType: strPtr("sys_new")}, ID: i64p(1),
	})
	wantErr(t, err, errs.MsgDBFailed)
	wantTxEvents(t, rec, "BEGIN", "ROLLBACK")
	if len(f.snapshot()) != len(seed) {
		t.Errorf("回滚不应失效缓存: %v", f.snapshot())
	}
}

// TestUpdateTypeTxCommitsThenEvictsCache 成功路径：两写同事务、COMMIT 提交，
// 且缓存失效发生在**提交之后**（提交时刻四键仍在，返回后才被删）。
func TestUpdateTypeTxCommitsThenEvictsCache(t *testing.T) {
	pinNow(t)
	cache, f := newFakeCache(t)
	seed := map[string]string{
		"sys_dict:sys_old":     `[{"id":1}]`,
		"sys_dict_typesys_old": "old-type-cache",
		"sys_dict:sys_new":     "[]",
		"sys_dict_typesys_new": "new-type-cache",
		"sys_dict:sys_other":   "keep",
	}
	f.seed(seed)
	rec := &txRecorder{}
	var cacheAtCommit map[string]string
	rec.onCommit = func() { cacheAtCommit = f.snapshot() }
	svc := NewService(newTxDB(t, rec), testAuth(2), cache)
	old := typeRow(1, "旧名", "sys_old")
	var seq []string
	svc.typeRepo = &mockTypeRepo{
		get: func(_ context.Context, _ int64) (*SysDictType, error) {
			seq = append(seq, "get")
			return &old, nil
		},
		update: func(ctx context.Context, _ *SysDictType) error {
			seq = append(seq, "update")
			if crudx.DB(ctx, nil) == nil {
				t.Error("类型保存应在事务内")
			}
			return nil
		},
	}
	svc.dataRepo = &mockDataRepo{renameType: func(ctx context.Context, _, _ string) (int64, error) {
		seq = append(seq, "rename")
		if crudx.DB(ctx, nil) == nil {
			t.Error("联动改名应在事务内")
		}
		return 2, nil
	}}
	if err := svc.UpdateType(context.Background(), TypeUpdateIn{
		TypeCreateIn: TypeCreateIn{DictType: strPtr("sys_new")}, ID: i64p(1),
	}); err != nil {
		t.Fatalf("err = %v", err)
	}
	wantTxEvents(t, rec, "BEGIN", "COMMIT")
	if len(seq) != 3 || seq[0] != "get" || seq[1] != "rename" || seq[2] != "update" {
		t.Errorf("调用序 = %v, want [get rename update]", seq)
	}
	// 提交时刻缓存尚未失效（失效在 Transaction 返回之后）
	if len(cacheAtCommit) != len(seed) {
		t.Errorf("提交时刻缓存应原样: %v", cacheAtCommit)
	}
	snap := f.snapshot()
	if len(snap) != 1 || snap["sys_dict:sys_other"] != "keep" {
		t.Errorf("提交后缓存失效结果 = %v, want 仅留 sys_dict:sys_other", snap)
	}
	dels := f.delCommands()
	if len(dels) != 1 || dels[0] != "DEL sys_dict:sys_old sys_dict_typesys_old sys_dict:sys_new sys_dict_typesys_new" {
		t.Errorf("DEL 命令 = %v, want 单命令四键", dels)
	}
}
