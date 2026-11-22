package user

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/validatex"
)

// Pydantic v2 类型错误原文（handler 的 ID 串解析仍在用，单点托管于 bindx）。
const msgValidInt = bindx.MsgValidInt

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
			n, valid := bindx.PyQueryInt(raw)
			if !valid {
				msgs = append(msgs, msgValidInt)
				return
			}
			*dst = &n
		}
	}
	dateField := func(key string, dst *string) {
		if raw, ok := q.GetQuery(key); ok {
			v, derr := bindx.ParseDateQuery(raw)
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

// BindExportParam 复刻 Python export_user 的 form/query 合并绑定：
// query 打底，非空 form 值覆盖；pageNum/pageSize 丢弃；
// 校验错误透传 pydantic ValidationError 原文（与 post/export 同款排版）。
func BindExportParam(c *gin.Context) (QueryParam, error) {
	if err := c.Request.ParseForm(); err != nil {
		return QueryParam{}, errs.BadRequest(errs.MsgValidationFailed)
	}
	merged := make(map[string]string)
	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			merged[k] = v[0]
		}
	}
	for k, v := range c.Request.PostForm {
		if len(v) > 0 && v[0] != "" {
			merged[k] = v[0]
		}
	}
	delete(merged, "pageNum")
	delete(merged, "pageSize")

	q := QueryParam{}
	q.UserName = strings.TrimSpace(merged["userName"])
	q.NickName = strings.TrimSpace(merged["nickName"])
	q.Status = strings.TrimSpace(merged["status"])
	q.PhoneNumber = strings.TrimSpace(merged["phonenumber"])
	var ferrs []validatex.FormFieldError
	for _, key := range []string{"id", "deptId"} {
		if raw, ok := merged[key]; ok {
			n, valid := bindx.PyQueryInt(raw)
			if !valid {
				ferrs = append(ferrs, validatex.FormFieldError{Loc: key, Msg: msgValidInt, Typ: "int_parsing", Input: raw})
				continue
			}
			if key == "deptId" {
				q.DeptID = &n
			}
		}
	}
	for _, key := range []string{"beginTime", "endTime"} {
		if raw, ok := merged[key]; ok {
			v, derr := bindx.ParseDateQuery(raw)
			if derr != nil {
				if se := errs.As(derr); se != nil {
					ferrs = append(ferrs, validatex.FormFieldError{Loc: key, Msg: se.Msg, Typ: validatex.DateErrType(se.Msg), Input: raw})
				}
				continue
			}
			if key == "beginTime" {
				q.BeginTime = v
			} else {
				q.EndTime = v
			}
		}
	}
	if len(ferrs) > 0 {
		return QueryParam{}, errs.BadRequest(validatex.ValidationErrorText("UserQueryParam", ferrs))
	}
	return q, nil
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

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 字段声明序镜像旧 decodeCreateFields：deptId → userName → nickName → email →
// phonenumber → sex → password → status → remark → roleIds → postIds；
// UpdateReq 的 id 追加在继承字段之后（validator 按声明序全量收集错误）。
//
// 相对手写 Decode* 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - lax 强转退役（deptId/id 的 "5"/"1_0" 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）；JSON 类型错误短路，
//     校验级错误仍全量按声明序拼接
//   - 空/空白体视为空对象（旧 FieldsStrict 空白体报 JSON 解析失败）
//
// 严格镜像旧语义（不新增约束，Python schema 层 password/role_ids 均可选）：
//   - password 仅在 DecodeCreate 后置手工校验（在场且非 null 才跑复杂度；
//     缺省留给 service InsertUser 兜底 500「用户密码不能为空」）
//   - roleIds 仅在场且非 null 时拒空数组（null/缺省→nil→放行，service no-op）
//   - phonenumber 仅个人中心（ProfileUpdateReq）跑 mobile 校验，新增/修改不校验

func init() {
	// password 复刻 checkPasswordComplexity 的字符类逻辑（保留原函数，包装复用）；
	// ResetPwd/UpdatePwd 走此 tag（required 承担空白拦截），CreateReq 的密码
	// 复杂度在 DecodeCreate 后置手工校验（复刻旧「在场才校验」语义）。
	bindx.RegisterTag("password", func(fl validator.FieldLevel) bool {
		return checkPasswordComplexity(fl.Field().String(), "用户密码") == ""
	})
	bindx.RegisterTag("pyemail", func(fl validator.FieldLevel) bool {
		return emailRE.MatchString(fl.Field().String())
	})
	// pyemailOpt 对齐 Python UserBaseSchema.check_email / UserProfileSchema.check_email：
	// null/空串不校验格式（"" 直接放行），仅有值时跑正则。dept 的 pyemail 只放行
	// null（空串仍拒），语义不同，故用户模块单独用此 tag，不复用共享 pyemail。
	bindx.RegisterTag("pyemailOpt", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return s == "" || emailRE.MatchString(s)
	})
	// mobile 对齐 Python UserProfileSchema.check_phonenumber：null/空串放行，
	// 仅有值时校验手机号格式（仅 ProfileUpdateReq 使用）。
	bindx.RegisterTag("mobile", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return s == "" || mobileRE.MatchString(s)
	})
}

