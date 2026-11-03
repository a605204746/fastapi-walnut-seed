// 本文件定义通知公告域的入参/出参 DTO，对应 Python 端 module_system/notice/schema.py。
//
// ⚠️ notice_title 带 validate_default：缺省也跑校验器（「公告标题不能为空」）；
// update 的 id 同样 validate_default，缺省报「公告ID不能为空」（不是 Field required）。
// ⚠️ 出参键序 = 声明序；create_by_name 是回填字段（创建人账号翻译）。

package notice

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

// Out 通知公告出参，对应 NoticeOutSchema（alias_generator=to_camel）。
type Out struct {
	ID            *idtype.JSInt   `json:"id"`
	NoticeTitle   *string         `json:"noticeTitle"`
	NoticeType    *string         `json:"noticeType"`
	NoticeContent *string         `json:"noticeContent"`
	Status        *string         `json:"status"`
	Remark        *string         `json:"remark"`
	CreateBy      *idtype.JSInt   `json:"createBy"`
	CreateByName  *string         `json:"createByName"`
	CreateTime    *timex.JSONTime `json:"createTime"`
}

// NewOut 由实体构造出参；CreateByName 留空由 service 回填。
//
// notice_content 是 BLOB：Python 端 decode("utf-8", errors="replace")，
// 种子与业务写入均为合法 UTF-8，Go 直接 string(b) 等价。
func NewOut(r *SysNotice) Out {
	title := r.NoticeTitle
	ntype := r.NoticeType
	out := Out{
		ID:          jsIntPtr(r.ID),
		NoticeTitle: &title,
		NoticeType:  &ntype,
		Status:      r.Status,
		Remark:      r.Remark,
		CreateBy:    jsIntPtrOrNilPtr(r.CreateBy),
	}
	if r.NoticeContent != nil {
		s := string(r.NoticeContent)
		out.NoticeContent = &s
	}
	if r.CreateTime != nil {
		jt := timex.FromTime(*r.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

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

// Pydantic v2 类型错误原文（属契约）。
const (
	msgValidInt  = "Input should be a valid integer, unable to parse string as an integer"
	msgValidIntT = "Input should be a valid integer"
	msgValidStr  = "Input should be a valid string"
)

// htmlMarkRe 对应 xss_util.RE_HTML_MARK = (<[^<]*?>)|(<[<]*?[^<]*?>)，忽略大小写。
var htmlMarkRe = regexp.MustCompile(`(?i)(<[^<]*?>)|(<[<]*?[^<]*?>)`)

// valueErr 校验器错误对外文案（"Value error, " 前缀已被全局剥掉）。
func valueErr(msg string) string { return msg }

// validateTitle 对应 check_notice_title（缺省也跑）。
func validateTitle(v *string) string {
	switch {
	case v == nil || strings.TrimSpace(*v) == "":
		return valueErr("公告标题不能为空")
	case utf8.RuneCountInString(*v) > 50:
		return valueErr("公告标题不能超过50个字符")
	case htmlMarkRe.MatchString(*v):
		return valueErr("公告标题不能包含脚本字符")
	}
	return ""
}

// DecodeCreate 解析并校验新增入参。声明序：notice_title → notice_type → notice_content → status → remark。
func DecodeCreate(body []byte) (CreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return CreateIn{}, err
	}
	var in CreateIn
	var msgs []string

	// notice_title：validate_default → 缺省也校验
	if raw, ok := fields["noticeTitle"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.NoticeTitle = v
			if m := validateTitle(v); m != "" {
				msgs = append(msgs, m)
			}
		}
	} else if m := validateTitle(nil); m != "" {
		msgs = append(msgs, m)
	}
	if raw, ok := fields["noticeType"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.NoticeType = v
		}
	}
	if raw, ok := fields["noticeContent"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.NoticeContent = v
		}
	}
	if raw, ok := fields["status"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.Status = v
		}
	}
	if raw, ok := fields["remark"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.Remark = v
		}
	}

	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// DecodeUpdate 解析修改入参。id 追加在父类字段之后，且缺省/null 报「公告ID不能为空」。
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
		msgs = append(msgs, valueErr("公告ID不能为空"))
	} else if string(raw) == "null" {
		msgs = append(msgs, valueErr("公告ID不能为空"))
	} else if v, terr := decodeInt(raw); terr != "" {
		msgs = append(msgs, terr)
	} else {
		in.ID = v
		if v == nil {
			msgs = append(msgs, valueErr("公告ID不能为空"))
		}
	}
	if len(msgs) > 0 {
		return in, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return in, nil
}

// ---- 内部工具（与 dict/config dto 同构）----

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

func decodeInt(raw json.RawMessage) (*int64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, msgValidStr
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, msgValidInt
		}
		return &n, ""
	}
	if text == "true" || text == "false" {
		return nil, msgValidIntT
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, msgValidInt
	}
	if f != float64(int64(f)) {
		return nil, "Input should be a valid integer, got a number with a fractional part"
	}
	n := int64(f)
	return &n, ""
}

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

func jsIntPtr(v int64) *idtype.JSInt {
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
