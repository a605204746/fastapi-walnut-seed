package user

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

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/module/system/post"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

// mockRepo 嵌入 userRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	userRepo
	existsByField        func(ctx context.Context, field, value string, excludeID *int64) (bool, error)
	create               func(ctx context.Context, row *SysUser) error
	update               func(ctx context.Context, row *SysUser) error
	findByID             func(ctx context.Context, userID int64) (*SysUser, error)
	findByIDWithScope    func(ctx context.Context, userID int64, scope permx.Scope) (*SysUser, error)
	countByIDWithScope   func(ctx context.Context, userID int64, scope permx.Scope) (int64, error)
	findRoles            func(ctx context.Context, userID int64) ([]role.SysRole, error)
	roleIDsByUserID      func(ctx context.Context, userID int64) ([]int64, error)
	postIDsByUserID      func(ctx context.Context, userID int64) ([]int64, error)
	mapDeptNames         func(ctx context.Context, deptIDs []int64) (map[int64]*string, error)
	updatePassword       func(ctx context.Context, userID int64, hashedPwd string) (int64, error)
	updateStatus         func(ctx context.Context, userID int64, status string) (int64, error)
	softDelete           func(ctx context.Context, ids []int64) (int64, error)
	deleteUserRoles      func(ctx context.Context, userID int64) error
	insertUserRoles      func(ctx context.Context, userID int64, roleIDs []int64) error
	deleteUserRolesBatch func(ctx context.Context, userIDs []int64) error
	deleteUserPosts      func(ctx context.Context, userID int64) error
	insertUserPosts      func(ctx context.Context, userID int64, postIDs []int64) error
	deleteUserPostsBatch func(ctx context.Context, userIDs []int64) error
}

func (m *mockRepo) ExistsByField(ctx context.Context, field, value string, excludeID *int64) (bool, error) {
	return m.existsByField(ctx, field, value, excludeID)
}

func (m *mockRepo) Create(ctx context.Context, row *SysUser) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysUser) error {
	return m.update(ctx, row)
}

func (m *mockRepo) FindByID(ctx context.Context, userID int64) (*SysUser, error) {
	return m.findByID(ctx, userID)
}

func (m *mockRepo) FindByIDWithScope(ctx context.Context, userID int64, scope permx.Scope) (*SysUser, error) {
	return m.findByIDWithScope(ctx, userID, scope)
}

func (m *mockRepo) CountByIDWithScope(ctx context.Context, userID int64, scope permx.Scope) (int64, error) {
	return m.countByIDWithScope(ctx, userID, scope)
}

func (m *mockRepo) FindRoles(ctx context.Context, userID int64) ([]role.SysRole, error) {
	return m.findRoles(ctx, userID)
}

func (m *mockRepo) RoleIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	return m.roleIDsByUserID(ctx, userID)
}

func (m *mockRepo) PostIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	return m.postIDsByUserID(ctx, userID)
}

func (m *mockRepo) MapDeptNames(ctx context.Context, deptIDs []int64) (map[int64]*string, error) {
	return m.mapDeptNames(ctx, deptIDs)
}

func (m *mockRepo) UpdatePassword(ctx context.Context, userID int64, hashedPwd string) (int64, error) {
	return m.updatePassword(ctx, userID, hashedPwd)
}

func (m *mockRepo) UpdateStatus(ctx context.Context, userID int64, status string) (int64, error) {
	return m.updateStatus(ctx, userID, status)
}

func (m *mockRepo) SoftDelete(ctx context.Context, ids []int64) (int64, error) {
	return m.softDelete(ctx, ids)
}

func (m *mockRepo) DeleteUserRoles(ctx context.Context, userID int64) error {
	return m.deleteUserRoles(ctx, userID)
}

func (m *mockRepo) InsertUserRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	return m.insertUserRoles(ctx, userID, roleIDs)
}

func (m *mockRepo) DeleteUserRolesBatch(ctx context.Context, userIDs []int64) error {
	return m.deleteUserRolesBatch(ctx, userIDs)
}

func (m *mockRepo) DeleteUserPosts(ctx context.Context, userID int64) error {
	return m.deleteUserPosts(ctx, userID)
}

func (m *mockRepo) InsertUserPosts(ctx context.Context, userID int64, postIDs []int64) error {
	return m.insertUserPosts(ctx, userID, postIDs)
}

func (m *mockRepo) DeleteUserPostsBatch(ctx context.Context, userIDs []int64) error {
	return m.deleteUserPostsBatch(ctx, userIDs)
}

// mockPosts / mockRoles 覆盖 Deps 注入的跨模块 provider（consumer-side 窄接口）。
type mockPosts struct {
	selectList func(ctx context.Context, q post.QueryParam) ([]post.Out, error)
}

func (m *mockPosts) SelectList(ctx context.Context, q post.QueryParam) ([]post.Out, error) {
	return m.selectList(ctx, q)
}

type mockRoles struct {
	selectList func(ctx context.Context, q role.QueryParam) ([]role.SysRole, error)
}

func (m *mockRoles) SelectList(ctx context.Context, q role.QueryParam) ([]role.SysRole, error) {
	return m.selectList(ctx, q)
}

// ---- 假 DB：在 database/sql 驱动层伪造 permx / 可见性计数查询 ----
//
// checkRolesVisible/checkPostsVisible/checkDeptDataScope 与 permx.Build 直接
// 访问 s.db（绕过 userRepo），nil DB 报错、DryRun 不执行 Count（恒 0），
// 都无法构造「可见数命中」的成功路径。这里用 sql.OpenDB + 假 driver.Conn
// 按 SQL 形态返回预设结果，使数据权限分支可由 authctx 身份驱动。

type fakeSQL struct {
	mu        sync.Mutex
	scopes    [][]driver.Value // fetchRoleScopes 结果行：(role_id, data_scope)
	roleCount int64            // sys_role 可见数
	postCount int64            // sys_post 可见数
	deptCount int64            // sys_dept 可见数
	queries   []string
	txEvents  []string // 事务事件序：begin / commit / rollback（证明回滚语义）
}

