// Package permguard 实现权限串/角色校验守卫，对应 Python 端
// app/core/dependencies.py 的 AuthPermission。
//
// # 为什么是独立叶子包
//
// 与 authctx 同理：业务模块（module/system/*）需要它，而它不能反向依赖 web 层。
// 故只依赖 authctx + errs + gin，放在 pkg 下。
//
// # 用法
//
// 经 routex.Binder 注册时作为前置 handler 传入（Binder 的注册函数是变参的）：
//
//	b.GET("/list", g.Require("system:post:list"), h.List)
//	b.POST("", g.Require("system:post:add"), idempotentx.Guard(5000), h.Add)
//
// # 后端必须二次校验
//
// 前端隐藏按钮不是权限控制。每个端点都必须挂守卫，否则拿到 token 就能直接调。
// 启动期路由认证审计只保证「挂了认证中间件」，**不保证挂了权限守卫**——
// 后者靠契约 diff 的 403 用例兜底。
package permguard

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/errs"
)

// Mode 是多权限/多角色的组合方式，对应 Python 端 AuthPermission 的 mode / role_mode。
type Mode string

const (
	// And 要求全部满足（Python 端 permissions 的默认值）
	And Mode = "AND"
	// Or 要求满足其一（Python 端 roles 的默认值）
	Or Mode = "OR"
)

// Rule 描述一个端点的访问要求。
//
// 零值 Rule{} 表示「只需登录，不校验权限/角色」——对应 Python 端依赖
// get_current_user 而非 AuthPermission 的端点（如 getInfo、getRouters）。
type Rule struct {
	// Permissions 所需权限标识（如 system:user:add）
	Permissions []string
	// Roles 所需角色标识（如 superadmin）
	Roles []string
	// PermMode 权限组合方式，默认 And
	PermMode Mode
	// RoleMode 角色组合方式，默认 Or
	RoleMode Mode
}

// Guard 是权限守卫的工厂，持有渲染错误所需的 errs.Handler。
//
// 各业务模块的 Handler 里存一个它，注册路由时用 g.Require("...") 取中间件。
type Guard struct {
	errs *errs.Handler
}

// New 构造守卫工厂。eh 为 nil 时退化为不记日志、用 errs 的默认渲染。
func New(eh *errs.Handler) *Guard { return &Guard{errs: eh} }

// Require 构造「需要全部给定权限串」的中间件（最常见形态）。
func (g *Guard) Require(permissions ...string) gin.HandlerFunc {
	return g.Check(Rule{Permissions: permissions})
}

// RequireAny 构造「满足其一即可」的中间件。
func (g *Guard) RequireAny(permissions ...string) gin.HandlerFunc {
	return g.Check(Rule{Permissions: permissions, PermMode: Or})
}

// RequireRoles 构造只校验角色的中间件。
func (g *Guard) RequireRoles(roles ...string) gin.HandlerFunc {
	return g.Check(Rule{Roles: roles})
}

// Check 按 Rule 校验当前登录用户，不通过则中止请求。
//
// 校验顺序与 Python 端 AuthPermission.__call__ 严格一致：
//  1. 超级管理员直接放行
//  2. 角色校验（先于权限）
//  3. 权限校验：持有 "*" 或 "*:*:*" 通配符直接放行；
//     权限集为空直接拒绝；否则按 mode 匹配
func (g *Guard) Check(rule Rule) gin.HandlerFunc {
	permMode := rule.PermMode
	if permMode == "" {
		permMode = And
	}
	roleMode := rule.RoleMode
	if roleMode == "" {
		roleMode = Or
	}

	return func(c *gin.Context) {
		ac, ok := authctx.From(c)
		if !ok {
			// 未挂认证中间件的路由不该走到这里；fail-closed 拒绝而非放行
			g.abort(c, errs.NotLogin())
			return
		}

		// 1. 超级管理员放行（id==1 或 is_superuser，两者取或，与 Python 一致）
		if ac.User.ID == 1 || ac.User.IsSuperuser {
			c.Next()
			return
		}

		// 2. 角色校验
		if len(rule.Roles) > 0 && !satisfies(ac.Roles, rule.Roles, roleMode) {
			g.abortLog(c, errs.NotRole(), "用户缺少所需角色", rule.Roles)
			return
		}

		// 3. 权限校验
		if len(rule.Permissions) > 0 {
			if hasWildcard(ac.Permissions) {
				c.Next()
				return
			}
			if len(ac.Permissions) == 0 {
				g.abortLog(c, errs.NotPermission(), "用户缺少所需权限", rule.Permissions)
				return
			}
			if !satisfies(ac.Permissions, rule.Permissions, permMode) {
				g.abortLog(c, errs.NotPermission(), "用户缺少所需权限", rule.Permissions)
				return
			}
		}

		c.Next()
	}
}

// hasWildcard 判断是否持有全部权限通配符。
//
// Python 端是 `"*" in user_permissions or "*:*:*" in user_permissions`——
// 集合成员判断，**不是子串匹配**。故 "*" 必须作为一个完整元素出现；
// 写成 strings.Contains 会让 "system:*:list" 之类的串误命中通配符。
func hasWildcard(perms []string) bool {
	for _, p := range perms {
		if p == "*" || p == "*:*:*" {
			return true
		}
	}
	return false
}

// satisfies 按 mode 判断 user 集合是否满足 required。
func satisfies(user, required []string, mode Mode) bool {
	set := make(map[string]struct{}, len(user))
	for _, u := range user {
		set[u] = struct{}{}
	}
	if mode == Or {
		for _, r := range required {
			if _, ok := set[r]; ok {
				return true
			}
		}
		return false
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}

// abortLog 记录审计日志并中止。
//
// 日志内容与 Python 端 logger.error 对齐（便于三端日志比对），
// 但**不外泄到响应体**——响应只有固定的 403 文案。
func (g *Guard) abortLog(c *gin.Context, se *errs.ServiceError, reason string, required []string) {
	if g.errs != nil && g.errs.Logger != nil {
		g.errs.Logger.Error(reason,
			slog.String("path", c.Request.URL.Path),
			slog.String("required", strings.Join(required, ",")),
		)
	}
	g.abort(c, se)
}

// abort 渲染业务异常并中止后续 handler。
func (g *Guard) abort(c *gin.Context, se *errs.ServiceError) {
	if g.errs != nil {
		g.errs.Abort(c, se)
		return
	}
	// 没有 errs.Handler 时的兜底：直接写信封，避免返回 gin 的裸响应
	c.Abort()
	c.JSON(se.Status(), gin.H{"code": se.Code, "msg": se.Msg, "data": nil})
}
