package role

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/modelx"
	"walnut-seed-go/internal/pkg/timex"
)

const (
	msgFieldRequired   = "Field required"
	msgValidInt        = "Input should be a valid integer, unable to parse string as an integer"
	msgValidStr        = "Input should be a valid string"
	msgRoleNameEmpty   = "角色名称不能为空"
	msgRoleKeyEmpty    = "角色权限字符串不能为空"
	msgRoleSortEmpty   = "显示顺序不能为空"
	msgRoleNameTooLong = "角色名称长度不能超过30个字符"
	msgRoleKeyTooLong  = "权限字符长度不能超过100个字符"
	msgRoleIDEmpty     = "角色ID不能为空"
	msgUserIDEmpty     = "用户ID不能为空"
	msgMaxLen30        = "角色名称长度不能超过30个字符"
	msgMaxLen100       = "权限字符长度不能超过100个字符"
)

type Out struct {
	ID       *idtype.JSInt `json:"id"`
	RoleName *string       `json:"roleName"`
	RoleKey  *string       `json:"roleKey"`
	RoleSort *idtype.JSInt `json:"roleSort"`
	// DataScope 数据范围（1-6），CHAR(1) 字符串契约
	DataScope *string `json:"dataScope"`
	// MenuCheckStrictly / DeptCheckStrictly 实体列是整型 1/0，
	// 出参经 _int_to_bool 转成布尔（None 保持 None）
	MenuCheckStrictly *bool           `json:"menuCheckStrictly"`
	DeptCheckStrictly *bool           `json:"deptCheckStrictly"`
	Status            *string         `json:"status"`
	Remark            *string         `json:"remark"`
	CreateTime        *timex.JSONTime `json:"createTime"`
	// Flag 用户是否持有此角色，默认 false（authRole 接口才会置 true）
	Flag bool `json:"flag"`
	// SuperAdmin 由 ID == 1 派生，不是数据库列
	SuperAdmin bool `json:"superAdmin"`
}

func NewOut(r *SysRole) Out {
	out := Out{
		ID:                jsIntPtr(r.ID),
		RoleName:          strPtr(r.RoleName),
		RoleKey:           strPtr(r.RoleKey),
		RoleSort:          jsIntPtr(int64(r.RoleSort)),
		DataScope:         r.DataScope,
		MenuCheckStrictly: intToBool(r.MenuCheckStrictly),
		DeptCheckStrictly: intToBool(r.DeptCheckStrictly),
		Status:            strPtr(r.Status),
		Remark:            r.Remark,
		Flag:              false,
		SuperAdmin:        r.ID == modelx.SuperAdminID,
	}
	if r.CreateTime != nil {
		t := timex.FromTime(*r.CreateTime)
		out.CreateTime = &t
	}
	return out
}

func toOuts(rows []SysRole) []Out {
	outs := make([]Out, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewOut(&rows[i]))
	}
	return outs
}

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

func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	// roleName: 校验器仅在字段**出现**时运行（含显式 null）；
	// 缺省时 Pydantic 未开 validate_default，默认 None 跳过校验器（由 service 层报 500）。
	raw, has := fields["roleName"]
	if has {
		if isNull(raw) {
			msgs = append(msgs, msgRoleNameEmpty)
		} else if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleName = &v
			if strings.TrimSpace(v) == "" {
				msgs = append(msgs, msgRoleNameEmpty)
			} else if utf8.RuneCountInString(v) > 30 {
				msgs = append(msgs, msgMaxLen30)
			}
		}
	}

	// roleKey: 同 roleName（出现即校验，含 null；纯空白拒绝）
	raw, has = fields["roleKey"]
	if has {
		if isNull(raw) {
			msgs = append(msgs, msgRoleKeyEmpty)
		} else if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleKey = &v
			if strings.TrimSpace(v) == "" {
				msgs = append(msgs, msgRoleKeyEmpty)
			} else if utf8.RuneCountInString(v) > 100 {
				msgs = append(msgs, msgMaxLen100)
			}
		}
	}

	// roleSort: 出现即校验（null → 「显示顺序不能为空」）；缺省跳过校验器
	raw, has = fields["roleSort"]
	if has {
		if isNull(raw) {
			msgs = append(msgs, msgRoleSortEmpty)
		} else if v, terr := decodeSmallInt(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleSort = &v
		}
	}

	// dataScope: optional
	if raw, ok := fields["dataScope"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DataScope = &v
		}
	}

	// menuCheckStrictly: optional bool
	if raw, ok := fields["menuCheckStrictly"]; ok && !isNull(raw) {
		if v, terr := decodeBool(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.MenuCheckStrictly = &v
		}
	}

	// deptCheckStrictly: optional bool
	if raw, ok := fields["deptCheckStrictly"]; ok && !isNull(raw) {
		if v, terr := decodeBool(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DeptCheckStrictly = &v
		}
	}

	// status: optional
	if raw, ok := fields["status"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Status = &v
		}
	}

	// remark: optional
	if raw, ok := fields["remark"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Remark = &v
		}
	}

	// menuIds: optional list[int]
	if raw, ok := fields["menuIds"]; ok && !isNull(raw) {
		if v, terr := decodeIntSlice(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.MenuIDs = v
		}
	}

	// deptIds: optional list[int]
	if raw, ok := fields["deptIds"]; ok && !isNull(raw) {
		if v, terr := decodeIntSlice(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DeptIDs = v
		}
	}

	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
}

