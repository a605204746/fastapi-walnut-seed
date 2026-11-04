package menu

// 本文件定义 getRouters 的出参 DTO，对应 Python 端 menu/schema.py 的
// MetaResp / RouterResp 与 menu/service.py 的 router_to_dict。

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// Meta 路由元信息，对应 MetaResp。
//
// ⚠️ 与 Router 不同：meta 是**全量输出**——Python 端
// `router.meta.model_dump(by_alias=True)` 不做空值过滤，5 个键恒定出现、
// 无值为 null。故这里一律不加 omitempty。
type Meta struct {
	Title   *string `json:"title"`
	Icon    *string `json:"icon"`
	NoCache *bool   `json:"noCache"`
	// Link 内链地址（http(s):// 开头），非内链时为 null
	Link *string `json:"link"`
	// ActiveMenu 高亮菜单（备注以 / 开头时取备注），否则 null
	ActiveMenu *string `json:"activeMenu"`
}

// Router 前端路由节点，对应 RouterResp + router_to_dict 的空值过滤。
//
// ⚠️ omitempty 在这里是**契约**而不是优化。Python 端 router_to_dict 的规则是
// 「None 与空字符串跳过，布尔值（含 false）保留，空集合跳过」，逐条对应：
//   - 字符串字段用 *string，构造时把 "" 归一成 nil（见 strOrNil）
//   - hidden / alwaysShow 用 *bool —— 指向 false 的指针不是零值，omitempty 不会丢它
//   - children 用切片，nil 与空切片都被 omitempty 丢掉，正好对应 `if router.children:`
//   - meta 用 *Meta，nil 时整个键消失（is_menu_frame 分支就会把它置空）
//
// 字段顺序照抄 router_to_dict 的写入顺序。
type Router struct {
	Name       *string  `json:"name,omitempty"`
	Path       *string  `json:"path,omitempty"`
	Hidden     *bool    `json:"hidden,omitempty"`
	Redirect   *string  `json:"redirect,omitempty"`
	Component  *string  `json:"component,omitempty"`
	Query      *string  `json:"query,omitempty"`
	AlwaysShow *bool    `json:"alwaysShow,omitempty"`
	Meta       *Meta    `json:"meta,omitempty"`
	Children   []Router `json:"children,omitempty"`
}

// ==================== 管理端 DTO（menu 模块 8 端点） ====================

// Out 菜单出参，对应 MenuOutSchema（键序 = 声明序）。
//
// ⚠️ Children 恒为 []（default_factory=list），不能 omitempty。
type Out struct {
	ID         *idtype.JSInt   `json:"id"`
	MenuName   *string         `json:"menuName"`
	ParentID   *idtype.JSInt   `json:"parentId"`
	OrderNum   *idtype.JSInt   `json:"orderNum"`
	Path       *string         `json:"path"`
	Component  *string         `json:"component"`
	QueryParam *string         `json:"queryParam"`
	IsFrame    *string         `json:"isFrame"`
	IsCache    *string         `json:"isCache"`
	MenuType   *string         `json:"menuType"`
	Visible    *string         `json:"visible"`
	Status     *string         `json:"status"`
	Perms      *string         `json:"perms"`
	Icon       *string         `json:"icon"`
	CreateDept *idtype.JSInt   `json:"createDept"`
	Remark     *string         `json:"remark"`
	CreateTime *timex.JSONTime `json:"createTime"`
	Children   []Out           `json:"children"`
}

