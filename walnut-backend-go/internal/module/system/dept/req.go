// 本文件定义部门域的入参 DTO，对应 Python 端 module_system/dept/schema.py。
//
// 写入入参走阶段 4 惯用绑定（validate tag + bindx.BindJSON + i18n 文案）；
// 查询 cluster（BindQueryParam*）仍保留 Pydantic 兼容原文（解析器单点托管于
// bindx.PyQueryInt / bindx.ParseDateQuery）。

package dept

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

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
// 与 post 模块同款的已知偏差（见 post/req.go queryInt 注释）：非法格式不在
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

// queryInt 读取可选整型查询参数，非法值视为未提供（同 post/req.go）。
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

// BindQueryParamChecked 带契约校验的查询参数绑定（dept/list、dept/treeselect 用）。
//
// Python DeptQueryParam：id/parentId/belongDeptId 是 int|None——出现（含空串）
// 即走 Pydantic lax int，非法 → 400 int_parsing 原文；beginTime/endTime 是
// DateStr——非法 → 400 date 解析原文（**不是**「无效的日期格式」，见 bindx.ParseDateQuery）。
// 多个错误按字段声明序拼接：id → parentId → … → belongDeptId → beginTime → endTime。
//
// ⚠️ 旧 BindQueryParam（静默忽略非法值）仍被 user 模块引用，保留不动。
func BindQueryParamChecked(q crudx.Querier) (QueryParam, error) {
	var out QueryParam
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
	out.DeptName = q.Query("deptName")
	out.DeptCategory = q.Query("deptCategory")
	out.Status = q.Query("status")
	intField("belongDeptId", &out.BelongDeptID)
	dateField("beginTime", &out.BeginTime)
	dateField("endTime", &out.EndTime)
	if len(msgs) > 0 {
		return out, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return out, nil
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

// Pydantic v2 的类型错误原文（handler 的 ID 串解析仍在用，单点托管于 bindx）。
const msgValidInt = bindx.MsgValidInt

// emailRe 对应 RegexConstants.EMAIL。
//
// ⚠️ Python 用 re.match 且模式以 $ 结尾：$ 在 re.match 下也匹配
// 「字符串末尾的换行之前」，故 "a@b.co\n" 在 Python 端**合法**。
// Go 的 $ 默认只匹配文本末尾，需补一个带换行的分支（同 crudx orderBy 的处理）。
var (
	emailRe   = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	emailReNL = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\n$`)
)

func init() {
	// pyemail：逐字复刻 RegexConstants.EMAIL（含末尾换行分支）。
	// 中央 validatex 已把 pyemail 映射到 validation.email（「邮箱格式不正确」）。
	bindx.RegisterTag("pyemail", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return emailRe.MatchString(s) || emailReNL.MatchString(s)
	})
}

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - lax/严格字符串回退均退役（"5"/"3.0"/"1_0" 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - parentId/id 缺省或 null 统一报必填（旧实现缺省报 Field required、null 报 int_type）
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集

// CreateReq 新增部门请求体，对应 DeptCreateSchema。
type CreateReq struct {
	ParentID     *int64  `json:"parentId" validate:"required" label:"父部门ID"`
	DeptName     string  `json:"deptName" validate:"required,notblank,max=30" label:"部门名称"`
	DeptCategory *string `json:"deptCategory" validate:"omitnil,max=100" label:"部门类别编码"`
	OrderNum     *int    `json:"orderNum" validate:"required" label:"显示顺序"`
	Leader       *int64  `json:"leader" label:"负责人"`
	Phone        *string `json:"phone" validate:"omitnil,max=11" label:"联系电话"`
	Email        *string `json:"email" validate:"omitnil,max=50,pyemail" label:"邮箱"`
	Status       *string `json:"status" label:"部门状态"`
}

// UpdateReq 修改部门请求体（id 必填，追加在父类字段之后）。
type UpdateReq struct {
	CreateReq
	ID *int64 `json:"id" validate:"required" label:"部门ID"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		ParentID:     r.ParentID,
		DeptName:     &r.DeptName,
		DeptCategory: r.DeptCategory,
		OrderNum:     r.OrderNum,
		Leader:       r.Leader,
		Phone:        r.Phone,
		Email:        r.Email,
		Status:       r.Status,
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