func (f *fakeSQL) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
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
	switch {
	case strings.Contains(q, "sys_role AS r"): // permx.fetchRoleScopes
		return &fakeRows{cols: []string{"role_id", "data_scope"}, data: c.f.scopes}, nil
	case strings.Contains(q, "count(*)"):
		var n int64
		switch {
		case strings.Contains(q, "sys_post"):
			n = c.f.postCount
		case strings.Contains(q, "sys_dept"):
			n = c.f.deptCount
		case strings.Contains(q, "sys_role"):
			n = c.f.roleCount
		default:
			return nil, fmt.Errorf("unexpected count query: %s", q)
		}
		return &fakeRows{cols: []string{"count"}, data: [][]driver.Value{{n}}}, nil
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

// superAuth 超管身份：CheckUser/DeptDataScope 短路放行，permx NoFilter。
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

func newSvc(db *gorm.DB, ac authctx.Context, repo userRepo, posts PostProvider, roles RoleProvider) *Service {
	svc := NewService(db, ac, Deps{
		NewPosts: func(authctx.Context) PostProvider { return posts },
		NewRoles: func(authctx.Context) RoleProvider { return roles },
		NewDepts: func(authctx.Context) DeptProvider { return nil },
	})
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

type existsCall struct {
	field, value string
	excludeID    *int64
}

// ---- 既有守卫用例 ----

func TestInsertUserDuplicateUserName(t *testing.T) {
	svc := NewService(nil, testAuth(2), Deps{
		NewPosts: func(authctx.Context) PostProvider { return nil },
		NewRoles: func(authctx.Context) RoleProvider { return nil },
		NewDepts: func(authctx.Context) DeptProvider { return nil },
	})
	svc.repo = &mockRepo{existsByField: func(_ context.Context, field, _ string, _ *int64) (bool, error) {
		if field != "user_name" {
			t.Errorf("首个唯一性校验应为 user_name, got %s", field)
		}
		return true, nil
	}}
	name, pwd := "admin", "Abc12345!"
	ok, err := svc.InsertUser(context.Background(), CreateIn{UserName: &name, Password: &pwd})
	if ok {
		t.Error("重复账号应新增失败")
	}
	if se := errs.As(err); se == nil || se.Msg != "新增用户'admin'失败，登录账号已存在" {
		t.Errorf("err = %v, want 登录账号已存在", err)
	}
}

func TestInsertUserMissingPassword(t *testing.T) {
	svc := NewService(nil, testAuth(2), Deps{
		NewPosts: func(authctx.Context) PostProvider { return nil },
		NewRoles: func(authctx.Context) RoleProvider { return nil },
		NewDepts: func(authctx.Context) DeptProvider { return nil },
	})
	name := "admin"
	_, err := svc.InsertUser(context.Background(), CreateIn{UserName: &name})
	if se := errs.As(err); se == nil || se.Msg != "用户密码不能为空" {
		t.Errorf("err = %v, want 用户密码不能为空", err)
	}
}

func TestDeleteUserByIDsSelfGuard(t *testing.T) {
	svc := NewService(nil, testAuth(7), Deps{
		NewPosts: func(authctx.Context) PostProvider { return nil },
		NewRoles: func(authctx.Context) RoleProvider { return nil },
		NewDepts: func(authctx.Context) DeptProvider { return nil },
	})
	n, err := svc.DeleteUserByIDs(context.Background(), []int64{7})
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	if se := errs.As(err); se == nil || se.Msg != "当前用户不能删除" {
		t.Errorf("err = %v, want 当前用户不能删除", err)
	}
}

// ---- InsertUser 深度业务流 ----

func TestInsertUserSuccessFullFlow(t *testing.T) {
	now := fixNow(t)
	fake := &fakeSQL{postCount: 2, roleCount: 2}
	db := newFakeDB(t, fake)

	var calls []string
	var exists []existsCall
	var created *SysUser
	var postUID, roleUID int64
	repo := &mockRepo{
		existsByField: func(_ context.Context, field, value string, excludeID *int64) (bool, error) {
			exists = append(exists, existsCall{field, value, excludeID})
			return false, nil
		},
		create: func(_ context.Context, row *SysUser) error {
			calls = append(calls, "create")
			cp := *row
			created = &cp
			return nil
		},
		deleteUserPosts: func(_ context.Context, uid int64) error {
			calls = append(calls, "deleteUserPosts")
			postUID = uid
			return nil
		},
		insertUserPosts: func(_ context.Context, uid int64, pids []int64) error {
			calls = append(calls, fmt.Sprintf("insertUserPosts%v", pids))
			postUID = uid
			return nil
		},
		deleteUserRoles: func(_ context.Context, uid int64) error {
			calls = append(calls, "deleteUserRoles")
			roleUID = uid
			return nil
		},
		insertUserRoles: func(_ context.Context, uid int64, rids []int64) error {
			calls = append(calls, fmt.Sprintf("insertUserRoles%v", rids))
			roleUID = uid
			return nil
		},
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)

	pwd := "Abc12345!"
	in := CreateIn{
		DeptID:      i64(200),
		UserName:    strPtr("newbie"),
		NickName:    strPtr("新人"),
		Password:    &pwd,
		Email:       strPtr("n@example.com"),
		PhoneNumber: strPtr("13800000000"),
		Sex:         strPtr("0"),
		Remark:      strPtr("备注"),
		RoleIDs:     []int64{1, 2, 3}, // 1 是超管角色，目标非超管 → 应被剔除
		PostIDs:     []int64{4, 5},
	}
	ok, err := svc.InsertUser(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("InsertUser = (%v, %v), want (true, nil)", ok, err)
	}

	// 唯一性校验顺序 user_name → phonenumber → email，新增不排除任何 ID
	wantFields := []string{"user_name", "phonenumber", "email"}
	wantValues := []string{"newbie", "13800000000", "n@example.com"}
	if len(exists) != 3 {
		t.Fatalf("唯一性校验次数 = %d, want 3（%+v）", len(exists), exists)
	}
	for i, e := range exists {
		if e.field != wantFields[i] || e.value != wantValues[i] || e.excludeID != nil {
			t.Errorf("exists[%d] = %+v, want field=%s value=%s excludeID=nil", i, e, wantFields[i], wantValues[i])
		}
	}

	if created == nil {
		t.Fatal("Create 未被调用")
	}
	if created.ID == 0 {
		t.Error("Create 前应填充雪花 ID")
	}
	if created.UserType != "sys_user" || created.Status != modelx.Normal || created.DelFlag != modelx.Normal {
		t.Errorf("默认列 = user_type:%q status:%q del_flag:%q, want sys_user/0/0",
			created.UserType, created.Status, created.DelFlag)
	}
	if created.UserName != "newbie" || created.NickName != "新人" {
		t.Errorf("UserName/NickName = %q/%q", created.UserName, created.NickName)
	}
	if created.DeptID == nil || *created.DeptID != 200 {
		t.Errorf("DeptID = %v, want 200", created.DeptID)
	}
	if created.Email == nil || *created.Email != "n@example.com" ||
		created.Phonenumber == nil || *created.Phonenumber != "13800000000" ||
		created.Sex == nil || *created.Sex != "0" ||
		created.Remark == nil || *created.Remark != "备注" {
		t.Errorf("可选字段未落库: %+v", created)
	}
	// 密码必须是 bcrypt 哈希且可验证明文
	if created.Password == nil || *created.Password == pwd {
		t.Fatalf("Password = %v, want bcrypt 哈希（非明文）", created.Password)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*created.Password), []byte(pwd)); err != nil {
		t.Errorf("bcrypt 校验失败: %v", err)
	}
	// 审计字段：create_by/update_by=操作者，create_dept=操作者部门，时间=nowFunc
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

	// 关联重建：先岗位后角色；超管角色 1 被剔除；关联挂在新用户 ID 上
	wantCalls(t, calls, []string{
		"create", "deleteUserPosts", "insertUserPosts[4 5]", "deleteUserRoles", "insertUserRoles[2 3]",
	})
	if postUID != created.ID || roleUID != created.ID {
		t.Errorf("关联 userID = post:%d role:%d, want %d", postUID, roleUID, created.ID)
	}
	// 超管跳过部门数据权限 → 不应出现 sys_dept 计数查询
	for _, q := range fake.snapshot() {
		if strings.Contains(q, "sys_dept") {
			t.Errorf("超管不应触发部门数据权限查询: %s", q)
		}
	}
}

func TestInsertUserDuplicatePhone(t *testing.T) {
	var fields []string
	repo := &mockRepo{existsByField: func(_ context.Context, field, _ string, _ *int64) (bool, error) {
		fields = append(fields, field)
		return field == "phonenumber", nil
	}}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	in := CreateIn{UserName: strPtr("newbie"), Password: strPtr("Abc12345!"), PhoneNumber: strPtr("13800000000")}
	ok, err := svc.InsertUser(context.Background(), in)
	if ok {
		t.Error("手机号重复应新增失败")
	}
	wantSvcErr(t, err, "新增用户'newbie'失败，手机号码已存在")
	wantCalls(t, fields, []string{"user_name", "phonenumber"}) // 邮箱校验不应再执行
}

func TestInsertUserDuplicateEmail(t *testing.T) {
	var fields []string
	repo := &mockRepo{existsByField: func(_ context.Context, field, _ string, _ *int64) (bool, error) {
		fields = append(fields, field)
		return field == "email", nil
	}}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	in := CreateIn{
		UserName: strPtr("newbie"), Password: strPtr("Abc12345!"),
		PhoneNumber: strPtr("13800000000"), Email: strPtr("n@example.com"),
	}
	ok, err := svc.InsertUser(context.Background(), in)
	if ok {
		t.Error("邮箱重复应新增失败")
	}
	wantSvcErr(t, err, "新增用户'newbie'失败，邮箱账号已存在")
	wantCalls(t, fields, []string{"user_name", "phonenumber", "email"})
}

func TestInsertUserBlankPhoneEmailSkipsUniqueness(t *testing.T) {
	var fields []string
	var created bool
	repo := &mockRepo{
		existsByField: func(_ context.Context, field, _ string, _ *int64) (bool, error) {
			fields = append(fields, field)
			return false, nil
		},
		create: func(_ context.Context, _ *SysUser) error { created = true; return nil },
	}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	in := CreateIn{
		UserName: strPtr("newbie"), Password: strPtr("Abc12345!"),
		PhoneNumber: strPtr(""), Email: strPtr(""), // 空串 → 跳过唯一性校验
	}
	ok, err := svc.InsertUser(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("InsertUser = (%v, %v), want (true, nil)", ok, err)
	}
	wantCalls(t, fields, []string{"user_name"})
	if !created {
		t.Error("Create 应被调用（无角色/岗位 → 关联重建 no-op）")
	}
}

func TestInsertUserDeptDataScopeDenied(t *testing.T) {
	// 普通用户 + 仅本人范围 → 部门计数 0 → 拒绝；唯一性校验不应执行
	fake := &fakeSQL{scopes: [][]driver.Value{scopeRow(2, "5")}, deptCount: 0}
	db := newFakeDB(t, fake)
	repo := &mockRepo{} // existsByField 未覆盖：触发即 panic
	svc := newSvc(db, normalAuth(2, 100), repo, nil, nil)
	in := CreateIn{DeptID: i64(200), UserName: strPtr("newbie"), Password: strPtr("Abc12345!")}
	ok, err := svc.InsertUser(context.Background(), in)
	if ok {
		t.Error("无部门数据权限应新增失败")
	}
	wantSvcErr(t, err, "没有权限访问部门数据！")
}

func TestInsertUserSuperAdminRoleOnlyRejected(t *testing.T) {
	var created bool
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, _ *int64) (bool, error) { return false, nil },
		create:        func(_ context.Context, _ *SysUser) error { created = true; return nil },
		// delete/insertUserRoles 未覆盖：剔除后为空应早退，触发即 panic
	}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	in := CreateIn{UserName: strPtr("newbie"), Password: strPtr("Abc12345!"), RoleIDs: []int64{1}}
	ok, err := svc.InsertUser(context.Background(), in)
	if ok {
		t.Error("仅选超管角色应新增失败")
	}
	wantSvcErr(t, err, "不允许为普通用户分配超级管理员角色，请至少选择一个其他角色")
	// 钉住现状：用户行先落库、角色校验后失败（Go 无请求级事务，见报告疑似 bug；
	// Python 端 session.begin() 会整体回滚，不会留下孤儿用户）
	if !created {
		t.Error("Create 先于角色重建执行（当前实现顺序）")
	}
}

