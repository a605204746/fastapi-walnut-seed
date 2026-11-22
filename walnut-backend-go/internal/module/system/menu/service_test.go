package menu

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// mockRepo 嵌入 menuRepo（nil），仅以函数字段覆盖被测方法；
// 未覆盖的方法被调用即 panic，暴露测试外的意外依赖。
type mockRepo struct {
	menuRepo
	existsMenuName          func(ctx context.Context, name string, parentID, excludeID *int64) (bool, error)
	selectMenuByID          func(ctx context.Context, id int64) (*SysMenu, error)
	selectMenuTreeByUserID  func(ctx context.Context, ac authctx.Context) ([]SysMenu, error)
	selectMenuList          func(ctx context.Context, ac authctx.Context, q Query) ([]SysMenu, error)
	menuIDsByRoleID         func(ctx context.Context, roleID int64) ([]int64, error)
	routeConflictCandidates func(ctx context.Context, path, routeName string) ([]SysMenu, error)
	create                  func(ctx context.Context, row *SysMenu) error
	update                  func(ctx context.Context, row *SysMenu) error
	del                     func(ctx context.Context, id int64) (bool, error)
	deleteBatch             func(ctx context.Context, ids []int64) (int64, error)
	deleteRoleMenuByMenuIDs func(ctx context.Context, ids []int64) (int64, error)
	hasChildByMenuID        func(ctx context.Context, id int64) (bool, error)
	hasChildByMenuIDs       func(ctx context.Context, ids []int64) (bool, error)
	checkMenuExistRole      func(ctx context.Context, id int64) (bool, error)
}

func (m *mockRepo) ExistsMenuName(ctx context.Context, name string, parentID, excludeID *int64) (bool, error) {
	return m.existsMenuName(ctx, name, parentID, excludeID)
}

func (m *mockRepo) SelectMenuByID(ctx context.Context, id int64) (*SysMenu, error) {
	return m.selectMenuByID(ctx, id)
}

func (m *mockRepo) SelectMenuTreeByUserID(ctx context.Context, ac authctx.Context) ([]SysMenu, error) {
	return m.selectMenuTreeByUserID(ctx, ac)
}

func (m *mockRepo) SelectMenuList(ctx context.Context, ac authctx.Context, q Query) ([]SysMenu, error) {
	return m.selectMenuList(ctx, ac, q)
}

func (m *mockRepo) MenuIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	return m.menuIDsByRoleID(ctx, roleID)
}

func (m *mockRepo) RouteConflictCandidates(ctx context.Context, path, routeName string) ([]SysMenu, error) {
	return m.routeConflictCandidates(ctx, path, routeName)
}

func (m *mockRepo) Create(ctx context.Context, row *SysMenu) error {
	return m.create(ctx, row)
}

func (m *mockRepo) Update(ctx context.Context, row *SysMenu) error {
	return m.update(ctx, row)
}

func (m *mockRepo) Delete(ctx context.Context, id int64) (bool, error) {
	return m.del(ctx, id)
}

func (m *mockRepo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	return m.deleteBatch(ctx, ids)
}

func (m *mockRepo) DeleteRoleMenuByMenuIDs(ctx context.Context, ids []int64) (int64, error) {
	return m.deleteRoleMenuByMenuIDs(ctx, ids)
}

func (m *mockRepo) HasChildByMenuID(ctx context.Context, id int64) (bool, error) {
	return m.hasChildByMenuID(ctx, id)
}

func (m *mockRepo) HasChildByMenuIDs(ctx context.Context, ids []int64) (bool, error) {
	return m.hasChildByMenuIDs(ctx, ids)
}

func (m *mockRepo) CheckMenuExistRole(ctx context.Context, id int64) (bool, error) {
	return m.checkMenuExistRole(ctx, id)
}

func testAuth(id int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: id, UserName: "tester"}}
}

func TestCheckNameUniqueDuplicate(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{existsMenuName: func(_ context.Context, name string, _, _ *int64) (bool, error) {
		if name != "系统管理" {
			t.Errorf("name = %s, want 系统管理", name)
		}
		return true, nil
	}}
	mn := "系统管理"
	err := svc.CheckNameUnique(context.Background(), CreateIn{MenuName: &mn})
	if se := errs.As(err); se == nil || se.Msg != "菜单名称已存在" {
		t.Errorf("err = %v, want 菜单名称已存在", err)
	}
}

func TestCheckRouteConfigUniqueButtonSkips(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	// 按钮(F)直接放行，不触达 repo（触达即 panic）
	mt := TypeButton
	if err := svc.CheckRouteConfigUnique(context.Background(), CreateIn{MenuType: &mt}); err != nil {
		t.Errorf("按钮应直接放行, got %v", err)
	}
}

