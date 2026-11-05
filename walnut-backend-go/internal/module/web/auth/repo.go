package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/client"
	sysconfig "walnut-seed-go/internal/module/system/config"
	"walnut-seed-go/internal/module/system/dept"
	logmodel "walnut-seed-go/internal/module/system/log"
	"walnut-seed-go/internal/module/system/menu"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/module/system/user"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 认证模块的数据访问层（对应 Python 端 AuthService 里内联的 SQLAlchemy 查询）。
//
// P3 会把 user/role/menu 等模块的查询迁到各自的 repo.go，届时本文件只保留
// 认证专属的查询；当前先集中在这里，避免为几个只读查询提前建一堆空壳包。
type Repo struct {
	db *gorm.DB
}

// NewRepo 构造数据访问层。
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ErrNotFound 记录不存在。
//
// ⚠️ 必须用 gorm.ErrRecordNotFound 判定而不是「返回零值」：
// Go 的零值语义会掩盖「没找到」这一事实，让调用方拿着空结构体继续跑，
// 最终在某个字段上炸掉，错误信息完全指不到根因。
var ErrNotFound = errors.New("记录不存在")

// FindUserByUsername 按账号查可登录用户。
//
// 找不到返回 (nil, nil) 而**不是** error —— 调用方需要区分
// 「用户不存在」与「数据库出错」：前者要走防枚举的统一失败路径，后者要报 500。
// 这与 Python 端 `result.scalars().first()` 返回 None 的语义一致。
func (r *Repo) FindUserByUsername(ctx context.Context, username string) (*user.SysUser, error) {
	var u user.SysUser
	err := r.db.WithContext(ctx).
		Where("user_name = ? AND del_flag = ?", username, modelx.Normal).
		First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// FindClientByClientID 按 clientid 查客户端授权记录。
func (r *Repo) FindClientByClientID(ctx context.Context, clientID string) (*client.SysClient, error) {
	var c client.SysClient
	err := r.db.WithContext(ctx).
		Where("client_id = ? AND del_flag = ?", clientID, modelx.Normal).
		First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// GetConfigValue 读取参数配置值；不存在返回 ("", false, nil)。
func (r *Repo) GetConfigValue(ctx context.Context, key string) (string, bool, error) {
	var cfg sysconfig.SysConfig
	err := r.db.WithContext(ctx).
		Select("config_value").
		Where("config_key = ?", key).
		First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if cfg.ConfigValue == nil {
		return "", true, nil
	}
	return *cfg.ConfigValue, true, nil
}

// FindDeptByID 查部门（取 dept_name / dept_category 写入会话与 JWT claims）。
func (r *Repo) FindDeptByID(ctx context.Context, deptID int64) (*dept.SysDept, error) {
	var d dept.SysDept
	err := r.db.WithContext(ctx).Where("id = ?", deptID).First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// FindActiveRolesByUserID 查用户的有效角色（状态正常且未删除）。
//
// 对应 Python 端 build_session_info 里的 join 查询：
//
//	select RoleModel join UserRoleModel on UserRoleModel.role_id == RoleModel.id
//	where UserRoleModel.user_id == :uid and RoleModel.status == '0' and RoleModel.del_flag == '0'
func (r *Repo) FindActiveRolesByUserID(ctx context.Context, userID int64) ([]role.SysRole, error) {
	var roles []role.SysRole
	err := r.db.WithContext(ctx).
		Table("sys_role").
		Select("sys_role.*").
		Joins("JOIN sys_user_role ON sys_user_role.role_id = sys_role.id").
		Where("sys_user_role.user_id = ? AND sys_role.status = ? AND sys_role.del_flag = ?",
			userID, modelx.Normal, modelx.Normal).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// FindMenusByRoleIDs 查角色集合关联的有效菜单。
//
// roleIDs 为空时直接返回空切片——**不能**生成 `IN ()`，
// 那是非法 SQL，MySQL 会报语法错误。
func (r *Repo) FindMenusByRoleIDs(ctx context.Context, roleIDs []int64) ([]menu.SysMenu, error) {
	if len(roleIDs) == 0 {
		return []menu.SysMenu{}, nil
	}
	var menus []menu.SysMenu
	err := r.db.WithContext(ctx).
		Table("sys_menu").
		Select("sys_menu.*").
		Joins("JOIN sys_role_menu ON sys_role_menu.menu_id = sys_menu.id").
		Where("sys_role_menu.role_id IN ? AND sys_menu.status = ?", roleIDs, modelx.Normal).
		Find(&menus).Error
	if err != nil {
		return nil, err
	}
	return menus, nil
}

// FindAllActiveMenuIDs 查全部有效菜单 ID（超管用）。
func (r *Repo) FindAllActiveMenuIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).
		Model(&menu.SysMenu{}).
		Where("status = ?", modelx.Normal).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// UpdateLoginInfo 更新最后登录 IP 与时间。
//
// update_by 设为用户自己的 id，与 Python 端 record_login_info 一致
// （登录是用户自身触发的操作，不是管理员代操作）。
func (r *Repo) UpdateLoginInfo(ctx context.Context, userID int64, ip string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&user.SysUser{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"login_ip":   ip,
			"login_date": at,
			"update_by":  userID,
		}).Error
}

// CountActiveUsersByUsername 统计同名未删除用户数（注册查重用）。
func (r *Repo) CountActiveUsersByUsername(ctx context.Context, username string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&user.SysUser{}).
		Where("user_name = ? AND del_flag = ?", username, modelx.Normal).
		Count(&n).Error
	return n, err
}

// CreateUser 新建用户（注册）。
func (r *Repo) CreateUser(ctx context.Context, u *user.SysUser) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// InsertLogininfor 写登录日志。
func (r *Repo) InsertLogininfor(ctx context.Context, entry *logmodel.SysLogininfor) error {
	return r.db.WithContext(ctx).Create(entry).Error
}
