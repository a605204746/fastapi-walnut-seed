package role

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// 仍被 handler（500 兜底文案）与查询 cluster 引用的常量；
// msgRoleSortEmpty/msgUserIDEmpty 亦用于 Decode* 的显式 null 后置文案（HEAD 原文）。
const (
	msgFieldRequired = "Field required"
	msgValidInt      = bindx.MsgValidInt
	msgRoleNameEmpty = "角色名称不能为空"
	msgRoleKeyEmpty  = "角色权限字符串不能为空"
	msgRoleSortEmpty = "显示顺序不能为空"
	msgRoleIDEmpty   = "角色ID不能为空"
	msgUserIDEmpty   = "用户ID不能为空"
)

// ---- 查询参数 ----

type QueryParam struct {
	Page      crudx.Page
	RoleName  string
	RoleKey   string
	Status    string
	BeginTime string
	EndTime   string
}

func BindQueryParam(q crudx.Querier) (QueryParam, error) {
	page, pageErr := crudx.BindPageQuery(q)
	var msgs []string
	if pageErr != nil {
		if se := errs.As(pageErr); se != nil {
			msgs = append(msgs, se.Msg)
		}
	}
	out := QueryParam{
		Page:     page,
		RoleName: strings.TrimSpace(q.Query("roleName")),
		RoleKey:  strings.TrimSpace(q.Query("roleKey")),
		Status:   strings.TrimSpace(q.Query("status")),
	}
	// beginTime/endTime 是 DateStr：非法 → 400（Pydantic date 解析原文）；
	// 合法 → 规范化 YYYY-MM-DD。错误按声明序排在分页错误之后。
	// ⚠️ 不 trim：Python 端原串进 Pydantic，' 2024-01-01' 同样非法。
	if raw, ok := q.GetQuery("beginTime"); ok {
		v, derr := parseDateQuery(raw)
		if derr != nil {
			if se := errs.As(derr); se != nil {
				msgs = append(msgs, se.Msg)
			}
		} else {
			out.BeginTime = v
		}
	}
	if raw, ok := q.GetQuery("endTime"); ok {
		v, derr := parseDateQuery(raw)
		if derr != nil {
			if se := errs.As(derr); se != nil {
				msgs = append(msgs, se.Msg)
			}
		} else {
			out.EndTime = v
		}
	}
	if len(msgs) > 0 {
		return out, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return out, nil
}

// ---- 入参 ----

type CreateIn struct {
	RoleName          *string
	RoleKey           *string
	RoleSort          *int
	DataScope         *string
	MenuCheckStrictly *bool
	DeptCheckStrictly *bool
	Status            *string
	Remark            *string
	MenuIDs           []int64
	DeptIDs           []int64
}

type UpdateIn struct {
	CreateIn
	ID *int64
}

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的文案演进（接受语义冻结于旧 Go 行为，不收紧）：
//   - lax 强转退役（IntDigits 纯数字循环的 "5"/"-"/"--5" 怪癖不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集
//
// 接受语义（与 HEAD 逐字镜像）：roleName/roleKey/roleSort 缺省跳过（*string/*int
// + omitnil 前缀，nil 跳过全部 tag），显式 null 由 Decode* 后置 presence 检查补
// HEAD 原文「不能为空」；Update/DataScope/Status/Cancel 的 id 旧实现缺省跳过
// （无 else 分支，由 controller 报 500），故去掉 required，仅显式 null 后置报错。
// null 后置消息追加在 tag 错误之后，多字段同时 null+校验错的拼接序与旧实现可能
// 不同（排序边缘让步，见测试注释）。

// CreateReq 新增角色请求体，对应 RoleCreateSchema。
// roleName/roleKey/roleSort 缺省跳过（omitnil），显式 null 由 DecodeCreate 后置报错。
type CreateReq struct {
	RoleName          *string `json:"roleName" validate:"omitnil,notblank,max=30" label:"角色名称"`
	RoleKey           *string `json:"roleKey" validate:"omitnil,notblank,max=100" label:"角色权限字符串"`
	RoleSort          *int    `json:"roleSort" label:"显示顺序"`
	DataScope         *string `json:"dataScope" label:"数据范围"`
	MenuCheckStrictly *bool   `json:"menuCheckStrictly" label:"菜单树选择项是否关联显示"`
	DeptCheckStrictly *bool   `json:"deptCheckStrictly" label:"部门树选择项是否关联显示"`
	Status            *string `json:"status" label:"角色状态"`
	Remark            *string `json:"remark" label:"备注"`
	MenuIDs           []int64 `json:"menuIds" label:"菜单组"`
	DeptIDs           []int64 `json:"deptIds" label:"部门组"`
}

// UpdateReq 修改角色请求体（id 缺省跳过、显式 null 后置报错，追加在父类字段之后）。
type UpdateReq struct {
	CreateReq
	ID *int64 `json:"id" label:"角色ID"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		RoleName:          r.RoleName,
		RoleKey:           r.RoleKey,
		RoleSort:          r.RoleSort,
		DataScope:         r.DataScope,
		MenuCheckStrictly: r.MenuCheckStrictly,
		DeptCheckStrictly: r.DeptCheckStrictly,
		Status:            r.Status,
		Remark:            r.Remark,
		MenuIDs:           r.MenuIDs,
		DeptIDs:           r.DeptIDs,
	}
}

// nullMsg 后置 presence 检查：键存在且原始值为显式 null（指针解为 nil）时，
// 追加 HEAD 原文「不能为空」。*string/*int64 无法区分缺省与 null（都 nil），故用
// bindx.Fields 的键存在性 + IsNull 精确定位 null；缺省（键不存在）跳过。
// ⚠️ IsNull 守卫确保 JSON 类型错误字段（present 非 null 但解为 nil）不误报。
func nullMsg(msgs []string, fields map[string]json.RawMessage, key string, isNil bool, msg string) []string {
	if raw, ok := fields[key]; ok && bindx.IsNull(raw) && isNil {
		return append(msgs, msg)
	}
	return msgs
}

// bindMsgs 提取 BindJSON 错误信封文案（nil 错误返回空切片）。
func bindMsgs(err error) ([]string, error) {
	if err == nil {
		return nil, nil
	}
	se := errs.As(err)
	if se == nil {
		return nil, err
	}
	return []string{se.Msg}, nil
}

// DecodeCreate 解析并校验新增入参（service 消费 CreateIn，零改动）。
func DecodeCreate(ctx context.Context, body []byte) (CreateIn, error) {
	var req CreateReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return CreateIn{}, err
	}
	fields, _ := bindx.Fields(body)
	// 显式 null 后置（声明序：roleName → roleKey → roleSort），文案逐字 HEAD 原文。
	msgs = nullMsg(msgs, fields, "roleName", req.RoleName == nil, msgRoleNameEmpty)
	msgs = nullMsg(msgs, fields, "roleKey", req.RoleKey == nil, msgRoleKeyEmpty)
	msgs = nullMsg(msgs, fields, "roleSort", req.RoleSort == nil, msgRoleSortEmpty)
	if len(msgs) > 0 {
		return CreateIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return req.toIn(), nil
}

// DecodeUpdate 解析修改入参（id 缺省跳过、显式 null 后置报错，排在父类字段之后）。
func DecodeUpdate(ctx context.Context, body []byte) (UpdateIn, error) {
	var req UpdateReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return UpdateIn{}, err
	}
	fields, _ := bindx.Fields(body)
	msgs = nullMsg(msgs, fields, "roleName", req.RoleName == nil, msgRoleNameEmpty)
	msgs = nullMsg(msgs, fields, "roleKey", req.RoleKey == nil, msgRoleKeyEmpty)
	msgs = nullMsg(msgs, fields, "roleSort", req.RoleSort == nil, msgRoleSortEmpty)
	msgs = nullMsg(msgs, fields, "id", req.ID == nil, msgRoleIDEmpty)
	if len(msgs) > 0 {
		return UpdateIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return UpdateIn{CreateIn: req.CreateReq.toIn(), ID: req.ID}, nil
}

// ---- DataScope 入参 ----

type DataScopeIn struct {
	ID                *int64
	DataScope         *string
	DeptCheckStrictly *bool
	DeptIDs           []int64
}

// DataScopeReq 数据范围修改请求体，对应 RoleDataScopeSchema。
// id 缺省跳过（由 controller 报 500），显式 null 由 DecodeDataScope 后置报错。
type DataScopeReq struct {
	ID                *int64  `json:"id" label:"角色ID"`
	DataScope         *string `json:"dataScope" label:"数据范围"`
	DeptCheckStrictly *bool   `json:"deptCheckStrictly" label:"部门树选择项是否关联显示"`
	DeptIDs           []int64 `json:"deptIds" label:"部门组"`
}

// DecodeDataScope 解析数据范围入参。
func DecodeDataScope(ctx context.Context, body []byte) (DataScopeIn, error) {
	var req DataScopeReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return DataScopeIn{}, err
	}
	fields, _ := bindx.Fields(body)
	msgs = nullMsg(msgs, fields, "id", req.ID == nil, msgRoleIDEmpty)
	if len(msgs) > 0 {
		return DataScopeIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return DataScopeIn{ID: req.ID, DataScope: req.DataScope, DeptCheckStrictly: req.DeptCheckStrictly, DeptIDs: req.DeptIDs}, nil
}

// ---- Status 入参 ----

type StatusIn struct {
	ID     *int64
	Status *string
}

// StatusReq 状态修改请求体（status 无校验器，缺省/null 由 service 层报空；
// id 缺省跳过由 controller 报 500，显式 null 由 DecodeStatus 后置报错）。
type StatusReq struct {
	ID     *int64  `json:"id" label:"角色ID"`
	Status *string `json:"status" label:"角色状态"`
}

// DecodeStatus 解析状态修改入参。
func DecodeStatus(ctx context.Context, body []byte) (StatusIn, error) {
	var req StatusReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return StatusIn{}, err
	}
	fields, _ := bindx.Fields(body)
	msgs = nullMsg(msgs, fields, "id", req.ID == nil, msgRoleIDEmpty)
	if len(msgs) > 0 {
		return StatusIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return StatusIn{ID: req.ID, Status: req.Status}, nil
}

// ---- Cancel 入参 ----

type CancelIn struct {
	RoleID *int64
	UserID *int64
}

// CancelReq 取消用户授权请求体，对应 RoleCancelSchema。
// roleId/userId 缺省跳过（由 service 报 500），显式 null 由 DecodeCancel 后置报错。
type CancelReq struct {
	RoleID *int64 `json:"roleId" label:"角色ID"`
	UserID *int64 `json:"userId" label:"用户ID"`
}

// DecodeCancel 解析取消授权入参。
func DecodeCancel(ctx context.Context, body []byte) (CancelIn, error) {
	var req CancelReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return CancelIn{}, err
	}
	fields, _ := bindx.Fields(body)
	// 显式 null 后置（声明序：roleId → userId），文案逐字 HEAD 原文。
	msgs = nullMsg(msgs, fields, "roleId", req.RoleID == nil, msgRoleIDEmpty)
	msgs = nullMsg(msgs, fields, "userId", req.UserID == nil, msgUserIDEmpty)
	if len(msgs) > 0 {
		return CancelIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return CancelIn{RoleID: req.RoleID, UserID: req.UserID}, nil
}

// ---- AuthUser 查询 ----

type AuthUserQueryParam struct {
	Page crudx.Page
	// RoleID 可选（Python AuthUserQueryParam.role_id: int | None = None）；
	// nil 表示不按角色过滤（allocatedList 仍 JOIN 关联表，unallocatedList 不排除）
	RoleID      *int64
	UserName    string
	PhoneNumber string
	Status      string
}

func BindAuthUserQueryParam(q crudx.Querier) (AuthUserQueryParam, error) {
	page, pageErr := crudx.BindPageQuery(q)
	var msgs []string
	if pageErr != nil {
		if se := errs.As(pageErr); se != nil {
			msgs = append(msgs, se.Msg)
		}
	}
	out := AuthUserQueryParam{
		Page:        page,
		UserName:    strings.TrimSpace(q.Query("userName")),
		PhoneNumber: strings.TrimSpace(q.Query("phonenumber")),
		Status:      strings.TrimSpace(q.Query("status")),
	}
	// roleId 缺省 → 不过滤；出现（含空串）→ Pydantic lax int，非法报 int_parsing 原文
	if raw, ok := q.GetQuery("roleId"); ok {
		n, valid := parsePyQueryInt(raw)
		if !valid {
			msgs = append(msgs, msgValidInt)
		} else {
			out.RoleID = &n
		}
	}
	if len(msgs) > 0 {
		return out, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return out, nil
}

// ---- 查询参数解析（镜像 Pydantic / Python int 语义）----

// parsePyQueryInt 镜像 Pydantic v2 的 str→int（查询参数 lax 模式）：
// 允许首尾空白、+/- 号、数字间单个下划线、".全零" 小数（'5.00'→5、'1_0'→10）；
// 不允许指数形式。失败返回 false（调用方报 int_parsing 原文）。
func parsePyQueryInt(raw string) (int64, bool) { return parsePyInt(raw, true) }

// parsePyIntLiteral 镜像 Python 裸 int()（optionselect 用）：
// 规则同上但**不接受小数**（'5.0' → ValueError）。
func parsePyIntLiteral(raw string) (int64, bool) { return parsePyInt(raw, false) }

func parsePyInt(raw string, allowFraction bool) (int64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	neg := false
	if s[0] == '+' || s[0] == '-' {
		neg = s[0] == '-'
		s = s[1:]
	}
	if allowFraction {
		if i := strings.IndexByte(s, '.'); i >= 0 {
			frac := s[i+1:]
			if frac == "" {
				return 0, false
			}
			for _, ch := range frac {
				if ch != '0' {
					return 0, false
				}
			}
			s = s[:i]
		}
	}
	var b strings.Builder
	prevDigit := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= '0' && ch <= '9':
			b.WriteByte(ch)
			prevDigit = true
		case ch == '_' && prevDigit && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			prevDigit = false
		default:
			return 0, false
		}
	}
	if b.Len() == 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(b.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}

// parseDateQuery 镜像 Pydantic v2（speedate）对 DateStr 查询串的解析。
// 成功返回规范化 YYYY-MM-DD；失败返回 Pydantic 原文（code=400）。
//
// ⚠️ GET 查询串先经 Pydantic 的 date 类型强转：非法值报 date_from_datetime_parsing
// 原文（如 "input is too short"），**不是** validator 的「无效的日期格式」——
// 后者只在强转成功后的值上运行，查询场景不可达（已用 Python 实测钉住）。
// ⚠️ 纯数字串走 Unix 时间戳路径且优先于日期格式（'0000000000' → 1970-01-01）；
// 仅当时间戳恰好是整天时才是合法 date，否则报 inexact 原文。
func parseDateQuery(raw string) (string, error) {
	const prefix = "Input should be a valid date or datetime, "
	const inexact = "Datetimes provided to dates should have zero time - e.g. be exact dates"
	bad := func(msg string) (string, error) { return "", errs.BadRequest(prefix + msg) }

	if isPlainNumeric(raw) {
		f, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) &&
			f == math.Trunc(f) && f >= math.MinInt64 && f <= math.MaxInt64 && int64(f)%86400 == 0 {
			return time.Unix(int64(f), 0).UTC().Format("2006-01-02"), nil
		}
		return "", errs.BadRequest(inexact)
	}
	if len(raw) < 10 {
		return bad("input is too short")
	}
	for i := 0; i < 4; i++ {
		if !isDigitByte(raw[i]) {
			return bad("invalid character in year")
		}
	}
	if raw[4] != '-' {
		return bad("invalid date separator, expected `-`")
	}
	if !isDigitByte(raw[5]) || !isDigitByte(raw[6]) {
		return bad("invalid character in month")
	}
	if raw[7] != '-' {
		return bad("invalid date separator, expected `-`")
	}
	if !isDigitByte(raw[8]) || !isDigitByte(raw[9]) {
		return bad("invalid character in day")
	}
	year := int(raw[0]-'0')*1000 + int(raw[1]-'0')*100 + int(raw[2]-'0')*10 + int(raw[3]-'0')
	month := int(raw[5]-'0')*10 + int(raw[6]-'0')
	day := int(raw[8]-'0')*10 + int(raw[9]-'0')
	if year == 0 {
		return "", errs.BadRequest("Input should be a valid date in the format YYYY-MM-DD, year 0 is out of range")
	}
	if month < 1 || month > 12 {
		return bad("month value is outside expected range of 1-12")
	}
	if day < 1 || day > daysInMonth(year, month) {
		return bad("day value is outside expected range")
	}
	if len(raw) == 10 {
		return raw, nil
	}
	// datetime 后缀：分隔符 + 时间（仅零时间合法）+ 可选时区
	switch raw[10] {
	case 'T', 't', '_', ' ':
	default:
		return bad("invalid datetime separator, expected `T`, `t`, `_` or space")
	}
	tp := raw[11:]
	if len(tp) < 5 {
		return bad("input is too short")
	}
	if !isDigitByte(tp[0]) || !isDigitByte(tp[1]) {
		return bad("invalid character in hour")
	}
	hour := int(tp[0]-'0')*10 + int(tp[1]-'0')
	if hour > 23 {
		return bad("hour value is outside expected range of 0-23")
	}
	if tp[2] != ':' {
		return bad("invalid time separator, expected `:`")
	}
	if !isDigitByte(tp[3]) || !isDigitByte(tp[4]) {
		return bad("invalid character in minute")
	}
	minute := int(tp[3]-'0')*10 + int(tp[4]-'0')
	if minute > 59 {
		return bad("minute value is outside expected range of 0-59")
	}
	second, fracZero := 0, true
	rest := tp[5:]
	if len(rest) > 0 && rest[0] == ':' {
		if len(rest) < 3 || !isDigitByte(rest[1]) || !isDigitByte(rest[2]) {
			return bad("invalid character in second")
		}
		second = int(rest[1]-'0')*10 + int(rest[2]-'0')
		if second > 59 {
			return bad("second value is outside expected range of 0-59")
		}
		rest = rest[3:]
		if len(rest) > 0 && rest[0] == '.' {
			rest = rest[1:]
			n := 0
			for n < len(rest) && isDigitByte(rest[n]) {
				if rest[n] != '0' {
					fracZero = false
				}
				n++
			}
			if n == 0 {
				return bad("second fraction digits missing after `.`")
			}
			rest = rest[n:]
		}
	}
	// 时区：Z/z 或 ±HH:MM（偏移不改变「零时间」判定，Python 实测 +08:00 仍合法）
	if len(rest) > 0 && rest != "Z" && rest != "z" {
		if (rest[0] != '+' && rest[0] != '-') || len(rest) != 6 ||
			!isDigitByte(rest[1]) || !isDigitByte(rest[2]) || rest[3] != ':' ||
			!isDigitByte(rest[4]) || !isDigitByte(rest[5]) {
			return bad("invalid timezone sign")
		}
		oh := int(rest[1]-'0')*10 + int(rest[2]-'0')
		om := int(rest[4]-'0')*10 + int(rest[5]-'0')
		if oh*60+om >= 24*60 {
			return bad("timezone offset must be less than 24 hours")
		}
	}
	if hour != 0 || minute != 0 || second != 0 || !fracZero {
		return "", errs.BadRequest(inexact)
	}
	return raw[:10], nil
}

func isDigitByte(ch byte) bool { return ch >= '0' && ch <= '9' }

// isPlainNumeric 判定 Pydantic 时间戳路径接受的数字串：
// 可选符号 + 数字 + 至多一个小数点（'.5'、'5.' 均可），**不含下划线/指数**
// （Go 的 ParseFloat 接受 '1_0'，Pydantic 不接受，须先过滤）。
func isPlainNumeric(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	digits, dot := 0, false
	for ; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			digits++
			continue
		}
		if ch == '.' && !dot {
			dot = true
			continue
		}
		return false
	}
	return digits > 0
}

func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	default:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	}
}
