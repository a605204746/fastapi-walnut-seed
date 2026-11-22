package user

import (
	"time"

	"walnut-seed-go/internal/module/system/post"
	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// ---- Output DTOs ----

type Out struct {
	ID          *idtype.JSInt   `json:"id"`
	DeptID      *idtype.JSInt   `json:"deptId"`
	UserName    *string         `json:"userName"`
	NickName    *string         `json:"nickName"`
	UserType    *string         `json:"userType"`
	Email       *string         `json:"email"`
	Phonenumber *string         `json:"phonenumber"`
	Sex         *string         `json:"sex"`
	Avatar      *string         `json:"avatar"`
	Status      *string         `json:"status"`
	LoginIP     *string         `json:"loginIp"`
	LoginDate   *timex.JSONTime `json:"loginDate"`
	Remark      *string         `json:"remark"`
	CreateTime  *timex.JSONTime `json:"createTime"`
	DeptName    *string         `json:"deptName"`
	Roles       []role.Out      `json:"roles"`
	RoleIDs     []idtype.JSInt  `json:"roleIds"`
	PostIDs     []idtype.JSInt  `json:"postIds"`
	RoleID      *idtype.JSInt   `json:"roleId"`
}

func (o *Out) SlimOnly() {
	o.DeptID = nil
	o.UserType = nil
	o.Email = nil
	o.Phonenumber = nil
	o.Sex = nil
	o.Avatar = nil
	o.Status = nil
	o.LoginIP = nil
	o.LoginDate = nil
	o.Remark = nil
	o.CreateTime = nil
	o.DeptName = nil
	o.Roles = nil
	o.RoleIDs = nil
	o.PostIDs = nil
	o.RoleID = nil
}

func NewOut(u *SysUser) Out {
	out := Out{
		ID:          jsIntPtr(u.ID),
		DeptID:      jsIntPtrOrNil(u.DeptID),
		UserName:    strPtr(u.UserName),
		NickName:    strPtr(u.NickName),
		UserType:    strPtr(u.UserType),
		Email:       u.Email,
		Phonenumber: u.Phonenumber,
		Sex:         u.Sex,
		Avatar:      u.Avatar,
		Status:      strPtr(u.Status),
		LoginIP:     u.LoginIP,
		Remark:      u.Remark,
	}
	out.LoginDate = jsonTimeOrNil(u.LoginDate)
	out.CreateTime = jsonTimeOrNil(u.CreateTime)
	return out
}

type GetInfoOut struct {
	User        *Out     `json:"user"`
	Permissions []string `json:"permissions"`
	Roles       []string `json:"roles"`
}

type UserInfoOut struct {
	User    *Out           `json:"user"`
	RoleIDs []idtype.JSInt `json:"roleIds"`
	PostIDs []idtype.JSInt `json:"postIds"`
	Roles   []role.Out     `json:"roles"`
	Posts   []post.Out     `json:"posts"`
}

type ProfileUserOut struct {
	ID          *idtype.JSInt   `json:"id"`
	TenantID    *string         `json:"tenantId"`
	DeptID      *idtype.JSInt   `json:"deptId"`
	UserName    *string         `json:"userName"`
	NickName    *string         `json:"nickName"`
	UserType    *string         `json:"userType"`
	Email       *string         `json:"email"`
	Phonenumber *string         `json:"phonenumber"`
	Sex         *string         `json:"sex"`
	Avatar      *string         `json:"avatar"`
	LoginIP     *string         `json:"loginIp"`
	LoginDate   *timex.JSONTime `json:"loginDate"`
	DeptName    *string         `json:"deptName"`
}

type ProfileOut struct {
	User      *ProfileUserOut `json:"user"`
	RoleGroup string          `json:"roleGroup"`
	PostGroup string          `json:"postGroup"`
}

func newProfileUserOut(u *SysUser) *ProfileUserOut {
	return &ProfileUserOut{
		ID:          jsIntPtr(u.ID),
		DeptID:      jsIntPtrOrNil(u.DeptID),
		UserName:    strPtr(u.UserName),
		NickName:    strPtr(u.NickName),
		UserType:    strPtr(u.UserType),
		Email:       u.Email,
		Phonenumber: u.Phonenumber,
		Sex:         u.Sex,
		Avatar:      u.Avatar,
		LoginIP:     u.LoginIP,
		LoginDate:   jsonTimeOrNil(u.LoginDate),
	}
}

// ---- Export ----

const exportSheetName = "用户数据"

var exportHeaders = []string{
	"用户序号", "用户账号", "用户昵称", "用户邮箱", "手机号码",
	"用户性别", "账号状态", "最后登录IP", "最后登录时间", "部门名称", "部门负责人",
}

func ExportColumns() []string {
	return []string{
		"id", "user_name", "nick_name", "email", "phonenumber",
		"sex", "status", "login_ip", "login_date", "dept_name", "leader_name",
	}
}

func ExportRowToMap(r ExportRow) map[string]any {
	status := r.User.Status
	sex := r.User.Sex
	// *string 必须解引用：excelize 对指针按 %v 输出会写成内存地址
	return map[string]any{
		"id":          r.User.ID,
		"user_name":   r.User.UserName,
		"nick_name":   r.User.NickName,
		"email":       derefStr(r.User.Email),
		"phonenumber": derefStr(r.User.Phonenumber),
		"sex":         sexLabel(sex),
		"status":      statusLabel(&status),
		"login_ip":    derefStr(r.User.LoginIP),
		"login_date":  loginDateStr(r.User.LoginDate),
		"dept_name":   derefStr(r.DeptName),
		"leader_name": derefStr(r.LeaderName),
	}
}

func sexLabel(s *string) string {
	if s == nil {
		return ""
	}
	switch *s {
	case "0":
		return "男"
	case "1":
		return "女"
	case "2":
		return "未知"
	}
	return ""
}

func statusLabel(s *string) string {
	if s == nil {
		return ""
	}
	switch *s {
	case "0":
		return "正常"
	case "1":
		return "停用"
	}
	return ""
}

func loginDateStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// ---- Basic helpers ----

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func jsIntPtrOrNil(v *int64) *idtype.JSInt {
	if v == nil {
		return nil
	}
	return jsIntPtr(*v)
}

func strPtr(v string) *string {
	s := v
	return &s
}

func jsonTimeOrNil(t *time.Time) *timex.JSONTime {
	if t == nil {
		return nil
	}
	jt := timex.FromTime(*t)
	return &jt
}
