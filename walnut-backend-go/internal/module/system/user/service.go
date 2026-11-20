package user

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/post"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/excelx"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

var nowFunc = time.Now

const bcryptCost = 12

type Service struct {
	db   *gorm.DB
	repo *Repo
	auth authctx.Context
}

func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{db: db, repo: NewRepo(db), auth: ac}
}

type PageOut struct {
	Rows  []Out        `json:"rows"`
	Total idtype.JSInt `json:"total"`
}

func (s *Service) buildScope(ctx context.Context) (permx.Scope, error) {
	return permx.Build(ctx, s.repo.db, s.auth, permx.UserList)
}

func (s *Service) buildConditions(ctx context.Context, q QueryParam) ([]crudx.Cond, error) {
	conds := make([]crudx.Cond, 0, 8)
	if q.ID != nil {
		conds = append(conds, crudx.Eq("id", *q.ID))
	}
	if crudx.IsNotBlank(q.UserName) {
		conds = append(conds, crudx.Like("user_name", q.UserName))
	}
	if crudx.IsNotBlank(q.NickName) {
		conds = append(conds, crudx.Like("nick_name", q.NickName))
	}
	if crudx.IsNotBlank(q.Status) {
		conds = append(conds, crudx.Eq("status", q.Status))
	}
	if crudx.IsNotBlank(q.PhoneNumber) {
		conds = append(conds, crudx.Like("phonenumber", q.PhoneNumber))
	}
	if q.BeginTime != "" && q.EndTime != "" {
		conds = append(conds, crudx.Between("create_time", q.BeginTime, q.EndTime))
	}
	if q.DeptID != nil {
		ids, err := s.repo.DescendantAndSelfIDs(ctx, *q.DeptID)
		if err != nil {
			return nil, err
		}
		conds = append(conds, crudx.In("dept_id", ids))
	}
	if len(q.UserIDs) > 0 {
		conds = append(conds, crudx.In("id", q.UserIDs))
	}
	if len(q.ExcludeUserIDs) > 0 {
		for _, eid := range q.ExcludeUserIDs {
			conds = append(conds, crudx.Cond{SQL: "id <> ?", Args: []any{eid}})
		}
	}
	return conds, nil
}

