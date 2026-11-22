package role

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

// mockRepo 嵌入 roleRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	roleRepo
	listByIDs             func(ctx context.Context, ids []int64) ([]SysRole, error)
	getByIDNoScope        func(ctx context.Context, id int64) (*SysRole, error)
	checkedDeptIDs        func(ctx context.Context, roleID int64, deptCheckStrictly bool) ([]int64, error)
	existsByName          func(ctx context.Context, name string, excludeID *int64) (bool, error)
	existsByKey           func(ctx context.Context, key string, excludeID *int64) (bool, error)
	create                func(ctx context.Context, row *SysRole) error
	update                func(ctx context.Context, row *SysRole) error
	get                   func(ctx context.Context, id int64, scope permx.Scope) (*SysRole, error)
	countVisible          func(ctx context.Context, ids []int64, scope permx.Scope) (int64, error)
	countUserRoleByRoleID func(ctx context.Context, roleID int64) (int64, error)
	currentUserRoleIDs    func(ctx context.Context, userID int64) ([]int64, error)
	deleteRoleMenu        func(ctx context.Context, roleID int64) error
	insertRoleMenu        func(ctx context.Context, roleID int64, menuIDs []int64) (int, error)
	deleteRoleDept        func(ctx context.Context, roleID int64) error
	insertRoleDept        func(ctx context.Context, roleID int64, deptIDs []int64) (int, error)
	deleteRoleMenuBatch   func(ctx context.Context, roleIDs []int64) error
	deleteRoleDeptBatch   func(ctx context.Context, roleIDs []int64) error
	softDelete            func(ctx context.Context, ids []int64) (int64, error)
}

func (m *mockRepo) ListByIDs(ctx context.Context, ids []int64) ([]SysRole, error) {
	return m.listByIDs(ctx, ids)
}

func (m *mockRepo) GetByIDNoScope(ctx context.Context, id int64) (*SysRole, error) {
	return m.getByIDNoScope(ctx, id)
}

func (m *mockRepo) CheckedDeptIDs(ctx context.Context, roleID int64, deptCheckStrictly bool) ([]int64, error) {
	return m.checkedDeptIDs(ctx, roleID, deptCheckStrictly)
}

func (m *mockRepo) ExistsByName(ctx context.Context, name string, excludeID *int64) (bool, error) {
	return m.existsByName(ctx, name, excludeID)
}

func (m *mockRepo) ExistsByKey(ctx context.Context, key string, excludeID *int64) (bool, error) {
	return m.existsByKey(ctx, key, excludeID)
}

func (m *mockRepo) Create(ctx context.Context, row *SysRole) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysRole) error {
	return m.update(ctx, row)
}

func (m *mockRepo) Get(ctx context.Context, id int64, scope permx.Scope) (*SysRole, error) {
	return m.get(ctx, id, scope)
}

func (m *mockRepo) CountVisible(ctx context.Context, ids []int64, scope permx.Scope) (int64, error) {
	return m.countVisible(ctx, ids, scope)
}

func (m *mockRepo) CountUserRoleByRoleID(ctx context.Context, roleID int64) (int64, error) {
	return m.countUserRoleByRoleID(ctx, roleID)
}

func (m *mockRepo) CurrentUserRoleIDs(ctx context.Context, userID int64) ([]int64, error) {
	return m.currentUserRoleIDs(ctx, userID)
}

func (m *mockRepo) DeleteRoleMenu(ctx context.Context, roleID int64) error {
	return m.deleteRoleMenu(ctx, roleID)
}

func (m *mockRepo) InsertRoleMenu(ctx context.Context, roleID int64, menuIDs []int64) (int, error) {
	return m.insertRoleMenu(ctx, roleID, menuIDs)
}

func (m *mockRepo) DeleteRoleDept(ctx context.Context, roleID int64) error {
	return m.deleteRoleDept(ctx, roleID)
}

func (m *mockRepo) InsertRoleDept(ctx context.Context, roleID int64, deptIDs []int64) (int, error) {
	return m.insertRoleDept(ctx, roleID, deptIDs)
}

func (m *mockRepo) DeleteRoleMenuBatch(ctx context.Context, roleIDs []int64) error {
	return m.deleteRoleMenuBatch(ctx, roleIDs)
}

func (m *mockRepo) DeleteRoleDeptBatch(ctx context.Context, roleIDs []int64) error {
	return m.deleteRoleDeptBatch(ctx, roleIDs)
}

func (m *mockRepo) SoftDelete(ctx context.Context, ids []int64) (int64, error) {
	return m.softDelete(ctx, ids)
}

// mockDepts 仅覆盖 TreeSelectAll；未赋值时被调用即 panic。
type mockDepts struct {
	treeSelectAll func(ctx context.Context) ([]*dept.TreeNode, error)
}

func (m *mockDepts) TreeSelectAll(ctx context.Context) ([]*dept.TreeNode, error) {
	return m.treeSelectAll(ctx)
}

// ---- 假 DB：在 database/sql 驱动层伪造 permx.fetchRoleScopes ----
//
// buildScope → permx.Build 直接访问 s.db（绕过 roleRepo）：nil DB 报错、
// DryRun 不执行查询。用 sql.OpenDB + 假 driver.Conn 返回预设角色数据范围，
// 使「超管 NoFilter / 普通用户自定义范围」等分支可由 authctx 身份驱动。

type fakeSQL struct {
	mu       sync.Mutex
	scopes   [][]driver.Value // fetchRoleScopes 结果行：(role_id, data_scope)
	queries  []string
	txEvents []string // 事务事件序：begin / commit / rollback（证明回滚语义）
}

// txSnapshot 返回事务事件序副本，供断言「失败必 rollback、成功才 commit」。
func (f *fakeSQL) txSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.txEvents...)
}

func (f *fakeSQL) recordTx(event string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txEvents = append(f.txEvents, event)
}

type fakeRows struct {
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.pos])
	r.pos++
	return nil
}

type fakeConn struct{ f *fakeSQL }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected Prepare") }
func (c *fakeConn) Close() error                        { return nil }

// Begin 记录事务开启并返回可记录 Commit/Rollback 的假 Tx。
// service 多写方法经 db.Transaction 包裹后会驱动这里；成功路径 Commit、
// 失败路径 Rollback 都被记入 fakeSQL.txEvents，供回滚语义断言。
func (c *fakeConn) Begin() (driver.Tx, error) {
	c.f.recordTx("begin")
	return &fakeTx{f: c.f}, nil
}

// fakeTx 记录事务收尾事件（commit / rollback），不执行真实 SQL。
type fakeTx struct{ f *fakeSQL }

