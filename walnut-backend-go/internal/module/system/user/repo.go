package user

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond, scope permx.Scope) ([]SysUser, int64, error) {
	newQuery := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&SysUser{}).Where("del_flag = ?", modelx.Normal)
		q = scope.Apply(q)
		return crudx.Apply(q, conds...)
	}
	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := newQuery().Order("id ASC")
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}
	rows := make([]SysUser, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

type ExportRow struct {
	User       SysUser
	DeptName   *string
	LeaderName *string
}

func (r *Repo) ListExport(ctx context.Context, conds []crudx.Cond, scope permx.Scope) ([]ExportRow, error) {
	type joinRow struct {
		SysUser
		DeptName   *string `gorm:"column:dept_name"`
		LeaderName *string `gorm:"column:leader_name"`
	}
	q := r.db.WithContext(ctx).
		Table("sys_user u").
		Select("u.*, d.dept_name, leader.nick_name AS leader_name").
		Joins("LEFT JOIN sys_dept d ON d.id = u.dept_id AND d.del_flag = ?", modelx.Normal).
		Joins("LEFT JOIN sys_user leader ON leader.id = d.leader").
		Where("u.del_flag = ?", modelx.Normal)
	q = scope.Apply(q)
	q = crudx.Apply(q, conds...)
	q = q.Order("u.id ASC")
	var rows []joinRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ExportRow, 0, len(rows))
	for _, jr := range rows {
		out = append(out, ExportRow{User: jr.SysUser, DeptName: jr.DeptName, LeaderName: jr.LeaderName})
	}
	return out, nil
}

func (r *Repo) FindByID(ctx context.Context, userID int64) (*SysUser, error) {
	var u SysUser
	err := r.db.WithContext(ctx).Where("id = ? AND del_flag = ?", userID, modelx.Normal).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repo) FindByIDWithScope(ctx context.Context, userID int64, scope permx.Scope) (*SysUser, error) {
	var u SysUser
	q := r.db.WithContext(ctx).Where("id = ? AND del_flag = ?", userID, modelx.Normal)
	q = scope.Apply(q)
	if err := q.First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repo) CountByIDWithScope(ctx context.Context, userID int64, scope permx.Scope) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Model(&SysUser{}).Where("id = ? AND del_flag = ?", userID, modelx.Normal)
	q = scope.Apply(q)
	err := q.Count(&n).Error
	return n, err
}

func (r *Repo) FindByUserName(ctx context.Context, userName string) (*SysUser, error) {
	var u SysUser
	err := r.db.WithContext(ctx).Where("user_name = ? AND del_flag = ?", userName, modelx.Normal).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repo) ExistsByField(ctx context.Context, field, value string, excludeID *int64) (bool, error) {
	if value == "" {
		return false, nil
	}
	q := r.db.WithContext(ctx).Model(&SysUser{}).
		Where(field+" = ? AND del_flag = ?", value, modelx.Normal)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repo) ListByDeptID(ctx context.Context, deptID int64) ([]SysUser, error) {
	rows := make([]SysUser, 0)
	err := r.db.WithContext(ctx).Where("dept_id = ? AND del_flag = ?", deptID, modelx.Normal).
		Order("id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repo) ListOptionByIDs(ctx context.Context, ids []int64, deptID *int64) ([]SysUser, error) {
	q := r.db.WithContext(ctx).Model(&SysUser{}).
		Where("status = ? AND del_flag = ?", modelx.Normal, modelx.Normal)
	if deptID != nil {
		q = q.Where("dept_id = ?", *deptID)
	}
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	rows := make([]SysUser, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) Create(ctx context.Context, row *SysUser) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *Repo) Update(ctx context.Context, row *SysUser) error {
	return r.db.WithContext(ctx).Save(row).Error
}

func (r *Repo) UpdatePassword(ctx context.Context, userID int64, hashedPwd string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&SysUser{}).
		Where("id = ? AND del_flag = ?", userID, modelx.Normal).
		Update("password", hashedPwd)
	return res.RowsAffected, res.Error
}

func (r *Repo) UpdateStatus(ctx context.Context, userID int64, status string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&SysUser{}).
		Where("id = ? AND del_flag = ?", userID, modelx.Normal).
		Update("status", status)
	return res.RowsAffected, res.Error
}