func TestUpdateMissingRow(t *testing.T) {
	svc := NewService(nil, testAuth(2))
	svc.repo = &mockRepo{selectMenuByID: func(_ context.Context, _ int64) (*SysMenu, error) {
		return nil, nil
	}}
	id := int64(5)
	n, err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ID: &id}})
	if n != 0 || err != nil {
		t.Errorf("n = %d, err = %v; want 0, nil（记录不存在）", n, err)
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

// menuRow 构造最小菜单实体：isFrame 默认 '1'（非外链）。
func menuRow(id int64, parent *int64, name, path, typ string) SysMenu {
	m := SysMenu{
		MenuName: name,
		ParentID: parent,
		Path:     strPtr(path),
		MenuType: strPtr(typ),
		IsFrame:  strPtr(NoFrame),
	}
	m.ID = id
	return m
}

func dstr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// TestGetRoutersTreeAndAuth getRouters 全链路：auth 透传 → 树构建 → 路由形态。
func TestGetRoutersTreeAndAuth(t *testing.T) {
	t.Run("目录带子菜单_路由树形态", func(t *testing.T) {
		ac := authctx.Context{User: authctx.User{ID: 2, UserName: "tester"}, MenuIDs: []int64{1, 2}}
		svc := NewService(nil, ac)
		dir := menuRow(1, i64p(0), "系统管理", "system", TypeDir)
		dir.Icon = strPtr("system")
		child := menuRow(2, i64p(1), "用户管理", "user", TypeMenu)
		child.Component = strPtr("system/user/index")
		child.IsCache = strPtr("1") // 不缓存
		child.Visible = strPtr("1") // 隐藏
		child.Remark = strPtr("/system/other")
		svc.repo = &mockRepo{
			selectMenuTreeByUserID: func(_ context.Context, got authctx.Context) ([]SysMenu, error) {
				// 权限过滤在 repo 侧按会话 MenuIDs 收敛，service 必须原样透传 auth
				if got.User.ID != 2 || len(got.MenuIDs) != 2 || got.MenuIDs[0] != 1 || got.MenuIDs[1] != 2 {
					t.Errorf("auth 透传错误: %#v", got)
				}
				return []SysMenu{dir, child}, nil
			},
		}
		routers, err := svc.GetRouters(context.Background())
		if err != nil || len(routers) != 1 {
			t.Fatalf("routers = %d, err = %v", len(routers), err)
		}
		r := routers[0]
		if dstr(r.Name) != "System1" || dstr(r.Path) != "/system" || dstr(r.Component) != "Layout" {
			t.Errorf("顶级目录路由 = %#v, want System1 //system Layout", r)
		}
		if r.AlwaysShow == nil || !*r.AlwaysShow || dstr(r.Redirect) != "noRedirect" {
			t.Errorf("目录应 alwaysShow + noRedirect: %#v", r)
		}
		if r.Hidden == nil || *r.Hidden {
			t.Errorf("visible=0 应 hidden=false")
		}
		if r.Meta == nil || dstr(r.Meta.Title) != "系统管理" || dstr(r.Meta.Icon) != "system" ||
			r.Meta.NoCache == nil || *r.Meta.NoCache {
			t.Errorf("目录 meta = %#v", r.Meta)
		}
		if len(r.Children) != 1 {
			t.Fatalf("children = %d, want 1", len(r.Children))
		}
		c := r.Children[0]
		if dstr(c.Name) != "User2" || dstr(c.Path) != "user" || dstr(c.Component) != "system/user/index" {
			t.Errorf("子路由 = %#v", c)
		}
		if c.Hidden == nil || !*c.Hidden {
			t.Errorf("visible=1 应 hidden=true")
		}
		if c.Meta == nil || c.Meta.NoCache == nil || !*c.Meta.NoCache ||
			dstr(c.Meta.ActiveMenu) != "/system/other" || c.Meta.Link != nil {
			t.Errorf("子路由 meta = %#v（noCache=true, activeMenu=备注）", c.Meta)
		}
		if c.AlwaysShow != nil || c.Redirect != nil || len(c.Children) != 0 {
			t.Errorf("叶子不应带 alwaysShow/redirect/children: %#v", c)
		}
	})

	t.Run("无授权菜单_空数组而非null", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{selectMenuTreeByUserID: func(_ context.Context, _ authctx.Context) ([]SysMenu, error) {
			return nil, nil
		}}
		routers, err := svc.GetRouters(context.Background())
		if err != nil || routers == nil || len(routers) != 0 {
			t.Fatalf("routers = %#v, err = %v; want 非 nil 空数组", routers, err)
		}
	})

	t.Run("超管auth透传", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{selectMenuTreeByUserID: func(_ context.Context, got authctx.Context) ([]SysMenu, error) {
			if !got.User.IsSuperAdmin() {
				t.Errorf("超管标识未透传: %#v", got.User)
			}
			return nil, nil
		}}
		if _, err := svc.GetRouters(context.Background()); err != nil {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("查询失败_包装查询菜单失败", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{selectMenuTreeByUserID: func(_ context.Context, _ authctx.Context) ([]SysMenu, error) {
			return nil, boom
		}}
		_, err := svc.GetRouters(context.Background())
		if err == nil || !strings.Contains(err.Error(), "查询菜单失败") || !errors.Is(err, boom) {
			t.Errorf("err = %v, want 查询菜单失败 且可 unwrap 到原始错误", err)
		}
	})
}

// TestBuildMenusSpecialBranches 路由构建的三个互斥分支 + NULL 父级丢弃。
func TestBuildMenusSpecialBranches(t *testing.T) {
	svc := NewService(nil, testAuth(1))

	t.Run("菜单内部跳转_外层无meta", func(t *testing.T) {
		m := menuRow(2, i64p(0), "用户管理", "user", TypeMenu)
		m.Component = strPtr("system/user/index")
		routers := svc.BuildMenus([]SysMenu{m})
		if len(routers) != 1 {
			t.Fatalf("routers = %d", len(routers))
		}
		r := routers[0]
		if r.Meta != nil {
			t.Errorf("menuFrame 外层 meta 必须置空: %#v", r.Meta)
		}
		// routeName 为空但 id 后缀仍拼接 → Name 只剩 "2"
		if dstr(r.Path) != "/" || dstr(r.Name) != "2" || dstr(r.Component) != "Layout" {
			t.Errorf("外层 = %#v, want path=/ name=2 component=Layout", r)
		}
		if len(r.Children) != 1 {
			t.Fatalf("children = %d, want 1", len(r.Children))
		}
		c := r.Children[0]
		if dstr(c.Path) != "user" || dstr(c.Component) != "system/user/index" || dstr(c.Name) != "User2" {
			t.Errorf("真实页面子路由 = %#v", c)
		}
		if c.Meta == nil || dstr(c.Meta.Title) != "用户管理" {
			t.Errorf("子路由应带完整 meta: %#v", c.Meta)
		}
	})

	t.Run("顶级内链_路径收敛与域名替换", func(t *testing.T) {
		// 顶级内链必须是目录(M)：顶级 C+非外链会先命中 menuFrame 分支
		m := menuRow(3, i64p(0), "外部文档", "http://www.example.com:8080/x", TypeDir)
		m.Icon = strPtr("guide")
		routers := svc.BuildMenus([]SysMenu{m})
		if len(routers) != 1 {
			t.Fatalf("routers = %d", len(routers))
		}
		r := routers[0]
		if dstr(r.Path) != "/" || r.Meta == nil || dstr(r.Meta.Title) != "外部文档" || dstr(r.Meta.Icon) != "guide" {
			t.Errorf("顶级内链外层 = %#v", r)
		}
		if len(r.Children) != 1 {
			t.Fatalf("children = %d, want 1", len(r.Children))
		}
		c := r.Children[0]
		// 替换顺序契约：剥 http:// → 剥 www. → '.'→'/' → ':'→'/'
		if dstr(c.Path) != "example/com/8080/x" || dstr(c.Component) != InnerLink ||
			dstr(c.Name) != "Example/com/8080/x3" {
			t.Errorf("内链子路由 = %#v, want example/com/8080/x + InnerLink", c)
		}
		if c.Meta == nil || dstr(c.Meta.Link) != "http://www.example.com:8080/x" {
			t.Errorf("内链 meta.link 应为原始地址: %#v", c.Meta)
		}
	})

	t.Run("非顶级内链_路径改写与组件", func(t *testing.T) {
		// buildTree 从顶级 0 递归：非顶级菜单必须带上其父节点才会进树
		parent := menuRow(1, i64p(0), "父目录", "parent", TypeDir)
		m := menuRow(4, i64p(1), "若依", "https://www.ruoyi.vip", TypeMenu)
		routers := svc.BuildMenus([]SysMenu{parent, m})
		if len(routers) != 1 || len(routers[0].Children) != 1 {
			t.Fatalf("routers = %#v, want 父目录带 1 个子路由", routers)
		}
		r := routers[0].Children[0]
		if dstr(r.Path) != "ruoyi/vip" || dstr(r.Component) != InnerLink {
			t.Errorf("非顶级内链 = %#v, want ruoyi/vip + InnerLink", r)
		}
		// routeName 读原始 path（首字母大写），不是改写后的
		if dstr(r.Name) != "Https://www.ruoyi.vip4" {
			t.Errorf("Name = %s, want Https://www.ruoyi.vip4", dstr(r.Name))
		}
		if r.Meta == nil || dstr(r.Meta.Link) != "https://www.ruoyi.vip" {
			t.Errorf("meta.link = %#v", r.Meta)
		}
		if len(r.Children) != 0 {
			t.Errorf("非顶级内链不应有 children")
		}
	})

	t.Run("非顶级目录_ParentView组件", func(t *testing.T) {
		parent := menuRow(1, i64p(0), "父目录", "parent", TypeDir)
		m := menuRow(5, i64p(1), "嵌套目录", "nested", TypeDir)
		routers := svc.BuildMenus([]SysMenu{parent, m})
		if len(routers) != 1 || len(routers[0].Children) != 1 {
			t.Fatalf("routers = %#v, want 父目录带 1 个子路由", routers)
		}
		child := routers[0].Children[0]
		if dstr(child.Component) != ParentView {
			t.Fatalf("child = %#v, want ParentView", child)
		}
		if child.AlwaysShow != nil {
			t.Errorf("无子节点的目录不应 alwaysShow")
		}
	})

	t.Run("NULL父级_必须丢弃", func(t *testing.T) {
		ghost := menuRow(6, nil, "幽灵", "ghost", TypeDir)
		routers := svc.BuildMenus([]SysMenu{ghost})
		if len(routers) != 0 {
			t.Fatalf("NULL 父级菜单必须丢弃, got %#v", routers)
		}
	})
}

// TestTreeSelectOutAndRootRule 菜单下拉树：根取第一行 parent_id（含 NULL）。
func TestTreeSelectOutAndRootRule(t *testing.T) {
	t.Run("根为0_树组装与字段透传", func(t *testing.T) {
		ac := authctx.Context{User: authctx.User{ID: 2}, MenuIDs: []int64{1, 2, 3}}
		svc := NewService(nil, ac)
		root := menuRow(1, i64p(0), "系统管理", "system", TypeDir)
		root.OrderNum = func() *int { v := 1; return &v }()
		root.Icon = strPtr("system")
		root.Visible = strPtr("0")
		root.Status = strPtr("0")
		child := menuRow(2, i64p(1), "用户管理", "user", TypeMenu)
		other := menuRow(3, i64p(0), "日志管理", "log", TypeDir)
		svc.repo = &mockRepo{selectMenuList: func(_ context.Context, got authctx.Context, q Query) ([]SysMenu, error) {
			if got.User.ID != 2 || len(got.MenuIDs) != 3 {
				t.Errorf("auth 透传错误: %#v", got)
			}
			if q.MenuName != "管理" {
				t.Errorf("query 透传错误: %#v", q)
			}
			return []SysMenu{root, child, other}, nil
		}}
		tree, err := svc.TreeSelectOut(context.Background(), Query{MenuName: "管理"})
		if err != nil || len(tree) != 2 {
			t.Fatalf("tree = %d, err = %v; want 2 个根", len(tree), err)
		}
		if tree[0].ID.Int64() != 1 || tree[1].ID.Int64() != 3 {
			t.Errorf("根序 = %v/%v, want 1/3（保持 SQL 顺序，不排序）", tree[0].ID, tree[1].ID)
		}
		n := tree[0]
		if dstr(n.Label) != "系统管理" || n.Weight == nil || n.Weight.Int64() != 1 ||
			dstr(n.MenuType) != TypeDir || dstr(n.Icon) != "system" || dstr(n.Visible) != "0" || dstr(n.Status) != "0" {
			t.Errorf("节点字段透传错误: %#v", n)
		}
		if len(n.Children) != 1 || n.Children[0].ID.Int64() != 2 {
			t.Fatalf("children 组装错误: %#v", n.Children)
		}
		if n.Children[0].Children != nil || tree[1].Children != nil {
			t.Errorf("叶子不应带 children 键")
		}
	})

	t.Run("第一行父级NULL_根键含NULL", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		rows := []SysMenu{
			menuRow(5, nil, "甲", "a", TypeDir),
			menuRow(6, nil, "乙", "b", TypeDir),
			menuRow(7, i64p(5), "丙", "c", TypeMenu),
		}
		svc.repo = &mockRepo{selectMenuList: func(_ context.Context, _ authctx.Context, _ Query) ([]SysMenu, error) {
			return rows, nil
		}}
		tree, err := svc.TreeSelectOut(context.Background(), Query{})
		if err != nil || len(tree) != 2 {
			t.Fatalf("tree = %d, err = %v; want 2 个 NULL 根", len(tree), err)
		}
		if tree[0].ID.Int64() != 5 || tree[1].ID.Int64() != 6 || tree[0].ParentID != nil {
			t.Errorf("NULL 根形态错误: %#v", tree)
		}
		if len(tree[0].Children) != 1 || tree[0].Children[0].ID.Int64() != 7 {
			t.Errorf("NULL 根下应挂 parent=5 的子节点: %#v", tree[0].Children)
		}
	})

	t.Run("空列表_空数组", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{selectMenuList: func(_ context.Context, _ authctx.Context, _ Query) ([]SysMenu, error) {
			return nil, nil
		}}
		tree, err := svc.TreeSelectOut(context.Background(), Query{})
		if err != nil || tree == nil || len(tree) != 0 {
			t.Fatalf("tree = %#v, err = %v; want 非 nil 空数组", tree, err)
		}
	})

	t.Run("查询失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{selectMenuList: func(_ context.Context, _ authctx.Context, _ Query) ([]SysMenu, error) {
			return nil, boom
		}}
		if _, err := svc.TreeSelectOut(context.Background(), Query{}); !errors.Is(err, boom) {
			t.Errorf("err = %v, want 原始错误", err)
		}
	})
}

