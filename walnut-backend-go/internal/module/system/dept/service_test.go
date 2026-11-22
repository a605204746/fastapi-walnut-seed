package dept

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

func deptRow(id, parent int64, name string, order int, status string) SysDept {
	p := parent
	n := name
	s := status
	o := order
	return SysDept{
		BaseModel: modelx.BaseModel{ID: id},
		ParentID:  &p,
		DeptName:  &n,
		OrderNum:  &o,
		Status:    &s,
		Ancestors: &s,
		DelFlag:   modelx.Normal,
	}
}

// TestBuildTreeSelect 覆盖下拉树的契约形态：键序由 struct 标签保证，
// 这里钉住的是**结构语义**——排序、children 键有无、多根、NULL 父级。
func TestBuildTreeSelect(t *testing.T) {
	t.Run("空列表返回空数组而非 null", func(t *testing.T) {
		got := BuildTreeSelect(nil)
		if got == nil || len(got) != 0 {
			t.Fatalf("want 空数组, got %#v", got)
		}
	})

	t.Run("排序与 children 键有无", func(t *testing.T) {
		rows := []SysDept{
			deptRow(100, 0, "根", 0, "0"),
			// 故意乱序传入：建树排序必须与输入顺序无关
			deptRow(102, 100, "二", 2, "0"),
			deptRow(101, 100, "一", 1, "0"),
			deptRow(103, 101, "叶", 1, "1"),
		}
		got := BuildTreeSelect(rows)
		if len(got) != 1 {
			t.Fatalf("want 1 个根, got %d", len(got))
		}
		root := got[0]
		if root.ID.Int64() != 100 || len(root.Children) != 2 {
			t.Fatalf("根形态错误: %#v", root)
		}
		if root.Children[0].ID.Int64() != 101 || root.Children[1].ID.Int64() != 102 {
			t.Fatalf("children 未按 orderNum 排序: %v, %v",
				root.Children[0].ID.Int64(), root.Children[1].ID.Int64())
		}
		leaf := root.Children[0].Children
		if len(leaf) != 1 || leaf[0].Children != nil {
			t.Fatalf("叶子不应带 children 键: %#v", leaf)
		}
		if !leaf[0].Disabled {
			t.Fatalf("status=1 应 disabled=true")
		}
	})

	t.Run("多根与 NULL 父级", func(t *testing.T) {
		orphan := deptRow(200, 999, "孤", 1, "0")
		orphan.ParentID = nil
		rows := []SysDept{
			deptRow(100, 0, "根", 0, "0"),
			deptRow(300, 100, "子", 5, "0"),
			orphan,
			// 父不在结果集内 → 也是根（过滤查询后的多根形态）
			deptRow(400, 888, "半孤", 2, "0"),
		}
		got := BuildTreeSelect(rows)
		if len(got) != 3 {
			t.Fatalf("want 3 个根(NULL/父缺失/正常根), got %d", len(got))
		}
		// 根按 (orderNum, id) 排序：根(0) < 孤(1) < 半孤(2)
		want := []int64{100, 200, 400}
		for i, w := range want {
			if got[i].ID.Int64() != w {
				t.Fatalf("根序[%d] want %d got %d", i, w, got[i].ID.Int64())
			}
		}
	})
}

// mockRepo 嵌入 deptRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	deptRepo
	get                 func(ctx context.Context, id int64) (*SysDept, error)
	list                func(ctx context.Context, conds ...crudx.Cond) ([]SysDept, error)
	count               func(ctx context.Context, conds ...crudx.Cond) (int64, error)
	listDescendants     func(ctx context.Context, deptID int64) ([]SysDept, error)
	descendantIDs       func(ctx context.Context, deptID int64) ([]int64, error)
	countNormalChildren func(ctx context.Context, deptID int64) (int64, error)
	countUserInDept     func(ctx context.Context, deptID int64) (int64, error)
	countPostInDept     func(ctx context.Context, deptID int64) (int64, error)
	deptNameMap         func(ctx context.Context, ids []int64) (map[int64]*string, error)
	leaderNameMap       func(ctx context.Context, ids []int64) (map[int64]*string, error)
	roleScopes          func(ctx context.Context, userID int64) ([]permx.RoleScope, error)
	updateStatusNormal  func(ctx context.Context, ids []int64) (int64, error)
	logicalDelete       func(ctx context.Context, id int64) (int64, error)
	create              func(ctx context.Context, row *SysDept) error
	update              func(ctx context.Context, row *SysDept) error
}

func (m *mockRepo) Get(ctx context.Context, id int64) (*SysDept, error) {
	return m.get(ctx, id)
}

func (m *mockRepo) List(ctx context.Context, conds ...crudx.Cond) ([]SysDept, error) {
	return m.list(ctx, conds...)
}

func (m *mockRepo) Count(ctx context.Context, conds ...crudx.Cond) (int64, error) {
	return m.count(ctx, conds...)
}

func (m *mockRepo) ListDescendants(ctx context.Context, deptID int64) ([]SysDept, error) {
	return m.listDescendants(ctx, deptID)
}

func (m *mockRepo) DescendantIDs(ctx context.Context, deptID int64) ([]int64, error) {
	return m.descendantIDs(ctx, deptID)
}

func (m *mockRepo) CountNormalChildren(ctx context.Context, deptID int64) (int64, error) {
	return m.countNormalChildren(ctx, deptID)
}

func (m *mockRepo) CountUserInDept(ctx context.Context, deptID int64) (int64, error) {
	return m.countUserInDept(ctx, deptID)
}

func (m *mockRepo) CountPostInDept(ctx context.Context, deptID int64) (int64, error) {
	return m.countPostInDept(ctx, deptID)
}

func (m *mockRepo) DeptNameMap(ctx context.Context, ids []int64) (map[int64]*string, error) {
	return m.deptNameMap(ctx, ids)
}

func (m *mockRepo) LeaderNameMap(ctx context.Context, ids []int64) (map[int64]*string, error) {
	return m.leaderNameMap(ctx, ids)
}

func (m *mockRepo) RoleScopes(ctx context.Context, userID int64) ([]permx.RoleScope, error) {
	return m.roleScopes(ctx, userID)
}

func (m *mockRepo) UpdateStatusNormal(ctx context.Context, ids []int64) (int64, error) {
	return m.updateStatusNormal(ctx, ids)
}

func (m *mockRepo) LogicalDelete(ctx context.Context, id int64) (int64, error) {
	return m.logicalDelete(ctx, id)
}

func (m *mockRepo) Create(ctx context.Context, row *SysDept) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysDept) error {
	return m.update(ctx, row)
}

func testAuth(id int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: id, UserName: "tester"}}
}

func TestInsertParentMissing(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysDept, error) {
		return nil, nil
	}}
	pid := int64(100)
	err := svc.Insert(context.Background(), CreateIn{ParentID: &pid})
	if se := errs.As(err); se == nil || se.Msg != "上级部门不存在，无法新增" {
		t.Errorf("err = %v, want 上级部门不存在，无法新增", err)
	}
}