func (tx *fakeTx) Commit() error   { tx.f.recordTx("commit"); return nil }
func (tx *fakeTx) Rollback() error { tx.f.recordTx("rollback"); return nil }

func (c *fakeConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	c.f.mu.Lock()
	c.f.queries = append(c.f.queries, q)
	c.f.mu.Unlock()
	if strings.Contains(q, "sys_role AS r") { // permx.fetchRoleScopes
		return &fakeRows{cols: []string{"role_id", "data_scope"}, data: c.f.scopes}, nil
	}
	return nil, fmt.Errorf("unexpected query: %s", q)
}

type fakeConnector struct{ f *fakeSQL }

func (c fakeConnector) Connect(context.Context) (driver.Conn, error) { return &fakeConn{f: c.f}, nil }
func (c fakeConnector) Driver() driver.Driver                        { return fakeDriver{} }

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use sql.OpenDB") }

func newFakeDB(t *testing.T, f *fakeSQL) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sql.OpenDB(fakeConnector{f: f}),
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		t.Fatalf("构造假 DB 失败: %v", err)
	}
	return db
}

func scopeRow(roleID int64, dataScope string) []driver.Value {
	return []driver.Value{roleID, dataScope}
}

// ---- 测试辅助 ----

func testAuth(id int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: id, UserName: "tester"}}
}

// superAuth 超管身份：CheckRoleDataScope/CheckRoleSelfEdit 短路放行，permx NoFilter。
func superAuth() authctx.Context {
	dept := int64(100)
	return authctx.Context{User: authctx.User{
		ID: modelx.SuperAdminID, UserName: "admin", DeptID: &dept, IsSuperuser: true,
	}}
}

// normalAuth 普通用户身份：数据权限走 permx.Build + 可见数校验。
func normalAuth(id, deptID int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: id, UserName: "tester", DeptID: &deptID}}
}

func fixNow(t *testing.T) time.Time {
	t.Helper()
	old := nowFunc
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = old })
	return now
}

func newRoleSvc(db *gorm.DB, ac authctx.Context, repo roleRepo) *Service {
	svc := NewService(db, ac, Deps{})
	svc.repo = repo
	return svc
}

func wantSvcErr(t *testing.T, err error, msg string) {
	t.Helper()
	se := errs.As(err)
	if se == nil || se.Msg != msg {
		t.Fatalf("err = %v, want %q", err, msg)
	}
}

func wantCalls(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("调用序列 = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("调用序列[%d] = %q, want %q（完整: %v）", i, got[i], want[i], got)
		}
	}
}

func i64(v int64) *int64 { return &v }
func iptr(v int) *int    { return &v }
func bptr(v bool) *bool  { return &v }

// commonRole 非内置角色（CheckRoleAllowed 放行）。
func commonRole(id int64) SysRole {
	r := SysRole{RoleName: "普通角色", RoleKey: "common", Status: "0", DelFlag: "0"}
	r.ID = id
	return r
}

// ---- 既有守卫 / DeptTree 用例 ----

func TestInsertRoleMissingDefaults(t *testing.T) {
	svc := NewService(nil, testAuth(2), Deps{})
	svc.repo = &mockRepo{} // 守卫先于任何数据访问，触发 repo 调用即 panic
	roleSort := 1
	cases := []struct {
		name string
		in   CreateIn
	}{
		{"缺status与roleSort", CreateIn{}},
		{"缺roleSort", CreateIn{Status: strPtr("0")}},
		{"缺status", CreateIn{RoleSort: &roleSort}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := svc.InsertRole(context.Background(), tc.in)
			if ok {
				t.Error("缺省 nullable=False 列应新增失败")
			}
			se := errs.As(err)
			if se == nil || se.Code != errs.CodeConflict || se.Msg != errs.MsgDBIntegrity {
				t.Errorf("err = %v, want 409 %s", err, errs.MsgDBIntegrity)
			}
		})
	}
}

func TestCancelAuthUserSelfGuard(t *testing.T) {
	svc := NewService(nil, testAuth(7), Deps{})
	svc.repo = &mockRepo{}
	roleID, userID := int64(3), int64(7)
	n, err := svc.CancelAuthUser(context.Background(), CancelIn{RoleID: &roleID, UserID: &userID})
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	if se := errs.As(err); se == nil || se.Msg != "不允许修改当前用户角色!" {
		t.Errorf("err = %v, want 不允许修改当前用户角色!", err)
	}
}

func TestCheckRoleAllowedBuiltinKey(t *testing.T) {
	svc := NewService(nil, testAuth(2), Deps{})
	builtin := SysRole{RoleKey: "admin"}
	builtin.ID = 2
	svc.repo = &mockRepo{listByIDs: func(_ context.Context, ids []int64) ([]SysRole, error) {
		if len(ids) != 1 || ids[0] != 2 {
			t.Errorf("ids = %v, want [2]", ids)
		}
		return []SysRole{builtin}, nil
	}}
	id := int64(2)
	// dataScope/changeStatus 传 role_key=None：DB key 非空 → 恒视为「变化」→ 拒绝
	if err := svc.CheckRoleAllowed(context.Background(), &id, nil); errs.As(err) == nil ||
		errs.As(err).Msg != "不允许修改系统内置管理员角色标识符!" {
		t.Errorf("err = %v, want 不允许修改系统内置管理员角色标识符!", err)
	}
	// 保持原 key（未变化）→ 放行
	key := "admin"
	if err := svc.CheckRoleAllowed(context.Background(), &id, &key); err != nil {
		t.Errorf("保持原 key 应放行, got %v", err)
	}
}

func TestDeptTreeRoleMissing(t *testing.T) {
	// 不注入 depts、不覆盖 checkedDeptIDs：角色不存在应短路，
	// 走到后续编排即 panic，暴露意外依赖。
	svc := NewService(nil, testAuth(2), Deps{})
	svc.repo = &mockRepo{getByIDNoScope: func(_ context.Context, id int64) (*SysRole, error) {
		if id != 9 {
			t.Errorf("id = %d, want 9", id)
		}
		return nil, nil
	}}
	_, err := svc.DeptTree(context.Background(), 9)
	se := errs.As(err)
	if se == nil || se.Code != errs.CodeError || se.Msg != "角色不存在" {
		t.Errorf("err = %v, want 500 角色不存在", err)
	}
}

func TestDeptTreeDBErrorWrapped(t *testing.T) {
	svc := NewService(nil, testAuth(2), Deps{})
	svc.repo = &mockRepo{getByIDNoScope: func(_ context.Context, _ int64) (*SysRole, error) {
		return nil, errors.New("boom")
	}}
	_, err := svc.DeptTree(context.Background(), 5)
	se := errs.As(err)
	if se == nil || se.Code != errs.CodeError || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
		t.Errorf("err = %v, want 500 %s (detail=boom)", err, errs.MsgDBFailed)
	}
}

