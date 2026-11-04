package role

import (
	"encoding/json"
	"strings"
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
	msgRoleKeyEmpty    = "角色权限字符不能为空"
	msgRoleSortEmpty   = "显示顺序不能为空"
	msgRoleNameTooLong = "角色名称长度不能超过30个字符"
	msgRoleKeyTooLong  = "角色权限字符长度不能超过100个字符"
	msgRoleIDEmpty     = "角色ID不能为空"
	msgMaxLen30        = "角色名称长度不能超过30个字符"
	msgMaxLen100       = "角色权限字符长度不能超过100个字符"
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
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return QueryParam{}, err
	}
	return QueryParam{
		Page:      page,
		RoleName:  strings.TrimSpace(q.Query("roleName")),
		RoleKey:   strings.TrimSpace(q.Query("roleKey")),
		Status:    strings.TrimSpace(q.Query("status")),
		BeginTime: strings.TrimSpace(q.Query("beginTime")),
		EndTime:   strings.TrimSpace(q.Query("endTime")),
	}, nil
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

	// roleName: optional at DTO level (handler checks required), max 30
	raw, has := fields["roleName"]
	if has && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleName = &v
			if utf8.RuneCountInString(v) > 30 {
				msgs = append(msgs, msgMaxLen30)
			}
		}
	}

	// roleKey: optional at DTO level, max 100
	raw, has = fields["roleKey"]
	if has && !isNull(raw) {
		if v, terr := decodeStr(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleKey = &v
			if utf8.RuneCountInString(v) > 100 {
				msgs = append(msgs, msgMaxLen100)
			}
		}
	}

	// roleSort: optional at DTO level (handler checks required)
	raw, has = fields["roleSort"]
	if has && !isNull(raw) {
		if v, terr := decodeSmallInt(raw); terr != nil {
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
	createIn, err := DecodeCreate(body)
	if err != nil {
		return UpdateIn{CreateIn: createIn}, err
	}
	fields, _ := rawFields(body)
	var msgs []string
	var idPtr *int64
	raw, has := fields["id"]
	if has && !isNull(raw) {
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

	raw, has := fields["id"]
	if !has || isNull(raw) {
		msgs = append(msgs, msgFieldRequired)
	} else if v, terr := decodeInt64(raw); terr != nil {
		msgs = append(msgs, *terr)
	} else {
		in.ID = &v
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

	raw, has := fields["id"]
	if !has || isNull(raw) {
		msgs = append(msgs, msgFieldRequired)
	} else if v, terr := decodeInt64(raw); terr != nil {
		msgs = append(msgs, *terr)
	} else {
		in.ID = &v
	}

	raw, has = fields["status"]
	if !has || isNull(raw) {
		msgs = append(msgs, msgFieldRequired)
	} else if v, terr := decodeStr(raw); terr != nil {
		msgs = append(msgs, *terr)
	} else {
		in.Status = &v
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

	if raw, ok := fields["roleId"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
			msgs = append(msgs, *terr)
		} else {
			in.RoleID = &v
		}
	}

	if raw, ok := fields["userId"]; ok && !isNull(raw) {
		if v, terr := decodeInt64(raw); terr != nil {
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
	Page        crudx.Page
	RoleID      int64
	UserName    string
	PhoneNumber string
	Status      string
}

func BindAuthUserQueryParam(q crudx.Querier) (AuthUserQueryParam, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return AuthUserQueryParam{}, err
	}
	roleIDStr := strings.TrimSpace(q.Query("roleId"))
	if roleIDStr == "" {
		return AuthUserQueryParam{}, errs.BadRequest(msgFieldRequired)
	}
	roleID, err := parseLooseInt64(roleIDStr)
	if err != nil {
		return AuthUserQueryParam{}, errs.BadRequest(msgValidInt)
	}
	return AuthUserQueryParam{
		Page:        page,
		RoleID:      roleID,
		UserName:    strings.TrimSpace(q.Query("userName")),
		PhoneNumber: strings.TrimSpace(q.Query("phonenumber")),
		Status:      strings.TrimSpace(q.Query("status")),
	}, nil
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
	if r.DataScope != nil {
		m["data_scope"] = dataScopeLabels[*r.DataScope]
	}
	m["status"] = statusLabels[r.Status]
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