// NewOut 由实体构造出参。
func NewOut(m *SysMenu) Out {
	out := Out{
		ID:         jsIntPtr(m.ID),
		MenuName:   strPtr(m.MenuName),
		ParentID:   jsIntPtrOrNil(m.ParentID),
		OrderNum:   intPtrToJSInt(m.OrderNum),
		Path:       m.Path,
		Component:  m.Component,
		QueryParam: m.QueryParam,
		IsFrame:    m.IsFrame,
		IsCache:    m.IsCache,
		MenuType:   m.MenuType,
		Visible:    m.Visible,
		Status:     m.Status,
		Perms:      m.Perms,
		Icon:       m.Icon,
		CreateDept: jsIntPtrOrNil(m.CreateDept),
		Remark:     m.Remark,
		Children:   []Out{},
	}
	if m.CreateTime != nil {
		jt := timex.FromTime(*m.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// TreeNode 菜单下拉树节点，对应 build_menu_tree_select 的手工字典。
//
// ⚠️ 键序 = 字典字面量顺序；children 后加且仅非空时出现。
type TreeNode struct {
	ID       *idtype.JSInt `json:"id"`
	ParentID *idtype.JSInt `json:"parentId"`
	Label    *string       `json:"label"`
	Weight   *idtype.JSInt `json:"weight"`
	MenuType *string       `json:"menuType"`
	Icon     *string       `json:"icon"`
	Visible  *string       `json:"visible"`
	Status   *string       `json:"status"`
	Children []*TreeNode   `json:"children,omitempty"`
}

// TreeSelectVo 角色菜单树响应，对应 MenuTreeSelectVoSchema。
type TreeSelectVo struct {
	CheckedKeys []idtype.JSInt `json:"checkedKeys"`
	Menus       []*TreeNode    `json:"menus"`
}

// Query 菜单列表/下拉树查询参数，对应 MenuQuerySchema（不分页）。
type Query struct {
	MenuName string
	Visible  string
	Status   string
	MenuType string
	ParentID *int64
}

// BindQuery 从查询串解析。parentId 非法整数 → Pydantic 400 原文。
func BindQuery(q crudx.Querier) (Query, error) {
	out := Query{
		MenuName: q.Query("menuName"),
		Visible:  q.Query("visible"),
		Status:   q.Query("status"),
		MenuType: q.Query("menuType"),
	}
	if raw, ok := q.GetQuery("parentId"); ok && strings.TrimSpace(raw) != "" {
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return Query{}, errs.BadRequest(msgValidInt)
		}
		out.ParentID = &n
	}
	return out, nil
}

// CreateIn 新增/修改菜单入参，对应 MenuCreateSchema。
//
// ⚠️ 模型级 validate_default=True：**缺省字段也跑校验器**。
type CreateIn struct {
	ID         *int64
	ParentID   *int64
	MenuName   *string
	OrderNum   *int
	Path       *string
	Component  *string
	QueryParam *string
	IsFrame    *string
	IsCache    *string
	MenuType   *string
	Visible    *string
	Status     *string
	Perms      *string
	Icon       *string
	Remark     *string
}

// UpdateIn 修改菜单入参（id 必填）。
type UpdateIn struct {
	CreateIn
}

// Pydantic v2 类型错误原文（属契约）。
const (
	msgFieldRequired = "Field required"
	msgValidInt      = "Input should be a valid integer, unable to parse string as an integer"
	msgValidIntNum   = "Input should be a valid integer, got a number with a fractional part"
	msgValidIntT     = "Input should be a valid integer"
	msgValidStr      = "Input should be a valid string"
)

// permsRe 对应 RegexConstants.PERMISSION_STRING（re.match + $ → 补 NL 分支）。
var (
	permsRe   = regexp.MustCompile(`^$|^[a-zA-Z0-9_]+:[a-zA-Z0-9_*]+:[a-zA-Z0-9_*]+$`)
	permsReNL = regexp.MustCompile(`^$|^[a-zA-Z0-9_]+:[a-zA-Z0-9_*]+:[a-zA-Z0-9_*]+\n$`)
)

// valueErr 校验器错误对外文案（"Value error, " 前缀已被全局剥掉）。
func valueErr(msg string) string { return msg }

// DecodeCreate 解析并校验新增入参。
//
// 校验器声明序：menu_name → order_num → path → component → query_param → menu_type → perms；
// 模型级 validate_default=True → **缺省也校验**。
func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	// id（create schema 里也有，可选）
	if raw, ok := fields["id"]; ok {
		v, terr := decodeInt(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ID = v
		}
	}
	if raw, ok := fields["parentId"]; ok {
		v, terr := decodeInt(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ParentID = v
		}
	}
	// menu_name：缺省也校验
	v, terr := optStr(fields, "menuName")
	if terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.MenuName = v
		switch {
		case v == nil || strings.TrimSpace(*v) == "":
			msgs = append(msgs, valueErr("菜单名称不能为空"))
		case utf8.RuneCountInString(*v) > 50:
			msgs = append(msgs, valueErr("菜单名称长度不能超过50个字符"))
		}
	}
	// order_num：缺省也校验
	n, terr := optInt(fields, "orderNum")
	if terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.OrderNum = intPtr(n)
		if n == nil {
			msgs = append(msgs, valueErr("显示顺序不能为空"))
		}
	}
	// path
	v, terr = optStr(fields, "path")
	if terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Path = v
		if v != nil && utf8.RuneCountInString(*v) > 200 {
			msgs = append(msgs, valueErr("路由地址不能超过200个字符"))
		}
	}
	// component
	v, terr = optStr(fields, "component")
	if terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Component = v
		if v != nil && utf8.RuneCountInString(*v) > 200 {
			msgs = append(msgs, valueErr("组件路径不能超过200个字符"))
		}
	}
	// query_param：非空必须是 JSON 对象
	v, terr = optStr(fields, "queryParam")
	if terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.QueryParam = v
		if v != nil && strings.TrimSpace(*v) != "" {
			var obj map[string]any
			if json.Unmarshal([]byte(*v), &obj) != nil {
				msgs = append(msgs, valueErr("路由参数必须符合JSON格式"))
			}
		}
	}
	// 其余无校验器字段
	for _, f := range []struct {
		key string
		dst **string
	}{
		{"isFrame", &in.IsFrame},
		{"isCache", &in.IsCache},
		{"visible", &in.Visible},
		{"status", &in.Status},
		{"icon", &in.Icon},
		{"remark", &in.Remark},
	} {
		v, terr := optStr(fields, f.key)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			*f.dst = v
		}
	}
	// menu_type：缺省也校验
	v, terr = optStr(fields, "menuType")
	if terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.MenuType = v
		if v == nil || strings.TrimSpace(*v) == "" {
			msgs = append(msgs, valueErr("菜单类型不能为空"))
		}
	}
	// perms：长度 + 格式
	v, terr = optStr(fields, "perms")
	if terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Perms = v
		switch {
		case v != nil && utf8.RuneCountInString(*v) > 100:
			msgs = append(msgs, valueErr("权限标识长度不能超过100个字符"))
		case v != nil && !permsRe.MatchString(*v) && !permsReNL.MatchString(*v):
			msgs = append(msgs, valueErr("权限标识必须符合 tool:build:list 格式"))
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeUpdate 解析修改入参。id 为 Field(...) 必填：缺省报 Field required。
func DecodeUpdate(body []byte) (UpdateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return UpdateIn{}, err
	}
	base, baseErr := DecodeCreate(body)
	in := UpdateIn{CreateIn: base}

	var msgs []string
	if baseErr != nil {
		if se := errs.As(baseErr); se != nil {
			msgs = append(msgs, se.Msg)
		}
	}
	if raw, ok := fields["id"]; !ok {
		msgs = append(msgs, msgFieldRequired)
		in.ID = nil
	} else if v, terr := decodeRequiredInt(raw); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.ID = v
	}
	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// ---- 内部工具 ----