func (r *Repo) UpdateAvatar(ctx context.Context, userID int64, avatar string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&SysUser{}).
		Where("id = ? AND del_flag = ?", userID, modelx.Normal).
		Update("avatar", avatar)
	return res.RowsAffected, res.Error
}

func (r *Repo) UpdateProfile(ctx context.Context, userID int64, nickName *string, phone *string, email *string, sex *string) (int64, error) {
	updates := map[string]any{}
	if nickName != nil {
		updates["nick_name"] = *nickName
	}
	if phone != nil {
		updates["phonenumber"] = *phone
	}
	if email != nil {
		updates["email"] = *email
	}
	if sex != nil {
		updates["sex"] = *sex
	}
	if len(updates) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&SysUser{}).
		Where("id = ? AND del_flag = ?", userID, modelx.Normal).
		Updates(updates)
	return res.RowsAffected, res.Error
}

func (r *Repo) SoftDelete(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&SysUser{}).
		Where("id IN ? AND del_flag = ?", ids, modelx.Normal).
		Update("del_flag", "1")
	return res.RowsAffected, res.Error
}

// ---- sys_user_role ----

func (r *Repo) FindRoles(ctx context.Context, userID int64) ([]role.SysRole, error) {
	var roles []role.SysRole
	err := r.db.WithContext(ctx).
		Table("sys_role").
		Select("sys_role.*").
		Joins("JOIN sys_user_role ON sys_user_role.role_id = sys_role.id").
		Where("sys_user_role.user_id = ? AND sys_role.del_flag = ?", userID, modelx.Normal).
		Order("sys_role.id ASC").
		Find(&roles).Error
	return roles, err
}

func (r *Repo) RoleIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	ids := make([]int64, 0)
	err := r.db.WithContext(ctx).Table("sys_user_role").
		Where("user_id = ?", userID).Pluck("role_id", &ids).Error
	return ids, err
}

func (r *Repo) DeleteUserRoles(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&SysUserRole{}).Error
}

func (r *Repo) InsertUserRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	rows := make([]SysUserRole, 0, len(roleIDs))
	for _, rid := range roleIDs {
		rows = append(rows, SysUserRole{UserID: userID, RoleID: rid})
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}

func (r *Repo) DeleteUserRolesBatch(ctx context.Context, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("user_id IN ?", userIDs).Delete(&SysUserRole{}).Error
}

// ---- sys_user_post ----

func (r *Repo) PostIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	ids := make([]int64, 0)
	err := r.db.WithContext(ctx).Table("sys_user_post").
		Where("user_id = ?", userID).Pluck("post_id", &ids).Error
	return ids, err
}

func (r *Repo) DeleteUserPosts(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&SysUserPost{}).Error
}

