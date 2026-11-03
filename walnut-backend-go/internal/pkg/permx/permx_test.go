package permx

import (
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/errs"
)

func sp(s string) *string { return &s }
func ip(i int64) *int64   { return &i }

// auth 构造一个普通用户（非超管）的认证上下文。
func auth(userID int64, deptID *int64) authctx.Context {
	return authctx.Context{
		User: authctx.User{
			ID:          userID,
			UserName:    "tester",
			DeptID:      deptID,
			IsSuperuser: userID == 1,
		},
	}
}

func scopes(pairs ...any) []RoleScope {
	out := make([]RoleScope, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		rid := pairs[i].(int64)
		var ds *string
		if s, ok := pairs[i+1].(string); ok {
			ds = sp(s)
		}
		out = append(out, RoleScope{RoleID: rid, DataScope: ds})
	}
	return out
}

// ---- buildCondition：纯函数，穷举全部数据范围与 fail-closed 分支 ----

func TestBuildCondition(t *testing.T) {
	const deptChildSQL = "create_dept IN (SELECT id FROM sys_dept WHERE del_flag = ? " +
		"AND (id = ? OR FIND_IN_SET(?, ancestors) != 0))"

	tests := []struct {
		name     string
		auth     authctx.Context
		opt      Option
		scopes   []RoleScope
		noFilter bool
		sql      string
		args     []any
		wantErr  string // 期望的面向用户文案（契约的一部分）
	}{
		{
			name:     "超级管理员不过滤",
			auth:     auth(1, ip(100)),
			opt:      Default,
			scopes:   scopes(int64(1), "5"),
			noFilter: true,
		},
		{
			name:     "用户ID为0不过滤（防御性分支）",
			auth:     auth(0, nil),
			opt:      Default,
			scopes:   scopes(int64(2), "5"),
			noFilter: true,
		},
		{
			name:   "无角色退化为仅本人（fail-closed）",
			auth:   auth(7, ip(100)),
			opt:    Default,
			scopes: nil,
			sql:    "create_by = ?",
			args:   []any{int64(7)},
		},
		{
			name:   "无角色且模型无create_by则恒假（fail-closed）",
			auth:   auth(7, ip(100)),
			opt:    Option{DeptColumn: "create_dept"},
			scopes: nil,
			sql:    "1 = 0",
			args:   nil,
		},
		{
			name:     "范围1全部→整体放行，且忽略其余角色",
			auth:     auth(7, ip(100)),
			opt:      Default,
			scopes:   scopes(int64(2), "5", int64(3), "1"),
			noFilter: true,
		},
		{
			name:   "范围2自定义→角色授权部门子查询",
			auth:   auth(7, ip(100)),
			opt:    Default,
			scopes: scopes(int64(2), "2"),
			sql:    "create_dept IN (SELECT dept_id FROM sys_role_dept WHERE role_id = ?)",
			args:   []any{int64(2)},
		},
		{
			name:   "范围2但模型无部门列→不产生条件→兜底仅本人",
			auth:   auth(7, ip(100)),
			opt:    Option{CreateByColumn: "create_by"},
			scopes: scopes(int64(2), "2"),
			sql:    "create_by = ?",
			args:   []any{int64(7)},
		},
		{
			name:   "范围3本部门",
			auth:   auth(7, ip(100)),
			opt:    Default,
			scopes: scopes(int64(2), "3"),
			sql:    "create_dept = ?",
			args:   []any{int64(100)},
		},
		{
			name:   "范围3但用户无部门→用-1保证查不到",
			auth:   auth(7, nil),
			opt:    Default,
			scopes: scopes(int64(2), "3"),
			sql:    "create_dept = ?",
			args:   []any{int64(-1)},
		},
		{
			name:   "范围4本部门及以下",
			auth:   auth(7, ip(100)),
			opt:    Default,
			scopes: scopes(int64(2), "4"),
			sql:    "(" + deptChildSQL + ")",
			args:   []any{"0", int64(100), "100"},
		},
		{
			name:   "范围4但用户无部门→恒假",
			auth:   auth(7, nil),
			opt:    Default,
			scopes: scopes(int64(2), "4"),
			sql:    "(1 = 0)",
			args:   nil,
		},
		{
			name:   "范围4但模型无部门列→恒假",
			auth:   auth(7, ip(100)),
			opt:    Option{CreateByColumn: "create_by"},
			scopes: scopes(int64(2), "4"),
			sql:    "(1 = 0)",
			args:   nil,
		},
		{
			name:   "范围5仅本人",
			auth:   auth(7, ip(100)),
			opt:    Default,
			scopes: scopes(int64(2), "5"),
			sql:    "create_by = ?",
			args:   []any{int64(7)},
		},
		{
			name:   "范围6本部门及以下或本人",
			auth:   auth(7, ip(100)),
			opt:    Default,
			scopes: scopes(int64(2), "6"),
			sql:    "(" + deptChildSQL + ") OR create_by = ?",
			args:   []any{"0", int64(100), "100", int64(7)},
		},
		{
			name:   "多角色以OR连接，参数顺序与占位符一致",
			auth:   auth(7, ip(100)),
			opt:    Default,
			scopes: scopes(int64(2), "5", int64(3), "3"),
			sql:    "create_by = ? OR create_dept = ?",
			args:   []any{int64(7), int64(100)},
		},
		{
			name:    "data_scope取值非法→契约文案",
			auth:    auth(7, ip(100)),
			opt:     Default,
			scopes:  scopes(int64(2), "9"),
			wantErr: "角色数据范围异常 => 9",
		},
		{
			name:    "data_scope为NULL→Python格式化成字面量None",
			auth:    auth(7, ip(100)),
			opt:     Default,
			scopes:  []RoleScope{{RoleID: 2, DataScope: nil}},
			wantErr: "角色数据范围异常 => None",
		},
		{
			name:   "UserList配置用dept_id而非create_dept",
			auth:   auth(7, ip(100)),
			opt:    UserList,
			scopes: scopes(int64(2), "3"),
			sql:    "dept_id = ?",
			args:   []any{int64(100)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildCondition(tt.auth, tt.opt, tt.scopes)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("期望报错 %q，实际无错误（这是安全回归：非法 data_scope 被静默放行）", tt.wantErr)
				}
				se := errs.As(err)
				if se == nil {
					t.Fatalf("期望 *errs.ServiceError，实际 %T: %v", err, err)
				}
				if se.Msg != tt.wantErr {
					t.Errorf("文案不一致\n got: %q\nwant: %q", se.Msg, tt.wantErr)
				}
				if se.Code != errs.CodeError {
					t.Errorf("业务码 got=%d want=%d", se.Code, errs.CodeError)
				}
				// 出错时必须 denied，否则 Apply 会放行
				if !got.denied {
					t.Error("出错时 Scope 未标记 denied，Apply 会退化成查全表")
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			if got.denied {
				t.Fatal("不应标记 denied")
			}
			if got.NoFilter != tt.noFilter {
				t.Errorf("NoFilter got=%v want=%v", got.NoFilter, tt.noFilter)
			}
			if got.SQL != tt.sql {
				t.Errorf("SQL\n got: %q\nwant: %q", got.SQL, tt.sql)
			}
			if len(got.Args) != len(tt.args) {
				t.Fatalf("参数个数 got=%d want=%d（got=%v）", len(got.Args), len(tt.args), got.Args)
			}
			for i := range tt.args {
				if got.Args[i] != tt.args[i] {
					t.Errorf("参数[%d] got=%#v want=%#v", i, got.Args[i], tt.args[i])
				}
			}
			// 占位符个数必须与参数个数一致，否则 GORM 会错位绑定
			if n := strings.Count(got.SQL, "?"); n != len(got.Args) {
				t.Errorf("占位符 %d 个但参数 %d 个，SQL=%q", n, len(got.Args), got.SQL)
			}
		})
	}
}