func (s *Service) PageList(ctx context.Context, q QueryParam) (*PageOut, error) {
	scope, err := s.buildScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	conds, err := s.buildConditions(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.repo.Page(ctx, q.Page, conds, scope)
	if err != nil {
		return nil, err
	}
	outs := s.toOuts(ctx, rows)
	for i := range outs {
		outs[i].Roles = nil
	}
	if err := s.fillDeptName(ctx, outs); err != nil {
		return nil, err
	}
	valOuts := make([]Out, 0, len(outs))
	for _, o := range outs {
		valOuts = append(valOuts, *o)
	}
	return &PageOut{Rows: valOuts, Total: idtype.JSInt(total)}, nil
}

func (s *Service) SelectList(ctx context.Context, q QueryParam) ([]ExportRow, error) {
	scope, err := s.buildScope(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	conds, err := s.buildConditions(ctx, q)
	if err != nil {
		return nil, err
	}
	return s.repo.ListExport(ctx, conds, scope)
}

func (s *Service) ListByDeptID(ctx context.Context, deptID int64) ([]Out, error) {
	rows, err := s.repo.ListByDeptID(ctx, deptID)
	if err != nil {
		return nil, err
	}
	outs := s.toOuts(ctx, rows)
	for i := range outs {
		outs[i].Roles = nil
	}
	if err := s.fillDeptName(ctx, outs); err != nil {
		return nil, err
	}
	valOuts := make([]Out, 0, len(outs))
	for _, o := range outs {
		valOuts = append(valOuts, *o)
	}
	return valOuts, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*UserInfoOut, error) {
	if err := s.CheckUserDataScope(ctx, id); err != nil {
		return nil, err
	}
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	info := &UserInfoOut{}
	if u != nil {
		out := NewOut(u)
		if err := s.fillDeptName(ctx, []*Out{&out}); err != nil {
			return nil, err
		}
		roles, err := s.repo.FindRoles(ctx, id)
		if err != nil {
			return nil, err
		}
		roleOuts := make([]role.Out, 0, len(roles))
		for i := range roles {
			roleOuts = append(roleOuts, role.NewOut(&roles[i]))
		}
		out.Roles = roleOuts
		// user 对象内的 roleIds/postIds/roleId 恒 null（Python select_user_by_id 不填充）
		info.User = &out
	}

	roleIDs, err := s.repo.RoleIDsByUserID(ctx, id)
	if err != nil {
		return nil, err
	}
	info.RoleIDs = toJSIntSlice(roleIDs)

	if u != nil && u.DeptID != nil {
		posts, err := post.NewService(s.db, s.auth).SelectList(ctx, post.QueryParam{DeptID: u.DeptID})
		if err != nil {
			return nil, err
		}
		info.Posts = posts
		postIDs, err := s.repo.PostIDsByUserID(ctx, id)
		if err != nil {
			return nil, err
		}
		info.PostIDs = toJSIntSlice(postIDs)
	}

	allRoles, err := s.scopedRoles(ctx, modelx.Normal)
	if err != nil {
		return nil, err
	}
	// 超管角色过滤按**目标用户 id** 判定（与当前登录者无关）
	if id != modelx.SuperAdminID {
		allRoles = filterOutSuperAdminOut(allRoles)
	}
	info.Roles = allRoles
	return info, nil
}

func (s *Service) GetUserInfoNoID(ctx context.Context) (*UserInfoOut, error) {
	allRoles, err := s.scopedRoles(ctx, modelx.Normal)
	if err != nil {
		return nil, err
	}
	// GET / 无目标用户 → Python _is_super_admin_id(None)=False → 恒过滤超管角色
	allRoles = filterOutSuperAdminOut(allRoles)
	return &UserInfoOut{Roles: allRoles}, nil
}

// scopedRoles 经 RoleService.SelectList 取角色（含角色数据权限），
// status 传空串表示不过滤状态（authRole 用）。
func (s *Service) scopedRoles(ctx context.Context, status string) ([]role.Out, error) {
	rows, err := role.NewService(s.db, s.auth).SelectList(ctx, role.QueryParam{Status: status})
	if err != nil {
		return nil, err
	}
	outs := make([]role.Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, role.NewOut(&rows[i]))
	}
	return outs, nil
}

func (s *Service) GetInfo(ctx context.Context) (*GetInfoOut, error) {
	u, err := s.repo.FindByID(ctx, s.auth.User.ID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	out := NewOut(u)
	roles, err := s.repo.FindRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	roleOuts := make([]role.Out, 0, len(roles))
	for i := range roles {
		roleOuts = append(roleOuts, role.NewOut(&roles[i]))
	}
	out.Roles = roleOuts
	if u.DeptID != nil {
		names, err := s.repo.MapDeptNames(ctx, []int64{*u.DeptID})
		if err != nil {
			return nil, err
		}
		out.DeptName = names[*u.DeptID]
	}
	return &GetInfoOut{
		User:        &out,
		Permissions: nonNilStrings(s.auth.Permissions),
		Roles:       nonNilStrings(s.auth.Roles),
	}, nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func (s *Service) GetAuthRole(ctx context.Context, userID int64) (*UserInfoOut, error) {
	if err := s.CheckUserDataScope(ctx, userID); err != nil {
		return nil, err
	}
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	info := &UserInfoOut{}
	if u != nil {
		out := NewOut(u)
		if err := s.fillDeptName(ctx, []*Out{&out}); err != nil {
			return nil, err
		}
		roles, err := s.repo.FindRoles(ctx, userID)
		if err != nil {
			return nil, err
		}
		roleOuts := make([]role.Out, 0, len(roles))
		for i := range roles {
			roleOuts = append(roleOuts, role.NewOut(&roles[i]))
		}
		out.Roles = roleOuts
		info.User = &out
	}
	userRoleIDs, err := s.repo.RoleIDsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Python _select_all_role_models：全部状态 + 角色数据权限
	allRoles, err := s.scopedRoles(ctx, "")
	if err != nil {
		return nil, err
	}
	if userID != modelx.SuperAdminID {
		allRoles = filterOutSuperAdminOut(allRoles)
	}
	roleIDSet := make(map[int64]bool, len(userRoleIDs))
	for _, rid := range userRoleIDs {
		roleIDSet[rid] = true
	}
	roleOuts := make([]role.Out, 0, len(allRoles))
	for i := range allRoles {
		o := allRoles[i]
		if o.ID != nil && roleIDSet[int64(*o.ID)] {
			o.Flag = true
		}
		roleOuts = append(roleOuts, o)
	}
	info.Roles = roleOuts
	return info, nil
}

func (s *Service) OptionSelect(ctx context.Context, ids []int64, deptID *int64) ([]Out, error) {
	rows, err := s.repo.ListOptionByIDs(ctx, ids, deptID)
	if err != nil {
		return nil, err
	}
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		o := NewOut(&rows[i])
		o.SlimOnly()
		outs = append(outs, o)
	}
	return outs, nil
}

func (s *Service) InsertUser(ctx context.Context, in CreateIn) (bool, error) {
	// Python controller 先查 userName/password 缺省（ServiceException 500），
	// 再做部门数据权限与唯一性校验
	if in.UserName == nil {
		return false, errs.New("用户账号不能为空")
	}
	if in.Password == nil {
		return false, errs.New("用户密码不能为空")
	}
	if in.DeptID != nil {
		if err := s.checkDeptDataScope(ctx, *in.DeptID); err != nil {
			return false, err
		}
	}
	exists, err := s.repo.ExistsByField(ctx, "user_name", *in.UserName, nil)
	if err != nil {
		return false, err
	}
	if exists {
		return false, errs.Newf("新增用户'%s'失败，登录账号已存在", *in.UserName)
	}
	if in.PhoneNumber != nil && *in.PhoneNumber != "" {
		exists, err = s.repo.ExistsByField(ctx, "phonenumber", *in.PhoneNumber, nil)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.Newf("新增用户'%s'失败，手机号码已存在", *in.UserName)
		}
	}
	if in.Email != nil && *in.Email != "" {
		exists, err = s.repo.ExistsByField(ctx, "email", *in.Email, nil)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.Newf("新增用户'%s'失败，邮箱账号已存在", *in.UserName)
		}
	}
	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(*in.Password), bcryptCost)
	if err != nil {
		return false, err
	}
	pwdStr := string(hashedPwd)
	row := SysUser{
		UserType: "sys_user",
		Status:   modelx.Normal,
		DelFlag:  modelx.Normal,
		Password: &pwdStr,
	}
	applyCreateFields(&row, in)
	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return false, err
	}
	// Python insert_user 顺序：先岗位后角色（含可见性/超管校验，缺省 no-op）
	if err := s.rebuildUserPosts(ctx, row.ID, in.PostIDs); err != nil {
		return false, err
	}
	if err := s.rebuildUserRoles(ctx, row.ID, in.RoleIDs); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) UpdateUser(ctx context.Context, in UpdateIn) (bool, error) {
	if in.ID == nil {
		return false, errs.New("用户ID不能为空")
	}
	if err := s.CheckUserAllowed(ctx, *in.ID); err != nil {
		return false, err
	}
	if err := s.CheckUserDataScope(ctx, *in.ID); err != nil {
		return false, err
	}
	if in.DeptID != nil {
		if err := s.checkDeptDataScope(ctx, *in.DeptID); err != nil {
			return false, err
		}
	}
	if in.UserName != nil {
		exists, err := s.repo.ExistsByField(ctx, "user_name", *in.UserName, in.ID)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.Newf("修改用户'%s'失败，登录账号已存在", *in.UserName)
		}
	}
	if in.PhoneNumber != nil && *in.PhoneNumber != "" {
		exists, err := s.repo.ExistsByField(ctx, "phonenumber", *in.PhoneNumber, in.ID)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.Newf("修改用户'%s'失败，手机号码已存在", derefStr(in.UserName))
		}
	}
	if in.Email != nil && *in.Email != "" {
		exists, err := s.repo.ExistsByField(ctx, "email", *in.Email, in.ID)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.Newf("修改用户'%s'失败，邮箱账号已存在", derefStr(in.UserName))
		}
	}
	scope, err := s.buildScope(ctx)
	if err != nil {
		return false, errs.Wrap(err, errs.MsgDBFailed)
	}
	// Python update_user 先重建关联（缺省 no-op），再取用户、更新字段
	if err := s.rebuildUserRoles(ctx, *in.ID, in.RoleIDs); err != nil {
		return false, err
	}
	if err := s.rebuildUserPosts(ctx, *in.ID, in.PostIDs); err != nil {
		return false, err
	}
	u, err := s.repo.FindByIDWithScope(ctx, *in.ID, scope)
	if err != nil {
		return false, err
	}
	if u == nil {
		return false, errs.Newf("修改用户%s信息失败", derefStr(in.UserName))
	}
	applyUpdateFields(u, in)
	crudx.FillUpdate(&u.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, u); err != nil {
		return false, err
	}
	return true, nil
}

