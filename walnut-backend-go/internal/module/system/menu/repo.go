package menu

import (
	"context"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
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

// ==================== 管理端查询（menu 模块 8 端点） ====================

// SelectMenuList 菜单列表，对应 MenuCrud.select_menu_list。
//
// ⚠️ 非超管按会话 menu_ids 收敛；排序 parent_id, order_num（**无 id 兜底**）。
func (r *Repo) SelectMenuList(ctx context.Context, ac authctx.Context, q Query) ([]SysMenu, error) {
	query := r.db.WithContext(ctx).Model(&SysMenu{})
	if !ac.User.IsSuperAdmin() {
		query = query.Where("id IN ?", ac.MenuIDs)
	}
	if crudx.IsNotBlank(q.MenuName) {
		query = query.Where("menu_name LIKE ?", "%"+q.MenuName+"%")
	}
	if crudx.IsNotBlank(q.Visible) {
		query = query.Where("visible = ?", q.Visible)
	}
	if crudx.IsNotBlank(q.Status) {
		query = query.Where("status = ?", q.Status)
	}
	if crudx.IsNotBlank(q.MenuType) {
		query = query.Where("menu_type = ?", q.MenuType)
	}
	if q.ParentID != nil {
		query = query.Where("parent_id = ?", *q.ParentID)
	}
	var menus []SysMenu
	if err := query.Order("parent_id ASC, order_num ASC").Find(&menus).Error; err != nil {
		return nil, err
	}
	return menus, nil
}

// SelectMenuByID 按主键查询（sys_menu 无 del_flag）。
func (r *Repo) SelectMenuByID(ctx context.Context, id int64) (*SysMenu, error) {
	var row SysMenu
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// HasChildByMenuID 是否存在子菜单。
func (r *Repo) HasChildByMenuID(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&SysMenu{}).Where("parent_id = ?", id).Count(&n).Error
	return n > 0, err
}

// HasChildByMenuIDs 级联删除前的子菜单检查：parent 在集合内且自身不在集合内。
func (r *Repo) HasChildByMenuIDs(ctx context.Context, ids []int64) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&SysMenu{}).
		Where("parent_id IN ? AND id NOT IN ?", ids, ids).Count(&n).Error
	return n > 0, err
}

// CheckMenuExistRole 菜单是否已分配给角色（sys_role_menu）。
func (r *Repo) CheckMenuExistRole(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("sys_role_menu").Where("menu_id = ?", id).Count(&n).Error
	return n > 0, err
}

// MenuIDsByRoleID 角色已勾选菜单 ID，对应 select_menu_list_by_role_id。
//
// ⚠️ 三段语义照抄：授权子查询带 LEFT JOIN role 且 role.status='0'；
// menu_check_strictly 为真时再剔掉「作为父节点」的菜单。
func (r *Repo) MenuIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	// ⚠️ 用 Pluck 取标量：GORM 的 First 不接受 *int 目的地（"model value required"，
	// 与 notice 的 createByName 反查同一个坑，契约 diff 实测抓到）
	flags := make([]int, 0, 1)
	err := r.db.WithContext(ctx).Table("sys_role").Select("menu_check_strictly").
		Where("id = ?", roleID).Limit(1).Pluck("menu_check_strictly", &flags).Error
	if err != nil {
		return nil, err
	}
	strictly := len(flags) > 0 && flags[0] != 0
	assigned := r.db.WithContext(ctx).Table("sys_role_menu AS rm").
		Select("rm.menu_id").
		Joins("LEFT JOIN sys_role r ON r.id = rm.role_id").
		Where("rm.role_id = ? AND r.status = ?", roleID, modelx.Normal)

	ids := make([]int64, 0)
	q := r.db.WithContext(ctx).Model(&SysMenu{}).Select("id").
		Where("id IN (?)", assigned).
		Order("parent_id ASC, order_num ASC")
	if strictly {
		parents := r.db.WithContext(ctx).Model(&SysMenu{}).Select("parent_id").
			Where("id IN (?)", assigned)
		q = q.Where("id NOT IN (?)", parents)
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// ExistsMenuName 同级同名菜单是否存在，对应 exists_menu_name。
//
// ⚠️ parent_id 为 None 时 Python 端生成 IS NULL，照抄。
func (r *Repo) ExistsMenuName(ctx context.Context, name string, parentID, excludeID *int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&SysMenu{}).Where("menu_name = ?", name)
	if parentID == nil {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", *parentID)
	}
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// RouteConflictCandidates 路由冲突候选（M/C 且 path 命中 path 或 routeName）。
func (r *Repo) RouteConflictCandidates(ctx context.Context, path, routeName string) ([]SysMenu, error) {
	var rows []SysMenu
	err := r.db.WithContext(ctx).Model(&SysMenu{}).
		Where("menu_type IN ? AND (path = ? OR path = ?)", []string{TypeDir, TypeMenu}, path, routeName).
		Find(&rows).Error
	return rows, err
}

// Create 新增（审计字段由调用方 FillCreate）。
func (r *Repo) Create(ctx context.Context, row *SysMenu) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 整体保存。
func (r *Repo) Update(ctx context.Context, row *SysMenu) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// Delete 物理删除单条（sys_menu 无 del_flag）。
func (r *Repo) Delete(ctx context.Context, id int64) (bool, error) {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&SysMenu{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// DeleteBatch 批量物理删除。
func (r *Repo) DeleteBatch(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&SysMenu{})
	return res.RowsAffected, res.Error
}

// DeleteRoleMenuByMenuIDs 清理角色-菜单关联，对应 delete_role_menu_by_menu_ids。
func (r *Repo) DeleteRoleMenuByMenuIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Table("sys_role_menu").Where("menu_id IN ?", ids).Delete(nil)
	return res.RowsAffected, res.Error
}