func TestInsertParentDisabled(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysDept, error) {
		row := deptRow(100, 0, "父", 0, "1")
		return &row, nil
	}}
	pid := int64(100)
	err := svc.Insert(context.Background(), CreateIn{ParentID: &pid})
	if se := errs.As(err); se == nil || se.Msg != "部门停用，不允许新增" {
		t.Errorf("err = %v, want 部门停用，不允许新增", err)
	}
}

func TestDescendantAndSelfIDsNilDept(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	// deptID 为 nil → [-1] 兜底，不触达 repo（触达即 panic）
	ids, err := svc.DescendantAndSelfIDs(context.Background(), nil)
	if err != nil || len(ids) != 1 || ids[0] != -1 {
		t.Errorf("ids = %v, err = %v; want [-1], nil", ids, err)
	}
}

// ==================== 深度业务流测试 ====================

// deptRowAnc 与 deptRow 的区别：ancestors 独立可控（deptRow 把 status 复用为
// ancestors，只适合不关心祖级链的建树测试）。
func deptRowAnc(id, parent int64, ancestors, name string, order int, status string) SysDept {
	row := deptRow(id, parent, name, order, status)
	row.Ancestors = &ancestors
	return row
}

func strp(s string) *string { return &s }
func i64p(v int64) *int64   { return &v }

// pinNow 钉住 nowFunc，返回固定时间；测试结束自动还原。
func pinNow(t *testing.T) time.Time {
	t.Helper()
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	old := nowFunc
	nowFunc = func() time.Time { return fixed }
	t.Cleanup(func() { nowFunc = old })
	return fixed
}

// condSQLs 提取条件 SQL 序列，便于断言「查询条件 + 数据权限条件」的拼装顺序。
func condSQLs(conds []crudx.Cond) []string {
	out := make([]string, 0, len(conds))
	for _, c := range conds {
		out = append(out, c.SQL)
	}
	return out
}

func wantErr(t *testing.T, err error, msg string) {
	t.Helper()
	se := errs.As(err)
	if se == nil || se.Msg != msg {
		t.Errorf("err = %v, want %q", err, msg)
	}
}

// TestTreeSelectOutScopeAndAssembly 钉住 TreeSelectOut 的完整链路：
// 查询条件拼装 → 数据权限分支 → 建树（排序/children/disabled）。
func TestTreeSelectOutScopeAndAssembly(t *testing.T) {
	t.Run("超管不过滤_查询条件与树组装", func(t *testing.T) {
		svc := NewService(nil, testAuth(1)) // ID=1 → IsSuperAdmin
		var gotConds []crudx.Cond
		svc.repo = &mockRepo{
			// roleScopes 未覆盖：超管分支触达即 panic
			list: func(_ context.Context, conds ...crudx.Cond) ([]SysDept, error) {
				gotConds = conds
				// 故意乱序：建树排序必须与输入顺序无关
				return []SysDept{
					deptRowAnc(102, 100, "0,100", "二", 2, "0"),
					deptRowAnc(100, 0, "0", "根", 0, "0"),
					deptRowAnc(103, 101, "0,100,101", "叶", 1, "1"),
					deptRowAnc(101, 100, "0,100", "一", 1, "0"),
				}, nil
			},
		}
		q := QueryParam{DeptName: "研发", Status: "0"}
		tree, err := svc.TreeSelectOut(context.Background(), q)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantSQL := []string{"dept_name LIKE ?", "status = ?"}
		gotSQL := condSQLs(gotConds)
		if len(gotSQL) != len(wantSQL) || gotSQL[0] != wantSQL[0] || gotSQL[1] != wantSQL[1] {
			t.Fatalf("conds = %v, want %v（超管不追加数据权限条件）", gotSQL, wantSQL)
		}
		if gotConds[0].Args[0] != "%研发%" {
			t.Errorf("Like 参数 = %v, want %%研发%%", gotConds[0].Args[0])
		}
		if len(tree) != 1 || tree[0].ID.Int64() != 100 || len(tree[0].Children) != 2 {
			t.Fatalf("树形态错误: %#v", tree)
		}
		if tree[0].Children[0].ID.Int64() != 101 || tree[0].Children[1].ID.Int64() != 102 {
			t.Errorf("children 未按 (orderNum,id) 排序")
		}
		leaf := tree[0].Children[0].Children
		if len(leaf) != 1 || leaf[0].Children != nil || !leaf[0].Disabled {
			t.Errorf("叶子形态错误: %#v（status=1 应 disabled 且无 children 键）", leaf)
		}
	})

	t.Run("空结果返回空数组", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{list: func(_ context.Context, _ ...crudx.Cond) ([]SysDept, error) {
			return []SysDept{}, nil
		}}
		tree, err := svc.TreeSelectOut(context.Background(), QueryParam{})
		if err != nil || tree == nil || len(tree) != 0 {
			t.Fatalf("tree = %#v, err = %v; want 非 nil 空数组", tree, err)
		}
	})

	// scopeCases 穷举数据权限分支：每个用例钉住追加到 List 的条件 SQL 与参数。
	scopeCases := []struct {
		name    string
		auth    authctx.Context
		scopes  []permx.RoleScope
		descIDs []int64 // ScopeDeptAndChild 时 DescendantIDs 的返回
		wantSQL string  // 期望的数据权限条件（空串表示不追加）
		wantArg any     // 期望的首个绑定参数（nil 表示不校验）
	}{
		{
			name:   "角色集为空_不过滤_dept特有语义",
			auth:   testAuth(2),
			scopes: nil,
		},
		{
			name:   "任一角色ALL_整体放行",
			auth:   testAuth(2),
			scopes: []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeSelf)}, {RoleID: 10, DataScope: strp(permx.ScopeAll)}},
		},
		{
			name:    "本部门及以下_自身ID追加在末位",
			auth:    authctx.Context{User: authctx.User{ID: 2, DeptID: i64p(100)}},
			scopes:  []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeDeptAndChild)}},
			descIDs: []int64{101, 102},
			wantSQL: "(id IN ?)",
			wantArg: []int64{101, 102, 100},
		},
		{
			name:    "范围6等价范围4_不做或本人",
			auth:    authctx.Context{User: authctx.User{ID: 2, DeptID: i64p(100)}},
			scopes:  []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeDeptChildOrSelf)}},
			descIDs: []int64{101},
			wantSQL: "(id IN ?)",
			wantArg: []int64{101, 100},
		},
		{
			name:    "自定义_角色授权部门子查询",
			auth:    testAuth(2),
			scopes:  []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeCustom)}},
			wantSQL: "(id IN (SELECT dept_id FROM sys_role_dept WHERE role_id = ?))",
			wantArg: int64(9),
		},
		{
			name:    "本部门_按自身id列过滤",
			auth:    authctx.Context{User: authctx.User{ID: 2, DeptID: i64p(100)}},
			scopes:  []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeDept)}},
			wantSQL: "(id = ?)",
			wantArg: int64(100),
		},
		{
			name:    "本部门_无部门兜底负1",
			auth:    testAuth(2),
			scopes:  []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeDept)}},
			wantSQL: "(id = ?)",
			wantArg: int64(-1),
		},
		{
			name:    "仅本人_部门表恒假",
			auth:    testAuth(2),
			scopes:  []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeSelf)}},
			wantSQL: "(1 = 0)",
		},
		{
			name:    "多角色OR连接",
			auth:    authctx.Context{User: authctx.User{ID: 2, DeptID: i64p(100)}},
			scopes:  []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeDept)}, {RoleID: 10, DataScope: strp(permx.ScopeCustom)}},
			wantSQL: "(id = ? OR id IN (SELECT dept_id FROM sys_role_dept WHERE role_id = ?))",
			wantArg: int64(100),
		},
	}
	for _, tc := range scopeCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil, tc.auth)
			var gotConds []crudx.Cond
			svc.repo = &mockRepo{
				roleScopes: func(_ context.Context, userID int64) ([]permx.RoleScope, error) {
					if userID != tc.auth.User.ID {
						t.Errorf("userID = %d, want %d", userID, tc.auth.User.ID)
					}
					return tc.scopes, nil
				},
				descendantIDs: func(_ context.Context, deptID int64) ([]int64, error) {
					if tc.auth.User.DeptID == nil || deptID != *tc.auth.User.DeptID {
						t.Errorf("deptID = %d, want 用户部门", deptID)
					}
					return tc.descIDs, nil
				},
				list: func(_ context.Context, conds ...crudx.Cond) ([]SysDept, error) {
					gotConds = conds
					return []SysDept{}, nil
				},
			}
			if _, err := svc.TreeSelectOut(context.Background(), QueryParam{}); err != nil {
				t.Fatalf("err = %v", err)
			}
			if tc.wantSQL == "" {
				if len(gotConds) != 0 {
					t.Fatalf("conds = %v, want 不追加任何条件", condSQLs(gotConds))
				}
				return
			}
			if len(gotConds) != 1 || gotConds[0].SQL != tc.wantSQL {
				t.Fatalf("conds = %v, want [%s]", condSQLs(gotConds), tc.wantSQL)
			}
			if tc.wantArg != nil {
				if len(gotConds[0].Args) == 0 {
					t.Fatalf("args 为空, want %v", tc.wantArg)
				}
				got, want := gotConds[0].Args[0], tc.wantArg
				if gs, ok := got.([]int64); ok {
					ws := want.([]int64)
					if len(gs) != len(ws) {
						t.Fatalf("args = %v, want %v", gs, ws)
					}
					for i := range gs {
						if gs[i] != ws[i] {
							t.Fatalf("args = %v, want %v", gs, ws)
						}
					}
				} else if got != want {
					t.Fatalf("args[0] = %v, want %v", got, want)
				}
			}
		})
	}

	t.Run("非法scope取值_契约文案", func(t *testing.T) {
		for _, tc := range []struct {
			scope *string
			want  string
		}{
			{strp("9"), "角色数据范围异常 => 9"},
			{nil, "角色数据范围异常 => None"},
		} {
			svc := NewService(nil, testAuth(2))
			svc.repo = &mockRepo{
				roleScopes: func(_ context.Context, _ int64) ([]permx.RoleScope, error) {
					return []permx.RoleScope{{RoleID: 9, DataScope: tc.scope}}, nil
				},
				// list 未覆盖：求解失败必须短路，触达即 panic
			}
			_, err := svc.TreeSelectOut(context.Background(), QueryParam{})
			wantErr(t, err, tc.want)
		}
	})

	t.Run("角色查询失败_failClosed", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{
			roleScopes: func(_ context.Context, _ int64) ([]permx.RoleScope, error) {
				return nil, errors.New("boom")
			},
		}
		_, err := svc.TreeSelectOut(context.Background(), QueryParam{})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s (detail=boom)，绝不吞错放行", err, errs.MsgDBFailed)
		}
	})
}

