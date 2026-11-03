// Package permx 实现行级数据权限，对应 Python 端 app/core/permission.py 的 Permission。
//
// # 语义（必须与 Python 端逐条一致）
//
// 角色表 data_scope 取值（DataScopeType），多角色条件以 OR 连接：
//
//	1 全部             → 不过滤（任一角色命中即整体放行）
//	2 自定义           → dept_column IN (SELECT dept_id FROM sys_role_dept WHERE role_id = ?)
//	3 本部门           → dept_column = 用户部门（无部门时用 -1，保证查不到任何行）
//	4 本部门及以下      → dept_column IN (本部门 + ancestors 含本部门的子孙部门)
//	5 仅本人           → create_by = 用户ID
//	6 本部门及以下或本人 → 范围4 OR create_by = 用户ID
//
// # fail-closed（本包存在的唯一理由）
//
// 权限组件出错时**必须拒绝访问，不是放行**。Go 里最容易误写成
//
//	scope, err := permx.Build(...)
//	if err != nil { return nil }        // ❌ 严重安全回归：等于放行全部数据
//
// 正确写法是把 err 直接向上返回，由 handler 转成 500。为此 Build 在出错时
// **返回零值 Scope**——零值的 SQL 为空但 NoFilter 也为 false，
// 误用 Apply 会得到一个不带条件的查询吗？不会：见 Scope.Apply 的防御性判断。
//
// 另外三种「无角色 / 无有效条件」的情形一律退化为**仅本人数据**，同样不是放行。
package permx

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

// 角色数据范围取值，对应 Python 端 DataScopeType。
const (
	ScopeAll             = "1" // 全部数据权限
	ScopeCustom          = "2" // 自定义（角色授权部门）
	ScopeDept            = "3" // 本部门
	ScopeDeptAndChild    = "4" // 本部门及以下
	ScopeSelf            = "5" // 仅本人
	ScopeDeptChildOrSelf = "6" // 本部门及以下或本人
)

// Option 指定过滤使用的列名。
//
// 列名由调用方（业务模块）给出而非反射模型，因此**只接受本包与调用方硬编码的
// 常量**，不接受任何来自请求的数据——这是 SQL 注入防线的一部分。
type Option struct {
	// CreateByColumn 「仅本人」过滤列。留空表示模型无 create_by，
	// 此时无法表达「仅本人」，一律退化为「查不到任何行」（fail-closed）。
	CreateByColumn string
	// DeptColumn 部门过滤列。留空表示模型无 create_dept，
	// 依赖部门列的范围（2/3/4）会退化：2、3 不产生条件，4 产生恒假条件。
	DeptColumn string
}

// Default 是绝大多数业务表的列配置（BaseModel 提供 create_by / create_dept）。
var Default = Option{CreateByColumn: "create_by", DeptColumn: "create_dept"}

// UserList 是 sys_user 列表的列配置。
//
// ⚠️ 按 RuoYi 契约，用户列表的行级过滤用 **dept_id**（用户所属部门）
// 而不是 create_dept（创建人部门）——两者语义不同，混用会让
// 「本部门」范围筛出错误的人群。对应 Python 端 Permission(dept_column=...)。
var UserList = Option{CreateByColumn: "create_by", DeptColumn: "dept_id"}

// Scope 是数据权限的求解结果。
type Scope struct {
	// NoFilter 为 true 表示不追加任何条件（超管 / 命中「全部」范围）。
	// 这是**唯一**允许查全表的形态。
	NoFilter bool
	// SQL 是可直接交给 gorm Where 的条件片段，占位符一律用 ?，参数在 Args。
	// ⚠️ 列名来自 Option（硬编码），值一律走 Args，禁止把任何请求数据拼进 SQL。
	SQL  string
	Args []any

	// denied 标记「求解失败」，用于 Apply 的 fail-closed 防御。
	denied bool
}

// Apply 把数据权限条件作用到查询上。
//
// ⚠️ fail-closed：求解失败（denied）时返回一个**恒假条件**的查询，
// 而不是原样返回 db。这样即使调用方漏判了 Build 的 error，
// 最坏结果是查不到数据，绝不会泄露越权数据。
func (s Scope) Apply(db *gorm.DB) *gorm.DB {
	if s.denied {
		return db.Where("1 = 0")
	}
	if s.NoFilter || s.SQL == "" {
		return db
	}
	return db.Where(s.SQL, s.Args...)
}