// CreateReq 新增用户请求体，对应 UserCreateSchema。
// password/roleIds 在 schema 层可选（Python default=None），后置校验见 DecodeCreate。
type CreateReq struct {
	DeptID      *int64  `json:"deptId" label:"部门ID"`
	UserName    string  `json:"userName" validate:"required,notblank,min=2,max=30" label:"用户账号"`
	NickName    string  `json:"nickName" validate:"required,notblank,max=30" label:"用户昵称"`
	Email       *string `json:"email" validate:"omitempty,max=50,pyemailOpt" label:"邮箱"`
	PhoneNumber *string `json:"phonenumber" label:"手机号"`
	Sex         *string `json:"sex" label:"用户性别"`
	Password    *string `json:"password" label:"用户密码"`
	Status      *string `json:"status" label:"账号状态"`
	Remark      *string `json:"remark" label:"备注"`
	RoleIDs     []int64 `json:"roleIds" label:"用户角色"`
	PostIDs     []int64 `json:"postIds" label:"岗位"`
}

// UpdateReq 修改用户请求体，对应 UserUpdateSchema。独立声明（不嵌入 CreateReq）：
// 修改不跑密码复杂度后置校验（password 可选透传），id 追加在末位，
// 其余字段声明序与 CreateReq 一致。
type UpdateReq struct {
	DeptID      *int64  `json:"deptId" label:"部门ID"`
	UserName    string  `json:"userName" validate:"required,notblank,min=2,max=30" label:"用户账号"`
	NickName    string  `json:"nickName" validate:"required,notblank,max=30" label:"用户昵称"`
	Email       *string `json:"email" validate:"omitempty,max=50,pyemailOpt" label:"邮箱"`
	PhoneNumber *string `json:"phonenumber" label:"手机号"`
	Sex         *string `json:"sex" label:"用户性别"`
	Password    *string `json:"password" label:"用户密码"`
	Status      *string `json:"status" label:"账号状态"`
	Remark      *string `json:"remark" label:"备注"`
	RoleIDs     []int64 `json:"roleIds" label:"用户角色"`
	PostIDs     []int64 `json:"postIds" label:"岗位"`
	ID          *int64  `json:"id" validate:"required" label:"用户ID"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		DeptID:      r.DeptID,
		UserName:    &r.UserName,
		NickName:    &r.NickName,
		Password:    r.Password,
		Email:       r.Email,
		PhoneNumber: r.PhoneNumber,
		Sex:         r.Sex,
		Status:      r.Status,
		Remark:      r.Remark,
		RoleIDs:     r.RoleIDs,
		PostIDs:     r.PostIDs,
	}
}

func (r UpdateReq) toIn() CreateIn {
	return CreateIn{
		DeptID:      r.DeptID,
		UserName:    &r.UserName,
		NickName:    &r.NickName,
		Password:    r.Password,
		Email:       r.Email,
		PhoneNumber: r.PhoneNumber,
		Sex:         r.Sex,
		Status:      r.Status,
		Remark:      r.Remark,
		RoleIDs:     r.RoleIDs,
		PostIDs:     r.PostIDs,
	}
}

// DecodeCreate 解析并校验新增入参（service 消费 CreateIn，零改动）。
// 后置手工校验复刻旧「在场才校验」语义：password 仅在场且非 null 时跑复杂度
// （缺省留给 service InsertUser 兜底 500）；roleIds 仅在场且非 null 时拒空数组。
func DecodeCreate(ctx context.Context, body []byte) (CreateIn, error) {
	var req CreateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return CreateIn{}, err
	}
	if req.Password != nil {
		if m := checkPasswordComplexity(*req.Password, "用户密码"); m != "" {
			return CreateIn{}, errs.BadRequest(m)
		}
	}
	if req.RoleIDs != nil && len(req.RoleIDs) == 0 {
		return CreateIn{}, errs.BadRequest("用户角色不能为空")
	}
	return req.toIn(), nil
}

// DecodeUpdate 解析修改入参（password 可选且不校验复杂度；roleIds 同 Create
// 的「在场才校验」语义：null/缺省放行，空数组拒绝）。
func DecodeUpdate(ctx context.Context, body []byte) (UpdateIn, error) {
	var req UpdateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return UpdateIn{}, err
	}
	if req.RoleIDs != nil && len(req.RoleIDs) == 0 {
		return UpdateIn{}, errs.BadRequest("用户角色不能为空")
	}
	return UpdateIn{CreateIn: req.toIn(), ID: req.ID}, nil
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

// ResetPwdReq 重置密码请求体，对应 UserResetPwdSchema。
type ResetPwdReq struct {
	ID       *int64 `json:"id" validate:"required" label:"用户ID"`
	Password string `json:"password" validate:"required,password" label:"用户密码"`
}

// ChangeStatusReq 状态修改请求体，对应 UserChangeStatusSchema（status 可选）。
type ChangeStatusReq struct {
	ID     *int64  `json:"id" validate:"required" label:"用户ID"`
	Status *string `json:"status" label:"账号状态"`
}

// DecodeResetPwd 解析重置密码入参。
func DecodeResetPwd(ctx context.Context, body []byte) (ResetPwdIn, error) {
	var req ResetPwdReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return ResetPwdIn{}, err
	}
	return ResetPwdIn{ID: req.ID, Password: &req.Password}, nil
}

// DecodeChangeStatus 解析状态修改入参。
func DecodeChangeStatus(ctx context.Context, body []byte) (ChangeStatusIn, error) {
	var req ChangeStatusReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return ChangeStatusIn{}, err
	}
	return ChangeStatusIn{ID: req.ID, Status: req.Status}, nil
}

// ProfileUpdateReq 修改个人基本资料请求体，对应 UserProfileUpdateSchema（全可选）。
type ProfileUpdateReq struct {
	NickName    *string `json:"nickName" validate:"omitempty,max=30" label:"用户昵称"`
	Email       *string `json:"email" validate:"omitempty,max=50,pyemailOpt" label:"邮箱"`
	PhoneNumber *string `json:"phonenumber" validate:"omitempty,mobile" label:"手机号"`
	Sex         *string `json:"sex" label:"用户性别"`
}

// DecodeProfileUpdate 解析个人中心资料修改入参。
func DecodeProfileUpdate(ctx context.Context, body []byte) (ProfileUpdateIn, error) {
	var req ProfileUpdateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return ProfileUpdateIn{}, err
	}
	return ProfileUpdateIn{NickName: req.NickName, PhoneNumber: req.PhoneNumber, Email: req.Email, Sex: req.Sex}, nil
}

var mobileRE = regexp.MustCompile(`^1(3\d|4[4-9]|5[0-35-9]|6[67]|7[013-8]|8[0-9]|9[0-9])\d{8}$`)

// UpdatePwdReq 修改个人密码请求体，对应 UserPasswordUpdateSchema。
type UpdatePwdReq struct {
	OldPassword string `json:"oldPassword" validate:"required,notblank" label:"旧密码"`
	NewPassword string `json:"newPassword" validate:"required,password" label:"新密码"`
}

// DecodeUpdatePwd 解析个人密码修改入参。
func DecodeUpdatePwd(ctx context.Context, body []byte) (UpdatePwdIn, error) {
	var req UpdatePwdReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return UpdatePwdIn{}, err
	}
	return UpdatePwdIn{OldPassword: req.OldPassword, NewPassword: req.NewPassword}, nil
}
