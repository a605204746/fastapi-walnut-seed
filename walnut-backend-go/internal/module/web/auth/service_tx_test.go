package auth

// 本文件测试 Register 的事务包裹（多写：CreateUser + 注册日志 InsertLogininfor）。
//
// 假 DB 只承担事务边界（BEGIN/COMMIT/ROLLBACK 经 database/sql 假驱动记录），
// repo 仍是 mock——组合断言「失败整体回滚 / 成功整体提交 / 写操作经 ctx 同 tx」。
// 验证码失败分支的登录日志留在事务外（Python 端 record_login_infor 用独立
// session 立即提交），本文件一并钉住该边界。

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/config"
	logmodel "walnut-seed-go/internal/module/system/log"
	"walnut-seed-go/internal/module/system/user"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// ---- 假事务驱动：只记录 BEGIN/COMMIT/ROLLBACK ----

type txRecorder struct {
	mu     sync.Mutex
	events []string
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
	h.rec.record("COMMIT")
	return nil
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

// registerRepo 覆盖注册链路三个 repo 方法；未覆盖的方法被调用即 panic。
type registerRepo struct {
	authRepo
	count     func(ctx context.Context, username string) (int64, error)
	create    func(ctx context.Context, u *user.SysUser) error
	insertLog func(ctx context.Context, entry *logmodel.SysLogininfor) error
}

func (m *registerRepo) CountActiveUsersByUsername(ctx context.Context, username string) (int64, error) {
	return m.count(ctx, username)
}

func (m *registerRepo) CreateUser(ctx context.Context, u *user.SysUser) error {
	return m.create(ctx, u)
}

func (m *registerRepo) InsertLogininfor(ctx context.Context, entry *logmodel.SysLogininfor) error {
	return m.insertLog(ctx, entry)
}

func registerBody() *RegisterRequest {
	return &RegisterRequest{Username: "newuser", Password: "Abc12345!"}
}

// ---- Register 事务测试 ----

// TestRegisterTxSuccessCommits 成功路径：建号与注册日志同事务（ctx 携带 tx），
// 整体 COMMIT；日志内容（状态/文案/账号）不变。
func TestRegisterTxSuccessCommits(t *testing.T) {
	rec := &txRecorder{}
	svc := NewService(newTxDB(t, rec), nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{})
	var gotUser *user.SysUser
	var entries []*logmodel.SysLogininfor
	svc.repo = &registerRepo{
		count: func(_ context.Context, _ string) (int64, error) { return 0, nil },
		create: func(ctx context.Context, u *user.SysUser) error {
			if crudx.DB(ctx, nil) == nil {
				t.Error("CreateUser 应在事务内（ctx 应携带 tx 句柄）")
			}
			gotUser = u
			return nil
		},
		insertLog: func(ctx context.Context, e *logmodel.SysLogininfor) error {
			if crudx.DB(ctx, nil) == nil {
				t.Error("注册日志应与 CreateUser 同事务")
			}
			entries = append(entries, e)
			return nil
		},
	}
	if err := svc.Register(context.Background(), registerBody(), RequestMeta{}); err != nil {
		t.Fatalf("err = %v", err)
	}
	wantTxEvents(t, rec, "BEGIN", "COMMIT")
	if gotUser == nil || gotUser.UserName != "newuser" {
		t.Fatalf("建号参数错误: %+v", gotUser)
	}
	if len(entries) != 1 {
		t.Fatalf("注册日志条数 = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.UserName == nil || *e.UserName != "newuser" || e.Status == nil || *e.Status != logmodel.StatusSuccess ||
		e.Msg == nil || *e.Msg != "注册成功" {
		t.Errorf("注册日志内容错误: %+v", e)
	}
}

// TestRegisterTxCreateUserFailureRollsBack 建号失败：错误透传、整体 ROLLBACK、
// 不写注册日志（insertLog 未覆盖即 panic 兜底）。
func TestRegisterTxCreateUserFailureRollsBack(t *testing.T) {
	rec := &txRecorder{}
	svc := NewService(newTxDB(t, rec), nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{})
	boom := errors.New("boom")
	svc.repo = &registerRepo{
		count:  func(_ context.Context, _ string) (int64, error) { return 0, nil },
		create: func(_ context.Context, _ *user.SysUser) error { return boom },
	}
	err := svc.Register(context.Background(), registerBody(), RequestMeta{})
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "创建用户失败") {
		t.Fatalf("err = %v, want 包装「创建用户失败」且透传 boom", err)
	}
	wantTxEvents(t, rec, "BEGIN", "ROLLBACK")
}

// TestRegisterTxDuplicateKeyRollsBack 并发撞唯一约束：映射「注册账号已存在」
// 哨兵码（不暴露 SQL 错误），事务整体 ROLLBACK。
func TestRegisterTxDuplicateKeyRollsBack(t *testing.T) {
	rec := &txRecorder{}
	svc := NewService(newTxDB(t, rec), nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{})
	svc.repo = &registerRepo{
		count:  func(_ context.Context, _ string) (int64, error) { return 0, nil },
		create: func(_ context.Context, _ *user.SysUser) error { return gorm.ErrDuplicatedKey },
	}
	err := svc.Register(context.Background(), registerBody(), RequestMeta{})
	se := errs.As(err)
	if se == nil || se.Code != errs.AuthUserRegisterExists.Code {
		t.Fatalf("err = %v, want 注册账号已存在（code %d）", err, errs.AuthUserRegisterExists.Code)
	}
	wantTxEvents(t, rec, "BEGIN", "ROLLBACK")
}

// TestRegisterTxLogFailureSwallowed 注册日志写失败仅记日志不向上抛
// （旁路记录语义）：用户仍随事务提交，与 Python 端吞异常一致。
func TestRegisterTxLogFailureSwallowed(t *testing.T) {
	rec := &txRecorder{}
	svc := NewService(newTxDB(t, rec), nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{})
	svc.repo = &registerRepo{
		count:     func(_ context.Context, _ string) (int64, error) { return 0, nil },
		create:    func(_ context.Context, _ *user.SysUser) error { return nil },
		insertLog: func(_ context.Context, _ *logmodel.SysLogininfor) error { return errors.New("log down") },
	}
	if err := svc.Register(context.Background(), registerBody(), RequestMeta{}); err != nil {
		t.Fatalf("err = %v, want 日志失败不影响注册", err)
	}
	wantTxEvents(t, rec, "BEGIN", "COMMIT")
}

// TestRegisterCaptchaFailLogOutsideTx 验证码失败分支：登录日志经独立连接写入
// （ctx 无 tx、全程不开事务），不随注册失败回滚——对齐 Python 端
// record_login_infor 的独立 session 立即提交语义。
func TestRegisterCaptchaFailLogOutsideTx(t *testing.T) {
	rec := &txRecorder{}
	cache, _ := newFakeCache(t, "")
	svc := NewService(newTxDB(t, rec), cache, &config.Config{CaptchaEnable: true}, nil, "", Deps{})
	var entries []*logmodel.SysLogininfor
	svc.repo = &registerRepo{
		insertLog: func(ctx context.Context, e *logmodel.SysLogininfor) error {
			if crudx.DB(ctx, nil) != nil {
				t.Error("验证码失败日志不应在事务内（Python 端独立 session）")
			}
			entries = append(entries, e)
			return nil
		},
	}
	body := registerBody()
	body.Code = strPtr("abc")
	body.UUID = strPtr("u1") // 未种验证码键 → 过期分支
	err := svc.Register(context.Background(), body, RequestMeta{})
	se := errs.As(err)
	if se == nil || se.Code != errs.AuthCaptchaExpired.Code {
		t.Fatalf("err = %v, want 10001 验证码已失效", err)
	}
	wantTxEvents(t, rec) // 全程无 BEGIN/COMMIT/ROLLBACK
	if len(entries) != 1 || entries[0].Msg == nil || *entries[0].Msg != "验证码已失效" {
		t.Errorf("验证码失败日志错误: %+v", entries)
	}
}

// TestRegisterNilDBHarness nil-db 守卫：测试 harness 直调写部分，
// 不开事务；recordLogininfor 自身的 nil-db 守卫跳过日志写入。
func TestRegisterNilDBHarness(t *testing.T) {
	svc := NewService(nil, nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{})
	created := false
	svc.repo = &registerRepo{
		count: func(_ context.Context, _ string) (int64, error) { return 0, nil },
		create: func(ctx context.Context, _ *user.SysUser) error {
			if crudx.DB(ctx, nil) != nil {
				t.Error("nil-db 直调不应携带 tx")
			}
			created = true
			return nil
		},
	}
	if err := svc.Register(context.Background(), registerBody(), RequestMeta{}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !created {
		t.Error("nil-db 路径应直调建号")
	}
}