func TestDeptTreeAssemble(t *testing.T) {
	strictly := 1
	row := &SysRole{DeptCheckStrictly: &strictly}
	row.ID = 5
	tree := []*dept.TreeNode{{ID: 100}, {ID: 101}}
	var gotDCS bool
	svc := NewService(nil, testAuth(2), Deps{NewDepts: func(ac authctx.Context) DeptProvider {
		if ac.User.ID != 2 {
			t.Errorf("工厂收到 authctx user = %d, want 2", ac.User.ID)
		}
		return &mockDepts{treeSelectAll: func(_ context.Context) ([]*dept.TreeNode, error) {
			return tree, nil
		}}
	}})
	svc.repo = &mockRepo{
		getByIDNoScope: func(_ context.Context, id int64) (*SysRole, error) {
			if id != 5 {
				t.Errorf("id = %d, want 5", id)
			}
			return row, nil
		},
		checkedDeptIDs: func(_ context.Context, roleID int64, dcs bool) ([]int64, error) {
			if roleID != 5 {
				t.Errorf("roleID = %d, want 5", roleID)
			}
			gotDCS = dcs
			return []int64{100, 101}, nil
		},
	}
	out, err := svc.DeptTree(context.Background(), 5)
	if err != nil {
		t.Fatalf("DeptTree err = %v, want nil", err)
	}
	if !gotDCS {
		t.Error("DeptCheckStrictly=1 应推导 dcs=true")
	}
	wantKeys := []idtype.JSInt{100, 101}
	if len(out.CheckedKeys) != len(wantKeys) {
		t.Fatalf("CheckedKeys = %v, want %v", out.CheckedKeys, wantKeys)
	}
	for i, k := range wantKeys {
		if out.CheckedKeys[i] != k {
			t.Errorf("CheckedKeys[%d] = %d, want %d", i, out.CheckedKeys[i], k)
		}
	}
	if len(out.Depts) != len(tree) || out.Depts[0] != tree[0] || out.Depts[1] != tree[1] {
		t.Errorf("Depts = %v, want 原样透传 dept 树", out.Depts)
	}
}

func TestDeptTreeNilStrictlyEmptyKeys(t *testing.T) {
	// DeptCheckStrictly 为 NULL → dcs=false；勾选为空 → checkedKeys 仍是 []（非 null）
	row := &SysRole{}
	row.ID = 5
	svc := NewService(nil, testAuth(2), Deps{NewDepts: func(_ authctx.Context) DeptProvider {
		return &mockDepts{treeSelectAll: func(_ context.Context) ([]*dept.TreeNode, error) {
			return nil, nil
		}}
	}})
	svc.repo = &mockRepo{
		getByIDNoScope: func(_ context.Context, _ int64) (*SysRole, error) { return row, nil },
		checkedDeptIDs: func(_ context.Context, _ int64, dcs bool) ([]int64, error) {
			if dcs {
				t.Error("DeptCheckStrictly=NULL 应推导 dcs=false")
			}
			return nil, nil
		},
	}
	out, err := svc.DeptTree(context.Background(), 5)
	if err != nil {
		t.Fatalf("DeptTree err = %v, want nil", err)
	}
	if out.CheckedKeys == nil || len(out.CheckedKeys) != 0 {
		t.Errorf("CheckedKeys = %v, want 非 nil 空切片（JSON []）", out.CheckedKeys)
	}
}

// ---- InsertRole 深度业务流 ----

func TestInsertRoleSuccessDefaults(t *testing.T) {
	now := fixNow(t)
	var calls []string
	var created *SysRole
	var menuRoleID int64
	var menuIDs []int64
	repo := &mockRepo{
		create: func(_ context.Context, row *SysRole) error {
			calls = append(calls, "create")
			cp := *row
			created = &cp
			return nil
		},
		insertRoleMenu: func(_ context.Context, rid int64, mids []int64) (int, error) {
			calls = append(calls, "insertRoleMenu")
			menuRoleID, menuIDs = rid, mids
			return len(mids), nil
		},
		// insertRoleDept 未覆盖：非自定义数据范围不应建立部门关联
	}
	svc := newRoleSvc(nil, superAuth(), repo)
	in := CreateIn{
		RoleName: strPtr("测试角色"), RoleKey: strPtr("test"), RoleSort: iptr(3),
		Status: strPtr("0"), MenuIDs: []int64{10, 11}, Remark: strPtr("备注"),
	}
	ok, err := svc.InsertRole(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("InsertRole = (%v, %v), want (true, nil)", ok, err)
	}
	if created == nil || created.ID == 0 {
		t.Fatalf("Create 未填充雪花 ID: %+v", created)
	}
	if created.RoleName != "测试角色" || created.RoleKey != "test" || created.RoleSort != 3 {
		t.Errorf("基础字段 = %q/%q/%d", created.RoleName, created.RoleKey, created.RoleSort)
	}
	if created.Status != "0" || created.DelFlag != modelx.Normal {
		t.Errorf("Status/DelFlag = %q/%q, want 0/0", created.Status, created.DelFlag)
	}
	// 缺省列默认：data_scope='1'、menu/dept_check_strictly=1（Python 列默认对齐）
	if created.DataScope == nil || *created.DataScope != "1" {
		t.Errorf("DataScope = %v, want 默认 1", created.DataScope)
	}
	if created.MenuCheckStrictly == nil || *created.MenuCheckStrictly != 1 ||
		created.DeptCheckStrictly == nil || *created.DeptCheckStrictly != 1 {
		t.Errorf("strictly 默认 = %v/%v, want 1/1", created.MenuCheckStrictly, created.DeptCheckStrictly)
	}
	if created.Remark == nil || *created.Remark != "备注" {
		t.Errorf("Remark = %v", created.Remark)
	}
	// 审计字段
	if created.CreateBy == nil || *created.CreateBy != modelx.SuperAdminID ||
		created.UpdateBy == nil || *created.UpdateBy != modelx.SuperAdminID {
		t.Errorf("CreateBy/UpdateBy = %v/%v, want 1/1", created.CreateBy, created.UpdateBy)
	}
	if created.CreateDept == nil || *created.CreateDept != 100 {
		t.Errorf("CreateDept = %v, want 100（操作者部门）", created.CreateDept)
	}
	if created.CreateTime == nil || !created.CreateTime.Equal(now) ||
		created.UpdateTime == nil || !created.UpdateTime.Equal(now) {
		t.Errorf("CreateTime/UpdateTime = %v/%v, want %v", created.CreateTime, created.UpdateTime, now)
	}
	// 菜单关联挂在新角色 ID 上
	wantCalls(t, calls, []string{"create", "insertRoleMenu"})
	if menuRoleID != created.ID {
		t.Errorf("InsertRoleMenu roleID = %d, want %d", menuRoleID, created.ID)
	}
	if len(menuIDs) != 2 || menuIDs[0] != 10 || menuIDs[1] != 11 {
		t.Errorf("menuIDs = %v, want [10 11]", menuIDs)
	}
}

