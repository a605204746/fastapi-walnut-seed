package menu

import (
	"context"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 菜单域数据访问，对应 Python 端 module_system/menu/crud.py。
//
// P3 窄切片只需要 getRouters 用到的那一个查询；其余（列表、下拉树、
// 角色菜单、唯一性校验、增删改）随 menu 模块的 9 个端点一起补齐。
type Repo struct {
	db *gorm.DB
}

// NewRepo 构造数据访问层。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// SelectMenuTreeByUserID 查询构建前端路由所需的菜单，
// 对应 MenuCrud.select_menu_tree_by_user_id。
//
// 规则（逐条照抄）：
//   - 只要目录(M)与菜单(C)，按钮(F) 不进路由
//   - 只要状态正常的
//   - **非超管**再按会话里的 menu_ids 收敛；超管看全部
//   - 排序 parent_id, order_num —— 前端路由的展示顺序依赖它
//
// ⚠️ sys_menu 没有 del_flag 列，故不追加逻辑删除条件（与 Python 端一致）。
func (r *Repo) SelectMenuTreeByUserID(ctx context.Context, ac authctx.Context) ([]SysMenu, error) {
	q := r.db.WithContext(ctx).Model(&SysMenu{}).
		Where("menu_type IN ? AND status = ?", []string{TypeDir, TypeMenu}, modelx.Normal)

	if !ac.User.IsSuperAdmin() {
		// MenuIDs 为空时 GORM 生成 `IN (NULL)`（空集），
		// 与 Python 端 `MenuModel.id.in_(self.auth.menu_ids or [])` 的效果一致——
		// 没有任何授权菜单的用户拿到的就是空路由
		q = q.Where("id IN ?", ac.MenuIDs)
	}

	var menus []SysMenu
	if err := q.Order("parent_id ASC, order_num ASC").Find(&menus).Error; err != nil {
		return nil, err
	}
	return menus, nil
}