// TestTreeSelectAllNoQuery 无查询参数入口：不产生查询条件，仅数据权限。
func TestTreeSelectAllNoQuery(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	svc.repo = &mockRepo{list: func(_ context.Context, conds ...crudx.Cond) ([]SysDept, error) {
		if len(conds) != 0 {
			t.Errorf("conds = %v, want 空（param=None 且超管）", condSQLs(conds))
		}
		return []SysDept{deptRowAnc(100, 0, "0", "根", 0, "0")}, nil
	}}
	tree, err := svc.TreeSelectAll(context.Background())
	if err != nil || len(tree) != 1 || tree[0].ID.Int64() != 100 {
		t.Fatalf("tree = %#v, err = %v", tree, err)
	}

	t.Run("查询失败原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{list: func(_ context.Context, _ ...crudx.Cond) ([]SysDept, error) {
			return nil, boom
		}}
		if _, err := svc.TreeSelectAll(context.Background()); !errors.Is(err, boom) {
			t.Errorf("err = %v, want 原始错误", err)
		}
	})
}

// TestInsertAncestorsAndDefaults 新增部门：祖级链生成 + 模型默认值 + 审计字段。
func TestInsertAncestorsAndDefaults(t *testing.T) {
	t.Run("祖级链_默认status_审计字段", func(t *testing.T) {
		fixed := pinNow(t)
		svc := NewService(nil, testAuth(2))
		parent := deptRowAnc(101, 100, "0,100", "父", 0, "0")
		var got *SysDept
		svc.repo = &mockRepo{
			get: func(_ context.Context, id int64) (*SysDept, error) {
				if id != 101 {
					t.Errorf("get id = %d, want 101", id)
				}
				return &parent, nil
			},
			create: func(_ context.Context, row *SysDept) error {
				got = row
				return nil
			},
		}
		name, order := "研发部", 2
		in := CreateIn{ParentID: i64p(101), DeptName: &name, OrderNum: &order, Leader: i64p(7)}
		if err := svc.Insert(context.Background(), in); err != nil {
			t.Fatalf("err = %v", err)
		}
		if got == nil {
			t.Fatal("create 未被调用")
		}
		if got.Ancestors == nil || *got.Ancestors != "0,100,101" {
			t.Errorf("Ancestors = %v, want 0,100,101（父祖级链+父id）", got.Ancestors)
		}
		if got.Status == nil || *got.Status != "0" {
			t.Errorf("Status = %v, want 模型默认值 0（未提供时不吃 NULL）", got.Status)
		}
		if got.DelFlag != modelx.Normal {
			t.Errorf("DelFlag = %s, want 0", got.DelFlag)
		}
		if got.ParentID == nil || *got.ParentID != 101 || got.Leader == nil || *got.Leader != 7 {
			t.Errorf("ParentID/Leader 透传错误: %#v", got)
		}
		if got.ID == 0 {
			t.Error("ID 应由雪花填充")
		}
		if got.CreateBy == nil || *got.CreateBy != 2 || got.UpdateBy == nil || *got.UpdateBy != 2 {
			t.Errorf("审计人 = %v/%v, want 2/2", got.CreateBy, got.UpdateBy)
		}
		if got.CreateTime == nil || !got.CreateTime.Equal(fixed) || got.UpdateTime == nil || !got.UpdateTime.Equal(fixed) {
			t.Errorf("审计时间未钉住 nowFunc: %v/%v", got.CreateTime, got.UpdateTime)
		}
	})

	t.Run("父祖级NULL_拼出字面量None", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		parent := deptRowAnc(101, 0, "", "父", 0, "0")
		parent.Ancestors = nil
		var got *SysDept
		svc.repo = &mockRepo{
			get: func(_ context.Context, _ int64) (*SysDept, error) { return &parent, nil },
			create: func(_ context.Context, row *SysDept) error {
				got = row
				return nil
			},
		}
		if err := svc.Insert(context.Background(), CreateIn{ParentID: i64p(101)}); err != nil {
			t.Fatalf("err = %v", err)
		}
		// Python f-string 对 None 拼出字面量 "None"，契约照抄不修正
		if got.Ancestors == nil || *got.Ancestors != "None,101" {
			t.Errorf("Ancestors = %v, want None,101", got.Ancestors)
		}
	})

	t.Run("父status为NULL_视同停用", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		parent := deptRowAnc(101, 0, "0", "父", 0, "0")
		parent.Status = nil
		svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysDept, error) { return &parent, nil }}
		wantErr(t, svc.Insert(context.Background(), CreateIn{ParentID: i64p(101)}), "部门停用，不允许新增")
	})

	t.Run("父查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysDept, error) {
			return nil, errors.New("boom")
		}}
		se := errs.As(svc.Insert(context.Background(), CreateIn{ParentID: i64p(101)}))
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("want 500 %s (detail=boom)", errs.MsgDBFailed)
		}
	})

	t.Run("写入失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		parent := deptRowAnc(101, 0, "0", "父", 0, "0")
		svc.repo = &mockRepo{
			get:    func(_ context.Context, _ int64) (*SysDept, error) { return &parent, nil },
			create: func(_ context.Context, _ *SysDept) error { return errors.New("boom") },
		}
		se := errs.As(svc.Insert(context.Background(), CreateIn{ParentID: i64p(101)}))
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("want 500 %s (detail=boom)", errs.MsgDBFailed)
		}
	})
}