func TestInsertRoleCustomScopeAndStrictly(t *testing.T) {
	var calls []string
	var created *SysRole
	var deptRoleID int64
	var deptIDs []int64
	repo := &mockRepo{
		create: func(_ context.Context, row *SysRole) error {
			calls = append(calls, "create")
			cp := *row
			created = &cp
			return nil
		},
		insertRoleMenu: func(_ context.Context, _ int64, mids []int64) (int, error) {
			calls = append(calls, fmt.Sprintf("insertRoleMenu%v", mids))
			return len(mids), nil
		},
		insertRoleDept: func(_ context.Context, rid int64, dids []int64) (int, error) {
			calls = append(calls, fmt.Sprintf("insertRoleDept%v", dids))
			deptRoleID, deptIDs = rid, dids
			return len(dids), nil
		},
	}
	svc := newRoleSvc(nil, superAuth(), repo)
	in := CreateIn{
		RoleName: strPtr("自定义"), RoleKey: strPtr("custom"), RoleSort: iptr(1), Status: strPtr("0"),
		DataScope: strPtr(permx.ScopeCustom), DeptIDs: []int64{100, 101},
		MenuCheckStrictly: bptr(false), DeptCheckStrictly: bptr(true),
	}
	ok, err := svc.InsertRole(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("InsertRole = (%v, %v), want (true, nil)", ok, err)
	}
	if created.DataScope == nil || *created.DataScope != permx.ScopeCustom {
		t.Errorf("DataScope = %v, want 2", created.DataScope)
	}
	if created.MenuCheckStrictly == nil || *created.MenuCheckStrictly != 0 ||
		created.DeptCheckStrictly == nil || *created.DeptCheckStrictly != 1 {
		t.Errorf("bool→int 转换 = %v/%v, want 0/1", created.MenuCheckStrictly, created.DeptCheckStrictly)
	}
	wantCalls(t, calls, []string{"create", "insertRoleMenu[]", "insertRoleDept[100 101]"})
	if deptRoleID != created.ID {
		t.Errorf("InsertRoleDept roleID = %d, want %d", deptRoleID, created.ID)
	}
	if len(deptIDs) != 2 || deptIDs[0] != 100 || deptIDs[1] != 101 {
		t.Errorf("deptIDs = %v, want [100 101]", deptIDs)
	}
}

func TestInsertRoleCreateError(t *testing.T) {
	repo := &mockRepo{
		create: func(_ context.Context, _ *SysRole) error { return errors.New("boom") },
		// insertRoleMenu 未覆盖：落库失败不应建立关联
	}
	svc := newRoleSvc(nil, superAuth(), repo)
	in := CreateIn{RoleSort: iptr(1), Status: strPtr("0")}
	ok, err := svc.InsertRole(context.Background(), in)
	if ok || err == nil || err.Error() != "boom" {
		t.Fatalf("= (%v, %v), want (false, boom 原样透传)", ok, err)
	}
}

func TestInsertRoleMenuError(t *testing.T) {
	repo := &mockRepo{
		create:         func(_ context.Context, _ *SysRole) error { return nil },
		insertRoleMenu: func(_ context.Context, _ int64, _ []int64) (int, error) { return 0, errors.New("boom") },
	}
	svc := newRoleSvc(nil, superAuth(), repo)
	in := CreateIn{RoleSort: iptr(1), Status: strPtr("0"), MenuIDs: []int64{10}}
	ok, err := svc.InsertRole(context.Background(), in)
	if ok || err == nil || err.Error() != "boom" {
		t.Fatalf("= (%v, %v), want (false, boom)", ok, err)
	}
}

// ---- 唯一性校验（handler 在 Insert/Update 前调用，excludeID 语义是契约） ----

func TestCheckRoleNameUnique(t *testing.T) {
	t.Run("新增不排除", func(t *testing.T) {
		var gotName string
		var gotExclude *int64
		repo := &mockRepo{existsByName: func(_ context.Context, name string, excludeID *int64) (bool, error) {
			gotName, gotExclude = name, excludeID
			return true, nil
		}}
		svc := newRoleSvc(nil, superAuth(), repo)
		exists, err := svc.CheckRoleNameUnique(context.Background(), "管理员", nil)
		if err != nil || !exists {
			t.Fatalf("= (%v, %v), want (true, nil)", exists, err)
		}
		if gotName != "管理员" || gotExclude != nil {
			t.Errorf("name/excludeID = %q/%v, want 管理员/nil", gotName, gotExclude)
		}
	})
	t.Run("修改排除自身", func(t *testing.T) {
		var gotExclude *int64
		repo := &mockRepo{existsByName: func(_ context.Context, _ string, excludeID *int64) (bool, error) {
			gotExclude = excludeID
			return false, nil
		}}
		svc := newRoleSvc(nil, superAuth(), repo)
		exists, err := svc.CheckRoleNameUnique(context.Background(), "管理员", i64(5))
		if err != nil || exists {
			t.Fatalf("= (%v, %v), want (false, nil)", exists, err)
		}
		if gotExclude == nil || *gotExclude != 5 {
			t.Errorf("excludeID = %v, want 指向 5", gotExclude)
		}
	})
}

func TestCheckRoleKeyUnique(t *testing.T) {
	t.Run("新增不排除", func(t *testing.T) {
		var gotKey string
		var gotExclude *int64
		repo := &mockRepo{existsByKey: func(_ context.Context, key string, excludeID *int64) (bool, error) {
			gotKey, gotExclude = key, excludeID
			return true, nil
		}}
		svc := newRoleSvc(nil, superAuth(), repo)
		exists, err := svc.CheckRoleKeyUnique(context.Background(), "admin", nil)
		if err != nil || !exists {
			t.Fatalf("= (%v, %v), want (true, nil)", exists, err)
		}
		if gotKey != "admin" || gotExclude != nil {
			t.Errorf("key/excludeID = %q/%v, want admin/nil", gotKey, gotExclude)
		}
	})
	t.Run("修改排除自身", func(t *testing.T) {
		var gotExclude *int64
		repo := &mockRepo{existsByKey: func(_ context.Context, _ string, excludeID *int64) (bool, error) {
			gotExclude = excludeID
			return false, nil
		}}
		svc := newRoleSvc(nil, superAuth(), repo)
		exists, err := svc.CheckRoleKeyUnique(context.Background(), "admin", i64(5))
		if err != nil || exists {
			t.Fatalf("= (%v, %v), want (false, nil)", exists, err)
		}
		if gotExclude == nil || *gotExclude != 5 {
			t.Errorf("excludeID = %v, want 指向 5", gotExclude)
		}
	})
}

