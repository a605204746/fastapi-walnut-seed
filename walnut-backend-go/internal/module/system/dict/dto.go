// 本文件定义字典域的入参/出参 DTO，对应 Python 端 module_system/dict/schema.py。
//
// ⚠️ 与 post/dept 的关键差异：**本域校验器没有 validate_default**——
// 字段「缺省」时校验器**不跑**（值留 None），只有「显式提供」的值才校验
// （含显式 null）。故解码必须先区分键存在与否，缺省直接跳过校验。
// 缺省字段落库时的取值由 service 层按 Python 两条不同语义处理：
//   - 类型新增：req 字段原样传入构造器 → None 插 **NULL**（不吃列默认值）
//   - 数据新增：model_dump 后过滤 None → 缺省走**列默认值**（0/""/"N"）

package dict

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

// TypeOut 字典类型出参，对应 DictTypeOutSchema（键序 = 声明序）。
type TypeOut struct {
	ID         *idtype.JSInt   `json:"id"`
	DictName   *string         `json:"dictName"`
	DictType   *string         `json:"dictType"`
	Remark     *string         `json:"remark"`
	CreateTime *timex.JSONTime `json:"createTime"`
}

// NewTypeOut 由实体构造出参。
func NewTypeOut(r *SysDictType) TypeOut {
	out := TypeOut{ID: jsIntPtr(r.ID), DictName: r.DictName, DictType: r.DictType, Remark: r.Remark}
	if r.CreateTime != nil {
		jt := timex.FromTime(*r.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// DataOut 字典数据出参，对应 DictDataOutSchema（键序 = 声明序）。
type DataOut struct {
	ID         *idtype.JSInt   `json:"id"`
	DictSort   *idtype.JSInt   `json:"dictSort"`
	DictLabel  *string         `json:"dictLabel"`
	DictValue  *string         `json:"dictValue"`
	DictType   *string         `json:"dictType"`
	CssClass   *string         `json:"cssClass"`
	ListClass  *string         `json:"listClass"`
	IsDefault  *string         `json:"isDefault"`
	Remark     *string         `json:"remark"`
	CreateTime *timex.JSONTime `json:"createTime"`
}

// NewDataOut 由实体构造出参。
func NewDataOut(r *SysDictData) DataOut {
	out := DataOut{
		ID:        jsIntPtr(r.ID),
		DictSort:  intPtrToJSInt(r.DictSort),
		DictLabel: r.DictLabel,
		DictValue: r.DictValue,
		DictType:  r.DictType,
		CssClass:  r.CssClass,
		ListClass: r.ListClass,
		IsDefault: r.IsDefault,
		Remark:    r.Remark,
	}
	if r.CreateTime != nil {
		jt := timex.FromTime(*r.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

// ---- 查询入参 ----

// TypeQuery 字典类型列表/导出查询参数，对应 DictTypeQueryParam。
type TypeQuery struct {
	Page      crudx.Page
	DictName  string
	DictType  string
	BeginTime string
	EndTime   string
}

// BindTypeQuery 从查询串解析（分页用 BindPageQuery：缺省 10 条/上限 1000）。
func BindTypeQuery(q crudx.Querier) (TypeQuery, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return TypeQuery{}, err
	}
	return TypeQuery{
		Page:      page,
		DictName:  q.Query("dictName"),
		DictType:  q.Query("dictType"),
		BeginTime: q.Query("beginTime"),
		EndTime:   q.Query("endTime"),
	}, nil
}

// DataQuery 字典数据列表/导出查询参数，对应 DictDataQueryParam。
type DataQuery struct {
	Page      crudx.Page
	DictSort  *int
	DictLabel string
	DictType  string
}

// BindDataQuery 从查询串解析。
//
// ⚠️ dictSort 是 `int | None` 查询参数：显式出现即由 Pydantic 校验，
// 非法（含空串）→ 400 原文；缺省才是 nil。与 post/dept 的静默 queryInt 不同，
// 这里按契约返回 400（diff 脚本覆盖该分支）。
func BindDataQuery(q crudx.Querier) (DataQuery, error) {
	page, err := crudx.BindPageQuery(q)
	if err != nil {
		return DataQuery{}, err
	}
	out := DataQuery{Page: page, DictLabel: q.Query("dictLabel"), DictType: q.Query("dictType")}
	if raw, ok := q.GetQuery("dictSort"); ok {
		n, perr := strconv.Atoi(strings.TrimSpace(raw))
		if perr != nil {
			return DataQuery{}, errs.BadRequest(msgValidInt)
		}
		out.DictSort = &n
	}
	return out, nil
}

// ---- 写入入参 ----

// TypeCreateIn 新增字典类型入参，对应 DictTypeCreateSchema。
type TypeCreateIn struct {
	DictName *string
	DictType *string
	Remark   *string
}

// TypeUpdateIn 修改字典类型入参（id 必填）。
type TypeUpdateIn struct {
	TypeCreateIn
	ID *int64
}

// DataCreateIn 新增字典数据入参，对应 DictDataCreateSchema。
type DataCreateIn struct {
	DictSort  *int
	DictLabel *string
	DictValue *string
	DictType  *string
	CssClass  *string
	ListClass *string
	IsDefault *string
	Remark    *string
}

// DataUpdateIn 修改字典数据入参（id 必填）。
type DataUpdateIn struct {
	DataCreateIn
	ID *int64
}

// Pydantic v2 类型错误原文（属契约）。
const (
	msgFieldRequired = "Field required"
	msgValidInt      = "Input should be a valid integer, unable to parse string as an integer"
	msgValidIntNum   = "Input should be a valid integer, got a number with a fractional part"
	msgValidIntT     = "Input should be a valid integer"
	msgValidStr      = "Input should be a valid string"
)

// dictTypeRe 对应 RegexConstants.DICTIONARY_TYPE = ^[a-z][a-z0-9_]*$。
//
// ⚠️ Python re.match 的 $ 也匹配末尾换行之前，补 NL 分支（同 crudx orderBy 处理）。
var (
	dictTypeRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	dictTypeReNL = regexp.MustCompile(`^[a-z][a-z0-9_]*\n$`)
)

// valueErr 校验器错误对外文案（"Value error, " 前缀已被全局异常处理剥掉）。
func valueErr(msg string) string { return msg }

// DecodeTypeCreate 解析并校验新增字典类型入参。
//
// 字段声明序：dict_name → dict_type → remark。
// ⚠️ 校验器**只对显式提供的键**生效（无 validate_default）。
func DecodeTypeCreate(body []byte) (TypeCreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return TypeCreateIn{}, err
	}
	var in TypeCreateIn
	var msgs []string

	if raw, ok := fields["dictName"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.DictName = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("字典名称不能为空"))
			case utf8.RuneCountInString(*v) > 100:
				msgs = append(msgs, valueErr("字典类型名称长度不能超过100个字符"))
			}
		}
	}
	if raw, ok := fields["dictType"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.DictType = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("字典类型不能为空"))
			case utf8.RuneCountInString(*v) > 100:
				msgs = append(msgs, valueErr("字典类型类型长度不能超过100个字符"))
			case !dictTypeRe.MatchString(*v) && !dictTypeReNL.MatchString(*v):
				msgs = append(msgs, valueErr("字典类型必须以字母开头，且只能为（小写字母，数字，下滑线）"))
			}
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

// DecodeTypeUpdate 解析并校验修改字典类型入参（id 追加在父类字段之后）。
func DecodeTypeUpdate(body []byte) (TypeUpdateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return TypeUpdateIn{}, err
	}
	base, baseErr := DecodeTypeCreate(body)
	in := TypeUpdateIn{TypeCreateIn: base}

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

// DecodeDataCreate 解析并校验新增字典数据入参。
//
// 字段声明序：dict_sort → dict_label → dict_value → dict_type →
// css_class → list_class → is_default → remark。
func DecodeDataCreate(body []byte) (DataCreateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return DataCreateIn{}, err
	}
	var in DataCreateIn
	var msgs []string

	if raw, ok := fields["dictSort"]; ok {
		v, terr := decodeInt(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.DictSort = intPtr(v)
		}
	}
	if raw, ok := fields["dictLabel"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.DictLabel = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("字典标签不能为空"))
			case utf8.RuneCountInString(*v) > 100:
				msgs = append(msgs, valueErr("字典标签长度不能超过100个字符"))
			}
		}
	}
	if raw, ok := fields["dictValue"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.DictValue = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("字典键值不能为空"))
			case utf8.RuneCountInString(*v) > 100:
				msgs = append(msgs, valueErr("字典键值长度不能超过100个字符"))
			}
		}
	}
	if raw, ok := fields["dictType"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.DictType = v
			switch {
			case v == nil || strings.TrimSpace(*v) == "":
				msgs = append(msgs, valueErr("字典类型不能为空"))
			case utf8.RuneCountInString(*v) > 100:
				msgs = append(msgs, valueErr("字典类型长度不能超过100个字符"))
			}
		}
	}
	if raw, ok := fields["cssClass"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.CssClass = v
			if v != nil && utf8.RuneCountInString(*v) > 100 {
				msgs = append(msgs, valueErr("样式属性长度不能超过100个字符"))
			}
		}
	}
	if raw, ok := fields["listClass"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.ListClass = v
		}
	}
	if raw, ok := fields["isDefault"]; ok {
		v, terr := decodeStr(raw)
		if terr != "" {
			msgs = append(msgs, terr)
		} else {
			in.IsDefault = v
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

// DecodeDataUpdate 解析并校验修改字典数据入参（id 追加在末尾）。
func DecodeDataUpdate(body []byte) (DataUpdateIn, error) {
	fields, err := rawFields(body)
	if err != nil {
		return DataUpdateIn{}, err
	}
	base, baseErr := DecodeDataCreate(body)
	in := DataUpdateIn{DataCreateIn: base}

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

// ---- 内部工具（与 post/dept dto 同构）----

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

func strPtr(v string) *string { return &v }