// TestRoleMenuTreeSelect 角色菜单树：勾选键 + 全量树 + 调用序。
func TestRoleMenuTreeSelect(t *testing.T) {
	t.Run("勾选键与树_调用序", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		var seq []string
		svc.repo = &mockRepo{
			menuIDsByRoleID: func(_ context.Context, roleID int64) ([]int64, error) {
				seq = append(seq, "checked")
				if roleID != 9 {
					t.Errorf("roleID = %d, want 9", roleID)
				}
				return []int64{2, 3}, nil
			},
			selectMenuList: func(_ context.Context, _ authctx.Context, q Query) ([]SysMenu, error) {
				seq = append(seq, "list")
				if q != (Query{}) {
					t.Errorf("list 应使用零值 Query, got %#v", q)
				}
				return []SysMenu{menuRow(1, i64p(0), "根", "root", TypeDir)}, nil
			},
		}
		vo, err := svc.RoleMenuTreeSelect(context.Background(), 9)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(seq) != 2 || seq[0] != "checked" || seq[1] != "list" {
			t.Errorf("调用序 = %v, want [checked list]", seq)
		}
		if len(vo.CheckedKeys) != 2 || vo.CheckedKeys[0] != 2 || vo.CheckedKeys[1] != 3 {
			t.Errorf("CheckedKeys = %v, want [2 3]", vo.CheckedKeys)
		}
		if len(vo.Menus) != 1 || vo.Menus[0].ID.Int64() != 1 {
			t.Errorf("Menus = %#v", vo.Menus)
		}
	})

	t.Run("无勾选_空数组而非null", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{
			menuIDsByRoleID: func(_ context.Context, _ int64) ([]int64, error) { return nil, nil },
			selectMenuList: func(_ context.Context, _ authctx.Context, _ Query) ([]SysMenu, error) {
				return nil, nil
			},
		}
		vo, err := svc.RoleMenuTreeSelect(context.Background(), 9)
		if err != nil || vo.CheckedKeys == nil || len(vo.CheckedKeys) != 0 || vo.Menus == nil {
			t.Fatalf("vo = %#v, err = %v; want 空数组形态", vo, err)
		}
	})

	t.Run("勾选查询失败_短路列表", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{menuIDsByRoleID: func(_ context.Context, _ int64) ([]int64, error) {
			return nil, boom
		}}
		if _, err := svc.RoleMenuTreeSelect(context.Background(), 9); !errors.Is(err, boom) {
			t.Errorf("err = %v, want 原始错误", err)
		}
	})
}

