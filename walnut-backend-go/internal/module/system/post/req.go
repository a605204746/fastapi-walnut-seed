// 本文件定义岗位域的入参 DTO，对应 Python 端 module_system/post/schema.py。
//
// JSON body（DecodeCreate/DecodeUpdate）走阶段 4 惯用绑定：
// validate tag + bindx.BindJSON + i18n 文案（见 docs/go/04-接口契约）。
//
// ⚠️ 查询串（BindQueryParam/BindDeptTreeQuery）与导出 form（bindExportForm）
// 仍是 Pydantic 原文契约：form 校验失败的 msg 是 **pydantic ValidationError 原文**，
// 会被前端直接展示给用户，且 user 模块 BindExportParam 在复用——一字不动。

package post

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/validatex"
)

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
// deptId/belongDeptId 是 int|None、beginTime/endTime 是 DateStr|None：
// 出现（含空串）即校验，非法 → 400 Pydantic 原文；多个错误按声明序拼接
// （pageNum/pageSize → deptId → belongDeptId → beginTime → endTime，已实测钉住）。
func BindQueryParam(q crudx.Querier) (QueryParam, error) {
	page, pageErr := crudx.BindPageQuery(q)
	p := QueryParam{Page: page}
	var msgs []string
	if pageErr != nil {
		se := errs.As(pageErr)
		if se == nil {
			return QueryParam{}, pageErr
		}
		msgs = append(msgs, se.Msg)
	}
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
	intField("deptId", &p.DeptID)
	intField("belongDeptId", &p.BelongDeptID)
	p.PostCode = q.Query("postCode")
	p.PostCategory = q.Query("postCategory")
	p.PostName = q.Query("postName")
	p.Status = q.Query("status")
	dateField("beginTime", &p.BeginTime)
	dateField("endTime", &p.EndTime)
	if len(msgs) > 0 {
		return QueryParam{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
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
//
// id/parentId/belongDeptId 是 int|None、beginTime/endTime 是 DateStr|None：
// 出现（含空串）即校验，非法 → 400 Pydantic 原文，按声明序拼接
// （id → parentId → belongDeptId → beginTime → endTime）。
func BindDeptTreeQuery(q crudx.Querier) (DeptTreeQuery, error) {
	var out DeptTreeQuery
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
	intField("id", &out.ID)
	intField("parentId", &out.ParentID)
	intField("belongDeptId", &out.BelongDeptID)
	out.DeptName = q.Query("deptName")
	out.DeptCategory = q.Query("deptCategory")
	out.Status = q.Query("status")
	dateField("beginTime", &out.BeginTime)
	dateField("endTime", &out.EndTime)
	if len(msgs) > 0 {
		return DeptTreeQuery{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return out, nil
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

// Pydantic v2 的类型错误原文（查询/form 绑定仍在用，单点托管于 bindx）。
const msgValidInt = bindx.MsgValidInt

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - lax 强转退役（"3.0"/"1_0"/整值浮点 2.0 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集
//
// 字段序镜像 PostCreateSchema 声明序：
// dept_id → post_code → post_name → post_category → post_sort → status → remark，
// update 的 id 追加在末尾（Pydantic 继承时新字段追加）。

// CreateReq 新增岗位请求体，对应 PostCreateSchema（必填字段均 validate_default）。
type CreateReq struct {
	DeptID       *int64  `json:"deptId" validate:"required" label:"部门id"`
	PostCode     string  `json:"postCode" validate:"required,notblank,max=64" label:"岗位编码"`
	PostName     string  `json:"postName" validate:"required,notblank,max=50" label:"岗位名称"`
	PostCategory *string `json:"postCategory" validate:"omitempty,max=100" label:"类别编码"`
	PostSort     *int    `json:"postSort" validate:"required" label:"显示顺序"`
	Status       *string `json:"status"`
	Remark       *string `json:"remark"`
}

// UpdateReq 修改岗位请求体（= CreateReq + id）。
type UpdateReq struct {
	CreateReq
	ID *int64 `json:"id" validate:"required" label:"岗位ID"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		DeptID:       r.DeptID,
		PostCode:     &r.PostCode,
		PostName:     &r.PostName,
		PostCategory: r.PostCategory,
		PostSort:     r.PostSort,
		Status:       r.Status,
		Remark:       r.Remark,
	}
}

// DecodeCreate 解析并校验新增入参（service 消费 CreateIn，零改动）。
func DecodeCreate(ctx context.Context, body []byte) (CreateIn, error) {
	var req CreateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return CreateIn{}, err
	}
	return req.toIn(), nil
}

// DecodeUpdate 解析修改入参。
func DecodeUpdate(ctx context.Context, body []byte) (UpdateIn, error) {
	var req UpdateReq
	if err := bindx.BindJSON(ctx, body, &req); err != nil {
		return UpdateIn{}, err
	}
	return UpdateIn{CreateIn: req.CreateReq.toIn(), ID: req.ID}, nil
}

// ---- 导出 form 校验（POST /post/export）----
//
// ⚠️ Python 端 export_post 是**手工** `PostQueryParam.model_validate(form-data)`，
// 抛的是 pydantic ValidationError（ValueError 子类）→ 全局 ValueError 处理器
// **原样透传 str(exc)**——不是列表接口那种剥净的 ", " 拼接文案。
// 实测原文形如：
//
//	1 validation error for PostQueryParam
//	deptId
//	  Input should be a valid integer, unable to parse string as an integer [type=int_parsing, input_value='abc', input_type=str]
//	    For further information visit https://errors.pydantic.dev/2.13/v/int_parsing
//
// 多错误时首行是 "N validation errors for PostQueryParam"，各字段按声明序追加。
// 另两个实测钉住的点：form 打底（**query 不参与**）、pageSize 在校验前被 pop
// （pageNum 仍参与校验，ge=1 失败时 input_value 是原始字符串、input_type=str）。

// bindExportForm 镜像 PostQueryParam.model_validate(data)（python 模式、值全为 str）。
// data 已由调用方过滤空值并 pop pageSize。
// 共享件（FormFieldError/DateErrType/ValidationErrorText/PyRepr）单点托管于 validatex。
func bindExportForm(data map[string]string) (QueryParam, error) {
	var q QueryParam
	var ferrs []validatex.FormFieldError
	addErr := func(loc, msg, typ, input string) {
		ferrs = append(ferrs, validatex.FormFieldError{Loc: loc, Msg: msg, Typ: typ, Input: input})
	}

	// page_num: int = Field(default=1, ge=1)——解析失败报 int_parsing，
	// 解析成功但 <1 报 greater_than_equal（input_value 仍是原始串）。
	if raw, ok := data["pageNum"]; ok {
		n, valid := bindx.PyQueryInt(raw)
		switch {
		case !valid:
			addErr("pageNum", msgValidInt, "int_parsing", raw)
		case n < 1:
			addErr("pageNum", "Input should be greater than or equal to 1", "greater_than_equal", raw)
		}
	}
	intField := func(key string, dst **int64) {
		if raw, ok := data[key]; ok {
			n, valid := bindx.PyQueryInt(raw)
			if !valid {
				addErr(key, msgValidInt, "int_parsing", raw)
				return
			}
			*dst = &n
		}
	}
	intField("deptId", &q.DeptID)
	intField("belongDeptId", &q.BelongDeptID)
	q.PostCode = data["postCode"]
	q.PostCategory = data["postCategory"]
	q.PostName = data["postName"]
	q.Status = data["status"]
	dateField := func(key string, dst *string) {
		if raw, ok := data[key]; ok {
			v, derr := bindx.ParseDateQuery(raw)
			if derr != nil {
				if se := errs.As(derr); se != nil {
					addErr(key, se.Msg, validatex.DateErrType(se.Msg), raw)
				}
				return
			}
			*dst = v
		}
	}
	dateField("beginTime", &q.BeginTime)
	dateField("endTime", &q.EndTime)

	if len(ferrs) > 0 {
		return QueryParam{}, errs.BadRequest(validatex.ValidationErrorText("PostQueryParam", ferrs))
	}
	return q, nil
}

// pyIntArgRE 匹配 Python int() 接受的形态：可选符号 + 数字间可单个下划线。
var pyIntArgRE = regexp.MustCompile(`^[+-]?[0-9](_?[0-9])*$`)

// pyAtoi 复刻 Python int(str)（optionselect 的 postIds 分量用）：
// 接受 "1_0"=10、"+1"、"-1"，拒绝 "3.0"/"1e2"/"_1"/"1__0"/"1_"。
// 失败返回 ValueError 原文风格的 error（调用方按模块契约映射文案，
// post/optionselect 两端同步后报「岗位ID格式有误」）。
// Python int 无溢出概念，超出 int64 时钳到边界（后续 SQL 比较同样查不到行）。
func pyAtoi(s string) (int64, error) {
	if !pyIntArgRE.MatchString(s) {
		return 0, fmt.Errorf("invalid literal for int() with base 10: %s", validatex.PyRepr(s))
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)
	if err != nil {
		if strings.HasPrefix(s, "-") {
			return math.MinInt64, nil
		}
		return math.MaxInt64, nil
	}
	return n, nil
}
