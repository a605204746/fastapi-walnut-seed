// Package authctx 承载「当前登录用户上下文」，对应 Python 端
// app/core/base_schema.py 的 AuthSchema / CoreUserSchema。
//
// 为什么单独成一个叶子包：认证中间件（module/web/auth）**产出**它，
// 业务模块（module/system/user、menu…）**消费**它。若把类型定义在 auth 包里，
// 业务模块就得反向依赖 web 层——而 auth 包本身又要 import middleware，
// 很快就绕出导入环。放在 pkg 下，两侧都只依赖这一个不含业务语义的包。
package authctx

import (
	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/modelx"
)

// User 当前登录用户的核心信息，对应 CoreUserSchema。
type User struct {
	ID       int64
	UserName string
	Nickname string
	DeptID   *int64
	// IsSuperuser 由 ID 派生（Python 端 is_superuser=(user_id == SUPER_ADMIN_ID)），
	// 不是数据库列
	IsSuperuser bool
}

// IsSuperAdmin 是否为超级管理员，对应 MenuCrud._is_super_admin 等处的判断。
func (u User) IsSuperAdmin() bool { return u.ID == modelx.SuperAdminID }

// Context 请求级认证上下文，对应 AuthSchema。
//
// 三个集合字段由 Redis 会话快照（menu_permission / role_permission / menu_ids）填充，
// **不查库**——这是 Python 端的设计：权限在登录时算好并缓存进会话，
// 每次请求只做一次 Redis 读。改权限后需重新登录才生效，两端行为一致。
type Context struct {
	User User
	// Permissions 菜单权限标识串（如 system:user:list），超管为 ["*:*:*"]
	Permissions []string
	// Roles 角色权限字符串（如 superadmin / common）
	Roles []string
	// MenuIDs 角色授权的菜单 ID 列表，getRouters 与菜单列表都靠它过滤
	MenuIDs []int64
}

// ctxKey 是写入 gin.Context 的键。
//
// 用带包名前缀的字面量而非空串：gin.Context 是个开放的 map，
// 键名撞车会静默覆盖，加前缀成本为零。
const ctxKey = "walnut.authctx"

// Set 把认证上下文写入请求。由认证中间件调用。
func Set(c *gin.Context, v Context) { c.Set(ctxKey, v) }

// From 读取认证上下文。ok=false 表示该路由未经认证中间件，
// 调用方必须**拒绝访问**而不是拿零值继续跑（fail-closed）。
func From(c *gin.Context) (Context, bool) {
	raw, exists := c.Get(ctxKey)
	if !exists {
		return Context{}, false
	}
	v, ok := raw.(Context)
	return v, ok
}