func TestCheckRoleAllowedInsertBuiltinKey(t *testing.T) {
	svc := newRoleSvc(nil, superAuth(), &mockRepo{}) // listByIDs 未覆盖：新增分支不应查库
	for _, key := range []string{"admin", "superadmin"} {
		err := svc.CheckRoleAllowed(context.Background(), nil, &key)
		wantSvcErr(t, err, "不允许使用系统内置管理员角色标识符!")
	}
	common := "common"
	if err := svc.CheckRoleAllowed(context.Background(), nil, &common); err != nil {
		t.Errorf("非内置 key 应放行, got %v", err)
	}
	if err := svc.CheckRoleAllowed(context.Background(), nil, nil); err != nil {
		t.Errorf("缺省 key 应放行, got %v", err)
	}
}

// ---- UpdateRole 深度业务流 ----

func TestUpdateRoleSuccessFullFlow(t *testing.T) {
	now := fixNow(t)
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)

	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	loaded := &SysRole{
		RoleName: "旧名", RoleKey: "old", RoleSort: 1, Status: "0", DelFlag: "0",
		DataScope: strPtr("1"), MenuCheckStrictly: iptr(1), DeptCheckStrictly: iptr(1),
	}
	loaded.ID = 5
	loaded.CreateBy = i64(7)
	loaded.CreateTime = &past

	var calls []string
	var gotScope permx.Scope
	var updated *SysRole
	var menuIDs, deptIDs []int64
	repo := &mockRepo{
		get: func(_ context.Context, id int64, scope permx.Scope) (*SysRole, error) {
			calls = append(calls, "get")
			if id != 5 {
				t.Errorf("Get id = %d, want 5", id)
			}
			gotScope = scope
			return loaded, nil
		},
		update: func(_ context.Context, row *SysRole) error {
			calls = append(calls, "update")
			cp := *row
			updated = &cp
			return nil
		},
		deleteRoleMenu: func(_ context.Context, rid int64) error {
			calls = append(calls, fmt.Sprintf("deleteRoleMenu:%d", rid))
			return nil
		},
		insertRoleMenu: func(_ context.Context, _ int64, mids []int64) (int, error) {
			calls = append(calls, fmt.Sprintf("insertRoleMenu%v", mids))
			menuIDs = mids
			return len(mids), nil
		},
		deleteRoleDept: func(_ context.Context, rid int64) error {
			calls = append(calls, fmt.Sprintf("deleteRoleDept:%d", rid))
			return nil
		},
		insertRoleDept: func(_ context.Context, _ int64, dids []int64) (int, error) {
			calls = append(calls, fmt.Sprintf("insertRoleDept%v", dids))
			deptIDs = dids
			return len(dids), nil
		},
		// countUserRoleByRoleID 未覆盖：status=0 不触发禁用校验
	}
	svc := newRoleSvc(db, superAuth(), repo)
	in := UpdateIn{
		CreateIn: CreateIn{
			RoleName: strPtr("新名"), RoleKey: strPtr("newkey"), RoleSort: iptr(9),
			DataScope: strPtr(permx.ScopeCustom), MenuCheckStrictly: bptr(false), DeptCheckStrictly: bptr(false),
			Status: strPtr("0"), Remark: strPtr("新备注"), MenuIDs: []int64{10}, DeptIDs: []int64{100},
		},
		ID: i64(5),
	}
	ok, err := svc.UpdateRole(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("UpdateRole = (%v, %v), want (true, nil)", ok, err)
	}
	if !gotScope.NoFilter {
		t.Errorf("超管 scope = %+v, want NoFilter", gotScope)
	}
	// 顺序：取角色 → 更新 → 重建菜单关联 → 自定义范围才重建部门关联
	wantCalls(t, calls, []string{
		"get", "update", "deleteRoleMenu:5", "insertRoleMenu[10]", "deleteRoleDept:5", "insertRoleDept[100]",
	})
	if updated == nil {
		t.Fatal("Update 未被调用")
	}
	if updated.RoleName != "新名" || updated.RoleKey != "newkey" || updated.RoleSort != 9 ||
		updated.Status != "0" || updated.Remark == nil || *updated.Remark != "新备注" {
		t.Errorf("更新字段 = %+v", updated)
	}
	if updated.DataScope == nil || *updated.DataScope != permx.ScopeCustom ||
		updated.MenuCheckStrictly == nil || *updated.MenuCheckStrictly != 0 ||
		updated.DeptCheckStrictly == nil || *updated.DeptCheckStrictly != 0 {
		t.Errorf("DataScope/strictly = %v/%v/%v, want 2/0/0",
			updated.DataScope, updated.MenuCheckStrictly, updated.DeptCheckStrictly)
	}
	// FillUpdate 只覆盖 update_by/update_time
	if updated.UpdateBy == nil || *updated.UpdateBy != modelx.SuperAdminID ||
		updated.UpdateTime == nil || !updated.UpdateTime.Equal(now) {
		t.Errorf("UpdateBy/UpdateTime = %v/%v, want 1/%v", updated.UpdateBy, updated.UpdateTime, now)
	}
	if updated.CreateBy == nil || *updated.CreateBy != 7 ||
		updated.CreateTime == nil || !updated.CreateTime.Equal(past) {
		t.Errorf("CreateBy/CreateTime 被改动: %v/%v", updated.CreateBy, updated.CreateTime)
	}
	if len(menuIDs) != 1 || menuIDs[0] != 10 || len(deptIDs) != 1 || deptIDs[0] != 100 {
		t.Errorf("关联 IDs = menu:%v dept:%v, want [10]/[100]", menuIDs, deptIDs)
	}
}

func TestUpdateRoleSelfEditGuard(t *testing.T) {
	repo := &mockRepo{currentUserRoleIDs: func(_ context.Context, uid int64) ([]int64, error) {
		if uid != 2 {
			t.Errorf("CurrentUserRoleIDs uid = %d, want 2（操作者）", uid)
		}
		return []int64{5, 6}, nil
	}}
	svc := newRoleSvc(nil, normalAuth(2, 100), repo) // get 未覆盖：守卫应先于查库
	ok, err := svc.UpdateRole(context.Background(), UpdateIn{ID: i64(5)})
	if ok {
		t.Error("修改自身所属角色应被拒绝")
	}
	wantSvcErr(t, err, "不允许修改自身所属角色，防止自我提权")
}

