// 本文件定义岗位域的入参/出参 DTO，对应 Python 端 module_system/post/schema.py。
//
// ⚠️ 契约约束（与 user/dto.go 同一套，改动前先看 docs/go/01-续作交接.md §5.2）：
//   - 出参键名一律**驼峰**，字段顺序照抄 Pydantic 声明序（键序是响应字节的一部分）
//   - 出参字段**恒定出现**，无值为 null，故可空字段用指针且不加 omitempty
//   - 整数出参走 idtype.JSInt（Python 端对所有 int 施加 _encode_int）
//   - 入参校验失败的 msg 是 **Pydantic 原文**，会被前端直接展示给用户
//
// # 入参为什么要逐字段手工解析
//
// Pydantic 的行为有两点 gin 的 ShouldBindJSON 无法复刻：
//  1. **按字段声明顺序收集全部错误**，再用 ", " 拼接——不是一个错误就返回
//  2. 某字段类型错误时，该字段的 field_validator **不再执行**（每字段至多一条错误），
//     但其余字段照常校验
//
// 故这里先把 JSON 顶层键值保留为原始字节，再按声明序逐字段「先解析类型、后跑校验器」。
// 这套写法是后续所有 CRUD 模块的样板。

package post

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// ---- 出参 ----

// Out 岗位出参，对应 PostOutSchema。
type Out struct {
	ID           *idtype.JSInt   `json:"id"`
	DeptID       *idtype.JSInt   `json:"deptId"`
	PostCode     *string         `json:"postCode"`
	PostName     *string         `json:"postName"`
	PostCategory *string         `json:"postCategory"`
	PostSort     *idtype.JSInt   `json:"postSort"`
	Status       *string         `json:"status"`
	Remark       *string         `json:"remark"`
	CreateTime   *timex.JSONTime `json:"createTime"`
	// DeptName 部门名翻译，由 service 层批量回填（不是 sys_post 的列）
	DeptName *string `json:"deptName"`
}

// NewOut 由实体构造出参，对应 PostOutSchema.model_validate(post)。
// DeptName 留空，由 service 的 fillDeptName 回填。
func NewOut(p *SysPost) Out {
	out := Out{
		ID:           jsIntPtr(p.ID),
		DeptID:       jsIntPtr(p.DeptID),
		PostCode:     strPtr(p.PostCode),
		PostName:     strPtr(p.PostName),
		PostCategory: p.PostCategory,
		PostSort:     jsIntPtr(int64(p.PostSort)),
		Status:       strPtr(p.Status),
		Remark:       p.Remark,
	}
	out.CreateTime = jsonTimeOrNil(p.CreateTime)
	return out
}

// Info 岗位精简信息，对应 PostInfoSchema（供用户等模块关联使用）。
type Info struct {
	ID           *idtype.JSInt `json:"id"`
	DeptID       *idtype.JSInt `json:"deptId"`
	PostCode     *string       `json:"postCode"`
	PostName     *string       `json:"postName"`
	PostCategory *string       `json:"postCategory"`
}

// NewInfo 由实体构造精简出参。
func NewInfo(p *SysPost) Info {
	return Info{
		ID:           jsIntPtr(p.ID),
		DeptID:       jsIntPtr(p.DeptID),
		PostCode:     strPtr(p.PostCode),
		PostName:     strPtr(p.PostName),
		PostCategory: p.PostCategory,
	}
}

// ---- 查询入参 ----

// QueryParam 岗位列表查询参数，对应 PostQueryParam。
//
// 这些是 GET 查询串参数，缺省即 nil（不施加该条件），
// 与 POST 请求体的「缺省触发校验器」语义不同——查询参数没有任何 field_validator。
type QueryParam struct {
	Page         crudx.Page
	DeptID       *int64
	BelongDeptID *int64
	PostCode     string
	PostCategory string
	PostName     string
	Status       string
	BeginTime    string
	EndTime      string
}

// BindQueryParam 从查询串解析，对应 FastAPI 的 Depends() 注入。
//
// ⚠️ 用 crudx.BindPageQuery（pageSize 默认 10、上限 1000），不是 BindPage。
func BindQueryParam(q crudx.Querier) (QueryParam, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return QueryParam{}, err
	}
	p := QueryParam{Page: page}
	p.DeptID = queryInt(q, "deptId")
	p.BelongDeptID = queryInt(q, "belongDeptId")
	p.PostCode = q.Query("postCode")
	p.PostCategory = q.Query("postCategory")
	p.PostName = q.Query("postName")
	p.Status = q.Query("status")
	p.BeginTime = q.Query("beginTime")
	p.EndTime = q.Query("endTime")
	return p, nil
}

