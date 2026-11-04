package user

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"walnut-seed-go/internal/module/system/role"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

const (
	msgFieldRequired = "Field required"
	msgValidInt      = "Input should be a valid integer, unable to parse string as an integer"
	msgValidStr      = "Input should be a valid string"
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
	o.UserType = nil
	o.Sex = nil
	o.Avatar = nil
	o.LoginIP = nil
	o.LoginDate = nil
	o.Remark = nil
	o.CreateTime = nil
	o.DeptName = nil
	o.Roles = nil
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
	User    *Out           `json:"user,omitempty"`
	RoleIDs []idtype.JSInt `json:"roleIds,omitempty"`
	PostIDs []idtype.JSInt `json:"postIds,omitempty"`
	Roles   []role.Out     `json:"roles,omitempty"`
}

type ProfileUserOut struct {
	ID          *idtype.JSInt   `json:"id"`
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
	CreateTime  *timex.JSONTime `json:"createTime"`
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
		CreateTime:  jsonTimeOrNil(u.CreateTime),
	}
}

// ---- Query ----

type QueryParam struct {
	Page           crudx.Page
	ID             *int64
	DeptID         *int64
	UserName       string
	NickName       string
	Status         string
	PhoneNumber    string
	UserIDs        []int64
	ExcludeUserIDs []int64
	BeginTime      string
	EndTime        string
}

func BindQueryParam(q crudx.Querier) (QueryParam, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return QueryParam{}, err
	}
	qp := QueryParam{
		Page:        page,
		UserName:    strings.TrimSpace(q.Query("userName")),
		NickName:    strings.TrimSpace(q.Query("nickName")),
		Status:      strings.TrimSpace(q.Query("status")),
		PhoneNumber: strings.TrimSpace(q.Query("phonenumber")),
		BeginTime:   strings.TrimSpace(q.Query("beginTime")),
		EndTime:     strings.TrimSpace(q.Query("endTime")),
	}
	if raw := q.Query("deptId"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			qp.DeptID = &n
		}
	}
	if raw := q.Query("id"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			qp.ID = &n
		}
	}
	if raw := q.Query("userIds"); raw != "" {
		qp.UserIDs = parseIDList(raw)
	}
	if raw := q.Query("excludeUserIds"); raw != "" {
		qp.ExcludeUserIDs = parseIDList(raw)
	}
	return qp, nil
}

func parseIDList(raw string) []int64 {
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err == nil {
			ids = append(ids, n)
		}
	}
	return ids
}

// ---- Input DTOs ----

type CreateIn struct {
	DeptID      *int64
	UserName    *string
	NickName    *string
	Password    *string
	Email       *string
	PhoneNumber *string
	Sex         *string
	Status      *string
	Remark      *string
	RoleIDs     []int64
	PostIDs     []int64
}

type UpdateIn struct {
	CreateIn
	ID *int64
}

type ResetPwdIn struct {
	ID       *int64
	Password *string
}

type ChangeStatusIn struct {
	ID     *int64
	Status *string
}

type ProfileUpdateIn struct {
	NickName    *string
	PhoneNumber *string
	Email       *string
	Sex         *string
}

type UpdatePwdIn struct {
	OldPassword string
	NewPassword string
}

func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	if raw, ok := fields["deptId"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DeptID = &v
		}
	}
	if raw, ok := fields["userName"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.UserName = &v
			if utf8.RuneCountInString(v) < 2 || utf8.RuneCountInString(v) > 30 {
				msgs = append(msgs, "String should have at least 2 characters")
			}
		}
	}
	if raw, ok := fields["nickName"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.NickName = &v
		}
	}
	if raw, ok := fields["password"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Password = &v
		}
	}
	if raw, ok := fields["email"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Email = &v
		}
	}
	if raw, ok := fields["phonenumber"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.PhoneNumber = &v
		}
	}
	if raw, ok := fields["sex"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Sex = &v
		}
	}
	if raw, ok := fields["status"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Status = &v
		}
	}
	if raw, ok := fields["remark"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Remark = &v
		}
	}
	if raw, ok := fields["roleIds"]; ok && !isNull(raw) {
		if v, terr := decodeIntSlice(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleIDs = v
		}
	}
	if raw, ok := fields["postIds"]; ok && !isNull(raw) {
		if v, terr := decodeIntSlice(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.PostIDs = v
		}
	}
	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
}

func DecodeUpdate(body []byte) (UpdateIn, error) {
	createIn, err := DecodeCreate(body)
	if err != nil {
		return UpdateIn{CreateIn: createIn}, err
	}
	fields, _ := rawFields(body)
	var msgs []string
	var idPtr *int64
	if raw, ok := fields["id"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			idPtr = &v
		}
	}
	if len(msgs) > 0 {
		return UpdateIn{CreateIn: createIn}, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return UpdateIn{CreateIn: createIn, ID: idPtr}, nil
}

func DecodeResetPwd(body []byte) (ResetPwdIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return ResetPwdIn{}, err
	}
	var in ResetPwdIn
	var msgs []string
	if raw, ok := fields["id"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.ID = &v
		}
	}
	if raw, ok := fields["password"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Password = &v
		}
	}
	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
}

func DecodeChangeStatus(body []byte) (ChangeStatusIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return ChangeStatusIn{}, err
	}
	var in ChangeStatusIn
	var msgs []string
	if raw, ok := fields["id"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.ID = &v
		}
	}
	if raw, ok := fields["status"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Status = &v
		}
	}
	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
}

func DecodeProfileUpdate(body []byte) (ProfileUpdateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return ProfileUpdateIn{}, err
	}
	var in ProfileUpdateIn
	var msgs []string
	if raw, ok := fields["nickName"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.NickName = &v
		}
	}
	if raw, ok := fields["phonenumber"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.PhoneNumber = &v
		}
	}
	if raw, ok := fields["email"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Email = &v
		}
	}
	if raw, ok := fields["sex"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Sex = &v
		}
	}
	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
}

func DecodeUpdatePwd(body []byte) (UpdatePwdIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return UpdatePwdIn{}, err
	}
	var in UpdatePwdIn
	var msgs []string
	if raw, ok := fields["oldPassword"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.OldPassword = v
		}
	}
	if raw, ok := fields["newPassword"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.NewPassword = v
		}
	}
	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
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
	return map[string]any{
		"id":          r.User.ID,
		"user_name":   r.User.UserName,
		"nick_name":   r.User.NickName,
		"email":       r.User.Email,
		"phonenumber": r.User.Phonenumber,
		"sex":         sexLabel(sex),
		"status":      statusLabel(&status),
		"login_ip":    r.User.LoginIP,
		"login_date":  loginDateStr(r.User.LoginDate),
		"dept_name":   r.DeptName,
		"leader_name": r.LeaderName,
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

// ---- Decode helpers ----

func rawFields(body []byte) (map[string]json.RawMessage, error) {
	if len(body) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, errs.BadRequest(errs.MsgJSONInvalid)
	}
	return m, nil
}

func isNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}

func decodeStr(raw json.RawMessage) (string, *string) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		msg := msgValidStr
		return "", &msg
	}
	return v, nil
}

func decodeInt64(raw json.RawMessage) (int64, *string) {
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil {
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 == nil {
			n, err3 := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err3 == nil {
				return n, nil
			}
		}
		msg := msgValidInt
		return 0, &msg
	}
	return v, nil
}

func decodeIntSlice(raw json.RawMessage) ([]int64, *string) {
	var v []int64
	if err := json.Unmarshal(raw, &v); err != nil {
		msg := "Input should be a valid list"
		return nil, &msg
	}
	return v, nil
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
