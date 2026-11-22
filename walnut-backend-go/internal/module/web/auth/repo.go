package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	logmodel "walnut-seed-go/internal/module/system/log"
	"walnut-seed-go/internal/module/system/user"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
)

// Repo 认证模块的数据访问层（对应 Python 端 AuthService 里内联的 SQLAlchemy 查询）。
//
// 跨域只读查询（user/client/config/dept/role/menu）已按计划归还各域 repo，
// 经 provider.go 的窄接口由 bootstrap 注入；本文件只保留认证专属查询：
// 登录信息更新、注册查重/建号、登录日志写入。
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

// UpdateLoginInfo 更新最后登录 IP 与时间。
//
// update_by 设为用户自己的 id，与 Python 端 record_login_info 一致
// （登录是用户自身触发的操作，不是管理员代操作）。
func (r *Repo) UpdateLoginInfo(ctx context.Context, userID int64, ip string, at time.Time) error {
	return crudx.DB(ctx, r.db).WithContext(ctx).
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
	err := crudx.DB(ctx, r.db).WithContext(ctx).
		Model(&user.SysUser{}).
		Where("user_name = ? AND del_flag = ?", username, modelx.Normal).
		Count(&n).Error
	return n, err
}

// CreateUser 新建用户（注册）。
func (r *Repo) CreateUser(ctx context.Context, u *user.SysUser) error {
	return crudx.DB(ctx, r.db).WithContext(ctx).Create(u).Error
}

// InsertLogininfor 写登录日志。
func (r *Repo) InsertLogininfor(ctx context.Context, entry *logmodel.SysLogininfor) error {
	return crudx.DB(ctx, r.db).WithContext(ctx).Create(entry).Error
}