// DeptTreeQuery 部门树查询参数，对应 DeptTreeQueryParam（GET /post/deptTree）。
//
// ⚠️ 它继承的是 BaseModel 而**不是** PageQueryParam，故没有分页字段。
type DeptTreeQuery struct {
	ID           *int64
	ParentID     *int64
	BelongDeptID *int64
	DeptName     string
	DeptCategory string
	Status       string
	BeginTime    string
	EndTime      string
}

// BindDeptTreeQuery 从查询串解析部门树参数。
func BindDeptTreeQuery(q crudx.Querier) DeptTreeQuery {
	return DeptTreeQuery{
		ID:           queryInt(q, "id"),
		ParentID:     queryInt(q, "parentId"),
		BelongDeptID: queryInt(q, "belongDeptId"),
		DeptName:     q.Query("deptName"),
		DeptCategory: q.Query("deptCategory"),
		Status:       q.Query("status"),
		BeginTime:    q.Query("beginTime"),
		EndTime:      q.Query("endTime"),
	}
}

// queryInt 读取可选整型查询参数。
//
// 非法值返回 nil（视为未提供）。Python 端 Pydantic 会报校验错误，
// 但 GET 查询参数的类型错误在本项目前端不会触发；若后续契约比对发现偏差，
// 应改为返回错误而非静默忽略。
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

// CreateIn 新增岗位入参，对应 PostCreateSchema。
//
// 字段一律用指针：Pydantic 端是 `X | None = Field(default=None, validate_default=True)`，
// **缺省也要跑校验器**，故必须区分「未提供」与「提供了 null」——两者都触发同样的报错。
type CreateIn struct {
	DeptID       *int64
	PostCode     *string
	PostName     *string
	PostCategory *string
	PostSort     *int
	Status       *string
	Remark       *string
}

// UpdateIn 修改岗位入参，对应 PostUpdateSchema（= PostCreateSchema + id）。
type UpdateIn struct {
	CreateIn
	ID *int64
}

// Pydantic v2 的类型错误原文（属契约，前端直接展示）。
const (
	msgValidInt    = "Input should be a valid integer, unable to parse string as an integer"
	msgValidIntNum = "Input should be a valid integer, got a number with a fractional part"
	msgValidIntT   = "Input should be a valid integer"
	msgValidStr    = "Input should be a valid string"
)

// valueErr 返回校验器错误对外的最终文案。
//
// ⚠️ 这里有一个**跨语言的往返**，不做就会多出一截前缀（契约比对实测抓到过）：
//
//  1. Pydantic v2 把 field_validator 抛的 ValueError 包装成 "Value error, <原文>"
//  2. 项目的 _validation_msg 又把它剥掉：msg[11:].lstrip(" ,")
//     （11 正好是 "Value error" 的长度）
//
// 净效果 = **原文**，无前缀。这是 app/core/exceptions.py 里的全局规则，
// 对全部模块的所有 field_validator 都成立，故 Go 侧直接用原文。
// 保留这个空壳函数是为了把这层往返记录在调用点旁边，防止后人"顺手补上前缀"。
func valueErr(msg string) string { return msg }

