package user

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"walnut-seed-go/internal/module/system/dept"
	"walnut-seed-go/internal/module/system/post"
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
	}
	// Python UserQueryParam 绑定校验按声明序拼接：id → deptId → beginTime → endTime；
	// userIds/excludeUserIds 是 str 型，绑定不校验，service 条件构造时裸 int() →
	// ValueError 原文（parseIDListQuery），仅在绑定通过后才会触及。
	var msgs []string
	intField := func(key string, dst **int64) {
		if raw, ok := q.GetQuery(key); ok {
			n, valid := dept.ParsePyQueryInt(raw)
			if !valid {
				msgs = append(msgs, msgValidInt)
				return
			}
			*dst = &n
		}
	}
	dateField := func(key string, dst *string) {
		if raw, ok := q.GetQuery(key); ok {
			v, derr := dept.ParseDateQuery(raw)
			if derr != nil {
				if se := errs.As(derr); se != nil {
					msgs = append(msgs, se.Msg)
				}
				return
			}
			*dst = v
		}
	}
	intField("id", &qp.ID)
	intField("deptId", &qp.DeptID)
	dateField("beginTime", &qp.BeginTime)
	dateField("endTime", &qp.EndTime)
	if len(msgs) > 0 {
		return QueryParam{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	if raw := q.Query("userIds"); raw != "" {
		ids, err := parseIDListQuery(raw)
		if err != nil {
			return QueryParam{}, err
		}
		qp.UserIDs = ids
	}
	if raw := q.Query("excludeUserIds"); raw != "" {
		ids, err := parseIDListQuery(raw)
		if err != nil {
			return QueryParam{}, err
		}
		qp.ExcludeUserIDs = ids
	}
	return qp, nil
}

// pyIntRE 是 Starlette int 路径转换器的正则（仅纯数字，无符号/下划线）。
var pyIntRE = regexp.MustCompile(`^[0-9]+$`)

// pyIntArgRE 匹配 Python int() 接受的形态：可选符号 + 数字间可单个下划线。
var pyIntArgRE = regexp.MustCompile(`^[+-]?[0-9](_?[0-9])*$`)

// pyAtoi 复刻 Python int(str)：接受 "1_0"=10、"+1"、"-1"，拒绝 "_1"/"1__0"/"1_"。
func pyAtoi(s string) (int64, error) {
	if !pyIntArgRE.MatchString(s) {
		return 0, fmt.Errorf("invalid literal for int() with base 10: '%s'", s)
	}
	return strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)
}

// parseIDListStrict 复刻 _parse_id_list：任一分量非法 → 400「{label}格式有误」。
func parseIDListStrict(raw, label string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := pyAtoi(p)
		if err != nil {
			return nil, errs.BadRequest(label + "格式有误")
		}
		ids = append(ids, n)
	}
	return ids, nil
}

// parseIDListQuery 查询条件里的 ID 串解析：非法分量 → 400「用户ID格式有误」
// （Python 端已从裸 int() 透传 traceback 改为 ServiceException，两端同步）。
func parseIDListQuery(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := pyAtoi(p)
		if err != nil {
			return nil, errs.BadRequest("用户ID格式有误")
		}
		ids = append(ids, n)
	}
	return ids, nil
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
	in, msgs := decodeCreateFields(fields, true)
	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
}

