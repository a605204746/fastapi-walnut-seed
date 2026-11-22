// 本文件定义通知公告域的入参/查询 DTO，对应 Python 端 module_system/notice/schema.py。
//
// ⚠️ notice_title 带 validate_default：缺省也跑校验器（「公告标题不能为空」）；
// update 的 id 同样 validate_default，缺省报「公告ID不能为空」（不是 Field required）。
//
// JSON body 走阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案。

package notice

import (
	"context"
	"regexp"

	"github.com/go-playground/validator/v10"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
)

// ---- 查询入参 ----

// Query 通知公告列表查询参数，对应 NoticeQueryParam。
type Query struct {
	Page         crudx.Page
	NoticeTitle  string
	NoticeType   string
	CreateByName string
}

// BindQuery 从查询串解析。
func BindQuery(q crudx.Querier) (Query, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return Query{}, err
	}
	return Query{
		Page:         page,
		NoticeTitle:  q.Query("noticeTitle"),
		NoticeType:   q.Query("noticeType"),
		CreateByName: q.Query("createByName"),
	}, nil
}

// ---- 写入入参 ----

// CreateIn 新增通知公告入参，对应 NoticeCreateSchema。
type CreateIn struct {
	NoticeTitle   *string
	NoticeType    *string
	NoticeContent *string
	Status        *string
	Remark        *string
}

// UpdateIn 修改通知公告入参（id 带 validate_default 校验器）。
type UpdateIn struct {
	CreateIn
	ID *int64
}

// Pydantic v2 类型错误原文（handler 的 ID 串解析仍在用，单点托管于 bindx）。
const msgValidInt = bindx.MsgValidInt

// htmlMarkRe 对应 xss_util.RE_HTML_MARK = (<[^<]*?>)|(<[<]*?[^<]*?>)，忽略大小写。
var htmlMarkRe = regexp.MustCompile(`(?i)(<[^<]*?>)|(<[<]*?[^<]*?>)`)

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的语义演进（刻意，见 docs/go/04-接口契约）：
//   - lax 强转退役（"3.0"/"1_0" 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - 长度文案新格式（"公告标题长度不能超过50个字符"）
//   - 脚本字符检查经 noscript tag 复刻，文案走中央映射的原始业务文案
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集

func init() {
	// noscript 复刻 check_notice_title 的脚本字符检查（缺省/空白/超长先行拒绝，
	// 与旧 validateTitle 的 switch 顺序一致：tag 链首个失败即该字段唯一错误）。
	bindx.RegisterTag("noscript", func(fl validator.FieldLevel) bool {
		return !htmlMarkRe.MatchString(fl.Field().String())
	})
}

// CreateReq 新增通知公告请求体，对应 NoticeCreateSchema。
// 声明序：notice_title → notice_type → notice_content → status → remark。
type CreateReq struct {
	NoticeTitle   string  `json:"noticeTitle" validate:"required,notblank,max=50,noscript" label:"公告标题"`
	NoticeType    *string `json:"noticeType"`
	NoticeContent *string `json:"noticeContent"`
	Status        *string `json:"status"`
	Remark        *string `json:"remark"`
}

// UpdateReq 修改通知公告请求体（id 追加在父类字段之后，缺省/null 报「公告ID不能为空」）。
type UpdateReq struct {
	CreateReq
	ID *int64 `json:"id" validate:"required" label:"公告ID"`
}

func (r CreateReq) toIn() CreateIn {
	return CreateIn{
		NoticeTitle:   &r.NoticeTitle,
		NoticeType:    r.NoticeType,
		NoticeContent: r.NoticeContent,
		Status:        r.Status,
		Remark:        r.Remark,
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