// TestInsertMenuDefaultsAndOverride 新增菜单：列默认值兜底 + 显式覆盖 + 审计。
func TestInsertMenuDefaultsAndOverride(t *testing.T) {
	t.Run("缺省字段走列默认值", func(t *testing.T) {
		fixed := pinNow(t)
		svc := NewService(nil, testAuth(2))
		var got *SysMenu
		svc.repo = &mockRepo{create: func(_ context.Context, row *SysMenu) error {
			got = row
			return nil
		}}
		n, err := svc.Insert(context.Background(), CreateIn{MenuName: strPtr("测试")})
		if err != nil || n != 1 {
			t.Fatalf("n = %d, err = %v; want 1, nil", n, err)
		}
		if got.MenuName != "测试" || got.ParentID == nil || *got.ParentID != 0 ||
			got.OrderNum == nil || *got.OrderNum != 0 || dstr(got.Path) != "" ||
			dstr(got.IsFrame) != "1" || dstr(got.IsCache) != "0" || dstr(got.MenuType) != "" ||
			dstr(got.Visible) != "0" || dstr(got.Status) != "0" || dstr(got.Icon) != "#" || dstr(got.Remark) != "" {
			t.Errorf("列默认值错误: %#v", got)
		}
		if got.Component != nil || got.QueryParam != nil || got.Perms != nil {
			t.Errorf("component/queryParam/perms 缺省应为 NULL: %#v", got)
		}
		if got.ID == 0 || got.CreateBy == nil || *got.CreateBy != 2 ||
			got.CreateTime == nil || !got.CreateTime.Equal(fixed) || !got.UpdateTime.Equal(fixed) {
			t.Errorf("审计字段错误: %#v", got.BaseModel)
		}
	})

	t.Run("显式值全量覆盖", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		var got *SysMenu
		svc.repo = &mockRepo{create: func(_ context.Context, row *SysMenu) error {
			got = row
			return nil
		}}
		order := 3
		in := CreateIn{
			ParentID: i64p(5), MenuName: strPtr("菜单"), OrderNum: &order,
			Path: strPtr("x"), Component: strPtr("c"), QueryParam: strPtr("{}"),
			IsFrame: strPtr("0"), IsCache: strPtr("1"), MenuType: strPtr(TypeMenu),
			Visible: strPtr("1"), Status: strPtr("1"), Perms: strPtr("system:x:list"),
			Icon: strPtr("icon"), Remark: strPtr("r"),
		}
		if _, err := svc.Insert(context.Background(), in); err != nil {
			t.Fatalf("err = %v", err)
		}
		if *got.ParentID != 5 || *got.OrderNum != 3 || dstr(got.Path) != "x" || dstr(got.Component) != "c" ||
			dstr(got.QueryParam) != "{}" || dstr(got.IsFrame) != "0" || dstr(got.IsCache) != "1" ||
			dstr(got.MenuType) != TypeMenu || dstr(got.Visible) != "1" || dstr(got.Status) != "1" ||
			dstr(got.Perms) != "system:x:list" || dstr(got.Icon) != "icon" || dstr(got.Remark) != "r" {
			t.Errorf("显式值透传错误: %#v", got)
		}
	})

	t.Run("写入失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{create: func(_ context.Context, _ *SysMenu) error { return errors.New("boom") }}
		n, err := svc.Insert(context.Background(), CreateIn{MenuName: strPtr("x")})
		se := errs.As(err)
		if n != 0 || se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("n = %d, err = %v; want 0 + 500 %s", n, err, errs.MsgDBFailed)
		}
	})
}