// TestCheckNameUniqueConds 同父级名称唯一性：条件拼装（含 IS NULL / 排除自身）。
func TestCheckNameUniqueConds(t *testing.T) {
	t.Run("同名存在_返回false", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		var gotConds []crudx.Cond
		svc.repo = &mockRepo{count: func(_ context.Context, conds ...crudx.Cond) (int64, error) {
			gotConds = conds
			return 1, nil
		}}
		ok, err := svc.CheckNameUnique(context.Background(), "研发部", i64p(100), i64p(101))
		if err != nil || ok {
			t.Fatalf("ok = %v, err = %v; want false, nil", ok, err)
		}
		want := []string{"dept_name = ?", "parent_id = ?", "id <> ?"}
		got := condSQLs(gotConds)
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("conds = %v, want %v", got, want)
		}
		if gotConds[0].Args[0] != "研发部" || gotConds[1].Args[0] != int64(100) || gotConds[2].Args[0] != int64(101) {
			t.Errorf("绑定参数错误: %#v", gotConds)
		}
	})

	t.Run("无同名_返回true", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{count: func(_ context.Context, _ ...crudx.Cond) (int64, error) { return 0, nil }}
		ok, err := svc.CheckNameUnique(context.Background(), "研发部", i64p(100), nil)
		if err != nil || !ok {
			t.Fatalf("ok = %v, err = %v; want true, nil", ok, err)
		}
	})

	t.Run("parentID为NULL_生成IS_NULL", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		var gotConds []crudx.Cond
		svc.repo = &mockRepo{count: func(_ context.Context, conds ...crudx.Cond) (int64, error) {
			gotConds = conds
			return 0, nil
		}}
		if _, err := svc.CheckNameUnique(context.Background(), "根", nil, nil); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(gotConds) != 2 || gotConds[1].SQL != "parent_id IS NULL" {
			t.Fatalf("conds = %v, want parent_id IS NULL", condSQLs(gotConds))
		}
	})

	t.Run("计数失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{count: func(_ context.Context, _ ...crudx.Cond) (int64, error) { return 0, boom }}
		if _, err := svc.CheckNameUnique(context.Background(), "x", nil, nil); !errors.Is(err, boom) {
			t.Errorf("err = %v, want 原始错误", err)
		}
	})
}

// TestUpdateMoveParentCascade 换父部门：数据权限 → 新父祖级链 → 子孙联动 →
// 自身保存 → 启用时连带启用上级。调用序与每步入参全部钉住。
func TestUpdateMoveParentCascade(t *testing.T) {
	fixed := pinNow(t)
	svc := NewService(nil, testAuth(1)) // 超管：CheckDataScope 短路
	old := deptRowAnc(101, 100, "0,100", "旧名", 1, "1")
	newParent := deptRowAnc(200, 0, "0", "新父", 0, "0")
	child1 := deptRowAnc(102, 101, "0,100,101", "子", 1, "0")
	child2 := deptRowAnc(103, 102, "0,100,101,102", "孙", 1, "0")

	var getCalls []int64
	var updates []*SysDept
	var statusIDs []int64
	var descCalls []int64
	svc.repo = &mockRepo{
		get: func(_ context.Context, id int64) (*SysDept, error) {
			getCalls = append(getCalls, id)
			switch id {
			case 101:
				return &old, nil
			case 200:
				return &newParent, nil
			}
			t.Errorf("意外 get id = %d", id)
			return nil, nil
		},
		listDescendants: func(_ context.Context, deptID int64) ([]SysDept, error) {
			descCalls = append(descCalls, deptID)
			return []SysDept{child1, child2}, nil
		},
		update: func(_ context.Context, row *SysDept) error {
			cp := *row
			updates = append(updates, &cp)
			return nil
		},
		updateStatusNormal: func(_ context.Context, ids []int64) (int64, error) {
			statusIDs = append(statusIDs, ids...)
			return int64(len(ids)), nil
		},
	}
	name, status := "新名", "0"
	in := UpdateIn{CreateIn: CreateIn{ParentID: i64p(200), DeptName: &name, Status: &status}, ID: i64p(101)}
	if err := svc.Update(context.Background(), in); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(getCalls) != 2 || getCalls[0] != 101 || getCalls[1] != 200 {
		t.Fatalf("get 调用序 = %v, want [101 200]", getCalls)
	}
	if len(descCalls) != 1 || descCalls[0] != 101 {
		t.Fatalf("ListDescendants 调用 = %v, want [101]", descCalls)
	}
	if len(updates) != 3 {
		t.Fatalf("update 次数 = %d, want 3（两子孙+自身）", len(updates))
	}
	// 子孙先于自身更新；祖级链只替换首次出现的旧串
	if updates[0].ID != 102 || *updates[0].Ancestors != "0,200,101" {
		t.Errorf("子102 ancestors = %v, want 0,200,101", updates[0].Ancestors)
	}
	if updates[1].ID != 103 || *updates[1].Ancestors != "0,200,101,102" {
		t.Errorf("孙103 ancestors = %v, want 0,200,101,102", updates[1].Ancestors)
	}
	self := updates[2]
	if self.ID != 101 || self.ParentID == nil || *self.ParentID != 200 ||
		self.Ancestors == nil || *self.Ancestors != "0,200" || self.DeptName == nil || *self.DeptName != "新名" {
		t.Errorf("自身保存形态错误: %#v", self)
	}
	if self.UpdateBy == nil || *self.UpdateBy != 1 || self.UpdateTime == nil || !self.UpdateTime.Equal(fixed) {
		t.Errorf("自身审计字段错误: %v/%v", self.UpdateBy, self.UpdateTime)
	}
	if updates[0].UpdateBy == nil || *updates[0].UpdateBy != 1 || !updates[0].UpdateTime.Equal(fixed) {
		t.Errorf("子孙也应填审计字段: %#v", updates[0].BaseModel)
	}
	// 启用部门 → 连带启用新祖级链上的全部上级（"0" 也会被解析进 ids）
	if len(statusIDs) != 2 || statusIDs[0] != 0 || statusIDs[1] != 200 {
		t.Errorf("UpdateStatusNormal ids = %v, want [0 200]", statusIDs)
	}
}

