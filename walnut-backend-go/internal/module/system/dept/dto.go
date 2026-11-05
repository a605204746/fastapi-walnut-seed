// 本文件定义部门域的入参/出参 DTO，对应 Python 端 module_system/dept/schema.py。
//
// ⚠️ 契约约束（与 post/dto.go 同一套，改动前先看 docs/go/01-续作交接.md §5.2）：
//   - 出参键名一律**驼峰**，字段顺序照抄 Pydantic 声明序（键序是响应字节的一部分）
//   - 出参字段**恒定出现**，无值为 null，故可空字段用指针且不加 omitempty
//   - 整数出参走 idtype.JSInt（Python 端对所有 int 施加 _encode_int）
//   - 入参校验失败的 msg 是 **Pydantic 原文 / field_validator 原文**，前端直接展示
//
// 入参逐字段手工解析的原因（按声明序收集全部错误、类型错误字段跳过校验器）
// 见 post/dto.go 顶部注释，此处不重复。

package dept

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

// ---- 出参 ----

// Out 部门出参，对应 DeptOutSchema。
//
// ⚠️ Children 恒为 []（Pydantic default_factory=list），**不能加 omitempty**：
// 列表/详情每一行都带 "children": []，与 treeselect 的手工字典形态不同。
type Out struct {
	ID           *idtype.JSInt   `json:"id"`
	ParentID     *idtype.JSInt   `json:"parentId"`
	ParentName   *string         `json:"parentName"`
	Ancestors    *string         `json:"ancestors"`
	DeptName     *string         `json:"deptName"`
	DeptCategory *string         `json:"deptCategory"`
	OrderNum     *idtype.JSInt   `json:"orderNum"`
	Leader       *idtype.JSInt   `json:"leader"`
	LeaderName   *string         `json:"leaderName"`
	Phone        *string         `json:"phone"`
	Email        *string         `json:"email"`
	Status       *string         `json:"status"`
	CreateTime   *timex.JSONTime `json:"createTime"`
	Children     []Out           `json:"children"`
}