// rebuildUserRoles 复刻 Python _insert_user_role(clear=True)：
// null/缺省 → no-op 保留原关联；非超管目标剔除超管角色，剔空报错；
// 数据权限可见数不足报错；最后先删后插。
func (s *Service) rebuildUserRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	list := roleIDs
	if userID != modelx.SuperAdminID {
		list = filterSuperAdminFromIDs(list)
	}
	if len(list) == 0 {
		return errs.New("不允许为普通用户分配超级管理员角色，请至少选择一个其他角色")
	}
	if err := s.checkRolesVisible(ctx, list); err != nil {
		return err
	}
	if err := s.repo.DeleteUserRoles(ctx, userID); err != nil {
		return err
	}
	return s.repo.InsertUserRoles(ctx, userID, list)
}

// rebuildUserPosts 复刻 Python _insert_user_post(clear=True)。
func (s *Service) rebuildUserPosts(ctx context.Context, userID int64, postIDs []int64) error {
	if len(postIDs) == 0 {
		return nil
	}
	if err := s.checkPostsVisible(ctx, postIDs); err != nil {
		return err
	}
	if err := s.repo.DeleteUserPosts(ctx, userID); err != nil {
		return err
	}
	return s.repo.InsertUserPosts(ctx, userID, postIDs)
}