func DecodeUpdate(body []byte) (UpdateIn, error) {
	createIn, createErr := DecodeCreate(body)
	in := UpdateIn{CreateIn: createIn}
	fields, _ := rawFields(body)
	var msgs []string
	// Pydantic 按声明序收集全部错误：父类字段在前，子类新增的 id 在最后
	if createErr != nil {
		if se := errs.As(createErr); se != nil {
			msgs = append(msgs, se.Msg)
		}
	}
	// id: 出现即校验（null → 「角色ID不能为空」400）；
	// 缺省默认 None 跳过校验器，由 controller 报 500「角色ID不能为空」
	if raw, has := fields["id"]; has {
		if isNull(raw) {
			msgs = append(msgs, msgRoleIDEmpty)
		} else if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.ID = &v
		}
	}
	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
}

// ---- DataScope 入参 ----

type DataScopeIn struct {
	ID                *int64
	DataScope         *string
	DeptCheckStrictly *bool
	DeptIDs           []int64
}

func DecodeDataScope(body []byte) (DataScopeIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return DataScopeIn{}, err
	}
	var in DataScopeIn
	var msgs []string

	// id: 出现即校验（null → 「角色ID不能为空」400）；缺省跳过校验器，
	// 由 controller 报 500「角色ID不能为空」（RoleDataScopeSchema/RoleStatusSchema 同款）
	if raw, has := fields["id"]; has {
		if isNull(raw) {
			msgs = append(msgs, msgRoleIDEmpty)
		} else if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.ID = &v
		}
	}

	if raw, ok := fields["dataScope"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DataScope = &v
		}
	}

	if raw, ok := fields["deptCheckStrictly"]; ok && !isNull(raw) {
		if v, terr := decodeBool(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DeptCheckStrictly = &v
		}
	}

	if raw, ok := fields["deptIds"]; ok && !isNull(raw) {
		if v, terr := decodeIntSlice(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.DeptIDs = v
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// ---- Status 入参 ----

type StatusIn struct {
	ID     *int64
	Status *string
}

func DecodeStatus(body []byte) (StatusIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return StatusIn{}, err
	}
	var in StatusIn
	var msgs []string

	// id: 出现即校验（null → 「角色ID不能为空」400）；缺省跳过校验器，
	// 由 controller 报 500「角色ID不能为空」（RoleDataScopeSchema/RoleStatusSchema 同款）
	if raw, has := fields["id"]; has {
		if isNull(raw) {
			msgs = append(msgs, msgRoleIDEmpty)
		} else if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.ID = &v
		}
	}

	// status: 无校验器，缺省/null 均为 None（service 层报 500「角色状态不能为空」）
	if raw, ok := fields["status"]; ok && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.Status = &v
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// ---- Cancel 入参 ----

type CancelIn struct {
	RoleID *int64
	UserID *int64
}

func DecodeCancel(body []byte) (CancelIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CancelIn{}, err
	}
	var in CancelIn
	var msgs []string

	// roleId/userId: 出现即校验（null → 400 校验器原文，按声明序拼接）；
	// 缺省默认 None 跳过校验器，由 service 报 500「角色ID不能为空」/「用户ID不能为空」
	if raw, has := fields["roleId"]; has {
		if isNull(raw) {
			msgs = append(msgs, msgRoleIDEmpty)
		} else if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleID = &v
		}
	}

	if raw, has := fields["userId"]; has {
		if isNull(raw) {
			msgs = append(msgs, msgUserIDEmpty)
		} else if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.UserID = &v
		}
	}

	if len(msgs) > 0 {
		return in, errs.WithCode(errs.CodeBadRequest, strings.Join(msgs, ", "))
	}
	return in, nil
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