func TestUpdateRoleDisableAssigned(t *testing.T) {
	repo := &mockRepo{countUserRoleByRoleID: func(_ context.Context, rid int64) (int64, error) {
		if rid != 5 {
			t.Errorf("CountUserRoleByRoleID rid = %d, want 5", rid)
		}
		return 2, nil
	}}
	svc := newRoleSvc(nil, superAuth(), repo) // get 未覆盖：禁用校验应先于查库
	ok, err := svc.UpdateRole(context.Background(), UpdateIn{
		CreateIn: CreateIn{Status: strPtr(modelx.Disabled)}, ID: i64(5),
	})
	if ok {
		t.Error("已分配角色禁用应被拒绝")
	}
	wantSvcErr(t, err, "角色已分配，不能禁用!")
}

func TestUpdateRoleNotFound(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	repo := &mockRepo{
		get: func(_ context.Context, _ int64, _ permx.Scope) (*SysRole, error) { return nil, nil },
		// update 未覆盖：目标不存在不应落库
	}
	svc := newRoleSvc(db, superAuth(), repo)
	ok, err := svc.UpdateRole(context.Background(), UpdateIn{ID: i64(5)})
	if ok || err != nil {
		t.Fatalf("= (%v, %v), want (false, nil)", ok, err)
	}
}

func TestUpdateRoleMissingID(t *testing.T) {
	svc := newRoleSvc(nil, superAuth(), &mockRepo{})
	ok, err := svc.UpdateRole(context.Background(), UpdateIn{})
	if ok || err != nil {
		t.Fatalf("= (%v, %v), want (false, nil)", ok, err)
	}
}

func TestUpdateRoleNormalUserCustomScope(t *testing.T) {
	// 普通用户「自定义」范围：Get 必须带上角色授权部门子查询（Default 列配置）
	fake := &fakeSQL{scopes: [][]driver.Value{scopeRow(5, permx.ScopeCustom)}}
	db := newFakeDB(t, fake)
	loaded := &SysRole{RoleName: "旧名", RoleKey: "common", Status: "0", DelFlag: "0"}
	loaded.ID = 5
	var gotScope permx.Scope
	repo := &mockRepo{
		currentUserRoleIDs: func(_ context.Context, _ int64) ([]int64, error) { return []int64{6}, nil },
		get: func(_ context.Context, _ int64, scope permx.Scope) (*SysRole, error) {
			gotScope = scope
			return loaded, nil
		},
		update: func(_ context.Context, row *SysRole) error {
			if row.UpdateBy == nil || *row.UpdateBy != 2 {
				t.Errorf("UpdateBy = %v, want 2（操作者）", row.UpdateBy)
			}
			return nil
		},
		deleteRoleMenu: func(_ context.Context, _ int64) error { return nil },
		insertRoleMenu: func(_ context.Context, _ int64, _ []int64) (int, error) { return 0, nil },
		// deleteRoleDept 未覆盖：DataScope 缺省不重建部门关联
	}
	svc := newRoleSvc(db, normalAuth(2, 100), repo)
	ok, err := svc.UpdateRole(context.Background(), UpdateIn{
		CreateIn: CreateIn{RoleName: strPtr("x")}, ID: i64(5),
	})
	if err != nil || !ok {
		t.Fatalf("UpdateRole = (%v, %v), want (true, nil)", ok, err)
	}
	wantSQL := "create_dept IN (SELECT dept_id FROM sys_role_dept WHERE role_id = ?)"
	if gotScope.NoFilter || gotScope.SQL != wantSQL || len(gotScope.Args) != 1 || gotScope.Args[0] != int64(5) {
		t.Errorf("scope = %+v, want 自定义部门子查询 args [5]", gotScope)
	}
}

// ---- AuthDataScope 深度业务流 ----

func TestAuthDataScopeSuccess(t *testing.T) {
	now := fixNow(t)
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)

	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	row := &SysRole{RoleName: "普通角色", RoleKey: "common", Status: "0", DelFlag: "0",
		DataScope: strPtr("1"), DeptCheckStrictly: iptr(0)}
	row.ID = 5
	row.CreateBy = i64(7)
	row.CreateTime = &past

	var calls []string
	var updated *SysRole
	var deptRoleID int64
	var deptIDs []int64
	repo := &mockRepo{
		listByIDs: func(_ context.Context, ids []int64) ([]SysRole, error) {
			calls = append(calls, "listByIDs")
			if len(ids) != 1 || ids[0] != 5 {
				t.Errorf("ListByIDs ids = %v, want [5]", ids)
			}
			return []SysRole{commonRole(5)}, nil
		},
		get: func(_ context.Context, id int64, scope permx.Scope) (*SysRole, error) {
			calls = append(calls, "get")
			if id != 5 || !scope.NoFilter {
				t.Errorf("Get = (%d, %+v), want (5, NoFilter)", id, scope)
			}
			return row, nil
		},
		update: func(_ context.Context, r *SysRole) error {
			calls = append(calls, "update")
			cp := *r
			updated = &cp
			return nil
		},
		deleteRoleDept: func(_ context.Context, rid int64) error {
			calls = append(calls, fmt.Sprintf("deleteRoleDept:%d", rid))
			return nil
		},
		insertRoleDept: func(_ context.Context, rid int64, dids []int64) (int, error) {
			calls = append(calls, fmt.Sprintf("insertRoleDept%v", dids))
			deptRoleID, deptIDs = rid, dids
			return len(dids), nil
		},
		// deleteRoleMenu 未覆盖：数据权限变更不应动菜单关联
	}
	svc := newRoleSvc(db, superAuth(), repo)
	in := DataScopeIn{
		ID: i64(5), DataScope: strPtr(permx.ScopeCustom),
		DeptCheckStrictly: bptr(true), DeptIDs: []int64{100, 101},
	}
	ok, err := svc.AuthDataScope(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("AuthDataScope = (%v, %v), want (true, nil)", ok, err)
	}
	wantCalls(t, calls, []string{"listByIDs", "get", "update", "deleteRoleDept:5", "insertRoleDept[100 101]"})
	if updated == nil {
		t.Fatal("Update 未被调用")
	}
	if updated.DataScope == nil || *updated.DataScope != permx.ScopeCustom {
		t.Errorf("DataScope = %v, want 2", updated.DataScope)
	}
	if updated.DeptCheckStrictly == nil || *updated.DeptCheckStrictly != 1 {
		t.Errorf("DeptCheckStrictly = %v, want 1（true→1）", updated.DeptCheckStrictly)
	}
	if updated.UpdateBy == nil || *updated.UpdateBy != modelx.SuperAdminID ||
		updated.UpdateTime == nil || !updated.UpdateTime.Equal(now) {
		t.Errorf("UpdateBy/UpdateTime = %v/%v, want 1/%v", updated.UpdateBy, updated.UpdateTime, now)
	}
	if updated.CreateBy == nil || *updated.CreateBy != 7 || !updated.CreateTime.Equal(past) {
		t.Errorf("create_* 被改动: %v/%v", updated.CreateBy, updated.CreateTime)
	}
	if deptRoleID != 5 || len(deptIDs) != 2 || deptIDs[0] != 100 || deptIDs[1] != 101 {
		t.Errorf("部门关联 = (%d, %v), want (5, [100 101])", deptRoleID, deptIDs)
	}
}