// checkRolesVisible 对齐 count_visible_roles：del_flag='0' + 数据权限行级过滤，
// 可见数 != 请求数 → 500「没有权限访问角色的数据」。
func (s *Service) checkRolesVisible(ctx context.Context, ids []int64) error {
	scope, err := permx.Build(ctx, s.db, s.auth, permx.Default)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	var n int64
	q := s.db.WithContext(ctx).Table("sys_role").
		Where("id IN ? AND del_flag = ?", ids, modelx.Normal)
	if err := scope.Apply(q).Count(&n).Error; err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if n != int64(len(ids)) {
		return errs.New("没有权限访问角色的数据")
	}
	return nil
}

// checkPostsVisible 对齐 count_visible_posts（sys_post 无 del_flag 列）。
func (s *Service) checkPostsVisible(ctx context.Context, ids []int64) error {
	scope, err := permx.Build(ctx, s.db, s.auth, permx.Default)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	var n int64
	q := s.db.WithContext(ctx).Table("sys_post").Where("id IN ?", ids)
	if err := scope.Apply(q).Count(&n).Error; err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if n != int64(len(ids)) {
		return errs.New("没有权限访问岗位的数据")
	}
	return nil
}

func (s *Service) DeleteUserByIDs(ctx context.Context, ids []int64) (int64, error) {
	for _, id := range ids {
		if id == s.auth.User.ID {
			return 0, errs.New("当前用户不能删除")
		}
		if err := s.CheckUserAllowed(ctx, id); err != nil {
			return 0, err
		}
		if err := s.CheckUserDataScope(ctx, id); err != nil {
			return 0, err
		}
	}
	if err := s.repo.DeleteUserRolesBatch(ctx, ids); err != nil {
		return 0, err
	}
	if err := s.repo.DeleteUserPostsBatch(ctx, ids); err != nil {
		return 0, err
	}
	n, err := s.repo.SoftDelete(ctx, ids)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, errs.New("删除用户失败!")
	}
	return n, nil
}