func TestInsertUserRolesNotVisible(t *testing.T) {
	fake := &fakeSQL{roleCount: 1} // 请求 2 个角色仅 1 个可见
	db := newFakeDB(t, fake)
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, _ *int64) (bool, error) { return false, nil },
		create:        func(_ context.Context, _ *SysUser) error { return nil },
		// deleteUserRoles 未覆盖：可见数不足应早退
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)
	in := CreateIn{UserName: strPtr("newbie"), Password: strPtr("Abc12345!"), RoleIDs: []int64{2, 3}}
	ok, err := svc.InsertUser(context.Background(), in)
	if ok {
		t.Error("角色可见数不足应新增失败")
	}
	wantSvcErr(t, err, "没有权限访问角色的数据")
}

func TestInsertUserPostsNotVisible(t *testing.T) {
	fake := &fakeSQL{postCount: 1} // 请求 2 个岗位仅 1 个可见
	db := newFakeDB(t, fake)
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, _ *int64) (bool, error) { return false, nil },
		create:        func(_ context.Context, _ *SysUser) error { return nil },
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)
	in := CreateIn{UserName: strPtr("newbie"), Password: strPtr("Abc12345!"), PostIDs: []int64{4, 5}}
	ok, err := svc.InsertUser(context.Background(), in)
	if ok {
		t.Error("岗位可见数不足应新增失败")
	}
	wantSvcErr(t, err, "没有权限访问岗位的数据")
}