// NewOut 由实体构造出参，对应 DeptOutSchema.model_validate(dept)。
// ParentName / LeaderName 留空，由 service 的 fillOut 回填。
func NewOut(d *SysDept) Out {
	out := Out{
		ID:           jsIntPtrOrNil(d.ID),
		ParentID:     jsIntPtrOrNilPtr(d.ParentID),
		Ancestors:    d.Ancestors,
		DeptName:     d.DeptName,
		DeptCategory: d.DeptCategory,
		OrderNum:     intPtrToJSInt(d.OrderNum),
		Leader:       jsIntPtrOrNilPtr(d.Leader),
		Phone:        d.Phone,
		Email:        d.Email,
		Status:       d.Status,
		Children:     []Out{},
	}
	if d.CreateTime != nil {
		jt := timex.FromTime(*d.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// TreeNode 部门下拉树节点，对应 build_dept_tree_select 里构造的字典。
//
// ⚠️ 键序 = 字典字面量顺序：id, parentId, label, weight, disabled，
// children 是**后加的**故排在最后，且**只在非空时出现**（omitempty）。
type TreeNode struct {
	ID       idtype.JSInt  `json:"id"`
	ParentID *idtype.JSInt `json:"parentId"`
	Label    *string       `json:"label"`
	Weight   *idtype.JSInt `json:"weight"`
	Disabled bool          `json:"disabled"`
	Children []*TreeNode   `json:"children,omitempty"`
}

// ---- 查询入参 ----

// QueryParam 部门列表查询参数，对应 DeptQueryParam。
//
// ⚠️ 部门列表**不分页**（Python 端 list_dept 无 PageQueryParam 继承），
// 故这里没有 crudx.Page 字段——带上会让响应形态变成 {"rows","total"}。
type QueryParam struct {
	ID           *int64
	ParentID     *int64
	DeptName     string
	DeptCategory string
	Status       string
	BelongDeptID *int64
	BeginTime    string
	EndTime      string
}

// BindQueryParam 从查询串解析，对应 FastAPI 的 Depends() 注入。
//
// ⚠️ beginTime/endTime 在 Python 端是 DateStr（格式非法会 400），这里按原串透传，
// 与 post 模块同款的已知偏差（见 post/dto.go queryInt 注释）：非法格式不在
// 契约 diff 覆盖内，若后续比对抓到再补校验。
func BindQueryParam(q crudx.Querier) QueryParam {
	return QueryParam{
		ID:           queryInt(q, "id"),
		ParentID:     queryInt(q, "parentId"),
		DeptName:     q.Query("deptName"),
		DeptCategory: q.Query("deptCategory"),
		Status:       q.Query("status"),
		BelongDeptID: queryInt(q, "belongDeptId"),
		BeginTime:    q.Query("beginTime"),
		EndTime:      q.Query("endTime"),
	}
}

// queryInt 读取可选整型查询参数，非法值视为未提供（同 post/dto.go）。
func queryInt(q crudx.Querier, key string) *int64 {
	raw, ok := q.GetQuery(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// ---- 写入入参 ----

// CreateIn 新增部门入参，对应 DeptCreateSchema。
//
// 字段一律用指针：dept_name / order_num 是 `default=None, validate_default=True`，
// **缺省也要跑校验器**（报「不能为空」），必须区分「未提供」与「提供了 null」。
type CreateIn struct {
	ParentID     *int64
	DeptName     *string
	DeptCategory *string
	OrderNum     *int
	Leader       *int64
	Phone        *string
	Email        *string
	Status       *string
}

// UpdateIn 修改部门入参，对应 DeptUpdateSchema（= DeptCreateSchema + id）。
type UpdateIn struct {
	CreateIn
	ID *int64
}

// Pydantic v2 的类型错误原文（属契约，前端直接展示）。
const (
	msgFieldRequired = "Field required"
	msgValidInt      = "Input should be a valid integer, unable to parse string as an integer"
	msgValidIntNum   = "Input should be a valid integer, got a number with a fractional part"
	msgValidIntT     = "Input should be a valid integer"
	msgValidStr      = "Input should be a valid string"
)

// emailRe 对应 RegexConstants.EMAIL。
//
// ⚠️ Python 用 re.match 且模式以 $ 结尾：$ 在 re.match 下也匹配
// 「字符串末尾的换行之前」，故 "a@b.co\n" 在 Python 端**合法**。
// Go 的 $ 默认只匹配文本末尾，需补一个带换行的分支（同 crudx orderBy 的处理）。
var (
	emailRe   = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	emailReNL = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\n$`)
)

// valueErr 返回校验器错误对外的最终文案（"Value error, " 前缀已被
// Python 全局异常处理剥掉，净效果 = 原文）。详见 post/dto.go 同名注释。
func valueErr(msg string) string { return msg }

// DecodeCreate 解析并校验新增入参，返回 Pydantic 风格的错误文案。
//
// 字段顺序必须与 DeptCreateSchema 的声明序一致：
// parent_id → dept_name → dept_category → order_num → leader → phone → email → status
func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	// parent_id: 必填（无 field_validator，缺省报 Field required）
	if raw, ok := fields["parentId"]; !ok {
		msgs = append(msgs, msgFieldRequired)
	} else if v, terr := decodeRequiredInt(raw); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.ParentID = v
	}

	// dept_name: 非空 + 长度 ≤ 30（validate_default：缺省也报「不能为空」）
	if v, terr := decodeStr(fields["deptName"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.DeptName = v
		if v == nil || strings.TrimSpace(*v) == "" {
			msgs = append(msgs, valueErr("部门名称不能为空"))
		} else if utf8.RuneCountInString(*v) > 30 {
			msgs = append(msgs, valueErr("部门名称长度不能超过30个字符"))
		}
	}

	// dept_category: 可空，非空时长度 ≤ 100
	if v, terr := decodeStr(fields["deptCategory"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.DeptCategory = v
		if v != nil && utf8.RuneCountInString(*v) > 100 {
			msgs = append(msgs, valueErr("部门类别编码长度不能超过100个字符"))
		}
	}

	// order_num: 非空（validate_default：缺省报「显示顺序不能为空」）
	if v, terr := decodeInt(fields["orderNum"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.OrderNum = intPtr(v)
		if v == nil {
			msgs = append(msgs, valueErr("显示顺序不能为空"))
		}
	}

	// leader: 可空整型，无校验器
	if v, terr := decodeInt(fields["leader"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Leader = v
	}

	// phone: 可空，非空时长度 ≤ 11
	if v, terr := decodeStr(fields["phone"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Phone = v
		if v != nil && utf8.RuneCountInString(*v) > 11 {
			msgs = append(msgs, valueErr("联系电话长度不能超过11个字符"))
		}
	}

	// email: 可空，非空时长度 ≤ 50 且格式正确
	if v, terr := decodeStr(fields["email"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Email = v
		if v != nil {
			switch {
			case utf8.RuneCountInString(*v) > 50:
				msgs = append(msgs, valueErr("邮箱长度不能超过50个字符"))
			case !emailRe.MatchString(*v) && !emailReNL.MatchString(*v):
				msgs = append(msgs, valueErr("邮箱格式不正确"))
			}
		}
	}

	// status: 可空字符串，无校验器
	if v, terr := decodeStr(fields["status"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Status = v
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeUpdate 解析并校验修改入参。
//
// 字段顺序 = 父类 DeptCreateSchema 的字段 + 子类新增的 id（继承时追加在末尾）。
// ⚠️ 与 post 不同：dept 的 id 是 `Field(...)` 必填且**无 field_validator**，
// 缺省报 "Field required" 而不是「部门ID不能为空」。
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

// rawFields 解析 JSON 对象的顶层键值，保留原始字节（同 post/dto.go）。
func rawFields(body []byte) (map[string]json.RawMessage, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		// 空请求体：Pydantic 把所有字段当缺省处理，从而触发全部缺省校验
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

// decodeRequiredInt 解析**必填**整型字段：缺省报 Field required，
// null 报 int_type 原文（Pydantic 对非 Optional 的 int 收到 null 的报错）。
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

// decodeInt 解析可空整型字段，返回 (值, 类型错误文案)（同 post/dto.go）。
func decodeInt(raw json.RawMessage) (*int64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	n, ok, terr := parseLooseInt(raw)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, ""
	}
	return &n, ""
}

// decodeStr 解析可空字符串字段（Pydantic lax 模式不把数字强转成字符串）。
func decodeStr(raw json.RawMessage) (*string, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, msgValidStr
	}
	return &s, ""
}

// parseLooseInt 按 Pydantic lax 模式解析整数（同 post/dto.go）。
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

// ---- 小工具（与其它模块的 dto.go 同构）----

func jsIntPtrOrNil(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func jsIntPtrOrNilPtr(v *int64) *idtype.JSInt {
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