func (s *Service) ResetPwd(ctx context.Context, in ResetPwdIn) (bool, error) {
	if in.ID == nil {
		return false, nil
	}
	if in.Password == nil || *in.Password == "" {
		return false, errs.New("用户密码不能为空")
	}
	if err := s.CheckUserAllowed(ctx, *in.ID); err != nil {
		return false, err
	}
	if err := s.CheckUserDataScope(ctx, *in.ID); err != nil {
		return false, err
	}
	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(*in.Password), bcryptCost)
	if err != nil {
		return false, err
	}
	n, err := s.repo.UpdatePassword(ctx, *in.ID, string(hashedPwd))
	return n > 0, err
}

func (s *Service) ChangeStatus(ctx context.Context, in ChangeStatusIn) (bool, error) {
	if in.ID == nil {
		return false, nil
	}
	if err := s.CheckUserAllowed(ctx, *in.ID); err != nil {
		return false, err
	}
	if err := s.CheckUserDataScope(ctx, *in.ID); err != nil {
		return false, err
	}
	status := modelx.Normal
	if in.Status != nil {
		status = *in.Status
	}
	n, err := s.repo.UpdateStatus(ctx, *in.ID, status)
	return n > 0, err
}

func (s *Service) InsertAuthRole(ctx context.Context, userID int64, roleIDs []int64) (bool, error) {
	if err := s.CheckUserAllowed(ctx, userID); err != nil {
		return false, err
	}
	if err := s.CheckUserDataScope(ctx, userID); err != nil {
		return false, err
	}
	// 空 roleIds → no-op 保留原角色（Python _insert_user_role 早退）
	if err := s.rebuildUserRoles(ctx, userID, roleIDs); err != nil {
		return false, err
	}
	return true, nil
}