// ---- UpdateUser 深度业务流 ----

func TestUpdateUserSuccessFullFlow(t *testing.T) {
	now := fixNow(t)
	fake := &fakeSQL{roleCount: 1, postCount: 1}
	db := newFakeDB(t, fake)

	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	old := &SysUser{UserName: "old", NickName: "旧名", Status: "0", DelFlag: "0"}
	old.ID = 9
	old.CreateBy = i64(7)
	old.CreateTime = &past

	var calls []string
	var excludes []*int64
	var gotScope permx.Scope
	var updated *SysUser
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, excludeID *int64) (bool, error) {
			excludes = append(excludes, excludeID)
			return false, nil
		},
		deleteUserRoles: func(_ context.Context, uid int64) error {
			calls = append(calls, fmt.Sprintf("deleteUserRoles:%d", uid))
			return nil
		},
		insertUserRoles: func(_ context.Context, uid int64, rids []int64) error {
			calls = append(calls, fmt.Sprintf("insertUserRoles:%d%v", uid, rids))
			return nil
		},
		deleteUserPosts: func(_ context.Context, uid int64) error {
			calls = append(calls, fmt.Sprintf("deleteUserPosts:%d", uid))
			return nil
		},
		insertUserPosts: func(_ context.Context, uid int64, pids []int64) error {
			calls = append(calls, fmt.Sprintf("insertUserPosts:%d%v", uid, pids))
			return nil
		},
		findByIDWithScope: func(_ context.Context, uid int64, scope permx.Scope) (*SysUser, error) {
			calls = append(calls, "find")
			if uid != 9 {
				t.Errorf("FindByIDWithScope uid = %d, want 9", uid)
			}
			gotScope = scope
			return old, nil
		},
		update: func(_ context.Context, row *SysUser) error {
			calls = append(calls, "update")
			cp := *row
			updated = &cp
			return nil
		},
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)

	in := UpdateIn{
		CreateIn: CreateIn{
			DeptID:      i64(200),
			UserName:    strPtr("renamed"),
			NickName:    strPtr("新名"),
			Email:       strPtr("r@example.com"),
			PhoneNumber: strPtr("13900000000"),
			Sex:         strPtr("1"),
			Status:      strPtr("1"),
			Remark:      strPtr("新备注"),
			RoleIDs:     []int64{2},
			PostIDs:     []int64{4},
		},
		ID: i64(9),
	}
	ok, err := svc.UpdateUser(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("UpdateUser = (%v, %v), want (true, nil)", ok, err)
	}

	// 唯一性校验排除自身：三次校验的 excludeID 都必须是目标 ID
	if len(excludes) != 3 {
		t.Fatalf("唯一性校验次数 = %d, want 3", len(excludes))
	}
	for i, e := range excludes {
		if e == nil || *e != 9 {
			t.Errorf("excludes[%d] = %v, want 指向 9（排除自身）", i, e)
		}
	}
	if !gotScope.NoFilter {
		t.Errorf("超管 scope = %+v, want NoFilter", gotScope)
	}
	// 修改顺序：先重建角色、再岗位，最后取用户并更新（Python update_user 同款）
	wantCalls(t, calls, []string{
		"deleteUserRoles:9", "insertUserRoles:9[2]", "deleteUserPosts:9", "insertUserPosts:9[4]", "find", "update",
	})
	if updated == nil {
		t.Fatal("Update 未被调用")
	}
	if updated.UserName != "renamed" || updated.NickName != "新名" || updated.Status != "1" {
		t.Errorf("更新字段 = %q/%q/%q", updated.UserName, updated.NickName, updated.Status)
	}
	if updated.DeptID == nil || *updated.DeptID != 200 ||
		updated.Email == nil || *updated.Email != "r@example.com" ||
		updated.Phonenumber == nil || *updated.Phonenumber != "13900000000" ||
		updated.Sex == nil || *updated.Sex != "1" ||
		updated.Remark == nil || *updated.Remark != "新备注" {
		t.Errorf("可选字段未更新: %+v", updated)
	}
	// FillUpdate 只覆盖 update_by/update_time，create_* 保持原值
	if updated.UpdateBy == nil || *updated.UpdateBy != modelx.SuperAdminID ||
		updated.UpdateTime == nil || !updated.UpdateTime.Equal(now) {
		t.Errorf("UpdateBy/UpdateTime = %v/%v, want 1/%v", updated.UpdateBy, updated.UpdateTime, now)
	}
	if updated.CreateBy == nil || *updated.CreateBy != 7 ||
		updated.CreateTime == nil || !updated.CreateTime.Equal(past) {
		t.Errorf("CreateBy/CreateTime 被改动: %v/%v, want 7/%v", updated.CreateBy, updated.CreateTime, past)
	}
}