// TestUpdateMenuPartialFields 修改菜单：仅覆盖非空字段，其余保留。
func TestUpdateMenuPartialFields(t *testing.T) {
	t.Run("部分字段更新_其余保留", func(t *testing.T) {
		fixed := pinNow(t)
		svc := NewService(nil, testAuth(2))
		row := menuRow(5, i64p(1), "旧名", "old", TypeMenu)
		row.Component = strPtr("c")
		row.IsFrame = strPtr("0")
		row.IsCache = strPtr("1")
		row.Visible = strPtr("1")
		row.Status = strPtr("0")
		row.Perms = strPtr("p")
		row.Icon = strPtr("i")
		row.Remark = strPtr("r")
		row.QueryParam = strPtr("{}")
		var got *SysMenu
		svc.repo = &mockRepo{
			selectMenuByID: func(_ context.Context, id int64) (*SysMenu, error) {
				if id != 5 {
					t.Errorf("id = %d, want 5", id)
				}
				return &row, nil
			},
			update: func(_ context.Context, r *SysMenu) error {
				got = r
				return nil
			},
		}
		n, err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{
			ID: i64p(5), MenuName: strPtr("新名"), Status: strPtr("1"),
		}})
		if err != nil || n != 1 {
			t.Fatalf("n = %d, err = %v; want 1, nil", n, err)
		}
		if got.MenuName != "新名" || dstr(got.Status) != "1" {
			t.Errorf("提供字段应更新: %#v", got)
		}
		if dstr(got.Path) != "old" || dstr(got.Component) != "c" || dstr(got.IsFrame) != "0" ||
			dstr(got.IsCache) != "1" || dstr(got.Visible) != "1" || dstr(got.Perms) != "p" ||
			dstr(got.Icon) != "i" || dstr(got.Remark) != "r" || dstr(got.QueryParam) != "{}" || *got.ParentID != 1 {
			t.Errorf("未提供字段必须保留: %#v", got)
		}
		if got.UpdateBy == nil || *got.UpdateBy != 2 || got.UpdateTime == nil || !got.UpdateTime.Equal(fixed) {
			t.Errorf("审计字段错误: %#v", got.BaseModel)
		}
	})

	t.Run("查询失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{selectMenuByID: func(_ context.Context, _ int64) (*SysMenu, error) { return nil, boom }}
		n, err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ID: i64p(5)}})
		if n != 0 || !errors.Is(err, boom) {
			t.Errorf("n = %d, err = %v; want 0 + 原始错误", n, err)
		}
	})

	t.Run("保存失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		row := menuRow(5, i64p(0), "旧", "old", TypeMenu)
		svc.repo = &mockRepo{
			selectMenuByID: func(_ context.Context, _ int64) (*SysMenu, error) { return &row, nil },
			update:         func(_ context.Context, _ *SysMenu) error { return errors.New("boom") },
		}
		n, err := svc.Update(context.Background(), UpdateIn{CreateIn: CreateIn{ID: i64p(5)}})
		se := errs.As(err)
		if n != 0 || se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("n = %d, err = %v; want 0 + 500 %s", n, err, errs.MsgDBFailed)
		}
	})
}