// TestUpdateGuardAndEdgeCases 修改部门的守卫与边界分支。
func TestUpdateGuardAndEdgeCases(t *testing.T) {
	t.Run("部门不存在", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysDept, error) { return nil, nil }}
		wantErr(t, svc.Update(context.Background(), UpdateIn{ID: i64p(101)}), "部门不存在，无法修改")
	})

	t.Run("换父无数据权限_拒绝越权挂载", func(t *testing.T) {
		svc := NewService(nil, authctx.Context{User: authctx.User{ID: 2, DeptID: i64p(100)}})
		old := deptRowAnc(101, 100, "0,100", "旧", 1, "0")
		var countConds []crudx.Cond
		svc.repo = &mockRepo{
			get: func(_ context.Context, id int64) (*SysDept, error) {
				if id != 101 {
					t.Errorf("只应查旧部门, got id = %d", id)
				}
				return &old, nil
			},
			roleScopes: func(_ context.Context, _ int64) ([]permx.RoleScope, error) {
				return []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeDept)}}, nil
			},
			count: func(_ context.Context, conds ...crudx.Cond) (int64, error) {
				countConds = conds
				return 0, nil // 新父 200 不在本部门范围
			},
			// listDescendants/update 未覆盖：越权必须短路
		}
		err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ParentID: i64p(200)}, ID: i64p(101)})
		wantErr(t, err, msgNoScope)
		want := []string{"id = ?", "(id = ?)"}
		got := condSQLs(countConds)
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("CheckDataScope conds = %v, want %v", got, want)
		}
	})

	t.Run("新父不存在_祖级链保持不变", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		old := deptRowAnc(101, 100, "0,100", "旧", 1, "1")
		var got *SysDept
		svc.repo = &mockRepo{
			get: func(_ context.Context, id int64) (*SysDept, error) {
				if id == 101 {
					return &old, nil
				}
				return nil, nil // 新父 999 不存在
			},
			update: func(_ context.Context, row *SysDept) error {
				got = row
				return nil
			},
			// listDescendants/updateStatusNormal 未覆盖：不应触达
		}
		if err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ParentID: i64p(999)}, ID: i64p(101)}); err != nil {
			t.Fatalf("err = %v", err)
		}
		// Python 既有语义：newParent 为 None 时不重算 ancestors，但 parent_id 仍被覆盖
		if got.ParentID == nil || *got.ParentID != 999 || got.Ancestors == nil || *got.Ancestors != "0,100" {
			t.Errorf("ParentID/Ancestors = %v/%v, want 999/0,100", got.ParentID, got.Ancestors)
		}
	})

	t.Run("父级未变_不重算不联动", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		old := deptRowAnc(101, 100, "0,100", "旧", 1, "1")
		var got *SysDept
		svc.repo = &mockRepo{
			get: func(_ context.Context, id int64) (*SysDept, error) {
				if id != 101 {
					t.Errorf("父级未变不应查新父, got id = %d", id)
				}
				return &old, nil
			},
			update: func(_ context.Context, row *SysDept) error {
				got = row
				return nil
			},
		}
		name := "改名"
		if err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ParentID: i64p(100), DeptName: &name}, ID: i64p(101)}); err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.DeptName == nil || *got.DeptName != "改名" || *got.Ancestors != "0,100" {
			t.Errorf("仅改名应保留祖级链: %#v", got)
		}
		if got.Status == nil || *got.Status != "1" {
			t.Errorf("未提供 status 应保留原值 1: %v", got.Status)
		}
	})

	t.Run("旧祖级NULL_子级前缀插入", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		old := deptRowAnc(101, 100, "", "旧", 1, "1")
		old.Ancestors = nil
		newParent := deptRowAnc(200, 0, "0", "新父", 0, "0")
		child := deptRowAnc(102, 101, "101", "子", 1, "0")
		var childGot *SysDept
		svc.repo = &mockRepo{
			get: func(_ context.Context, id int64) (*SysDept, error) {
				if id == 101 {
					return &old, nil
				}
				return &newParent, nil
			},
			listDescendants: func(_ context.Context, _ int64) ([]SysDept, error) {
				return []SysDept{child}, nil
			},
			update: func(_ context.Context, row *SysDept) error {
				if row.ID == 102 {
					cp := *row
					childGot = &cp
				}
				return nil
			},
		}
		if err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ParentID: i64p(200)}, ID: i64p(101)}); err != nil {
			t.Fatalf("err = %v", err)
		}
		// replace("", new, 1) 等价前缀插入（Python str.replace 同款）
		if childGot == nil || childGot.Ancestors == nil || *childGot.Ancestors != "0,200101" {
			t.Errorf("子级 ancestors = %v, want 0,200101（前缀插入）", childGot.Ancestors)
		}
	})

	t.Run("启用根部门_不连带启用", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		old := deptRowAnc(100, 0, "0", "根", 0, "1")
		svc.repo = &mockRepo{
			get:    func(_ context.Context, _ int64) (*SysDept, error) { return &old, nil },
			update: func(_ context.Context, _ *SysDept) error { return nil },
			// updateStatusNormal 未覆盖：祖级 "0" 必须被排除
		}
		status := "0"
		if err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{Status: &status}, ID: i64p(100)}); err != nil {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("停用_不触发连带启用", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		old := deptRowAnc(101, 100, "0,100", "旧", 1, "0")
		svc.repo = &mockRepo{
			get:    func(_ context.Context, _ int64) (*SysDept, error) { return &old, nil },
			update: func(_ context.Context, _ *SysDept) error { return nil },
		}
		status := "1"
		if err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{Status: &status}, ID: i64p(101)}); err != nil {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("祖级含非数字_启用链路500", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		old := deptRowAnc(101, 100, "0,abc", "旧", 1, "1")
		svc.repo = &mockRepo{
			get:    func(_ context.Context, _ int64) (*SysDept, error) { return &old, nil },
			update: func(_ context.Context, _ *SysDept) error { return nil },
		}
		status := "0"
		err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{Status: &status}, ID: i64p(101)})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed {
			t.Errorf("err = %v, want 500 %s（int(item) 抛错等价）", err, errs.MsgDBFailed)
		}
	})

	t.Run("子孙更新失败_短路自身保存", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		old := deptRowAnc(101, 100, "0,100", "旧", 1, "0")
		newParent := deptRowAnc(200, 0, "0", "新父", 0, "0")
		child := deptRowAnc(102, 101, "0,100,101", "子", 1, "0")
		var updates int
		svc.repo = &mockRepo{
			get: func(_ context.Context, id int64) (*SysDept, error) {
				if id == 101 {
					return &old, nil
				}
				return &newParent, nil
			},
			listDescendants: func(_ context.Context, _ int64) ([]SysDept, error) {
				return []SysDept{child}, nil
			},
			update: func(_ context.Context, _ *SysDept) error {
				updates++
				return errors.New("boom")
			},
		}
		err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ParentID: i64p(200)}, ID: i64p(101)})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s (detail=boom)", err, errs.MsgDBFailed)
		}
		if updates != 1 {
			t.Errorf("update 次数 = %d, want 1（子级失败即短路）", updates)
		}
	})
}

