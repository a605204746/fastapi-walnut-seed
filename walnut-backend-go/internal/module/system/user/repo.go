package user

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 用户域数据访问，对应 Python 端 module_system/user/crud.py。
//
// P3 窄切片只需要 getInfo 用到的三个只读查询；其余（分页列表、导出、
// 增删改、数据权限）随 user 模块的 21 个端点一起补齐。
type Repo struct {
	db *gorm.DB
}

// NewRepo 构造数据访问层。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// FindByID 按 ID 查未删除用户（**不带数据权限**），对应 UserCrud.get_user_by_id。
//
// 找不到返回 (nil, nil) 而不是 error：调用方要把「用户不存在」翻译成
// 业务文案（"没有权限访问用户数据!"），与「数据库出错」的 500 区分开。
func (r *Repo) FindByID(ctx context.Context, userID int64) (*SysUser, error) {
	var u SysUser
	err := r.db.WithContext(ctx).
		Where("id = ? AND del_flag = ?", userID, modelx.Normal).
		First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// FindRoles 查用户关联的角色，对应 UserModel.roles（lazy="selectin" 关系）。
//
// Python 端是「ORM 关系加载全部关联角色 → service 层按 del_flag 过滤」，
// 这里把过滤下沉到 SQL，结果集等价。
//
// ⚠️ 显式 ORDER BY id：SQLAlchemy 的 selectin 加载不带 order_by，
// 实际顺序由 MySQL 对主键 IN 的扫描决定（升序）。Go 侧显式排序，
// 让 roles 数组的顺序成为确定值而不是数据库实现的副产品。
func (r *Repo) FindRoles(ctx context.Context, userID int64) ([]role.SysRole, error) {
	var roles []role.SysRole
	err := r.db.WithContext(ctx).
		Table("sys_role").
		Select("sys_role.*").
		Joins("JOIN sys_user_role ON sys_user_role.role_id = sys_role.id").
		Where("sys_user_role.user_id = ? AND sys_role.del_flag = ?", userID, modelx.Normal).
		Order("sys_role.id ASC").
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// deptNameRow 是部门名批量回填的投影行。
type deptNameRow struct {
	ID       int64   `gorm:"column:id"`
	DeptName *string `gorm:"column:dept_name"`
}

// MapDeptNames 批量查部门名（仅未删除），对应 UserCrud.map_dept_names。
//
// 返回值保留 *string：部门名为 NULL 时映射到 nil，与「部门不存在」
// （键缺失）在出参上都表现为 deptName=null，与 Python 端
// `dept_names.get(user.dept_id)` 的语义一致。
func (r *Repo) MapDeptNames(ctx context.Context, deptIDs []int64) (map[int64]*string, error) {
	out := make(map[int64]*string, len(deptIDs))
	if len(deptIDs) == 0 {
		// 空切片不能生成 IN ()——那是非法 SQL
		return out, nil
	}
	var rows []deptNameRow
	err := r.db.WithContext(ctx).
		Model(&dept.SysDept{}).
		Select("id, dept_name").
		Where("del_flag = ? AND id IN ?", modelx.Normal, deptIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.DeptName
	}
	return out, nil
}