func TestAuthDataScopeBuiltinRoleRejected(t *testing.T) {
	builtin := SysRole{RoleKey: "admin"}
	builtin.ID = 5
	repo := &mockRepo{listByIDs: func(_ context.Context, _ []int64) ([]SysRole, error) {
		return []SysRole{builtin}, nil
	}}
	svc := newRoleSvc(nil, superAuth(), repo) // get 未覆盖：守卫应先于查库
	ok, err := svc.AuthDataScope(context.Background(), DataScopeIn{ID: i64(5), DataScope: strPtr("2")})
	if ok {
		t.Error("内置角色数据权限变更应被拒绝")
	}
	wantSvcErr(t, err, "不允许修改系统内置管理员角色标识符!")
}

func TestAuthDataScopeSuperAdminRole(t *testing.T) {
	svc := newRoleSvc(nil, superAuth(), &mockRepo{}) // listByIDs 未覆盖：id=1 应短路
	ok, err := svc.AuthDataScope(context.Background(), DataScopeIn{ID: i64(1)})
	if ok {
		t.Error("超管角色应被拒绝")
	}
	wantSvcErr(t, err, "不允许操作超级管理员角色")
}

func TestAuthDataScopeNormalUserDenied(t *testing.T) {
	fake := &fakeSQL{scopes: [][]driver.Value{scopeRow(2, "5")}}
	db := newFakeDB(t, fake)
	var gotIDs []int64
	var gotScope permx.Scope
	repo := &mockRepo{
		listByIDs: func(_ context.Context, _ []int64) ([]SysRole, error) {
			return []SysRole{commonRole(5)}, nil
		},
		countVisible: func(_ context.Context, ids []int64, scope permx.Scope) (int64, error) {
			gotIDs, gotScope = ids, scope
			return 0, nil
		},
		// get 未覆盖：可见数不足应早退
	}
	svc := newRoleSvc(db, normalAuth(2, 100), repo)
	ok, err := svc.AuthDataScope(context.Background(), DataScopeIn{ID: i64(5), DataScope: strPtr("2")})
	if ok {
		t.Error("无角色数据权限应失败")
	}
	wantSvcErr(t, err, "没有权限访问部分角色数据！")
	if len(gotIDs) != 1 || gotIDs[0] != 5 {
		t.Errorf("CountVisible ids = %v, want [5]", gotIDs)
	}
	if gotScope.SQL != "create_by = ?" || len(gotScope.Args) != 1 || gotScope.Args[0] != int64(2) {
		t.Errorf("scope = %+v, want 仅本人 create_by = ? args [2]", gotScope)
	}
}

func TestAuthDataScopeRoleNotFound(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	repo := &mockRepo{
		listByIDs: func(_ context.Context, _ []int64) ([]SysRole, error) {
			return []SysRole{commonRole(5)}, nil
		},
		get: func(_ context.Context, _ int64, _ permx.Scope) (*SysRole, error) { return nil, nil },
		// update/deleteRoleDept 未覆盖：目标不存在不应写库
	}
	svc := newRoleSvc(db, superAuth(), repo)
	ok, err := svc.AuthDataScope(context.Background(), DataScopeIn{ID: i64(5), DataScope: strPtr("2")})
	if ok || err != nil {
		t.Fatalf("= (%v, %v), want (false, nil)", ok, err)
	}
}

func TestAuthDataScopeNonCustomStillRebuildsDepts(t *testing.T) {
	// 与 UpdateRole 不同：AuthDataScope 无论 data_scope 取值都先删后插部门关联
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	row := &SysRole{RoleName: "普通角色", RoleKey: "common", Status: "0", DelFlag: "0", DataScope: strPtr("2")}
	row.ID = 5
	var calls []string
	var gotDeptIDs []int64
	repo := &mockRepo{
		listByIDs: func(_ context.Context, _ []int64) ([]SysRole, error) { return []SysRole{commonRole(5)}, nil },
		get:       func(_ context.Context, _ int64, _ permx.Scope) (*SysRole, error) { return row, nil },
		update:    func(_ context.Context, _ *SysRole) error { calls = append(calls, "update"); return nil },
		deleteRoleDept: func(_ context.Context, rid int64) error {
			calls = append(calls, fmt.Sprintf("deleteRoleDept:%d", rid))
			return nil
		},
		insertRoleDept: func(_ context.Context, _ int64, dids []int64) (int, error) {
			calls = append(calls, "insertRoleDept")
			gotDeptIDs = dids
			return 0, nil
		},
	}
	svc := newRoleSvc(db, superAuth(), repo)
	ok, err := svc.AuthDataScope(context.Background(), DataScopeIn{ID: i64(5), DataScope: strPtr("1")})
	if err != nil || !ok {
		t.Fatalf("AuthDataScope = (%v, %v), want (true, nil)", ok, err)
	}
	wantCalls(t, calls, []string{"update", "deleteRoleDept:5", "insertRoleDept"})
	if len(gotDeptIDs) != 0 {
		t.Errorf("deptIDs = %v, want 空（清空授权部门）", gotDeptIDs)
	}
}

// ---- UpdateRoleStatus ----