// TestDeleteGuardsAndLogicalDelete 删除链的 service 构件：
// 有子级/已分配用户/有岗位的判定 + 逻辑删除结果语义。
func TestDeleteGuardsAndLogicalDelete(t *testing.T) {
	t.Run("有子级_HasChild为true", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		var gotConds []crudx.Cond
		svc.repo = &mockRepo{count: func(_ context.Context, conds ...crudx.Cond) (int64, error) {
			gotConds = conds
			return 1, nil
		}}
		has, err := svc.HasChild(context.Background(), 101)
		if err != nil || !has {
			t.Fatalf("has = %v, err = %v; want true, nil", has, err)
		}
		if len(gotConds) != 1 || gotConds[0].SQL != "parent_id = ?" || gotConds[0].Args[0] != int64(101) {
			t.Errorf("conds = %#v, want parent_id = 101", gotConds)
		}
	})

	t.Run("无子级_HasChild为false", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{count: func(_ context.Context, _ ...crudx.Cond) (int64, error) { return 0, nil }}
		if has, err := svc.HasChild(context.Background(), 101); err != nil || has {
			t.Fatalf("has = %v, err = %v; want false, nil", has, err)
		}
	})

	t.Run("已分配用户_ExistsUser为true", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{countUserInDept: func(_ context.Context, deptID int64) (int64, error) {
			if deptID != 101 {
				t.Errorf("deptID = %d, want 101", deptID)
			}
			return 3, nil
		}}
		if exists, err := svc.ExistsUser(context.Background(), 101); err != nil || !exists {
			t.Fatalf("exists = %v, err = %v; want true, nil", exists, err)
		}
	})

	t.Run("岗位数与正常子孙数透传", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{
			countPostInDept:     func(_ context.Context, _ int64) (int64, error) { return 2, nil },
			countNormalChildren: func(_ context.Context, _ int64) (int64, error) { return 4, nil },
		}
		if n, err := svc.CountPost(context.Background(), 101); err != nil || n != 2 {
			t.Errorf("CountPost = %d, %v; want 2, nil", n, err)
		}
		if n, err := svc.CountNormalChildren(context.Background(), 101); err != nil || n != 4 {
			t.Errorf("CountNormalChildren = %d, %v; want 4, nil", n, err)
		}
	})

	t.Run("逻辑删除_成功与未命中", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		var gotID int64
		svc.repo = &mockRepo{logicalDelete: func(_ context.Context, id int64) (int64, error) {
			gotID = id
			return 1, nil
		}}
		done, err := svc.Delete(context.Background(), 101)
		if err != nil || !done || gotID != 101 {
			t.Fatalf("done = %v, err = %v, id = %d", done, err, gotID)
		}
		svc.repo = &mockRepo{logicalDelete: func(_ context.Context, _ int64) (int64, error) { return 0, nil }}
		if done, err := svc.Delete(context.Background(), 999); err != nil || done {
			t.Fatalf("done = %v, err = %v; want false, nil（未命中）", done, err)
		}
	})

	t.Run("逻辑删除失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{logicalDelete: func(_ context.Context, _ int64) (int64, error) {
			return 0, errors.New("boom")
		}}
		done, err := svc.Delete(context.Background(), 101)
		se := errs.As(err)
		if done || se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("done = %v, err = %v; want false + 500 %s", done, err, errs.MsgDBFailed)
		}
	})
}

// TestCheckDataScopeBranches 数据权限校验的各分支。
func TestCheckDataScopeBranches(t *testing.T) {
	t.Run("nil部门与超管_不触达repo", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{} // 任何 repo 调用即 panic
		if err := svc.CheckDataScope(context.Background(), nil); err != nil {
			t.Errorf("nil 部门应放行, got %v", err)
		}
		if err := svc.CheckDataScope(context.Background(), i64p(101)); err != nil {
			t.Errorf("超管应放行, got %v", err)
		}
	})

	t.Run("普通用户_范围内放行_范围外拒绝", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		n := int64(1)
		svc.repo = &mockRepo{
			roleScopes: func(_ context.Context, _ int64) ([]permx.RoleScope, error) {
				return []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeDept)}}, nil
			},
			count: func(_ context.Context, _ ...crudx.Cond) (int64, error) { return n, nil },
		}
		if err := svc.CheckDataScope(context.Background(), i64p(101)); err != nil {
			t.Fatalf("范围内应放行, got %v", err)
		}
		n = 0
		wantErr(t, svc.CheckDataScope(context.Background(), i64p(101)), msgNoScope)
	})

	t.Run("计数失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{
			roleScopes: func(_ context.Context, _ int64) ([]permx.RoleScope, error) {
				return []permx.RoleScope{{RoleID: 9, DataScope: strp(permx.ScopeAll)}}, nil
			},
			count: func(_ context.Context, _ ...crudx.Cond) (int64, error) { return 0, errors.New("boom") },
		}
		se := errs.As(svc.CheckDataScope(context.Background(), i64p(101)))
		if se == nil || se.Msg != errs.MsgDBFailed {
			t.Errorf("want 500 %s", errs.MsgDBFailed)
		}
	})
}