func TestUpdateUserNormalUserDeptScope(t *testing.T) {
	// 普通用户「本部门」范围：UserList 配置按 dept_id 过滤（不是 create_dept）
	fake := &fakeSQL{scopes: [][]driver.Value{scopeRow(5, "3")}}
	db := newFakeDB(t, fake)
	old := &SysUser{UserName: "old", NickName: "旧名", Status: "0", DelFlag: "0"}
	old.ID = 9
	var countScope, findScope permx.Scope
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, _ *int64) (bool, error) { return false, nil },
		countByIDWithScope: func(_ context.Context, uid int64, scope permx.Scope) (int64, error) {
			if uid != 9 {
				t.Errorf("CountByIDWithScope uid = %d, want 9", uid)
			}
			countScope = scope
			return 1, nil
		},
		findByIDWithScope: func(_ context.Context, _ int64, scope permx.Scope) (*SysUser, error) {
			findScope = scope
			return old, nil
		},
		update: func(_ context.Context, row *SysUser) error {
			if row.UpdateBy == nil || *row.UpdateBy != 2 {
				t.Errorf("UpdateBy = %v, want 2（操作者）", row.UpdateBy)
			}
			return nil
		},
	}
	svc := newSvc(db, normalAuth(2, 100), repo, nil, nil)
	ok, err := svc.UpdateUser(context.Background(), UpdateIn{
		CreateIn: CreateIn{UserName: strPtr("renamed")}, ID: i64(9),
	})
	if err != nil || !ok {
		t.Fatalf("UpdateUser = (%v, %v), want (true, nil)", ok, err)
	}
	for name, sc := range map[string]permx.Scope{"count": countScope, "find": findScope} {
		if sc.NoFilter || sc.SQL != "dept_id = ?" || len(sc.Args) != 1 || sc.Args[0] != int64(100) {
			t.Errorf("%s scope = %+v, want dept_id = ? args [100]", name, sc)
		}
	}
}

func TestUpdateUserDataScopeDenied(t *testing.T) {
	// 无角色 → fail-closed 仅本人；计数 0 → 拒绝，且唯一性校验不应执行
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	var gotScope permx.Scope
	repo := &mockRepo{
		countByIDWithScope: func(_ context.Context, _ int64, scope permx.Scope) (int64, error) {
			gotScope = scope
			return 0, nil
		},
	}
	svc := newSvc(db, normalAuth(2, 100), repo, nil, nil)
	ok, err := svc.UpdateUser(context.Background(), UpdateIn{
		CreateIn: CreateIn{UserName: strPtr("renamed")}, ID: i64(9),
	})
	if ok {
		t.Error("无用户数据权限应修改失败")
	}
	wantSvcErr(t, err, "没有权限访问用户数据！")
	if gotScope.SQL != "create_by = ?" || len(gotScope.Args) != 1 || gotScope.Args[0] != int64(2) {
		t.Errorf("无角色应退化为仅本人, scope = %+v", gotScope)
	}
}

func TestUpdateUserSuperAdminTarget(t *testing.T) {
	svc := newSvc(nil, superAuth(), &mockRepo{}, nil, nil)
	ok, err := svc.UpdateUser(context.Background(), UpdateIn{ID: i64(1)})
	if ok {
		t.Error("修改超管应被拒绝")
	}
	wantSvcErr(t, err, "不允许操作超级管理员用户")
}

func TestUpdateUserMissingID(t *testing.T) {
	svc := newSvc(nil, superAuth(), &mockRepo{}, nil, nil)
	ok, err := svc.UpdateUser(context.Background(), UpdateIn{})
	if ok {
		t.Error("缺 ID 应修改失败")
	}
	wantSvcErr(t, err, "用户ID不能为空")
}

func TestUpdateUserPhoneDuplicateExcludesSelf(t *testing.T) {
	var gotField string
	var gotExclude *int64
	repo := &mockRepo{existsByField: func(_ context.Context, field, _ string, excludeID *int64) (bool, error) {
		gotField, gotExclude = field, excludeID
		return field == "phonenumber", nil
	}}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	in := UpdateIn{
		CreateIn: CreateIn{UserName: strPtr("renamed"), PhoneNumber: strPtr("13900000000")},
		ID:       i64(9),
	}
	ok, err := svc.UpdateUser(context.Background(), in)
	if ok {
		t.Error("手机号重复应修改失败")
	}
	wantSvcErr(t, err, "修改用户'renamed'失败，手机号码已存在")
	if gotField != "phonenumber" || gotExclude == nil || *gotExclude != 9 {
		t.Errorf("field/excludeID = %s/%v, want phonenumber/9", gotField, gotExclude)
	}
}

func TestUpdateUserNotFound(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, _ *int64) (bool, error) { return false, nil },
		findByIDWithScope: func(_ context.Context, _ int64, _ permx.Scope) (*SysUser, error) {
			return nil, nil
		},
		// update 未覆盖：目标不存在不应落库
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)
	ok, err := svc.UpdateUser(context.Background(), UpdateIn{
		CreateIn: CreateIn{UserName: strPtr("renamed")}, ID: i64(9),
	})
	if ok {
		t.Error("目标不存在应修改失败")
	}
	wantSvcErr(t, err, "修改用户renamed信息失败")
}

// ---- DeleteUserByIDs 深度业务流 ----

func TestDeleteUserByIDsSuccess(t *testing.T) {
	var calls []string
	repo := &mockRepo{
		deleteUserRolesBatch: func(_ context.Context, uids []int64) error {
			calls = append(calls, fmt.Sprintf("deleteUserRolesBatch%v", uids))
			return nil
		},
		deleteUserPostsBatch: func(_ context.Context, uids []int64) error {
			calls = append(calls, fmt.Sprintf("deleteUserPostsBatch%v", uids))
			return nil
		},
		softDelete: func(_ context.Context, ids []int64) (int64, error) {
			calls = append(calls, fmt.Sprintf("softDelete%v", ids))
			return 2, nil
		},
	}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	n, err := svc.DeleteUserByIDs(context.Background(), []int64{2, 3})
	if err != nil || n != 2 {
		t.Fatalf("DeleteUserByIDs = (%d, %v), want (2, nil)", n, err)
	}
	wantCalls(t, calls, []string{"deleteUserRolesBatch[2 3]", "deleteUserPostsBatch[2 3]", "softDelete[2 3]"})
}

func TestDeleteUserByIDsSoftDeleteZero(t *testing.T) {
	repo := &mockRepo{
		deleteUserRolesBatch: func(_ context.Context, _ []int64) error { return nil },
		deleteUserPostsBatch: func(_ context.Context, _ []int64) error { return nil },
		softDelete:           func(_ context.Context, _ []int64) (int64, error) { return 0, nil },
	}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	n, err := svc.DeleteUserByIDs(context.Background(), []int64{2, 3})
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	wantSvcErr(t, err, "删除用户失败!")
}

