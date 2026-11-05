package permguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/errs"
)

func init() { gin.SetMode(gin.TestMode) }

// newEngine 构造一个测试引擎：withAuth 为 nil 时不注入认证上下文，
// 用于验证「守卫在缺少上下文时 fail-closed」。
func newEngine(g *Guard, rule Rule, withAuth *authctx.Context) *gin.Engine {
	e := gin.New()
	e.GET("/t", func(c *gin.Context) {
		if withAuth != nil {
			authctx.Set(c, *withAuth)
		}
	}, g.Check(rule), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "操作成功", "data": "reached"})
	})
	return e
}

func do(e *gin.Engine) (int, map[string]any) {
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/t", nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

// ctxOf 构造带指定权限/角色的普通用户上下文（ID 非 1，非超管）。
func ctxOf(perms, roles []string) *authctx.Context {
	return &authctx.Context{
		User:        authctx.User{ID: 7, UserName: "tester"},
		Permissions: perms,
		Roles:       roles,
	}
}

func TestGuard(t *testing.T) {
	g := New(&errs.Handler{})

	tests := []struct {
		name     string
		rule     Rule
		auth     *authctx.Context
		wantCode int    // body.code
		wantMsg  string // body.msg（空表示不校验）
		reached  bool   // 业务 handler 是否被执行
	}{
		{
			name:     "无认证上下文→401拒绝（fail-closed）",
			rule:     Rule{Permissions: []string{"system:post:list"}},
			auth:     nil,
			wantCode: errs.CodeUnauthorized,
			wantMsg:  errs.MsgNotLogin,
		},
		{
			name:     "零值Rule只要求登录",
			rule:     Rule{},
			auth:     ctxOf(nil, nil),
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "超级管理员无视权限要求",
			rule:     Rule{Permissions: []string{"system:post:remove"}},
			auth:     &authctx.Context{User: authctx.User{ID: 1, IsSuperuser: true}},
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "is_superuser为真但ID非1也放行",
			rule:     Rule{Permissions: []string{"system:post:remove"}},
			auth:     &authctx.Context{User: authctx.User{ID: 99, IsSuperuser: true}},
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "权限命中→放行",
			rule:     Rule{Permissions: []string{"system:post:list"}},
			auth:     ctxOf([]string{"system:post:list"}, nil),
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "权限缺失→403固定文案",
			rule:     Rule{Permissions: []string{"system:post:list"}},
			auth:     ctxOf([]string{"system:user:list"}, nil),
			wantCode: errs.CodeForbidden,
			wantMsg:  errs.MsgNotPermission,
		},
		{
			name:     "权限集为空→403（先于匹配判断）",
			rule:     Rule{Permissions: []string{"system:post:list"}},
			auth:     ctxOf(nil, nil),
			wantCode: errs.CodeForbidden,
			wantMsg:  errs.MsgNotPermission,
		},
		{
			name:     "通配符*:*:*放行",
			rule:     Rule{Permissions: []string{"system:post:list"}},
			auth:     ctxOf([]string{"*:*:*"}, nil),
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "通配符*放行",
			rule:     Rule{Permissions: []string{"system:post:list"}},
			auth:     ctxOf([]string{"*"}, nil),
			wantCode: 200,
			reached:  true,
		},
		{
			// Python 端是集合成员判断而非子串匹配，这条是防回归的关键用例
			name:     "含星号的具体权限串不算通配符",
			rule:     Rule{Permissions: []string{"system:post:list"}},
			auth:     ctxOf([]string{"system:*:list"}, nil),
			wantCode: errs.CodeForbidden,
			wantMsg:  errs.MsgNotPermission,
		},
		{
			name:     "AND模式需全部满足→缺一个即拒绝",
			rule:     Rule{Permissions: []string{"a:b:c", "d:e:f"}, PermMode: And},
			auth:     ctxOf([]string{"a:b:c"}, nil),
			wantCode: errs.CodeForbidden,
		},
		{
			name:     "AND模式全部满足→放行",
			rule:     Rule{Permissions: []string{"a:b:c", "d:e:f"}, PermMode: And},
			auth:     ctxOf([]string{"a:b:c", "d:e:f"}, nil),
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "OR模式满足其一→放行",
			rule:     Rule{Permissions: []string{"a:b:c", "d:e:f"}, PermMode: Or},
			auth:     ctxOf([]string{"d:e:f"}, nil),
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "默认模式是AND",
			rule:     Rule{Permissions: []string{"a:b:c", "d:e:f"}},
			auth:     ctxOf([]string{"a:b:c"}, nil),
			wantCode: errs.CodeForbidden,
		},
		{
			name:     "角色OR模式命中其一→放行",
			rule:     Rule{Roles: []string{"admin", "common"}, RoleMode: Or},
			auth:     ctxOf(nil, []string{"common"}),
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "角色默认模式是OR",
			rule:     Rule{Roles: []string{"admin", "common"}},
			auth:     ctxOf(nil, []string{"common"}),
			wantCode: 200,
			reached:  true,
		},
		{
			name:     "角色AND模式缺一个→403",
			rule:     Rule{Roles: []string{"admin", "common"}, RoleMode: And},
			auth:     ctxOf(nil, []string{"common"}),
			wantCode: errs.CodeForbidden,
			wantMsg:  errs.MsgNotPermission,
		},
		{
			name:     "角色校验先于权限校验",
			rule:     Rule{Roles: []string{"admin"}, Permissions: []string{"a:b:c"}},
			auth:     ctxOf([]string{"a:b:c"}, []string{"common"}),
			wantCode: errs.CodeForbidden,
		},
		{
			name:     "角色与权限都满足→放行",
			rule:     Rule{Roles: []string{"admin"}, Permissions: []string{"a:b:c"}},
			auth:     ctxOf([]string{"a:b:c"}, []string{"admin"}),
			wantCode: 200,
			reached:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(newEngine(g, tt.rule, tt.auth))

			// 传输层恒为 200：业务错误码写在 body.code（三端一致的既有约定）
			if status != http.StatusOK {
				t.Errorf("HTTP 状态码 got=%d want=200（业务错误不应改传输层状态）", status)
			}
			if code, _ := body["code"].(float64); int(code) != tt.wantCode {
				t.Errorf("body.code got=%v want=%d（body=%v）", body["code"], tt.wantCode, body)
			}
			if tt.wantMsg != "" {
				if msg, _ := body["msg"].(string); msg != tt.wantMsg {
					t.Errorf("body.msg got=%q want=%q", msg, tt.wantMsg)
				}
			}
			reached := body["data"] == "reached"
			if reached != tt.reached {
				t.Errorf("业务 handler 执行 got=%v want=%v", reached, tt.reached)
			}
		})
	}
}