// ---- Apply：fail-closed 的最后一道防线 ----

// dryDB 构造一个不连库、只生成 SQL 的 *gorm.DB。
//
// 两个开关缺一不可：
//   - SkipInitializeWithVersion：mysql dialector 不去查服务端版本（否则需要真连接）
//   - DisableAutomaticPing：gorm.Open 默认会对 ConnPool 调 Ping，
//     go-sql-driver 的 sql.Open 本身是惰性的，正是这个 Ping 触发了真实拨号
//
// 加上 DryRun 后，Find 只构建语句不执行，故整个测试不触网、可进 CI 快速门禁。
func dryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		SkipInitializeWithVersion: true,
		DSN:                       "u:p@tcp(127.0.0.1:1)/d",
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               logger.Discard,
	})
	if err != nil {
		t.Fatalf("构造 DryRun DB 失败: %v", err)
	}
	return db
}

type fakeRow struct {
	ID int64 `gorm:"column:id"`
}

func (fakeRow) TableName() string { return "t_demo" }

func sqlOf(t *testing.T, s Scope) string {
	t.Helper()
	stmt := s.Apply(dryDB(t)).Find(&[]fakeRow{}).Statement
	return stmt.SQL.String()
}

func TestApply(t *testing.T) {
	t.Run("denied时注入恒假条件而非放行", func(t *testing.T) {
		sql := sqlOf(t, Scope{denied: true})
		if !strings.Contains(sql, "1 = 0") {
			t.Fatalf("denied 未产生恒假条件，等于越权放行。SQL=%s", sql)
		}
	})

	t.Run("denied优先于NoFilter", func(t *testing.T) {
		// 即使误设了 NoFilter，denied 也必须赢
		sql := sqlOf(t, Scope{denied: true, NoFilter: true})
		if !strings.Contains(sql, "1 = 0") {
			t.Fatalf("denied 未优先于 NoFilter。SQL=%s", sql)
		}
	})

	t.Run("NoFilter不加条件", func(t *testing.T) {
		sql := sqlOf(t, Scope{NoFilter: true})
		if strings.Contains(sql, "WHERE") {
			t.Fatalf("NoFilter 却生成了 WHERE。SQL=%s", sql)
		}
	})

	t.Run("正常条件带参数绑定", func(t *testing.T) {
		sql := sqlOf(t, Scope{SQL: "create_by = ?", Args: []any{int64(7)}})
		if !strings.Contains(sql, "create_by = ?") {
			t.Fatalf("条件未生效。SQL=%s", sql)
		}
		// 值必须走绑定参数，不能被拼进 SQL 文本
		if strings.Contains(sql, "7") {
			t.Fatalf("参数被拼接进 SQL，存在注入风险。SQL=%s", sql)
		}
	})
}

// TestScopeSQLHasNoUserInput 是注入防线的静态断言：
// 生成的 SQL 里只允许出现 Option 提供的列名与固定字面量，
// 不允许出现任何来自认证上下文的值（用户ID、部门ID、角色ID 都必须是 ?）。
func TestScopeSQLHasNoUserInput(t *testing.T) {
	a := auth(7, ip(100))
	cases := []Option{Default, UserList, {CreateByColumn: "create_by"}}
	for _, opt := range cases {
		for _, code := range []string{"2", "3", "4", "5", "6"} {
			got, err := buildCondition(a, opt, scopes(int64(2), code))
			if err != nil {
				t.Fatalf("opt=%+v code=%s: %v", opt, code, err)
			}
			for _, forbidden := range []string{"7", "100"} {
				// 列名里不含数字，故出现即为拼接了值
				if strings.Contains(got.SQL, forbidden) && !strings.Contains(got.SQL, "1 = 0") {
					t.Errorf("opt=%+v code=%s: SQL 中出现字面量 %q，疑似拼接了请求数据: %s",
						opt, code, forbidden, got.SQL)
				}
			}
		}
	}
}