// ImportUsers 从 Excel 批量导入用户，对应 Python 端 UserService.import_users。
// 任一行失败 → 整体 500「很抱歉，导入失败！…」；全部成功 → 恭喜消息。
func (s *Service) ImportUsers(ctx context.Context, data []byte, updateSupport bool) (string, error) {
	rows, headers, err := excelx.ReadRows(data)
	if err != nil {
		return "", err
	}

	headerIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		headerIdx[h] = i
	}

	// 初始密码：sys.user.initPassword 优先，回退 USER_IMPORT_INIT_PASSWORD
	initPwd, err := s.repo.ConfigValueByKey(ctx, "sys.user.initPassword")
	if err != nil || initPwd == "" {
		initPwd = "Walnut@123"
	}
	hashedInitPwd, err := hashPassword(initPwd)
	if err != nil {
		return "", err
	}

	// 性别/状态：字典 label→value，未命中回退原标签，空单元格 → None
	sexL2V := s.dictLabelToValue(ctx, "sys_user_sex")
	statusL2V := s.dictLabelToValue(ctx, "sys_normal_disable")

	successNum, failureNum := 0, 0
	var successMsg, failureMsg string
	fail := func(userName, reason string) {
		failureNum++
		failureMsg += fmt.Sprintf("<br/>%d、账号 %s 导入失败：%s", failureNum, userName, reason)
	}

	for _, row := range rows {
		userName := cellStr(row, headerIdx, "用户账号")
		nickName := cellStr(row, headerIdx, "用户昵称")
		email := cellStr(row, headerIdx, "用户邮箱")
		phone := cellStr(row, headerIdx, "手机号码")
		deptID := cellInt(row, headerIdx, "部门编号")
		sexLabel := cellStr(row, headerIdx, "用户性别")
		statusLabel := cellStr(row, headerIdx, "账号状态")

		var sex, status *string
		if sexLabel != "" {
			v, ok := sexL2V[sexLabel]
			if !ok {
				v = sexLabel
			}
			sex = &v
		}
		if statusLabel != "" {
			v, ok := statusL2V[statusLabel]
			if !ok {
				v = statusLabel
			}
			status = &v
		}

		var existing *SysUser
		if userName != "" {
			existing, err = s.repo.FindByUserName(ctx, userName)
			if err != nil {
				fail(userName, err.Error())
				continue
			}
		}

		// UserBaseSchema/UserUpdateSchema 校验器（userName/nickName/email，声明序拼接）
		var msgs []string
		if userName == "" {
			msgs = append(msgs, "用户账号不能为空")
		} else if n := utf8.RuneCountInString(userName); n < 2 || n > 30 {
			msgs = append(msgs, "用户账号长度必须在2到30个字符之间")
		}
		if nickName == "" {
			msgs = append(msgs, "用户昵称不能为空")
		} else if utf8.RuneCountInString(nickName) > 30 {
			msgs = append(msgs, "用户昵称长度不能超过30个字符")
		}
		if email != "" {
			if utf8.RuneCountInString(email) > 50 {
				msgs = append(msgs, "邮箱长度不能超过50个字符")
			} else if !emailRE.MatchString(email) {
				msgs = append(msgs, "邮箱格式不正确")
			}
		}
		if len(msgs) > 0 {
			fail(userName, strings.Join(msgs, ", "))
			continue
		}

		if existing == nil {
			// 新增：status 为 None 时 Python 显式 INSERT NULL → NOT NULL 违例 → 行失败
			if status == nil {
				err := s.db.WithContext(ctx).Table("sys_user").Create(map[string]any{
					"user_name": userName, "nick_name": nickName, "password": hashedInitPwd,
					"status": nil, "del_flag": modelx.Normal,
				}).Error
				reason := "Column 'status' cannot be null"
				if err != nil {
					reason = err.Error()
				}
				fail(userName, reason)
				continue
			}
			row := SysUser{
				UserType: "sys_user",
				Status:   *status,
				DelFlag:  modelx.Normal,
				Password: &hashedInitPwd,
			}
			in := CreateIn{
				UserName: &userName, NickName: &nickName,
				Email: strPtrOrNil(email), PhoneNumber: strPtrOrNil(phone),
				Sex: sex, DeptID: deptID,
			}
			applyCreateFields(&row, in)
			crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
			if err := s.repo.Create(ctx, &row); err != nil {
				fail(userName, err.Error())
				continue
			}
			successNum++
			successMsg += fmt.Sprintf("<br/>%d、账号 %s 导入成功", successNum, userName)
		} else if updateSupport {
			if err := s.CheckUserAllowed(ctx, existing.ID); err != nil {
				fail(userName, errMsg(err))
				continue
			}
			if err := s.CheckUserDataScope(ctx, existing.ID); err != nil {
				fail(userName, errMsg(err))
				continue
			}
			// update_user：exclude_none —— 空单元格（None）不覆盖旧值
			existing.UserName = userName
			existing.NickName = nickName
			if email != "" {
				existing.Email = &email
			}
			if phone != "" {
				existing.Phonenumber = &phone
			}
			if sex != nil {
				existing.Sex = sex
			}
			if status != nil {
				existing.Status = *status
			}
			if deptID != nil {
				existing.DeptID = deptID
			}
			crudx.FillUpdate(&existing.BaseModel, s.auth, nowFunc())
			if err := s.repo.Update(ctx, existing); err != nil {
				fail(userName, err.Error())
				continue
			}
			successNum++
			successMsg += fmt.Sprintf("<br/>%d、账号 %s 更新成功", successNum, userName)
		} else {
			failureNum++
			failureMsg += fmt.Sprintf("<br/>%d、账号 %s 已存在", failureNum, existing.UserName)
		}
	}

	if failureNum > 0 {
		return "", errs.Newf("很抱歉，导入失败！共 %d 条数据格式不正确，错误如下：%s", failureNum, failureMsg)
	}
	return fmt.Sprintf("恭喜您，数据已全部导入成功！共 %d 条，数据如下：%s", successNum, successMsg), nil
}