// TestGuardNilHandler 验证没有 errs.Handler 时的兜底路径不会 panic，
// 且仍返回信封形状（而非 gin 的裸响应）。
func TestGuardNilHandler(t *testing.T) {
	g := New(nil)
	auth := ctxOf([]string{"x:y:z"}, nil)
	status, body := do(newEngine(g, Rule{Permissions: []string{"a:b:c"}}, auth))

	if status != http.StatusOK {
		t.Errorf("HTTP 状态码 got=%d want=200", status)
	}
	if code, _ := body["code"].(float64); int(code) != errs.CodeForbidden {
		t.Errorf("body.code got=%v want=%d", body["code"], errs.CodeForbidden)
	}
	if _, ok := body["msg"]; !ok {
		t.Errorf("兜底响应缺少 msg 字段: %v", body)
	}
}

func TestHasWildcard(t *testing.T) {
	cases := []struct {
		perms []string
		want  bool
	}{
		{[]string{"*"}, true},
		{[]string{"*:*:*"}, true},
		{[]string{"system:*:list"}, false},
		{[]string{"*:*"}, false},
		{[]string{"system:user:list", "*:*:*"}, true},
		{nil, false},
		{[]string{""}, false},
	}
	for _, c := range cases {
		if got := hasWildcard(c.perms); got != c.want {
			t.Errorf("hasWildcard(%v) = %v, want %v", c.perms, got, c.want)
		}
	}
}