func TestDeleteUserByIDsSuperAdminTarget(t *testing.T) {
	// 操作者非超管（避免自删守卫先命中）；CheckUserAllowed 先于数据权限校验
	svc := newSvc(nil, testAuth(7), &mockRepo{}, nil, nil) // 任何 repo 调用即 panic
	n, err := svc.DeleteUserByIDs(context.Background(), []int64{1})
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	wantSvcErr(t, err, "不允许操作超级管理员用户")
}

func TestDeleteUserByIDsScopeDenied(t *testing.T) {
	fake := &fakeSQL{scopes: [][]driver.Value{scopeRow(2, "5")}}
	db := newFakeDB(t, fake)
	repo := &mockRepo{
		countByIDWithScope: func(_ context.Context, uid int64, _ permx.Scope) (int64, error) {
			if uid == 3 {
				return 1, nil
			}
			return 0, nil // 第二个目标不可见
		},
		// 批量删除未覆盖：校验失败不应触发任何写操作
	}
	svc := newSvc(db, normalAuth(2, 100), repo, nil, nil)
	n, err := svc.DeleteUserByIDs(context.Background(), []int64{3, 4})
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	wantSvcErr(t, err, "没有权限访问用户数据！")
}

// ---- ResetPwd / ChangeStatus ----

func TestResetPwdSuccess(t *testing.T) {
	var gotUID int64
	var gotHash string
	repo := &mockRepo{updatePassword: func(_ context.Context, uid int64, hash string) (int64, error) {
		gotUID, gotHash = uid, hash
		return 1, nil
	}}
	svc := newSvc(nil, superAuth(), repo, nil, nil)
	ok, err := svc.ResetPwd(context.Background(), ResetPwdIn{ID: i64(5), Password: strPtr("NewPass1!")})
	if err != nil || !ok {
		t.Fatalf("ResetPwd = (%v, %v), want (true, nil)", ok, err)
	}
	if gotUID != 5 {
		t.Errorf("UpdatePassword uid = %d, want 5", gotUID)
	}
	if gotHash == "NewPass1!" {
		t.Error("密码明文落库")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(gotHash), []byte("NewPass1!")); err != nil {
		t.Errorf("bcrypt 校验失败: %v", err)
	}
}

func TestResetPwdGuards(t *testing.T) {
	t.Run("空密码", func(t *testing.T) {
		svc := newSvc(nil, superAuth(), &mockRepo{}, nil, nil)
		ok, err := svc.ResetPwd(context.Background(), ResetPwdIn{ID: i64(5), Password: strPtr("")})
		if ok {
			t.Error("空密码应失败")
		}
		wantSvcErr(t, err, "用户密码不能为空")
	})
	t.Run("缺密码", func(t *testing.T) {
		svc := newSvc(nil, superAuth(), &mockRepo{}, nil, nil)
		_, err := svc.ResetPwd(context.Background(), ResetPwdIn{ID: i64(5)})
		wantSvcErr(t, err, "用户密码不能为空")
	})
	t.Run("缺ID静默no-op", func(t *testing.T) {
		svc := newSvc(nil, superAuth(), &mockRepo{}, nil, nil)
		ok, err := svc.ResetPwd(context.Background(), ResetPwdIn{Password: strPtr("NewPass1!")})
		if ok || err != nil {
			t.Errorf("= (%v, %v), want (false, nil)", ok, err)
		}
	})
	t.Run("超管目标", func(t *testing.T) {
		svc := newSvc(nil, superAuth(), &mockRepo{}, nil, nil)
		_, err := svc.ResetPwd(context.Background(), ResetPwdIn{ID: i64(1), Password: strPtr("NewPass1!")})
		wantSvcErr(t, err, "不允许操作超级管理员用户")
	})
	t.Run("未更新到行", func(t *testing.T) {
		repo := &mockRepo{updatePassword: func(_ context.Context, _ int64, _ string) (int64, error) { return 0, nil }}
		svc := newSvc(nil, superAuth(), repo, nil, nil)
		ok, err := svc.ResetPwd(context.Background(), ResetPwdIn{ID: i64(5), Password: strPtr("NewPass1!")})
		if ok || err != nil {
			t.Errorf("= (%v, %v), want (false, nil)", ok, err)
		}
	})
}

func TestChangeStatusSuccess(t *testing.T) {
	t.Run("显式停用", func(t *testing.T) {
		var gotUID int64
		var gotStatus string
		repo := &mockRepo{updateStatus: func(_ context.Context, uid int64, status string) (int64, error) {
			gotUID, gotStatus = uid, status
			return 1, nil
		}}
		svc := newSvc(nil, superAuth(), repo, nil, nil)
		ok, err := svc.ChangeStatus(context.Background(), ChangeStatusIn{ID: i64(5), Status: strPtr("1")})
		if err != nil || !ok {
			t.Fatalf("ChangeStatus = (%v, %v), want (true, nil)", ok, err)
		}
		if gotUID != 5 || gotStatus != "1" {
			t.Errorf("UpdateStatus = (%d, %q), want (5, \"1\")", gotUID, gotStatus)
		}
	})
	t.Run("缺省正常", func(t *testing.T) {
		var gotStatus string
		repo := &mockRepo{updateStatus: func(_ context.Context, _ int64, status string) (int64, error) {
			gotStatus = status
			return 1, nil
		}}
		svc := newSvc(nil, superAuth(), repo, nil, nil)
		ok, err := svc.ChangeStatus(context.Background(), ChangeStatusIn{ID: i64(5)})
		if err != nil || !ok {
			t.Fatalf("ChangeStatus = (%v, %v), want (true, nil)", ok, err)
		}
		if gotStatus != modelx.Normal {
			t.Errorf("status = %q, want 0（缺省正常）", gotStatus)
		}
	})
}

func TestChangeStatusGuards(t *testing.T) {
	t.Run("超管目标", func(t *testing.T) {
		svc := newSvc(nil, superAuth(), &mockRepo{}, nil, nil)
		_, err := svc.ChangeStatus(context.Background(), ChangeStatusIn{ID: i64(1), Status: strPtr("1")})
		wantSvcErr(t, err, "不允许操作超级管理员用户")
	})
	t.Run("普通用户无数据权限", func(t *testing.T) {
		fake := &fakeSQL{}
		db := newFakeDB(t, fake)
		repo := &mockRepo{countByIDWithScope: func(_ context.Context, _ int64, _ permx.Scope) (int64, error) {
			return 0, nil
		}}
		svc := newSvc(db, normalAuth(2, 100), repo, nil, nil)
		ok, err := svc.ChangeStatus(context.Background(), ChangeStatusIn{ID: i64(5), Status: strPtr("1")})
		if ok {
			t.Error("无数据权限应失败")
		}
		wantSvcErr(t, err, "没有权限访问用户数据！")
	})
}