// TestCheckRouteConfigUniqueConflicts 路由冲突三类判定 + 自身排除 + 大小写。
func TestCheckRouteConfigUniqueConflicts(t *testing.T) {
	cases := []struct {
		name          string
		in            CreateIn
		candidate     SysMenu
		wantDup       bool
		wantRouteName string // 传给 repo 的 routeName 参数（menuFrame 取原始 path）
	}{
		{
			name:          "同地址同父级_忽略大小写",
			in:            CreateIn{Path: strPtr("User"), ParentID: i64p(1), MenuType: strPtr(TypeMenu)},
			candidate:     menuRow(9, i64p(1), "候选", "user", TypeMenu),
			wantDup:       true,
			wantRouteName: "User",
		},
		{
			name:          "双顶级同地址",
			in:            CreateIn{Path: strPtr("user"), ParentID: i64p(0), MenuType: strPtr(TypeDir)},
			candidate:     menuRow(9, i64p(0), "候选", "USER", TypeDir),
			wantDup:       true,
			wantRouteName: "User",
		},
		{
			name:          "同路由名同类型_地址不同父级",
			in:            CreateIn{Path: strPtr("user"), ParentID: i64p(1), MenuType: strPtr(TypeMenu)},
			candidate:     menuRow(9, i64p(2), "候选", "User", TypeMenu),
			wantDup:       true,
			wantRouteName: "User",
		},
		{
			name:          "同路由名不同类型_放行",
			in:            CreateIn{Path: strPtr("user"), ParentID: i64p(1), MenuType: strPtr(TypeMenu)},
			candidate:     menuRow(9, i64p(2), "候选", "User", TypeDir),
			wantDup:       false,
			wantRouteName: "User",
		},
		{
			name:          "自身排除",
			in:            CreateIn{ID: i64p(9), Path: strPtr("user"), ParentID: i64p(1), MenuType: strPtr(TypeMenu)},
			candidate:     menuRow(9, i64p(1), "候选", "user", TypeMenu),
			wantDup:       false,
			wantRouteName: "User",
		},
		{
			// menuFrame 的 routeName 取原始 path（不大写）：差异体现在 repo 入参上
			// （冲突判定本身忽略大小写，capitalize 与否不影响 EqualFold 结果）
			name:          "menuFrame路由名取原始path",
			in:            CreateIn{Path: strPtr("user"), ParentID: i64p(0), MenuType: strPtr(TypeMenu), IsFrame: strPtr(NoFrame)},
			candidate:     menuRow(9, i64p(2), "候选", "User", TypeMenu),
			wantDup:       true,
			wantRouteName: "user",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil, testAuth(2))
			svc.repo = &mockRepo{routeConflictCandidates: func(_ context.Context, path, routeName string) ([]SysMenu, error) {
				if path != dstr(tc.in.Path) {
					t.Errorf("path 参数 = %s, want %s", path, dstr(tc.in.Path))
				}
				if routeName != tc.wantRouteName {
					t.Errorf("routeName 参数 = %s, want %s", routeName, tc.wantRouteName)
				}
				return []SysMenu{tc.candidate}, nil
			}}
			err := svc.CheckRouteConfigUnique(context.Background(), tc.in)
			if tc.wantDup {
				if !errors.Is(err, errDupRoute) {
					t.Fatalf("err = %v, want 路由名称或地址已存在", err)
				}
				wantErr(t, err, "路由名称或地址已存在")
			} else if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}

	t.Run("无候选_放行", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{routeConflictCandidates: func(_ context.Context, _, _ string) ([]SysMenu, error) {
			return nil, nil
		}}
		if err := svc.CheckRouteConfigUnique(context.Background(), CreateIn{Path: strPtr("x"), MenuType: strPtr(TypeMenu)}); err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("候选查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{routeConflictCandidates: func(_ context.Context, _, _ string) ([]SysMenu, error) {
			return nil, errors.New("boom")
		}}
		err := svc.CheckRouteConfigUnique(context.Background(), CreateIn{Path: strPtr("x"), MenuType: strPtr(TypeMenu)})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s", err, errs.MsgDBFailed)
		}
	})
}