// DecodeCreate 解析并校验新增入参，返回 Pydantic 风格的错误文案。
//
// 字段顺序必须与 PostCreateSchema 的声明序一致：
// dept_id → post_code → post_name → post_category → post_sort → status → remark
func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	// dept_id: 非空校验
	if v, terr := decodeInt(fields["deptId"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.DeptID = v
		if v == nil {
			msgs = append(msgs, valueErr("部门id不能为空"))
		}
	}

	// post_code: 非空 + 长度 ≤ 64
	if v, terr := decodeStr(fields["postCode"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.PostCode = v
		if v == nil || strings.TrimSpace(*v) == "" {
			msgs = append(msgs, valueErr("岗位编码不能为空"))
		} else if utf8.RuneCountInString(*v) > 64 {
			// ⚠️ Python 的 len() 数的是**字符**，不是字节；中文岗位编码会差 3 倍
			msgs = append(msgs, valueErr("岗位编码长度不能超过64个字符"))
		}
	}

	// post_name: 非空 + 长度 ≤ 50
	if v, terr := decodeStr(fields["postName"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.PostName = v
		if v == nil || strings.TrimSpace(*v) == "" {
			msgs = append(msgs, valueErr("岗位名称不能为空"))
		} else if utf8.RuneCountInString(*v) > 50 {
			msgs = append(msgs, valueErr("岗位名称长度不能超过50个字符"))
		}
	}

	// post_category: 可空，但非空时长度 ≤ 100
	if v, terr := decodeStr(fields["postCategory"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.PostCategory = v
		if v != nil && utf8.RuneCountInString(*v) > 100 {
			msgs = append(msgs, valueErr("类别编码长度不能超过100个字符"))
		}
	}

	// post_sort: 非空
	if v, terr := decodeSmallInt(fields["postSort"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.PostSort = v
		if v == nil {
			msgs = append(msgs, valueErr("显示顺序不能为空"))
		}
	}

	// status / remark: 无校验器，仅解析类型
	if v, terr := decodeStr(fields["status"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Status = v
	}
	if v, terr := decodeStr(fields["remark"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.Remark = v
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeUpdate 解析并校验修改入参。
//
// 字段顺序 = 父类 PostCreateSchema 的字段 + 子类新增的 id，
// 即 …remark 之后才是 id（Pydantic 继承时新字段追加在末尾）。
func DecodeUpdate(body []byte) (UpdateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return UpdateIn{}, err
	}
	// 复用新增的解析与校验，再把 id 追加到最后
	base, baseErr := DecodeCreate(body)
	in := UpdateIn{CreateIn: base}

	var msgs []string
	if baseErr != nil {
		if se := errs.As(baseErr); se != nil {
			msgs = append(msgs, se.Msg)
		}
	}
	if v, terr := decodeInt(fields["id"]); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.ID = v
		if v == nil {
			msgs = append(msgs, valueErr("岗位ID不能为空"))
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// rawFields 解析 JSON 对象的顶层键值，保留原始字节。
func rawFields(body []byte) (map[string]json.RawMessage, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		// 空请求体：Pydantic 会把所有字段当缺省处理，从而触发全部非空校验
		return map[string]json.RawMessage{}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, errs.BadRequest(validatexMsg(err))
	}
	if fields == nil {
		return map[string]json.RawMessage{}, nil
	}
	return fields, nil
}

// validatexMsg 把 JSON 语法错误映射成 Python 端的固定文案。
func validatexMsg(err error) string {
	// 与 validatex.MsgJSONInvalid 一致；此处内联避免为一个常量引入包依赖
	return "请求数据格式错误（JSON 解析失败）"
}

// decodeInt 解析可空整型字段，返回 (值, 类型错误文案)。
//
// 对齐 Pydantic v2 的 lax 模式：接受 JSON 数字、整值的浮点、以及数字字符串。
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

// decodeSmallInt 解析可空的小整型字段（post_sort 是 int 而非 bigint，不走 JSInt）。
func decodeSmallInt(raw json.RawMessage) (*int, string) {
	v, terr := decodeInt(raw)
	if terr != "" {
		return nil, terr
	}
	if v == nil {
		return nil, ""
	}
	n := int(*v)
	return &n, ""
}

// decodeStr 解析可空字符串字段。
//
// ⚠️ Pydantic v2 的 lax 模式**不**把数字强转成字符串，故 `"postCode": 123`
// 是类型错误而不是 "123"。
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

// parseLooseInt 按 Pydantic lax 模式解析整数。
// 返回值：解析出的数、是否为 null、类型错误文案（非空表示有错）。
func parseLooseInt(raw json.RawMessage) (int64, bool, string) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, false, ""
	}
	// 字符串形式：必须是纯整数字面量
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
	// 布尔不是整数（Pydantic lax 模式拒绝 bool → int）
	if text == "true" || text == "false" {
		return 0, false, msgValidIntT
	}
	// 数字：区分整值与带小数部分
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false, msgValidInt
	}
	if f != float64(int64(f)) {
		return 0, false, msgValidIntNum
	}
	return int64(f), true, ""
}

// ---- 小工具（与其它模块的 dto.go 同构，便于复制）----

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
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