func rawFields(body []byte) (map[string]json.RawMessage, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, errs.BadRequest(errs.MsgJSONInvalid)
	}
	if fields == nil {
		return map[string]json.RawMessage{}, nil
	}
	return fields, nil
}

// optStr 取可选字符串字段：缺省与 null 均为 nil。
func optStr(fields map[string]json.RawMessage, key string) (*string, string) {
	raw, ok := fields[key]
	if !ok || string(raw) == "null" {
		return nil, ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, msgValidStr
	}
	return &s, ""
}

// optInt 取可选整型字段（Pydantic lax）。
func optInt(fields map[string]json.RawMessage, key string) (*int64, string) {
	raw, ok := fields[key]
	if !ok || string(raw) == "null" {
		return nil, ""
	}
	return decodeInt(raw)
}

func decodeRequiredInt(raw json.RawMessage) (*int64, string) {
	if string(raw) == "null" {
		return nil, msgValidIntT
	}
	n, ok, terr := parseLooseInt(raw)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, msgValidIntT
	}
	return &n, ""
}

func decodeInt(raw json.RawMessage) (*int64, string) {
	n, ok, terr := parseLooseInt(raw)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, ""
	}
	return &n, ""
}

func parseLooseInt(raw json.RawMessage) (int64, bool, string) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, false, ""
	}
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, false, msgValidStr
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return 0, false, msgValidInt
		}
		return n, true, ""
	}
	if text == "true" || text == "false" {
		return 0, false, msgValidIntT
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false, msgValidInt
	}
	if f != float64(int64(f)) {
		return 0, false, msgValidIntNum
	}
	return int64(f), true, ""
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func jsIntPtrOrNil(v *int64) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(*v)
	return &j
}

func intPtrToJSInt(v *int) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(int64(*v))
	return &j
}

func intPtr(v *int64) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