// TestDeleteCascadeOrderAndErrors 级联删除：先删菜单再清角色关联，失败短路。
func TestDeleteCascadeOrderAndErrors(t *testing.T) {
	t.Run("成功_调用序与入参", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		var seq []string
		svc.repo = &mockRepo{
			deleteBatch: func(_ context.Context, ids []int64) (int64, error) {
				seq = append(seq, "batch")
				if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
					t.Errorf("ids = %v, want [1 2]", ids)
				}
				return 2, nil
			},
			deleteRoleMenuByMenuIDs: func(_ context.Context, ids []int64) (int64, error) {
				seq = append(seq, "rolemenu")
				if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
					t.Errorf("ids = %v, want [1 2]", ids)
				}
				return 3, nil
			},
		}
		if err := svc.DeleteCascade(context.Background(), []int64{1, 2}); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(seq) != 2 || seq[0] != "batch" || seq[1] != "rolemenu" {
			t.Errorf("调用序 = %v, want [batch rolemenu]", seq)
		}
	})

	t.Run("批量删除失败_短路关联清理", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{deleteBatch: func(_ context.Context, _ []int64) (int64, error) {
			return 0, errors.New("boom")
		}}
		err := svc.DeleteCascade(context.Background(), []int64{1})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s", err, errs.MsgDBFailed)
		}
	})

	t.Run("关联清理失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{
			deleteBatch:             func(_ context.Context, _ []int64) (int64, error) { return 1, nil },
			deleteRoleMenuByMenuIDs: func(_ context.Context, _ []int64) (int64, error) { return 0, errors.New("boom") },
		}
		se := errs.As(svc.DeleteCascade(context.Background(), []int64{1}))
		if se == nil || se.Msg != errs.MsgDBFailed {
			t.Errorf("want 500 %s", errs.MsgDBFailed)
		}
	})
}

// TestDeleteByIDAndGuards 单条删除结果语义 + 删除前置校验委托。
func TestDeleteByIDAndGuards(t *testing.T) {
	t.Run("删除成功与未命中", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{del: func(_ context.Context, id int64) (bool, error) {
			if id != 5 {
				t.Errorf("id = %d, want 5", id)
			}
			return true, nil
		}}
		if n, err := svc.DeleteByID(context.Background(), 5); err != nil || n != 1 {
			t.Fatalf("n = %d, err = %v; want 1, nil", n, err)
		}
		svc.repo = &mockRepo{del: func(_ context.Context, _ int64) (bool, error) { return false, nil }}
		if n, err := svc.DeleteByID(context.Background(), 999); err != nil || n != 0 {
			t.Fatalf("n = %d, err = %v; want 0, nil", n, err)
		}
	})

	t.Run("删除失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{del: func(_ context.Context, _ int64) (bool, error) { return false, errors.New("boom") }}
		n, err := svc.DeleteByID(context.Background(), 5)
		se := errs.As(err)
		if n != 0 || se == nil || se.Msg != errs.MsgDBFailed {
			t.Errorf("n = %d, err = %v; want 0 + 500 %s", n, err, errs.MsgDBFailed)
		}
	})

	t.Run("前置校验委托透传", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{
			hasChildByMenuID:   func(_ context.Context, id int64) (bool, error) { return id == 5, nil },
			hasChildByMenuIDs:  func(_ context.Context, ids []int64) (bool, error) { return len(ids) > 1, nil },
			checkMenuExistRole: func(_ context.Context, id int64) (bool, error) { return id == 7, nil },
		}
		if has, err := svc.HasChildByMenuID(context.Background(), 5); err != nil || !has {
			t.Errorf("HasChildByMenuID = %v, %v", has, err)
		}
		if has, err := svc.HasChildByMenuIDs(context.Background(), []int64{1, 2}); err != nil || !has {
			t.Errorf("HasChildByMenuIDs = %v, %v", has, err)
		}
		if assigned, err := svc.CheckMenuExistRole(context.Background(), 7); err != nil || !assigned {
			t.Errorf("CheckMenuExistRole = %v, %v", assigned, err)
		}
	})
}

// TestCheckNameUniquePassAndError 名称唯一性放行分支与错误包装。
func TestCheckNameUniquePassAndError(t *testing.T) {
	t.Run("无同名_放行且参数透传", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{existsMenuName: func(_ context.Context, name string, parentID, excludeID *int64) (bool, error) {
			if name != "用户管理" || parentID == nil || *parentID != 1 || excludeID == nil || *excludeID != 5 {
				t.Errorf("参数透传错误: %s %v %v", name, parentID, excludeID)
			}
			return false, nil
		}}
		err := svc.CheckNameUnique(context.Background(), CreateIn{
			MenuName: strPtr("用户管理"), ParentID: i64p(1), ID: i64p(5),
		})
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("查询失败_包装500", func(t *testing.T) {
		svc := NewService(nil, testAuth(2))
		svc.repo = &mockRepo{existsMenuName: func(_ context.Context, _ string, _, _ *int64) (bool, error) {
			return false, errors.New("boom")
		}}
		err := svc.CheckNameUnique(context.Background(), CreateIn{MenuName: strPtr("x")})
		se := errs.As(err)
		if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
			t.Errorf("err = %v, want 500 %s", err, errs.MsgDBFailed)
		}
	})
}

