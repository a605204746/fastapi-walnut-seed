package auth

// provider mock 单测：验证跨域查询归还各域后，auth 经 provider 窄接口的
// 接线语义与迁移前一致（登录用户查找 / 客户端前置 / 注册配置开关 / 会话构建）。
//
// mock 形态与 service_test.go 的 mockRepo 一致：嵌入 nil 接口，仅以函数字段
// 覆盖被测方法；未覆盖的方法被调用即 panic，暴露测试外的意外依赖。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/module/system/client"
	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/module/system/menu"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/module/system/user"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

type mockUserProvider struct {
	UserProvider
	findByUsername func(ctx context.Context, username string) (*user.SysUser, error)
}

func (m *mockUserProvider) FindByUsername(ctx context.Context, username string) (*user.SysUser, error) {
	return m.findByUsername(ctx, username)
}

type mockClientProvider struct {
	ClientProvider
	findByClientID func(ctx context.Context, clientID string) (*client.SysClient, error)
}

func (m *mockClientProvider) FindByClientID(ctx context.Context, clientID string) (*client.SysClient, error) {
	return m.findByClientID(ctx, clientID)
}

type mockConfigProvider struct {
	ConfigProvider
	getValue func(ctx context.Context, key string) (string, bool, error)
}

func (m *mockConfigProvider) GetValue(ctx context.Context, key string) (string, bool, error) {
	return m.getValue(ctx, key)
}

type mockDeptProvider struct {
	DeptProvider
	findByID func(ctx context.Context, deptID int64) (*dept.SysDept, error)
}

func (m *mockDeptProvider) FindByID(ctx context.Context, deptID int64) (*dept.SysDept, error) {
	return m.findByID(ctx, deptID)
}

type mockRoleProvider struct {
	RoleProvider
	findActiveByUserID func(ctx context.Context, userID int64) ([]role.SysRole, error)
}

func (m *mockRoleProvider) FindActiveByUserID(ctx context.Context, userID int64) ([]role.SysRole, error) {
	return m.findActiveByUserID(ctx, userID)
}

type mockMenuProvider struct {
	MenuProvider
	findByRoleIDs func(ctx context.Context, roleIDs []int64) ([]menu.SysMenu, error)
	allActiveIDs  func(ctx context.Context) ([]int64, error)
}

func (m *mockMenuProvider) FindByRoleIDs(ctx context.Context, roleIDs []int64) ([]menu.SysMenu, error) {
	return m.findByRoleIDs(ctx, roleIDs)
}

func (m *mockMenuProvider) AllActiveIDs(ctx context.Context) ([]int64, error) {
	return m.allActiveIDs(ctx)
}

func strPtr(s string) *string { return &s }

// ---- 登录用户查找（UserProvider） ----

// TestLoginPasswordUserLookup 验证登录查用户已走 UserProvider：
// 停用账号在密码校验前拒绝（不触达缓存/密码逻辑），且用户名原样透传。
func TestLoginPasswordUserLookup(t *testing.T) {
	var gotUsername string
	svc := NewService(nil, nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{
		Users: &mockUserProvider{findByUsername: func(_ context.Context, username string) (*user.SysUser, error) {
			gotUsername = username
			u := &user.SysUser{UserName: username, Status: modelx.Disabled}
			u.ID = 7
			return u, nil
		}},
	})
	_, err := svc.LoginPassword(context.Background(),
		&PasswordLoginRequest{Username: "admin", Password: "whatever"}, RequestMeta{}, nil)
	if gotUsername != "admin" {
		t.Errorf("provider 收到的 username = %q, want admin", gotUsername)
	}
	se := errs.As(err)
	if se == nil || se.Code != errs.AuthUserBlocked.Code {
		t.Errorf("err = %v, want 停用错误（code %d）", err, errs.AuthUserBlocked.Code)
	}
}

// TestLoginPasswordUserLookupDBError 验证「数据库出错」与「用户不存在」分流：
// provider 返回 error 时登录报查询失败（500 路径），而不是防枚举的统一失败。
func TestLoginPasswordUserLookupDBError(t *testing.T) {
	boom := errors.New("db down")
	svc := NewService(nil, nil, &config.Config{CaptchaEnable: false}, nil, "", Deps{
		Users: &mockUserProvider{findByUsername: func(_ context.Context, _ string) (*user.SysUser, error) {
			return nil, boom
		}},
	})
	_, err := svc.LoginPassword(context.Background(),
		&PasswordLoginRequest{Username: "admin", Password: "whatever"}, RequestMeta{}, nil)
	if err == nil || !strings.Contains(err.Error(), "查询用户失败") || !errors.Is(err, boom) {
		t.Errorf("err = %v, want 包装「查询用户失败」且保留原因", err)
	}
}

// ---- 客户端前置查询（ClientProvider → handlerRepo 适配） ----