// RoleScope 是一个角色的数据范围，来自 sys_role.data_scope。
type RoleScope struct {
	RoleID int64
	// DataScope 为 nil 表示该列是 NULL（Python 端同样会得到 None）。
	DataScope *string
}

// Build 求解当前用户的行级数据权限条件。
//
// 返回的 error 必须向上传递：它可能是 DB 故障，也可能是 data_scope 取值非法
// （后者是**面向用户的契约文案**，见 buildCondition）。
func Build(ctx context.Context, db *gorm.DB, auth authctx.Context, opt Option) (Scope, error) {
	scopes, err := fetchRoleScopes(ctx, db, auth.User.ID)
	if err != nil {
		// fail-closed：查询失败时返回 denied 的 Scope + error。
		// 即使调用方忽略了 error，Apply 也只会给出空结果集。
		return Scope{denied: true}, fmt.Errorf("查询角色数据范围失败: %w", err)
	}
	scope, err := buildCondition(auth, opt, scopes)
	if err != nil {
		return Scope{denied: true}, err
	}
	return scope, nil
}

// RoleScopes 暴露角色数据范围查询，供**自建数据权限条件**的业务模块复用
// （SQL 与 Build 内部用的完全一致）。
//
// ⚠️ 仅当模块的行级语义与 Build 不同时才该用它自己拼条件——例如 dept 模块
// 按自身 id 列过滤、且「角色集为空时不过滤」（与 Build 的 fail-closed 兜底相反，
// 是 Python 端 dept service 的既有语义）。拼条件时仍须遵守 fail-closed：
// 本函数返回的 error 必须向上传递，不得吞掉后放行。
func RoleScopes(ctx context.Context, db *gorm.DB, userID int64) ([]RoleScope, error) {
	return fetchRoleScopes(ctx, db, userID)
}