// ---- AuthUser 出参 ----

type AuthUserOut struct {
	ID          *idtype.JSInt   `json:"id"`
	DeptID      *idtype.JSInt   `json:"deptId"`
	UserName    *string         `json:"userName"`
	NickName    *string         `json:"nickName"`
	Email       *string         `json:"email"`
	PhoneNumber *string         `json:"phonenumber"`
	Status      *string         `json:"status"`
	CreateTime  *timex.JSONTime `json:"createTime"`
	DeptName    *string         `json:"deptName"`
}

type AuthUserPageOut struct {
	Rows  []AuthUserOut `json:"rows"`
	Total idtype.JSInt  `json:"total"`
}

// ---- 导出 ----

const sheetName = "角色数据"

var dataScopeLabels = map[string]string{
	"1": "全部数据权限",
	"2": "自定义数据权限",
	"3": "本部门数据权限",
	"4": "本部门及以下数据权限",
	"5": "仅本人数据权限",
	"6": "部门及以下或本人数据权限",
}

var statusLabels = map[string]string{
	"0": "正常",
	"1": "停用",
}

func ExportColumns() []string {
	return []string{
		"id", "role_name", "role_key", "role_sort", "data_scope",
		"menu_check_strictly", "dept_check_strictly", "status", "remark", "create_time",
	}
}

func ExportHeaders() []string {
	return []string{
		"角色序号", "角色名称", "角色权限", "角色排序", "数据范围",
		"菜单树选择项是否关联显示", "部门树选择项是否关联显示", "角色状态", "备注", "创建时间",
	}
}

func ExportRow(r SysRole) map[string]any {
	m := map[string]any{
		"id":                  r.ID,
		"role_name":           r.RoleName,
		"role_key":            r.RoleKey,
		"role_sort":           r.RoleSort,
		"data_scope":          "",
		"menu_check_strictly": boolOrNone(r.MenuCheckStrictly),
		"dept_check_strictly": boolOrNone(r.DeptCheckStrictly),
		"status":              "",
		"remark":              r.Remark,
		"create_time":         "",
	}
	// 字典未命中保留原值（Python dict.get(value, value)），None → ""
	if r.DataScope != nil {
		if label, ok := dataScopeLabels[*r.DataScope]; ok {
			m["data_scope"] = label
		} else {
			m["data_scope"] = *r.DataScope
		}
	}
	if label, ok := statusLabels[r.Status]; ok {
		m["status"] = label
	} else {
		m["status"] = r.Status
	}
	if r.CreateTime != nil {
		m["create_time"] = r.CreateTime.Format("2006-01-02 15:04:05")
	}
	return m
}

func boolOrNone(v *int) any {
	if v == nil {
		return nil
	}
	return *v != 0
}

// ---- 解码工具 ----

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

func decodeSmallInt(raw json.RawMessage) (int, *string) {
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		var f float64
		if err2 := json.Unmarshal(raw, &f); err2 == nil {
			msg := "Input should be a valid integer"
			return 0, &msg
		}
		msg := msgValidInt
		return 0, &msg
	}
	return v, nil
}

func decodeInt64(raw json.RawMessage) (int64, *string) {
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil {
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 == nil {
			n, err3 := parseLooseInt64(s)
			if err3 == nil {
				return n, nil
			}
		}
		msg := msgValidInt
		return 0, &msg
	}
	return v, nil
}

func decodeBool(raw json.RawMessage) (bool, *string) {
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		msg := "Input should be a valid boolean"
		return false, &msg
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

func parseLooseInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errs.BadRequest(msgValidInt)
	}
	var v int64
	for _, c := range s {
		if c < '0' || c > '9' {
			if c == '-' && v == 0 {
				continue
			}
			return 0, errs.BadRequest(msgValidInt)
		}
		v = v*10 + int64(c-'0')
	}
	if len(s) > 0 && s[0] == '-' {
		v = -v
	}
	return v, nil
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

// ---- 基础辅助 ----

func intToBool(v *int) *bool {
	if v == nil {
		return nil
	}
	b := *v != 0
	return &b
}

func boolToInt(v *bool) *int {
	if v == nil {
		return nil
	}
	n := 0
	if *v {
		n = 1
	}
	return &n
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func strPtr(v string) *string {
	s := v
	return &s
}