// TestProviderRepoClientDelegation 验证 handler 收编后 FindClientByClientID
// 经 providerRepo 原样委托 ClientProvider（含 (nil, nil) 未找到语义）。
func TestProviderRepoClientDelegation(t *testing.T) {
	want := &client.SysClient{}
	var gotID string
	repo := providerRepo{clients: &mockClientProvider{findByClientID: func(_ context.Context, clientID string) (*client.SysClient, error) {
		gotID = clientID
		return want, nil
	}}}
	got, err := repo.FindClientByClientID(context.Background(), "cid-1")
	if err != nil || got != want || gotID != "cid-1" {
		t.Errorf("got = %p, err = %v, clientID = %q; want 原样委托", got, err, gotID)
	}

	repoNil := providerRepo{clients: &mockClientProvider{findByClientID: func(_ context.Context, _ string) (*client.SysClient, error) {
		return nil, nil
	}}}
	got, err = repoNil.FindClientByClientID(context.Background(), "missing")
	if err != nil || got != nil {
		t.Errorf("未找到应透传 (nil, nil)，got = %p, err = %v", got, err)
	}
}

// ---- 注册配置开关（ConfigProvider → registerEnabled） ----

// TestRegisterEnabledViaConfigProvider 冻结注册开关语义：
// 只 lower() 后精确比较 "true"，**不 trim**；键不存在/值为 NULL 默认关闭。
func TestRegisterEnabledViaConfigProvider(t *testing.T) {
	cases := []struct {
		name  string
		value string
		found bool
		want  bool
	}{
		{"true开启", "true", true, true},
		{"大写TRUE开启", "TRUE", true, true},
		{"前导空格不算开启", " true", true, false},
		{"false关闭", "false", true, false},
		{"键不存在默认关闭", "", false, false},
		{"值为NULL默认关闭", "", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotKey string
			h := &Handler{repo: providerRepo{configs: &mockConfigProvider{getValue: func(_ context.Context, key string) (string, bool, error) {
				gotKey = key
				return tc.value, tc.found, nil
			}}}}
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", nil)
			got, err := h.registerEnabled(c)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if gotKey != "sys.account.registerUser" {
				t.Errorf("provider 收到的 key = %q, want sys.account.registerUser", gotKey)
			}
			if got != tc.want {
				t.Errorf("registerEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRegisterEnabledConfigDBError 验证配置查询出错时 registerEnabled 上抛 error
// （handler 侧报 DB 失败），而不是悄悄按关闭处理。
func TestRegisterEnabledConfigDBError(t *testing.T) {
	boom := errors.New("db down")
	h := &Handler{repo: providerRepo{configs: &mockConfigProvider{getValue: func(_ context.Context, _ string) (string, bool, error) {
		return "", false, boom
	}}}}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", nil)
	got, err := h.registerEnabled(c)
	if got || !errors.Is(err, boom) {
		t.Errorf("got = %v, err = %v; want (false, 原错误)", got, err)
	}
}

// ---- 会话构建（Dept/Role/Menu Provider） ----

// TestBuildSessionInfoProviders 验证普通用户会话经 provider 构建：
// 部门名/类别、角色 key 过滤、菜单去重排序，且各 provider 收到正确入参。
func TestBuildSessionInfoProviders(t *testing.T) {
	deptID := int64(100)
	var gotDeptID, gotUserID int64
	var gotRoleIDs []int64
	svc := NewService(nil, nil, &config.Config{}, nil, "", Deps{
		Depts: &mockDeptProvider{findByID: func(_ context.Context, id int64) (*dept.SysDept, error) {
			gotDeptID = id
			return &dept.SysDept{DeptName: strPtr("研发部"), DeptCategory: strPtr("tech")}, nil
		}},
		Roles: &mockRoleProvider{findActiveByUserID: func(_ context.Context, uid int64) ([]role.SysRole, error) {
			gotUserID = uid
			roles := []role.SysRole{{RoleKey: "admin"}, {RoleKey: ""}}
			roles[0].ID = 2
			roles[1].ID = 3
			return roles, nil
		}},
		Menus: &mockMenuProvider{findByRoleIDs: func(_ context.Context, ids []int64) ([]menu.SysMenu, error) {
			gotRoleIDs = ids
			menus := []menu.SysMenu{
				{Perms: strPtr("system:user:list")},
				{Perms: strPtr("system:user:list")}, // 多角色重复菜单
				{Perms: strPtr("  ")},               // 空白 perms 剔除
				{Perms: nil},                        // NULL perms 剔除
				{Perms: strPtr("system:role:list")},
			}
			menus[0].ID = 9
			menus[1].ID = 9
			menus[2].ID = 5
			menus[3].ID = 5
			menus[4].ID = 3
			return menus, nil
		}},
	})
	u := &user.SysUser{UserName: "u", NickName: "n", DeptID: &deptID, Status: modelx.Normal}
	u.ID = 7
	info, err := svc.buildSessionInfo(context.Background(), u)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotDeptID != 100 || gotUserID != 7 || len(gotRoleIDs) != 2 || gotRoleIDs[0] != 2 || gotRoleIDs[1] != 3 {
		t.Errorf("provider 入参错误: deptID=%d userID=%d roleIDs=%v", gotDeptID, gotUserID, gotRoleIDs)
	}
	if info.DeptName != "研发部" || info.DeptCategory != "tech" {
		t.Errorf("部门信息 = %q/%q, want 研发部/tech", info.DeptName, info.DeptCategory)
	}
	if len(info.RolePermission) != 1 || info.RolePermission[0] != "admin" {
		t.Errorf("RolePermission = %v, want [admin]（空 key 剔除）", info.RolePermission)
	}
	if len(info.MenuIDs) != 3 || info.MenuIDs[0] != 3 || info.MenuIDs[1] != 5 || info.MenuIDs[2] != 9 {
		t.Errorf("MenuIDs = %v, want [3 5 9]（去重+升序）", info.MenuIDs)
	}
	if len(info.MenuPermission) != 2 || info.MenuPermission[0] != "system:role:list" || info.MenuPermission[1] != "system:user:list" {
		t.Errorf("MenuPermission = %v, want 去重排序后的两条", info.MenuPermission)
	}
}

// TestBuildSessionInfoSuperAdmin 验证超管分支只走 MenuProvider.AllActiveIDs，
// 不触达部门/角色/按角色查菜单（触达即 panic）。
func TestBuildSessionInfoSuperAdmin(t *testing.T) {
	svc := NewService(nil, nil, &config.Config{}, nil, "", Deps{
		Depts: &mockDeptProvider{}, // 未覆盖 findByID，调用即 panic
		Roles: &mockRoleProvider{},
		Menus: &mockMenuProvider{
			findByRoleIDs: nil, // 超管不得按角色查菜单
			allActiveIDs: func(_ context.Context) ([]int64, error) {
				return []int64{9, 3}, nil
			},
		},
	})
	u := &user.SysUser{UserName: "admin"}
	u.ID = modelx.SuperAdminID
	info, err := svc.buildSessionInfo(context.Background(), u)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(info.RolePermission) != 1 || info.RolePermission[0] != modelx.SuperAdminRoleKey {
		t.Errorf("RolePermission = %v, want [superadmin]", info.RolePermission)
	}
	if len(info.MenuPermission) != 1 || info.MenuPermission[0] != modelx.AllPermission {
		t.Errorf("MenuPermission = %v, want [*:*:*]", info.MenuPermission)
	}
	if len(info.MenuIDs) != 2 || info.MenuIDs[0] != 9 || info.MenuIDs[1] != 3 {
		t.Errorf("MenuIDs = %v, want provider 原样返回 [9 3]", info.MenuIDs)
	}
}

// TestBuildSessionInfoNoRoles 验证无角色时不触达 FindByRoleIDs
// （service 侧 len(roleIDs)>0 守卫），权限数组保持空切片而非 null。
func TestBuildSessionInfoNoRoles(t *testing.T) {
	svc := NewService(nil, nil, &config.Config{}, nil, "", Deps{
		Roles: &mockRoleProvider{findActiveByUserID: func(_ context.Context, _ int64) ([]role.SysRole, error) {
			return []role.SysRole{}, nil
		}},
		Menus: &mockMenuProvider{}, // 未覆盖 findByRoleIDs，调用即 panic
	})
	u := &user.SysUser{UserName: "u"}
	u.ID = 7
	info, err := svc.buildSessionInfo(context.Background(), u)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.MenuIDs == nil || info.MenuPermission == nil || info.RolePermission == nil {
		t.Errorf("权限数组不得为 nil: %+v", info)
	}
	if len(info.MenuIDs) != 0 || len(info.MenuPermission) != 0 || len(info.RolePermission) != 0 {
		t.Errorf("无角色用户权限应为空: %+v", info)
	}
}

// TestBuildSessionInfoDeptNotFound 验证部门不存在（(nil, nil)）时
// 会话部门名/类别为空串而非报错（JWT claims 契约要求 "" 不是 null）。
func TestBuildSessionInfoDeptNotFound(t *testing.T) {
	deptID := int64(404)
	svc := NewService(nil, nil, &config.Config{}, nil, "", Deps{
		Depts: &mockDeptProvider{findByID: func(_ context.Context, _ int64) (*dept.SysDept, error) {
			return nil, nil
		}},
		Roles: &mockRoleProvider{findActiveByUserID: func(_ context.Context, _ int64) ([]role.SysRole, error) {
			return []role.SysRole{}, nil
		}},
		Menus: &mockMenuProvider{},
	})
	u := &user.SysUser{UserName: "u", DeptID: &deptID}
	u.ID = 7
	info, err := svc.buildSessionInfo(context.Background(), u)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.DeptName != "" || info.DeptCategory != "" {
		t.Errorf("部门不存在时应为空串, got %q/%q", info.DeptName, info.DeptCategory)
	}
}