// fetchRoleScopes 取当前用户所有**未删除**角色的数据范围。
//
// 对应 Python 端 join(UserRoleModel) + where(del_flag == NORMAL)。
func fetchRoleScopes(ctx context.Context, db *gorm.DB, userID int64) ([]RoleScope, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	var rows []RoleScope
	err := db.WithContext(ctx).
		Table("sys_role AS r").
		Select("r.id AS role_id, r.data_scope AS data_scope").
		Joins("JOIN sys_user_role ur ON ur.role_id = r.id").
		Where("ur.user_id = ? AND r.del_flag = ?", userID, modelx.Normal).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// buildCondition 是数据权限的**纯函数**核心，不碰数据库。
//
// 单独拆出来的原因是它承载了全部 fail-closed 分支，必须能离线穷举测试
// （见 permx_test.go）。DB 查询只有 fetchRoleScopes 一处。
func buildCondition(auth authctx.Context, opt Option, scopes []RoleScope) (Scope, error) {
	user := auth.User
	// 无用户 / 无 ID → 不过滤。对齐 Python「if not user or not user.id: return None」。
	// 这条分支只在认证上下文缺失时命中，而受保护路由必然已有上下文，
	// 故实际是不可达的防御性代码。
	if user.ID == 0 {
		return Scope{NoFilter: true}, nil
	}
	// 超级管理员不过滤
	if user.IsSuperuser {
		return Scope{NoFilter: true}, nil
	}

	// 无角色 → 仅本人数据（fail-closed）
	if len(scopes) == 0 {
		return selfOnly(opt, user.ID), nil
	}

	var (
		conds []string
		args  []any
	)
	for _, rs := range scopes {
		code := ""
		if rs.DataScope != nil {
			code = *rs.DataScope
		}
		switch code {
		case ScopeAll:
			// 任一角色为「全部」→ 整体不过滤（立即返回，不再看其余角色）
			return Scope{NoFilter: true}, nil

		case ScopeCustom:
			if opt.DeptColumn != "" {
				conds = append(conds, fmt.Sprintf(
					"%s IN (SELECT dept_id FROM sys_role_dept WHERE role_id = ?)", opt.DeptColumn))
				args = append(args, rs.RoleID)
			}

		case ScopeDept:
			if opt.DeptColumn != "" {
				// 无部门时用 -1：任何真实部门 ID 都不等于 -1，等价于查不到数据
				deptID := int64(-1)
				if user.DeptID != nil {
					deptID = *user.DeptID
				}
				conds = append(conds, opt.DeptColumn+" = ?")
				args = append(args, deptID)
			}

		case ScopeDeptAndChild, ScopeDeptChildOrSelf:
			// deptCond 默认恒假，对应 Python 的 sqlalchemy.false()
			deptCond, deptArgs := deptAndChildCond(opt, user.DeptID)
			if code == ScopeDeptAndChild {
				conds = append(conds, "("+deptCond+")")
				args = append(args, deptArgs...)
			} else {
				// 范围 6：本部门及以下 **或** 本人创建
				conds = append(conds, "("+deptCond+") OR "+selfCond(opt, user.ID))
				args = append(args, deptArgs...)
				args = append(args, selfArgs(opt, user.ID)...)
			}

		case ScopeSelf:
			conds = append(conds, selfCond(opt, user.ID))
			args = append(args, selfArgs(opt, user.ID)...)

		default:
			// data_scope 取值非法（含 NULL）。这是**面向用户的契约文案**，
			// 必须与 Python 端 f"角色数据范围异常 => {data_scope}" 逐字一致——
			// 注意 NULL 在 Python 里格式化成字面量 "None"。
			literal := code
			if rs.DataScope == nil {
				literal = "None"
			}
			return Scope{denied: true}, errs.Newf("角色数据范围异常 => %s", literal)
		}
	}

	// 兜底：无有效部门/本人条件时仅本人（fail-closed）
	if len(conds) == 0 {
		return selfOnly(opt, user.ID), nil
	}
	return Scope{SQL: strings.Join(conds, " OR "), Args: args}, nil
}

// deptAndChildCond 生成「本部门 + 所有 ancestors 包含本部门的子孙部门」子查询。
//
// 对应 Python 端 _dept_and_child_subquery：
//
//	SELECT id FROM sys_dept
//	WHERE del_flag = '0' AND (id = ? OR FIND_IN_SET(?, ancestors) != 0)
//
// ⚠️ FIND_IN_SET 的第一个参数在 Python 端是 str(int(dept_id))，即**字符串**，
// 这里保持一致（MySQL 会隐式转换，但契约比对看的是绑定参数的形态）。
//
// ⚠️ 别和 post/dept 模块里的「部门及以下」混用：那两处用的是
// (',' || ancestors || ',') LIKE '%,{id},%' 再手工 append 自身 ID，
// 是 Python 端另一套实现。两套并存是既有事实，各自对齐各自的调用点。
func deptAndChildCond(opt Option, deptID *int64) (string, []any) {
	if opt.DeptColumn == "" || deptID == nil {
		return "1 = 0", nil
	}
	sql := fmt.Sprintf(
		"%s IN (SELECT id FROM sys_dept WHERE del_flag = ? AND (id = ? OR FIND_IN_SET(?, ancestors) != 0))",
		opt.DeptColumn)
	return sql, []any{modelx.Normal, *deptID, strconv.FormatInt(*deptID, 10)}
}

// selfCond 返回「仅本人」的条件片段。
//
// 模型无 create_by 列时返回恒假条件——宁可查不到，不可越权。
func selfCond(opt Option, userID int64) string {
	if opt.CreateByColumn == "" {
		return "1 = 0"
	}
	return opt.CreateByColumn + " = ?"
}

// selfArgs 返回 selfCond 对应的绑定参数（恒假时无参数）。
func selfArgs(opt Option, userID int64) []any {
	if opt.CreateByColumn == "" {
		return nil
	}
	return []any{userID}
}

// selfOnly 生成「仅本人数据」的 Scope，是全部 fail-closed 分支的共同落点。
func selfOnly(opt Option, userID int64) Scope {
	cond := selfCond(opt, userID)
	args := selfArgs(opt, userID)
	if cond == "1 = 0" {
		return Scope{SQL: cond}
	}
	return Scope{SQL: cond, Args: args}
}