// dictLabelToValue 字典 label→value 映射（无 status 过滤，对齐 list_by_type）。
func (s *Service) dictLabelToValue(ctx context.Context, dictType string) map[string]string {
	type kv struct {
		DictLabel string `gorm:"column:dict_label"`
		DictValue string `gorm:"column:dict_value"`
	}
	var rows []kv
	s.db.WithContext(ctx).Table("sys_dict_data").
		Select("dict_label, dict_value").
		Where("dict_type = ?", dictType).
		Find(&rows)
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.DictLabel] = r.DictValue
	}
	return m
}

func errMsg(err error) string {
	if se := errs.As(err); se != nil {
		return se.Msg
	}
	return err.Error()
}

// cellInt 对齐 _cell_int：int(float(value))，非法/空 → None。
func cellInt(row []string, idx map[string]int, header string) *int64 {
	s := cellStr(row, idx, header)
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	n := int64(f)
	return &n
}

func cellStr(row []string, idx map[string]int, header string) string {
	i, ok := idx[header]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---- Profile ----

func (s *Service) GetProfile(ctx context.Context) (*ProfileOut, error) {
	u, err := s.repo.FindByID(ctx, s.auth.User.ID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	pu := newProfileUserOut(u)
	if u.DeptID != nil {
		names, err := s.repo.MapDeptNames(ctx, []int64{*u.DeptID})
		if err != nil {
			return nil, err
		}
		pu.DeptName = names[*u.DeptID]
	}
	roleGroup, err := s.repo.RoleNameGroup(ctx, s.auth.User.ID)
	if err != nil {
		return nil, err
	}
	postGroup, err := s.repo.PostNameGroup(ctx, s.auth.User.ID)
	if err != nil {
		return nil, err
	}
	return &ProfileOut{User: pu, RoleGroup: roleGroup, PostGroup: postGroup}, nil
}

func (s *Service) UpdateProfile(ctx context.Context, in ProfileUpdateIn) (bool, error) {
	if in.PhoneNumber != nil && *in.PhoneNumber != "" {
		exists, err := s.repo.ExistsByField(ctx, "phonenumber", *in.PhoneNumber, &s.auth.User.ID)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.Newf("修改用户'%s'失败，手机号码已存在", s.auth.User.UserName)
		}
	}
	if in.Email != nil && *in.Email != "" {
		exists, err := s.repo.ExistsByField(ctx, "email", *in.Email, &s.auth.User.ID)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.Newf("修改用户'%s'失败，邮箱账号已存在", s.auth.User.UserName)
		}
	}
	n, err := s.repo.UpdateProfile(ctx, s.auth.User.ID, in.NickName, in.PhoneNumber, in.Email, in.Sex)
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, errs.New("修改个人信息异常，请联系管理员")
	}
	return true, nil
}

func (s *Service) UpdatePwd(ctx context.Context, oldPwd, newPwd string) (bool, error) {
	if oldPwd == "" {
		return false, errs.New("修改密码失败，旧密码错误")
	}
	u, err := s.repo.FindByID(ctx, s.auth.User.ID)
	if err != nil {
		return false, err
	}
	if u == nil || u.Password == nil {
		return false, errs.New("修改密码失败，旧密码错误")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*u.Password), []byte(oldPwd)); err != nil {
		return false, errs.New("修改密码失败，旧密码错误")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*u.Password), []byte(newPwd)); err == nil {
		return false, errs.New("新密码不能与旧密码相同")
	}
	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcryptCost)
	if err != nil {
		return false, err
	}
	n, err := s.repo.UpdatePassword(ctx, s.auth.User.ID, string(hashedPwd))
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, errs.New("修改密码异常，请联系管理员")
	}
	return true, nil
}

// ---- Checks ----