func (r *Repo) InsertUserPosts(ctx context.Context, userID int64, postIDs []int64) error {
	if len(postIDs) == 0 {
		return nil
	}
	rows := make([]SysUserPost, 0, len(postIDs))
	for _, pid := range postIDs {
		rows = append(rows, SysUserPost{UserID: userID, PostID: pid})
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}

func (r *Repo) DeleteUserPostsBatch(ctx context.Context, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("user_id IN ?", userIDs).Delete(&SysUserPost{}).Error
}

// ---- 部门辅助 ----

func (r *Repo) MapDeptNames(ctx context.Context, deptIDs []int64) (map[int64]*string, error) {
	out := make(map[int64]*string, len(deptIDs))
	if len(deptIDs) == 0 {
		return out, nil
	}
	type row struct {
		ID       int64   `gorm:"column:id"`
		DeptName *string `gorm:"column:dept_name"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&dept.SysDept{}).
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

func (r *Repo) DescendantAndSelfIDs(ctx context.Context, deptID int64) ([]int64, error) {
	ids := make([]int64, 0)
	pattern := "%," + strconv.FormatInt(deptID, 10) + ",%"
	err := r.db.WithContext(ctx).Table("sys_dept").
		Where("del_flag = ? AND CONCAT(',', ancestors, ',') LIKE ?", modelx.Normal, pattern).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return append(ids, deptID), nil
}

func (r *Repo) CountVisibleRoles(ctx context.Context, roleIDs []int64, scope permx.Scope) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Model(&role.SysRole{}).
		Where("id IN ? AND del_flag = ?", roleIDs, modelx.Normal)
	q = scope.Apply(q)
	err := q.Count(&n).Error
	return n, err
}

func (r *Repo) CountVisiblePosts(ctx context.Context, postIDs []int64, scope permx.Scope) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Table("sys_post").Where("id IN ?", postIDs)
	q = scope.Apply(q)
	err := q.Count(&n).Error
	return n, err
}

func (r *Repo) RoleNameGroup(ctx context.Context, userID int64) (string, error) {
	var names []string
	err := r.db.WithContext(ctx).Table("sys_role").
		Select("role_name").
		Joins("JOIN sys_user_role ON sys_user_role.role_id = sys_role.id").
		Where("sys_user_role.user_id = ? AND sys_role.del_flag = ?", userID, modelx.Normal).
		Order("sys_role.id ASC").
		Pluck("role_name", &names).Error
	if err != nil {
		return "", err
	}
	join := ""
	for i, n := range names {
		if i > 0 {
			join += ","
		}
		join += n
	}
	return join, nil
}

func (r *Repo) PostNameGroup(ctx context.Context, userID int64) (string, error) {
	var names []string
	err := r.db.WithContext(ctx).Table("sys_post").
		Select("post_name").
		Joins("JOIN sys_user_post ON sys_user_post.post_id = sys_post.id").
		Where("sys_user_post.user_id = ?", userID).
		Pluck("post_name", &names).Error
	if err != nil {
		return "", err
	}
	join := ""
	for i, n := range names {
		if i > 0 {
			join += ","
		}
		join += n
	}
	return join, nil
}

func (r *Repo) AllNormalRoles(ctx context.Context) ([]role.SysRole, error) {
	rows := make([]role.SysRole, 0)
	err := r.db.WithContext(ctx).Model(&role.SysRole{}).
		Where("del_flag = ? AND status = ?", modelx.Normal, modelx.Normal).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "role_sort"}},
			{Column: clause.Column{Name: "create_time"}},
		}}).
		Find(&rows).Error
	return rows, err
}

func (r *Repo) PostsByDeptID(ctx context.Context, deptID int64) ([]PostInfo, error) {
	rows := make([]PostInfo, 0)
	err := r.db.WithContext(ctx).Table("sys_post").
		Where("dept_id = ?", deptID).
		Find(&rows).Error
	return rows, err
}

type PostInfo struct {
	ID       int64  `gorm:"column:id"`
	PostName string `gorm:"column:post_name"`
	PostCode string `gorm:"column:post_code"`
}

type DeptTreeNode struct {
	ID       int64   `gorm:"column:id"`
	ParentID *int64  `gorm:"column:parent_id"`
	DeptName *string `gorm:"column:dept_name"`
	OrderNum *int    `gorm:"column:order_num"`
	Status   *string `gorm:"column:status"`
}

func (r *Repo) DeptTreeNodes(ctx context.Context) ([]DeptTreeNode, error) {
	rows := make([]DeptTreeNode, 0)
	err := r.db.WithContext(ctx).Table("sys_dept").
		Select("id, parent_id, dept_name, order_num, status").
		Where("del_flag = ?", modelx.Normal).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "parent_id"}},
			{Column: clause.Column{Name: "order_num"}},
			{Column: clause.Column{Name: "id"}},
		}}).
		Find(&rows).Error
	return rows, err
}

func (r *Repo) LogininforByUserName(ctx context.Context, userName string) *string {
	var s string
	err := r.db.WithContext(ctx).Table("sys_logininfor").
		Where("user_name = ?", userName).
		Order("login_time DESC").
		Pluck("DATE_FORMAT(login_time, '%Y-%m-%d %H:%i:%s')", &s).Error
	if err != nil {
		return nil
	}
	return &s
}

// ConfigValueByKey 按键名查 sys_config 的 config_value，对应 Python 端
// ConfigCrud.select_config_by_key。不存在返回 ("", nil)。
func (r *Repo) ConfigValueByKey(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.WithContext(ctx).Table("sys_config").
		Select("config_value").
		Where("config_key = ?", key).
		Limit(1).
		Pluck("config_value", &value).Error
	if err != nil {
		return "", err
	}
	return value, nil
}