// TestListOutFillAndExclude 列表出参回填与 exclude 的元素级匹配。
func TestListOutFillAndExclude(t *testing.T) {
	t.Run("回填parentName与leaderName_去重批量", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		r1 := deptRowAnc(101, 100, "0,100", "一", 1, "0")
		r1.Leader = i64p(7)
		r2 := deptRowAnc(102, 100, "0,100", "二", 2, "0")
		r2.Leader = i64p(8)
		var parentIDs, leaderIDs []int64
		svc.repo = &mockRepo{
			list: func(_ context.Context, _ ...crudx.Cond) ([]SysDept, error) {
				return []SysDept{r1, r2}, nil
			},
			deptNameMap: func(_ context.Context, ids []int64) (map[int64]*string, error) {
				parentIDs = ids
				return map[int64]*string{100: strp("根")}, nil
			},
			leaderNameMap: func(_ context.Context, ids []int64) (map[int64]*string, error) {
				leaderIDs = ids
				return map[int64]*string{7: strp("alice"), 8: strp("bob")}, nil
			},
		}
		outs, err := svc.ListOut(context.Background(), QueryParam{})
		if err != nil || len(outs) != 2 {
			t.Fatalf("outs = %d, err = %v", len(outs), err)
		}
		if len(parentIDs) != 1 || parentIDs[0] != 100 {
			t.Errorf("parentIDs = %v, want [100]（去重）", parentIDs)
		}
		if len(leaderIDs) != 2 || leaderIDs[0] != 7 || leaderIDs[1] != 8 {
			t.Errorf("leaderIDs = %v, want [7 8]", leaderIDs)
		}
		if outs[0].ParentName == nil || *outs[0].ParentName != "根" ||
			outs[0].LeaderName == nil || *outs[0].LeaderName != "alice" ||
			outs[1].LeaderName == nil || *outs[1].LeaderName != "bob" {
			t.Errorf("回填错误: %#v / %#v", outs[0], outs[1])
		}
		if outs[0].Children == nil || len(outs[0].Children) != 0 {
			t.Errorf("列表行 children 恒为 []（非 null）")
		}
	})

	t.Run("空列表_不触达回填", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{list: func(_ context.Context, _ ...crudx.Cond) ([]SysDept, error) {
			return []SysDept{}, nil
		}}
		outs, err := svc.ListOut(context.Background(), QueryParam{})
		if err != nil || outs == nil || len(outs) != 0 {
			t.Fatalf("outs = %#v, err = %v; want 非 nil 空数组", outs, err)
		}
	})

	t.Run("exclude_元素级匹配不误伤子串", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		rows := []SysDept{
			deptRowAnc(1, 0, "0", "自身", 0, "0"),           // 自身排除
			deptRowAnc(100, 0, "0", "百", 1, "0"),          // ancestors 无 "1" 元素 → 保留
			deptRowAnc(101, 1, "0,1", "子", 2, "0"),        // 含 "1" 元素 → 排除
			deptRowAnc(102, 100, "0,10,100", "孙", 3, "0"), // "10"/"100" 不是 "1" → 保留
			deptRowAnc(103, 1, "0,1,100", "曾孙", 4, "0"),   // 含 "1" → 排除
		}
		svc.repo = &mockRepo{
			list: func(_ context.Context, _ ...crudx.Cond) ([]SysDept, error) { return rows, nil },
			deptNameMap: func(_ context.Context, _ []int64) (map[int64]*string, error) {
				return map[int64]*string{}, nil
			},
			leaderNameMap: func(_ context.Context, _ []int64) (map[int64]*string, error) {
				return map[int64]*string{}, nil
			},
		}
		outs, err := svc.ListOutExclude(context.Background(), 1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(outs) != 2 || outs[0].ID.Int64() != 100 || outs[1].ID.Int64() != 102 {
			t.Fatalf("exclude 结果 = %v, want [100 102]（str2list 元素级匹配）", outs)
		}
	})
}

// TestGetOutAndOptionSelect 详情与选择框出参形态。
func TestGetOutAndOptionSelect(t *testing.T) {
	t.Run("详情_存在与不存在", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		row := deptRowAnc(101, 100, "0,100", "一", 1, "0")
		row.Leader = i64p(7)
		svc.repo = &mockRepo{
			get: func(_ context.Context, _ int64) (*SysDept, error) { return &row, nil },
			deptNameMap: func(_ context.Context, _ []int64) (map[int64]*string, error) {
				return map[int64]*string{100: strp("根")}, nil
			},
			leaderNameMap: func(_ context.Context, _ []int64) (map[int64]*string, error) {
				return map[int64]*string{7: strp("alice")}, nil
			},
		}
		out, err := svc.GetOut(context.Background(), 101)
		if err != nil || out == nil || out.ParentName == nil || *out.ParentName != "根" ||
			out.LeaderName == nil || *out.LeaderName != "alice" {
			t.Fatalf("out = %#v, err = %v", out, err)
		}
		svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysDept, error) { return nil, nil }}
		if out, err := svc.GetOut(context.Background(), 999); err != nil || out != nil {
			t.Fatalf("out = %#v, err = %v; want nil, nil", out, err)
		}
	})

	t.Run("详情查询失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{get: func(_ context.Context, _ int64) (*SysDept, error) { return nil, boom }}
		if _, err := svc.GetOut(context.Background(), 101); !errors.Is(err, boom) {
			t.Errorf("err = %v, want 原始错误", err)
		}
	})

	t.Run("选择框_仅回填三字段", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		row := deptRowAnc(101, 100, "0,100", "一", 1, "0")
		row.Leader = i64p(7)
		var gotConds []crudx.Cond
		svc.repo = &mockRepo{list: func(_ context.Context, conds ...crudx.Cond) ([]SysDept, error) {
			gotConds = conds
			return []SysDept{row}, nil
		}}
		outs, err := svc.OptionSelectOut(context.Background(), []int64{101})
		if err != nil || len(outs) != 1 {
			t.Fatalf("outs = %d, err = %v", len(outs), err)
		}
		want := []string{"status = ?", "id IN ?"}
		got := condSQLs(gotConds)
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("conds = %v, want %v", got, want)
		}
		o := outs[0]
		if o.ID == nil || o.ID.Int64() != 101 || o.DeptName == nil || *o.DeptName != "一" ||
			o.Leader == nil || o.Leader.Int64() != 7 {
			t.Errorf("id/deptName/leader 应回填: %#v", o)
		}
		// Python 手工构造 DeptOutSchema(id, dept_name, leader)：其余字段恒 null
		if o.ParentID != nil || o.ParentName != nil || o.Status != nil || o.Ancestors != nil {
			t.Errorf("其余字段应为 null: %#v", o)
		}
		if o.Children == nil || len(o.Children) != 0 {
			t.Errorf("children 恒为 []")
		}
	})
}