func (s *Service) CheckUserAllowed(_ context.Context, userID int64) error {
	if userID == modelx.SuperAdminID {
		return errs.New("不允许操作超级管理员用户")
	}
	return nil
}

func (s *Service) CheckUserDataScope(ctx context.Context, userID int64) error {
	if s.auth.User.IsSuperAdmin() {
		return nil
	}
	scope, err := s.buildScope(ctx)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	n, err := s.repo.CountByIDWithScope(ctx, userID, scope)
	if err != nil {
		return err
	}
	if n == 0 {
		return errs.New("没有权限访问用户数据！")
	}
	return nil
}

func (s *Service) checkDeptDataScope(ctx context.Context, deptID int64) error {
	if s.auth.User.IsSuperAdmin() {
		return nil
	}
	scope, err := permx.Build(ctx, s.repo.db, s.auth, permx.Default)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if scope.NoFilter {
		return nil
	}
	var n int64
	q := s.repo.db.WithContext(ctx).Table("sys_dept").
		Where("id = ? AND del_flag = ?", deptID, modelx.Normal)
	q = scope.Apply(q)
	if err := q.Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return errs.New("没有权限访问部门数据！")
	}
	return nil
}

// ---- Helpers ----

func (s *Service) toOuts(ctx context.Context, rows []SysUser) []*Out {
	outs := make([]*Out, 0, len(rows))
	for i := range rows {
		o := NewOut(&rows[i])
		outs = append(outs, &o)
	}
	return outs
}

func (s *Service) fillDeptName(ctx context.Context, outs []*Out) error {
	deptIDs := make([]int64, 0)
	for _, o := range outs {
		if o.DeptID != nil {
			deptIDs = append(deptIDs, o.DeptID.Int64())
		}
	}
	if len(deptIDs) == 0 {
		return nil
	}
	seen := make(map[int64]bool)
	unique := make([]int64, 0, len(deptIDs))
	for _, id := range deptIDs {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	names, err := s.repo.MapDeptNames(ctx, unique)
	if err != nil {
		return err
	}
	for i := range outs {
		if outs[i].DeptID != nil {
			outs[i].DeptName = names[outs[i].DeptID.Int64()]
		}
	}
	return nil
}

func filterOutSuperAdminOut(roles []role.Out) []role.Out {
	out := make([]role.Out, 0, len(roles))
	for _, r := range roles {
		if r.SuperAdmin {
			continue
		}
		out = append(out, r)
	}
	return out
}

func filterSuperAdminFromIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != modelx.SuperAdminID {
			out = append(out, id)
		}
	}
	return out
}

func toJSIntSlice(ids []int64) []idtype.JSInt {
	out := make([]idtype.JSInt, 0, len(ids))
	for _, id := range ids {
		out = append(out, idtype.JSInt(id))
	}
	return out
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func applyCreateFields(row *SysUser, in CreateIn) {
	if in.DeptID != nil {
		row.DeptID = in.DeptID
	}
	if in.UserName != nil {
		row.UserName = *in.UserName
	}
	if in.NickName != nil {
		row.NickName = *in.NickName
	}
	if in.Email != nil {
		row.Email = in.Email
	}
	if in.PhoneNumber != nil {
		row.Phonenumber = in.PhoneNumber
	}
	if in.Sex != nil {
		row.Sex = in.Sex
	}
	if in.Status != nil {
		row.Status = *in.Status
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	}
}

func applyUpdateFields(row *SysUser, in UpdateIn) {
	if in.DeptID != nil {
		row.DeptID = in.DeptID
	}
	if in.UserName != nil {
		row.UserName = *in.UserName
	}
	if in.NickName != nil {
		row.NickName = *in.NickName
	}
	if in.Email != nil {
		row.Email = in.Email
	}
	if in.PhoneNumber != nil {
		row.Phonenumber = in.PhoneNumber
	}
	if in.Sex != nil {
		row.Sex = in.Sex
	}
	if in.Status != nil {
		row.Status = *in.Status
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	}
}

func hashPassword(pwd string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pwd), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}