// decodeCreateFields 按 Python UserBaseSchema 的字段声明序收集校验错误：
// deptId → userName → nickName → email → phonenumber → sex → password →
// status → remark → roleIds → postIds。validatePassword 仅新增时为 true
// （UserUpdateSchema 不校验密码复杂度，password 在修改时被忽略）。
func decodeCreateFields(fields map[string]json.RawMessage, validatePassword bool) (CreateIn, []string) {
	var in CreateIn
	var msgs []string

	if raw, ok := fields["deptId"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DeptID = &v
		}
	}
	// userName/nickName 带 validate_default：缺省/null 也触发非空校验；
	// 类型错误时 pydantic 不再跑自定义校验器，故只报类型错误。
	userNameFailed := false
	if raw, ok := fields["userName"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
			userNameFailed = true
		} else {
			in.UserName = &v
		}
	}
	if !userNameFailed {
		if in.UserName == nil || strings.TrimSpace(*in.UserName) == "" {
			msgs = append(msgs, "用户账号不能为空")
		} else if n := utf8.RuneCountInString(*in.UserName); n < 2 || n > 30 {
			msgs = append(msgs, "用户账号长度必须在2到30个字符之间")
		}
	}
	nickNameFailed := false
	if raw, ok := fields["nickName"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
			nickNameFailed = true
		} else {
			in.NickName = &v
		}
	}
	if !nickNameFailed {
		if in.NickName == nil || strings.TrimSpace(*in.NickName) == "" {
			msgs = append(msgs, "用户昵称不能为空")
		} else if utf8.RuneCountInString(*in.NickName) > 30 {
			msgs = append(msgs, "用户昵称长度不能超过30个字符")
		}
	}
	if raw, ok := fields["email"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Email = &v
			if v != "" {
				if utf8.RuneCountInString(v) > 50 {
					msgs = append(msgs, "邮箱长度不能超过50个字符")
				} else if !emailRE.MatchString(v) {
					msgs = append(msgs, "邮箱格式不正确")
				}
			}
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
	if raw, ok := fields["password"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Password = &v
			if validatePassword {
				if m := checkPasswordComplexity(v, "用户密码"); m != "" {
					msgs = append(msgs, m)
				}
			}
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
			if len(v) == 0 {
				msgs = append(msgs, "用户角色不能为空")
			}
		}
	}
	if raw, ok := fields["postIds"]; ok && !isNull(raw) {
		if v, terr := decodeIntSlice(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.PostIDs = v
		}
	}
	return in, msgs
}

var emailRE = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// checkPasswordComplexity 复刻 Python _check_password_complexity +
// RegexConstants.PASSWORD（RE2 不支持 lookahead，用等价手动检查）：
// 至少 8 位，仅限 A-Za-z0-9@$!%*?&，且四类字符各至少一个。
func checkPasswordComplexity(value, label string) string {
	if strings.TrimSpace(value) == "" {
		return label + "不能为空"
	}
	var hasLower, hasUpper, hasDigit, hasSpecial bool
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case strings.ContainsRune("@$!%*?&", r):
			hasSpecial = true
		default:
			return "密码必须包含大写字母、小写字母、数字和特殊字符"
		}
	}
	if utf8.RuneCountInString(value) < 8 || !hasLower || !hasUpper || !hasDigit || !hasSpecial {
		return "密码必须包含大写字母、小写字母、数字和特殊字符"
	}
	return ""
}

func DecodeUpdate(body []byte) (UpdateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return UpdateIn{}, err
	}
	createIn, msgs := decodeCreateFields(fields, false)
	// id 的 validate_default 校验排在继承字段之后（实测拼接序）
	var idPtr *int64
	if raw, ok := fields["id"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			idPtr = &v
		}
	}
	if idPtr == nil {
		msgs = append(msgs, "用户ID不能为空")
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
	if in.ID == nil {
		msgs = append(msgs, "用户ID不能为空")
	}
	if raw, ok := fields["password"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Password = &v
			if m := checkPasswordComplexity(v, "用户密码"); m != "" {
				msgs = append(msgs, m)
			}
		}
	} else {
		msgs = append(msgs, "用户密码不能为空")
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
	if in.ID == nil {
		msgs = append(msgs, "用户ID不能为空")
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
			if utf8.RuneCountInString(v) > 30 {
				msgs = append(msgs, "用户昵称长度不能超过30个字符")
			}
		}
	}
	if raw, ok := fields["email"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Email = &v
			if v != "" {
				if utf8.RuneCountInString(v) > 50 {
					msgs = append(msgs, "邮箱长度不能超过50个字符")
				} else if !emailRE.MatchString(v) {
					msgs = append(msgs, "邮箱格式不正确")
				}
			}
		}
	}
	if raw, ok := fields["phonenumber"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.PhoneNumber = &v
			if v != "" && !mobileRE.MatchString(v) {
				msgs = append(msgs, "手机号格式不正确")
			}
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

var mobileRE = regexp.MustCompile(`^1(3\d|4[4-9]|5[0-35-9]|6[67]|7[013-8]|8[0-9]|9[0-9])\d{8}$`)

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
			if strings.TrimSpace(v) == "" {
				msgs = append(msgs, "旧密码不能为空")
			}
		}
	} else {
		msgs = append(msgs, "旧密码不能为空")
	}
	if raw, ok := fields["newPassword"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.NewPassword = v
			if m := checkPasswordComplexity(v, "新密码"); m != "" {
				msgs = append(msgs, m)
			}
		}
	} else {
		msgs = append(msgs, "新密码不能为空")
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