// TestListOutAndGetOut 列表/详情出参形态与错误传播。
func TestListOutAndGetOut(t *testing.T) {
	t.Run("列表_children恒为空数组", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		row := menuRow(1, i64p(0), "根", "root", TypeDir)
		svc.repo = &mockRepo{selectMenuList: func(_ context.Context, _ authctx.Context, _ Query) ([]SysMenu, error) {
			return []SysMenu{row}, nil
		}}
		outs, err := svc.ListOut(context.Background(), Query{})
		if err != nil || len(outs) != 1 {
			t.Fatalf("outs = %d, err = %v", len(outs), err)
		}
		if outs[0].ID == nil || outs[0].ID.Int64() != 1 || dstr(outs[0].MenuName) != "根" ||
			outs[0].Children == nil || len(outs[0].Children) != 0 {
			t.Errorf("出参形态错误: %#v", outs[0])
		}
	})

	t.Run("详情_存在与不存在", func(t *testing.T) {
		svc := NewService(nil, testAuth(1))
		row := menuRow(1, i64p(0), "根", "root", TypeDir)
		svc.repo = &mockRepo{selectMenuByID: func(_ context.Context, _ int64) (*SysMenu, error) { return &row, nil }}
		out, err := svc.GetOut(context.Background(), 1)
		if err != nil || out == nil || out.ID.Int64() != 1 {
			t.Fatalf("out = %#v, err = %v", out, err)
		}
		svc.repo = &mockRepo{selectMenuByID: func(_ context.Context, _ int64) (*SysMenu, error) { return nil, nil }}
		if out, err := svc.GetOut(context.Background(), 999); err != nil || out != nil {
			t.Fatalf("out = %#v, err = %v; want nil, nil", out, err)
		}
	})

	t.Run("查询失败_原样上抛", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewService(nil, testAuth(1))
		svc.repo = &mockRepo{
			selectMenuList: func(_ context.Context, _ authctx.Context, _ Query) ([]SysMenu, error) { return nil, boom },
			selectMenuByID: func(_ context.Context, _ int64) (*SysMenu, error) { return nil, boom },
		}
		if _, err := svc.ListOut(context.Background(), Query{}); !errors.Is(err, boom) {
			t.Errorf("ListOut err = %v, want 原始错误", err)
		}
		if _, err := svc.GetOut(context.Background(), 1); !errors.Is(err, boom) {
			t.Errorf("GetOut err = %v, want 原始错误", err)
		}
	})
}

// ---- 事务假驱动：只记录 Begin/Commit/Rollback ----
//
// menu 的数据访问全经 mock repo，service 不直接查 s.db，故假驱动无需伪造查询，
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

// TestDeleteCascadeRollbackOnRoleMenuFailure 菜单批删后角色关联清理失败 →
// 事务整体 Rollback、绝不 Commit，菜单批删随之撤销（对齐 Python session.begin）。
func TestDeleteCascadeRollbackOnRoleMenuFailure(t *testing.T) {
	rec := &txRecorder{}
	db := newTxDB(t, rec)
	var batchDone bool
	svc := NewService(db, testAuth(2))
	svc.repo = &mockRepo{
		deleteBatch: func(ctx context.Context, _ []int64) (int64, error) {
			batchDone = true
			if crudx.DB(ctx, nil) == nil {
				t.Error("菜单批删未拿到事务句柄：WithTx 传播失效")
			}
			return 1, nil
		},
		deleteRoleMenuByMenuIDs: func(_ context.Context, _ []int64) (int64, error) { return 0, errors.New("boom") },
	}
	err := svc.DeleteCascade(context.Background(), []int64{1})
	se := errs.As(err)
	if se == nil || se.Msg != errs.MsgDBFailed || se.Detail != "boom" {
		t.Fatalf("err = %v, want 500 %s (detail=boom)", err, errs.MsgDBFailed)
	}
	if !batchDone {
		t.Error("菜单批删应先执行（随后被回滚）")
	}
	wantTxEvents(t, rec.snapshot(), []string{"begin", "rollback"})
}

// TestDeleteCascadeTxCommitOnSuccess 对照：两步均成功 → begin → commit。
func TestDeleteCascadeTxCommitOnSuccess(t *testing.T) {
	rec := &txRecorder{}
	db := newTxDB(t, rec)
	svc := NewService(db, testAuth(2))
	svc.repo = &mockRepo{
		deleteBatch:             func(_ context.Context, _ []int64) (int64, error) { return 1, nil },
		deleteRoleMenuByMenuIDs: func(_ context.Context, _ []int64) (int64, error) { return 1, nil },
	}
	if err := svc.DeleteCascade(context.Background(), []int64{1}); err != nil {
		t.Fatalf("err = %v", err)
	}
	wantTxEvents(t, rec.snapshot(), []string{"begin", "commit"})
}
