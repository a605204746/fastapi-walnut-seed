// 本文件集中维护路由声明，handler.go 只保留请求处理函数。
package auth

import (
	"walnut-seed-go/internal/pkg/routex"
)

// Mount 注册全部 5 条认证路由。
//
// 命名为 Mount 而非 Register：本包的 HTTP handler 已经有一个叫 Register
// （POST /auth/register 的用户注册），同名会冲突。
//
// ⚠️ 全部 5 条都在 WHITE_API_LIST_PATH 白名单内（无需认证）：
// login / code / tenant/list 是登录前置，logout 必须允许 token 失效时调用（幂等）。
// 故 bootstrap 侧要用 **Public** 分组注册，否则启动期路由认证审计会拒绝启动。
func (h *Handler) Mount(b routex.Binder) {
	b.POST("/login", h.Login)
	b.POST("/logout", h.Logout)
	b.POST("/register", h.Register)
	b.GET("/tenant/list", h.TenantList)
	b.GET("/code", h.Code)
}