// TestDescendantAndSelfIDsAppendsSelfLast 自身 ID 恒追加在查询结果之后。
func TestDescendantAndSelfIDsAppendsSelfLast(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{descendantIDs: func(_ context.Context, deptID int64) ([]int64, error) {
		if deptID != 100 {
			t.Errorf("deptID = %d, want 100", deptID)
		}
		return []int64{101, 102}, nil
	}}
	ids, err := svc.DescendantAndSelfIDs(context.Background(), i64p(100))
	if err != nil || len(ids) != 3 || ids[0] != 101 || ids[1] != 102 || ids[2] != 100 {
		t.Fatalf("ids = %v, err = %v; want [101 102 100]", ids, err)
	}

	t.Run("查询失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{descendantIDs: func(_ context.Context, _ int64) ([]int64, error) { return nil, boom }}
		if _, err := svc.DescendantAndSelfIDs(context.Background(), i64p(100)); !errors.Is(err, boom) {
			t.Errorf("err = %v, want 原始错误", err)
		}
	})
}

// TestBuildConditionsBelongAndTime belongDeptId 与时间范围的条件拼装。
func TestBuildConditionsBelongAndTime(t *testing.T) {
	svc := NewService(nil, testAuth(1))
	var gotConds []crudx.Cond
	svc.repo = &mockRepo{
		descendantIDs: func(_ context.Context, deptID int64) ([]int64, error) {
			if deptID != 100 {
				t.Errorf("deptID = %d, want 100", deptID)
			}
			return []int64{101}, nil
		},
		list: func(_ context.Context, conds ...crudx.Cond) ([]SysDept, error) {
			gotConds = conds
			return []SysDept{}, nil
		},
	}
	q := QueryParam{
		ID: i64p(101), ParentID: i64p(100), DeptCategory: "A",
		BeginTime: "2026-01-01", EndTime: "2026-01-31", BelongDeptID: i64p(100),
	}
	if _, err := svc.TreeSelectOut(context.Background(), q); err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"id = ?", "parent_id = ?", "dept_category LIKE ?", "create_time BETWEEN ? AND ?", "id IN ?"}
	got := condSQLs(gotConds)
	if len(got) != len(want) {
		t.Fatalf("conds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("conds[%d] = %s, want %s（声明序）", i, got[i], want[i])
		}
	}
	// belongDeptId → 子孙 + 自身
	inArgs := gotConds[4].Args[0].([]int64)
	if len(inArgs) != 2 || inArgs[0] != 101 || inArgs[1] != 100 {
		t.Errorf("belong In 参数 = %v, want [101 100]", inArgs)
	}

	t.Run("仅beginTime_不施加范围", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		var conds []crudx.Cond
		svc.repo = &mockRepo{list: func(_ context.Context, cs ...crudx.Cond) ([]SysDept, error) {
			conds = cs
			return []SysDept{}, nil
		}}
		if _, err := svc.TreeSelectOut(context.Background(), QueryParam{BeginTime: "2026-01-01"}); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(conds) != 0 {
			t.Errorf("conds = %v, want 空（begin/end 必须同时存在）", condSQLs(conds))
		}
	})
}

// ---- 事务假驱动：只记录 Begin/Commit/Rollback ----
//
// dept 的数据访问全经 mock repo，service 不直接查 s.db，故假驱动无需伪造查询，
// 只需支持事务边界并记录事件，供「多写失败整体回滚」断言（对齐 Python session.begin）。

type txRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *txRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func (r *txRecorder) record(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

type txFakeConn struct{ r *txRecorder }

func (c *txFakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare")
}
func (c *txFakeConn) Close() error { return nil }
func (c *txFakeConn) Begin() (driver.Tx, error) {
	c.r.record("begin")
	return &txFakeTx{r: c.r}, nil
}

type txFakeTx struct{ r *txRecorder }

func (tx *txFakeTx) Commit() error   { tx.r.record("commit"); return nil }
func (tx *txFakeTx) Rollback() error { tx.r.record("rollback"); return nil }

type txFakeConnector struct{ r *txRecorder }

func (c txFakeConnector) Connect(context.Context) (driver.Conn, error) {
	return &txFakeConn{r: c.r}, nil
}
func (c txFakeConnector) Driver() driver.Driver { return txFakeDriver{} }

type txFakeDriver struct{}

func (txFakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use sql.OpenDB") }

func newTxDB(t *testing.T, r *txRecorder) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sql.OpenDB(txFakeConnector{r: r}),
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		t.Fatalf("构造事务假 DB 失败: %v", err)
	}
	return db
}

func wantTxEvents(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("事务事件 = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("事务事件[%d] = %q, want %q（完整: %v）", i, got[i], want[i], got)
		}
	}
}

// TestUpdateRollbackOnChildFailure 换父部门时子孙 ancestors 联动更新失败 →
// 事务整体 Rollback、绝不 Commit，不留半截祖级链（对齐 Python session.begin）。
func TestUpdateRollbackOnChildFailure(t *testing.T) {
	rec := &txRecorder{}
	db := newTxDB(t, rec)
	old := deptRowAnc(101, 100, "0,100", "旧", 1, "0")
	newParent := deptRowAnc(200, 0, "0", "新父", 0, "0")
	child := deptRowAnc(102, 101, "0,100,101", "子", 1, "0")
	var selfUpdated bool
	svc := NewService(db, testAuth(1)) // 超管：CheckDataScope 短路
	svc.repo = &mockRepo{
		get: func(_ context.Context, id int64) (*SysDept, error) {
			if id == 101 {
				return &old, nil
			}
			return &newParent, nil
		},
		listDescendants: func(_ context.Context, _ int64) ([]SysDept, error) { return []SysDept{child}, nil },
		update: func(ctx context.Context, row *SysDept) error {
			if crudx.DB(ctx, nil) == nil {
				t.Error("子孙/自身更新未拿到事务句柄：WithTx 传播失效")
			}
			if row.ID == 102 {
				return errors.New("boom") // 子孙更新失败
			}
			selfUpdated = true
			return nil
		},
	}
	err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ParentID: i64p(200)}, ID: i64p(101)})
	se := errs.As(err)
	if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
		t.Fatalf("err = %v, want 500 %s (detail=boom)", err, errs.MsgDBFailed)
	}
	if selfUpdated {
		t.Error("子孙失败应短路自身保存")
	}
	wantTxEvents(t, rec.snapshot(), []string{"begin", "rollback"})
}

// TestUpdateTxCommitOnSuccess 对照：仅改名（无联动）成功 → begin → commit。
func TestUpdateTxCommitOnSuccess(t *testing.T) {
	rec := &txRecorder{}
	db := newTxDB(t, rec)
	old := deptRowAnc(101, 100, "0,100", "旧", 1, "1")
	svc := NewService(db, testAuth(1))
	svc.repo = &mockRepo{
		get: func(_ context.Context, _ int64) (*SysDept, error) { return &old, nil },
		update: func(ctx context.Context, _ *SysDept) error {
			if crudx.DB(ctx, nil) == nil {
				t.Error("更新未拿到事务句柄：WithTx 传播失效")
			}
			return nil
		},
	}
	name := "改名"
	if err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{DeptName: &name}, ID: i64p(101)}); err != nil {
		t.Fatalf("err = %v", err)
	}
	wantTxEvents(t, rec.snapshot(), []string{"begin", "commit"})
}
