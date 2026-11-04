package user

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/permx"
)

var nowFunc = time.Now

const bcryptCost = 12

type Service struct {
	repo *Repo
	auth authctx.Context
}

func NewService(db *gorm.DB, ac authctx.Context) *Service {
	return &Service{repo: NewRepo(db), auth: ac}
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
	if u == nil {
		return nil, nil
	}
	out := NewOut(u)
	if err := s.fillDeptName(ctx, []*Out{&out}); err != nil {
		return nil, err
	}
	roleIDs, err := s.repo.RoleIDsByUserID(ctx, id)
	if err != nil {
		return nil, err
	}
	out.RoleIDs = toJSIntSlice(roleIDs)

	roles, err := s.repo.FindRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	roleOuts := make([]role.Out, 0, len(roles))
	for i := range roles {
		roleOuts = append(roleOuts, role.NewOut(&roles[i]))
	}
	out.Roles = roleOuts

	if u.DeptID != nil {
		postIDs, err := s.repo.PostIDsByUserID(ctx, id)
		if err != nil {
			return nil, err
		}
		out.PostIDs = toJSIntSlice(postIDs)
	}

	allRoles, err := s.normalRoles()
	if err != nil {
		return nil, err
	}
	if !s.auth.User.IsSuperAdmin() && u.ID != modelx.SuperAdminID {
		allRoles = filterOutSuperAdminOut(allRoles)
	}

	return &UserInfoOut{
		User:    &out,
		RoleIDs: out.RoleIDs,
		PostIDs: out.PostIDs,
		Roles:   allRoles,
	}, nil
}

func (s *Service) GetUserInfoNoID(ctx context.Context) (*UserInfoOut, error) {
	allRoles, err := s.normalRoles()
	if err != nil {
		return nil, err
	}
	if !s.auth.User.IsSuperAdmin() {
		allRoles = filterOutSuperAdminOut(allRoles)
	}
	return &UserInfoOut{Roles: allRoles}, nil
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
		Permissions: s.auth.Permissions,
		Roles:       s.auth.Roles,
	}, nil
}

func (s *Service) GetAuthRole(ctx context.Context, userID int64) (*UserInfoOut, error) {
	if err := s.CheckUserDataScope(ctx, userID); err != nil {
		return nil, err
	}
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	out := NewOut(u)
	if err := s.fillDeptName(ctx, []*Out{&out}); err != nil {
		return nil, err
	}
	userRoleIDs, err := s.repo.RoleIDsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	allRoles, err := s.repo.AllNormalRoles(ctx)
	if err != nil {
		return nil, err
	}
	if u.ID != modelx.SuperAdminID {
		allRoles = filterOutSuperAdmin(allRoles)
	}
	roleOuts := make([]role.Out, 0, len(allRoles))
	roleIDSet := make(map[int64]bool, len(userRoleIDs))
	for _, rid := range userRoleIDs {
		roleIDSet[rid] = true
	}
	for i := range allRoles {
		o := role.NewOut(&allRoles[i])
		if roleIDSet[allRoles[i].ID] {
			o.Flag = true
		}
		roleOuts = append(roleOuts, o)
	}
	return &UserInfoOut{User: &out, Roles: roleOuts}, nil
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
	if in.DeptID != nil {
		if err := s.checkDeptDataScope(ctx, *in.DeptID); err != nil {
			return false, err
		}
	}
	if in.UserName == nil || *in.UserName == "" {
		return false, errs.New("用户账号不能为空")
	}
	if in.Password == nil || *in.Password == "" {
		return false, errs.New("用户密码不能为空")
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
	if len(in.RoleIDs) > 0 {
		if err := s.repo.InsertUserRoles(ctx, row.ID, in.RoleIDs); err != nil {
			return false, err
		}
	}
	if len(in.PostIDs) > 0 {
		if err := s.repo.InsertUserPosts(ctx, row.ID, in.PostIDs); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Service) UpdateUser(ctx context.Context, in UpdateIn) (bool, error) {
	if in.ID == nil {
		return false, nil
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
	u, err := s.repo.FindByIDWithScope(ctx, *in.ID, scope)
	if err != nil {
		return false, err
	}
	if u == nil {
		return false, nil
	}
	if err := s.repo.DeleteUserRoles(ctx, *in.ID); err != nil {
		return false, err
	}
	if len(in.RoleIDs) > 0 {
		if err := s.repo.InsertUserRoles(ctx, *in.ID, in.RoleIDs); err != nil {
			return false, err
		}
	}
	if err := s.repo.DeleteUserPosts(ctx, *in.ID); err != nil {
		return false, err
	}
	if len(in.PostIDs) > 0 {
		if err := s.repo.InsertUserPosts(ctx, *in.ID, in.PostIDs); err != nil {
			return false, err
		}
	}
	applyUpdateFields(u, in)
	crudx.FillUpdate(&u.BaseModel, s.auth, nowFunc())
	if err := s.repo.Update(ctx, u); err != nil {
		return false, err
	}
	return true, nil
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
	filtered := roleIDs
	if userID != modelx.SuperAdminID {
		filtered = filterSuperAdminFromIDs(filtered)
	}
	if err := s.repo.DeleteUserRoles(ctx, userID); err != nil {
		return false, err
	}
	if len(filtered) > 0 {
		if err := s.repo.InsertUserRoles(ctx, userID, filtered); err != nil {
			return false, err
		}
	}
	return true, nil
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
			return false, errs.New("修改用户失败，手机号码已存在")
		}
	}
	if in.Email != nil && *in.Email != "" {
		exists, err := s.repo.ExistsByField(ctx, "email", *in.Email, &s.auth.User.ID)
		if err != nil {
			return false, err
		}
		if exists {
			return false, errs.New("修改用户失败，邮箱账号已存在")
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

func (s *Service) normalRoles() ([]role.Out, error) {
	rows, err := s.repo.AllNormalRoles(context.Background())
	if err != nil {
		return nil, err
	}
	outs := make([]role.Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, role.NewOut(&rows[i]))
	}
	return outs, nil
}

func filterOutSuperAdmin(roles []role.SysRole) []role.SysRole {
	out := make([]role.SysRole, 0, len(roles))
	for _, r := range roles {
		if r.ID == modelx.SuperAdminID {
			continue
		}
		out = append(out, r)
	}
	return out
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