func TestUpdateRoleStatus(t *testing.T) {
	t.Run("成功停用未分配角色", func(t *testing.T) {
		fake := &fakeSQL{}
		db := newFakeDB(t, fake)
		row := &SysRole{RoleName: "普通角色", RoleKey: "common", Status: "0", DelFlag: "0"}
		row.ID = 5
		var updated *SysRole
		repo := &mockRepo{
			listByIDs:             func(_ context.Context, _ []int64) ([]SysRole, error) { return []SysRole{commonRole(5)}, nil },
			countUserRoleByRoleID: func(_ context.Context, _ int64) (int64, error) { return 0, nil },
			get:                   func(_ context.Context, _ int64, _ permx.Scope) (*SysRole, error) { return row, nil },
			update: func(_ context.Context, r *SysRole) error {
				cp := *r
				updated = &cp
				return nil
			},
		}
		svc := newRoleSvc(db, superAuth(), repo)
		ok, err := svc.UpdateRoleStatus(context.Background(), StatusIn{ID: i64(5), Status: strPtr(modelx.Disabled)})
		if err != nil || !ok {
			t.Fatalf("UpdateRoleStatus = (%v, %v), want (true, nil)", ok, err)
		}
		if updated == nil || updated.Status != modelx.Disabled {
			t.Errorf("Status = %v, want 1", updated)
		}
		if updated.UpdateBy == nil || *updated.UpdateBy != modelx.SuperAdminID {
			t.Errorf("UpdateBy = %v, want 1", updated.UpdateBy)
		}
	})
	t.Run("状态为空", func(t *testing.T) {
		repo := &mockRepo{listByIDs: func(_ context.Context, _ []int64) ([]SysRole, error) {
			return []SysRole{commonRole(5)}, nil
		}}
		svc := newRoleSvc(nil, superAuth(), repo)
		ok, err := svc.UpdateRoleStatus(context.Background(), StatusIn{ID: i64(5)})
		if ok {
			t.Error("缺状态应失败")
		}
		wantSvcErr(t, err, "角色状态不能为空")
	})
	t.Run("已分配不能禁用", func(t *testing.T) {
		repo := &mockRepo{
			listByIDs:             func(_ context.Context, _ []int64) ([]SysRole, error) { return []SysRole{commonRole(5)}, nil },
			countUserRoleByRoleID: func(_ context.Context, _ int64) (int64, error) { return 3, nil },
		}
		svc := newRoleSvc(nil, superAuth(), repo)
		ok, err := svc.UpdateRoleStatus(context.Background(), StatusIn{ID: i64(5), Status: strPtr(modelx.Disabled)})
		if ok {
			t.Error("已分配角色禁用应失败")
		}
		wantSvcErr(t, err, "角色已分配，不能禁用!")
	})
}

// ---- 事务回滚语义（对齐 Python session.begin 失败整体回滚） ----

// TestInsertRoleRollbackOnMenuFailure 角色行先写入、菜单关联插入失败 →
// 事务整体 Rollback、绝不 Commit，不留孤儿角色。
func TestInsertRoleRollbackOnMenuFailure(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	var created bool
	repo := &mockRepo{
		create: func(ctx context.Context, _ *SysRole) error {
			created = true
			if crudx.DB(ctx, nil) == nil {
				t.Error("Create 未拿到事务句柄：WithTx 传播失效")
			}
			return nil
		},
		insertRoleMenu: func(_ context.Context, _ int64, _ []int64) (int, error) { return 0, errors.New("boom") },
	}
	svc := newRoleSvc(db, superAuth(), repo)
	in := CreateIn{RoleSort: iptr(1), Status: strPtr("0"), MenuIDs: []int64{10}}
	ok, err := svc.InsertRole(context.Background(), in)
	if ok || err == nil || err.Error() != "boom" {
		t.Fatalf("InsertRole = (%v, %v), want (false, boom 透传)", ok, err)
	}
	if !created {
		t.Error("角色行应先写入（随后被回滚）")
	}
	wantCalls(t, fake.txSnapshot(), []string{"begin", "rollback"})
}

// TestAuthDataScopeRollbackOnDeptInsertFailure 数据权限更新 + 部门关联先删后插，
// 插入失败 → 更新与删除一并回滚（begin → rollback），错误透传。
func TestAuthDataScopeRollbackOnDeptInsertFailure(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	row := commonRole(5)
	var updated, deptDeleted bool
	repo := &mockRepo{
		listByIDs: func(_ context.Context, _ []int64) ([]SysRole, error) { return []SysRole{commonRole(5)}, nil },
		get:       func(_ context.Context, _ int64, _ permx.Scope) (*SysRole, error) { return &row, nil },
		update: func(ctx context.Context, _ *SysRole) error {
			updated = true
			if crudx.DB(ctx, nil) == nil {
				t.Error("Update 未拿到事务句柄")
			}
			return nil
		},
		deleteRoleDept: func(_ context.Context, _ int64) error { deptDeleted = true; return nil },
		insertRoleDept: func(_ context.Context, _ int64, _ []int64) (int, error) { return 0, errors.New("boom") },
	}
	svc := newRoleSvc(db, superAuth(), repo)
	in := DataScopeIn{ID: i64(5), DataScope: strPtr(permx.ScopeCustom), DeptIDs: []int64{100}}
	ok, err := svc.AuthDataScope(context.Background(), in)
	if ok || err == nil || err.Error() != "boom" {
		t.Fatalf("AuthDataScope = (%v, %v), want (false, boom 透传)", ok, err)
	}
	if !updated || !deptDeleted {
		t.Error("更新与部门关联删除应先执行（随后被回滚）")
	}
	wantCalls(t, fake.txSnapshot(), []string{"begin", "rollback"})
}

// TestDeleteRoleByIDsRollbackOnSoftDeleteFailure 关联批删后软删失败 → 整体回滚。
func TestDeleteRoleByIDsRollbackOnSoftDeleteFailure(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	var menuDeleted bool
	repo := &mockRepo{
		listByIDs:             func(_ context.Context, _ []int64) ([]SysRole, error) { return []SysRole{commonRole(5)}, nil },
		countUserRoleByRoleID: func(_ context.Context, _ int64) (int64, error) { return 0, nil },
		deleteRoleMenuBatch: func(ctx context.Context, _ []int64) error {
			menuDeleted = true
			if crudx.DB(ctx, nil) == nil {
				t.Error("菜单关联批删未拿到事务句柄")
			}
			return nil
		},
		deleteRoleDeptBatch: func(_ context.Context, _ []int64) error { return nil },
		softDelete:          func(_ context.Context, _ []int64) (int64, error) { return 0, errors.New("boom") },
	}
	svc := newRoleSvc(db, superAuth(), repo)
	n, err := svc.DeleteRoleByIDs(context.Background(), []int64{5})
	if n != 0 || err == nil || err.Error() != "boom" {
		t.Fatalf("DeleteRoleByIDs = (%d, %v), want (0, boom 透传)", n, err)
	}
	if !menuDeleted {
		t.Error("菜单关联批删应先执行（随后被回滚）")
	}
	wantCalls(t, fake.txSnapshot(), []string{"begin", "rollback"})
}