// ---- GetByID / GetAuthRole：provider 编排与超管角色过滤 ----

func TestGetByIDSuccessWithProviders(t *testing.T) {
	u := &SysUser{UserName: "u9", NickName: "用户9", DeptID: i64(100), Status: "0", DelFlag: "0"}
	u.ID = 9
	r1 := role.SysRole{RoleName: "超级管理员", RoleKey: "superadmin", Status: "0"}
	r1.ID = 1
	r2 := role.SysRole{RoleName: "普通角色", RoleKey: "common", Status: "0"}
	r2.ID = 2
	po := post.Out{PostCode: strPtr("ceo"), PostName: strPtr("董事长")}

	var gotPostQ post.QueryParam
	var gotRoleQ role.QueryParam
	repo := &mockRepo{
		findByID: func(_ context.Context, id int64) (*SysUser, error) {
			if id != 9 {
				t.Errorf("FindByID id = %d, want 9", id)
			}
			return u, nil
		},
		mapDeptNames: func(_ context.Context, ids []int64) (map[int64]*string, error) {
			if len(ids) != 1 || ids[0] != 100 {
				t.Errorf("MapDeptNames ids = %v, want [100]", ids)
			}
			return map[int64]*string{100: strPtr("研发部")}, nil
		},
		findRoles:       func(_ context.Context, _ int64) ([]role.SysRole, error) { return []role.SysRole{r2}, nil },
		roleIDsByUserID: func(_ context.Context, _ int64) ([]int64, error) { return []int64{2}, nil },
		postIDsByUserID: func(_ context.Context, _ int64) ([]int64, error) { return []int64{4}, nil },
	}
	posts := &mockPosts{selectList: func(_ context.Context, q post.QueryParam) ([]post.Out, error) {
		gotPostQ = q
		return []post.Out{po}, nil
	}}
	roles := &mockRoles{selectList: func(_ context.Context, q role.QueryParam) ([]role.SysRole, error) {
		gotRoleQ = q
		return []role.SysRole{r1, r2}, nil
	}}
	svc := newSvc(nil, superAuth(), repo, posts, roles)

	info, err := svc.GetByID(context.Background(), 9)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if info.User == nil || info.User.DeptName == nil || *info.User.DeptName != "研发部" {
		t.Fatalf("User.DeptName = %+v, want 研发部", info.User)
	}
	if len(info.User.Roles) != 1 || *info.User.Roles[0].RoleKey != "common" {
		t.Errorf("User.Roles = %+v, want 用户已持有角色", info.User.Roles)
	}
	// user 对象内 roleIds/postIds/roleId 恒 null（Python select_user_by_id 不填充）
	if info.User.RoleIDs != nil || info.User.PostIDs != nil || info.User.RoleID != nil {
		t.Errorf("User 内嵌 ID 列表应恒 null: %+v", info.User)
	}
	// posts provider 按目标用户部门查询
	if gotPostQ.DeptID == nil || *gotPostQ.DeptID != 100 {
		t.Errorf("posts.SelectList DeptID = %v, want 100", gotPostQ.DeptID)
	}
	// roles provider 只取正常状态
	if gotRoleQ.Status != modelx.Normal {
		t.Errorf("roles.SelectList Status = %q, want 0", gotRoleQ.Status)
	}
	if len(info.RoleIDs) != 1 || int64(info.RoleIDs[0]) != 2 {
		t.Errorf("RoleIDs = %v, want [2]", info.RoleIDs)
	}
	if len(info.PostIDs) != 1 || int64(info.PostIDs[0]) != 4 {
		t.Errorf("PostIDs = %v, want [4]", info.PostIDs)
	}
	if len(info.Posts) != 1 || *info.Posts[0].PostCode != "ceo" {
		t.Errorf("Posts = %+v, want 原样透传", info.Posts)
	}
	// 目标非超管 → 可选角色列表剔除超管角色
	if len(info.Roles) != 1 || info.Roles[0].SuperAdmin || int64(*info.Roles[0].ID) != 2 {
		t.Errorf("Roles = %+v, want 仅普通角色", info.Roles)
	}
}

func TestGetByIDSuperAdminTargetKeepsAdminRole(t *testing.T) {
	u := &SysUser{UserName: "admin", NickName: "超管", Status: "0", DelFlag: "0"} // DeptID nil → 不查岗位
	u.ID = 1
	r1 := role.SysRole{RoleName: "超级管理员", RoleKey: "superadmin", Status: "0"}
	r1.ID = 1
	r2 := role.SysRole{RoleName: "普通角色", RoleKey: "common", Status: "0"}
	r2.ID = 2
	repo := &mockRepo{
		findByID:        func(_ context.Context, _ int64) (*SysUser, error) { return u, nil },
		findRoles:       func(_ context.Context, _ int64) ([]role.SysRole, error) { return []role.SysRole{r1}, nil },
		roleIDsByUserID: func(_ context.Context, _ int64) ([]int64, error) { return []int64{1}, nil },
		// mapDeptNames / postIDsByUserID 未覆盖：DeptID nil 不应触发
	}
	roles := &mockRoles{selectList: func(_ context.Context, _ role.QueryParam) ([]role.SysRole, error) {
		return []role.SysRole{r1, r2}, nil
	}}
	svc := newSvc(nil, superAuth(), repo, nil, roles) // posts nil：触发即 panic

	info, err := svc.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	// 超管角色过滤按**目标用户 id** 判定：目标=1 → 保留超管角色
	if len(info.Roles) != 2 || !info.Roles[0].SuperAdmin {
		t.Errorf("Roles = %+v, want 保留超管角色", info.Roles)
	}
	if info.Posts != nil || info.PostIDs != nil {
		t.Errorf("无部门用户不应填充岗位: %+v", info)
	}
}

