package auth

// 本文件定义 auth 消费的跨域只读能力（consumer-side 窄接口）。
//
// 背景：auth/repo.go 曾集中直查 user/client/config/dept/role/menu 六个域的表
// （原作者的临时安排，见 repo.go 旧注释），现已按计划归还各域 repo：
// 各域 repo 持有逐字迁入的 SQL，bootstrap 经适配器把各域 repo 注入本文件的
// provider 接口，auth 不再穿透到任何跨域具体层。
//
// ⚠️ 方法签名保留各域模型类型（*user.SysUser 等），因此本文件仍 import 各域
// 模型包——这是**可接受的类型级耦合**：只依赖数据结构定义，不依赖各域的
// repo/service 具体实现；换成 auth 私有 DTO 反而要逐字段翻译、徒增漂移风险。

import (
	"context"

	"walnut-seed-go/internal/module/system/client"
	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/module/system/menu"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/module/system/user"
)

// UserProvider 登录用户查找。
// 找不到返回 (nil, nil)：调用方要区分「用户不存在」（防枚举统一失败路径）
// 与「数据库出错」（500）。实现：user.Repo.FindByUserName（语义完全一致，复用）。
type UserProvider interface {
	FindByUsername(ctx context.Context, username string) (*user.SysUser, error)
}

// ClientProvider 客户端授权记录查找（登录前置校验）。
// 找不到返回 (nil, nil)。实现：client.Repo.FindByClientID。
type ClientProvider interface {
	FindByClientID(ctx context.Context, clientID string) (*client.SysClient, error)
}

// ConfigProvider 参数配置值读取（注册开关）。
// 三态返回：不存在 ("", false, nil)；值为 NULL ("", true, nil)；正常 (值, true, nil)。
// 实现：config.Repo.GetValueByKey。
type ConfigProvider interface {
	GetValue(ctx context.Context, key string) (string, bool, error)
}

// DeptProvider 部门查找（会话与 JWT claims 取 dept_name / dept_category）。
// ⚠️ 实现必须是**不带 del_flag 过滤**的 dept.Repo.FindByID，
// 不是带过滤的 dept.Repo.Get（SQL 语义不同，见该方法注释）。
type DeptProvider interface {
	FindByID(ctx context.Context, deptID int64) (*dept.SysDept, error)
}

// RoleProvider 用户有效角色查找（status='0' 且 del_flag='0'，不排序）。
// ⚠️ 实现必须是 role.Repo.FindActiveByUserID，不是 user.Repo.FindRoles
// （后者无 status 过滤且带排序，SQL 语义不同）。
type RoleProvider interface {
	FindActiveByUserID(ctx context.Context, userID int64) ([]role.SysRole, error)
}

// MenuProvider 菜单权限查询（会话构建）。
// FindByRoleIDs：空 roleIDs 直接返回空切片（不得生成非法的 IN ()）；不排序。
// AllActiveIDs：超管分支取全部有效菜单 ID。
// 实现：menu.Repo.FindByRoleIDs / menu.Repo.AllActiveIDs。
type MenuProvider interface {
	FindByRoleIDs(ctx context.Context, roleIDs []int64) ([]menu.SysMenu, error)
	AllActiveIDs(ctx context.Context) ([]int64, error)
}

// Deps auth 模块依赖：跨域只读能力，由 bootstrap 用各域 repo 的适配器注入
// （参考 user.Deps 的形态；user 的 provider 是按请求 authctx 构造的服务工厂，
// auth 的 provider 全部是无状态 repo，与请求上下文无关，故直接注入实例）。
type Deps struct {
	Users   UserProvider
	Clients ClientProvider
	Configs ConfigProvider
	Depts   DeptProvider
	Roles   RoleProvider
	Menus   MenuProvider
}
