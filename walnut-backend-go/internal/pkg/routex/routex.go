// Package routex 提供业务模块向路由组合根注册端点所需的最小能力。
package routex

import "github.com/gin-gonic/gin"

// Binder 是「注册路由」这一能力的抽象，由各业务模块的 Mount 方法接收。
//
// 为什么用**函数类型的结构体**而不是接口：
//   - 业务模块不能 import bootstrap：bootstrap 要 import 它们才能装配路由，
//     反向依赖会形成导入环
//   - 接口也不行：bootstrap.Group.GET 的签名是
//     `func(string, ...gin.HandlerFunc) *Group`（返回自身以支持链式调用），
//     而 Go **不允许**把有返回值的函数赋给无返回值的函数类型，
//     接口方法签名必须完全一致才能满足
//
// 故由 bootstrap 侧用闭包适配（丢弃返回值，见 bootstrap.Group.Binder），
// 业务模块只看到无返回值的函数。附带好处：handler 可以脱离 bootstrap 做单元测试。
type Binder struct {
	GET    func(path string, handlers ...gin.HandlerFunc)
	POST   func(path string, handlers ...gin.HandlerFunc)
	PUT    func(path string, handlers ...gin.HandlerFunc)
	DELETE func(path string, handlers ...gin.HandlerFunc)
}