func TestGetAuthRoleFlagsHeldRoles(t *testing.T) {
	u := &SysUser{UserName: "u9", NickName: "用户9", Status: "0", DelFlag: "0"}
	u.ID = 9
	r1 := role.SysRole{RoleName: "超级管理员", RoleKey: "superadmin", Status: "0"}
	r1.ID = 1
	r2 := role.SysRole{RoleName: "普通角色", RoleKey: "common", Status: "0"}
	r2.ID = 2
	r3 := role.SysRole{RoleName: "停用角色", RoleKey: "disabled", Status: "1"}
	r3.ID = 3
	var gotQ role.QueryParam
	repo := &mockRepo{
		findByID:        func(_ context.Context, _ int64) (*SysUser, error) { return u, nil },
		findRoles:       func(_ context.Context, _ int64) ([]role.SysRole, error) { return []role.SysRole{r2}, nil },
		roleIDsByUserID: func(_ context.Context, _ int64) ([]int64, error) { return []int64{2}, nil },
	}
	roles := &mockRoles{selectList: func(_ context.Context, q role.QueryParam) ([]role.SysRole, error) {
		gotQ = q
		return []role.SysRole{r1, r2, r3}, nil
	}}
	svc := newSvc(nil, superAuth(), repo, nil, roles)

	info, err := svc.GetAuthRole(context.Background(), 9)
	if err != nil {
		t.Fatalf("GetAuthRole err = %v", err)
	}
	// authRole 取全部状态（Status 传空串）
	if gotQ.Status != "" {
		t.Errorf("roles.SelectList Status = %q, want 空串（全部状态）", gotQ.Status)
	}
	if len(info.Roles) != 2 {
		t.Fatalf("Roles = %+v, want 剔除超管后 2 个", info.Roles)
	}
	if int64(*info.Roles[0].ID) != 2 || !info.Roles[0].Flag {
		t.Errorf("Roles[0] = %+v, want id=2 flag=true（已持有）", info.Roles[0])
	}
	if int64(*info.Roles[1].ID) != 3 || info.Roles[1].Flag {
		t.Errorf("Roles[1] = %+v, want id=3 flag=false（未持有）", info.Roles[1])
	}
	// authRole 只用持有集打 Flag，不回填顶层 roleIds（与 GetByID 不同）
	if info.RoleIDs != nil {
		t.Errorf("RoleIDs = %v, want nil（authRole 不填充）", info.RoleIDs)
	}
}

// ---- 事务回滚语义（对齐 Python session.begin 失败整体回滚） ----

// TestInsertUserRollbackOnRoleRebuildFailure 证明孤儿用户缺陷已修：
// 用户行先写入、角色重建第二步（InsertUserRoles）失败 → 事务整体 Rollback，
// 绝不 Commit，用户写入随之撤销。假驱动在 database/sql 层记录事务事件。
func TestInsertUserRollbackOnRoleRebuildFailure(t *testing.T) {
	fake := &fakeSQL{roleCount: 2} // 可见数命中，使流程走到关联重建第二步
	db := newFakeDB(t, fake)
	var created bool
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, _ *int64) (bool, error) { return false, nil },
		create: func(ctx context.Context, _ *SysUser) error {
			created = true
			// 事务句柄必须经 ctx 传播到 repo 层（crudx.DB 取到 tx 而非 nil）
			if crudx.DB(ctx, nil) == nil {
				t.Error("Create 未拿到事务句柄：WithTx 传播失效")
			}
			return nil
		},
		deleteUserRoles: func(_ context.Context, _ int64) error { return nil },
		insertUserRoles: func(_ context.Context, _ int64, _ []int64) error { return errors.New("boom") },
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)
	in := CreateIn{UserName: strPtr("newbie"), Password: strPtr("Abc12345!"), RoleIDs: []int64{2, 3}}
	ok, err := svc.InsertUser(context.Background(), in)
	if ok || err == nil || err.Error() != "boom" {
		t.Fatalf("InsertUser = (%v, %v), want (false, boom 原样透传)", ok, err)
	}
	if !created {
		t.Error("用户行应先写入（随后被回滚），而非未执行")
	}
	// 失败必整体回滚：begin → rollback，绝不 commit
	wantCalls(t, fake.txSnapshot(), []string{"begin", "rollback"})
}

// TestInsertUserTxCommitOnSuccess 对照：成功路径 begin → commit，
// 证明上面的 rollback 不是「恒不提交」的假象。
func TestInsertUserTxCommitOnSuccess(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	repo := &mockRepo{
		existsByField: func(_ context.Context, _, _ string, _ *int64) (bool, error) { return false, nil },
		create: func(ctx context.Context, _ *SysUser) error {
			if crudx.DB(ctx, nil) == nil {
				t.Error("Create 未拿到事务句柄：WithTx 传播失效")
			}
			return nil
		},
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)
	in := CreateIn{UserName: strPtr("newbie"), Password: strPtr("Abc12345!")}
	ok, err := svc.InsertUser(context.Background(), in)
	if err != nil || !ok {
		t.Fatalf("InsertUser = (%v, %v), want (true, nil)", ok, err)
	}
	wantCalls(t, fake.txSnapshot(), []string{"begin", "commit"})
}

// TestDeleteUserByIDsRollbackOnSoftDeleteFailure 批删第二步之后软删失败 →
// 关联批删一并回滚（begin → rollback），错误透传。
func TestDeleteUserByIDsRollbackOnSoftDeleteFailure(t *testing.T) {
	fake := &fakeSQL{}
	db := newFakeDB(t, fake)
	var rolesDeleted bool
	repo := &mockRepo{
		deleteUserRolesBatch: func(ctx context.Context, _ []int64) error {
			rolesDeleted = true
			if crudx.DB(ctx, nil) == nil {
				t.Error("关联批删未拿到事务句柄")
			}
			return nil
		},
		deleteUserPostsBatch: func(_ context.Context, _ []int64) error { return nil },
		softDelete:           func(_ context.Context, _ []int64) (int64, error) { return 0, errors.New("boom") },
	}
	svc := newSvc(db, superAuth(), repo, nil, nil)
	n, err := svc.DeleteUserByIDs(context.Background(), []int64{2, 3})
	if n != 0 || err == nil || err.Error() != "boom" {
		t.Fatalf("DeleteUserByIDs = (%d, %v), want (0, boom 透传)", n, err)
	}
	if !rolesDeleted {
		t.Error("关联批删应先执行（随后被回滚）")
	}
	wantCalls(t, fake.txSnapshot(), []string{"begin", "rollback"})
}
