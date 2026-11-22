// 本文件定义字典域的入参/查询 DTO，对应 Python 端 module_system/dict/schema.py。
//
// 写入入参走阶段 4 惯用绑定（validate tag + bindx.BindJSON + i18n 文案）；
// 查询 cluster（BindTypeQuery/BindDataQuery）仍保留 Pydantic 兼容原文。
//
// ⚠️ 旧实现校验器没有 validate_default——**缺省跳过校验**（空体合法），只有
// 显式提供的值才校验（含显式 null → 「X不能为空」）。接受语义冻结于旧 Go 行为：
// 缺省跳过字段用 *string + omitnil 前缀（nil 指针跳过全部 tag），显式 null 由
// Decode* 后置 presence 检查补「不能为空」文案（逐字 HEAD 原文）。update 的 id
// 旧实现缺省即报 Field required，故 required 正确保留。缺省字段落库时的取值仍由
// service 层按 Python 两条语义处理：
//   - 类型新增：req 字段原样传入构造器 → None 插 **NULL**（不吃列默认值）
//   - 数据新增：model_dump 后过滤 None → 缺省走**列默认值**（0/""/"N"）

package dict

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

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
//
// beginTime/endTime 是 DateStr|None：出现（含空串）即走 Pydantic 解析，
// 非法 → 400 date 解析原文（bindx.ParseDateQuery）。多个错误按声明序拼接：
// pageNum/pageSize（crudx）→ beginTime → endTime（已用 Python 实测钉住）。
func BindTypeQuery(q crudx.Querier) (TypeQuery, error) {
	page, pageErr := crudx.BindPageQuery(q)
	out := TypeQuery{
		Page:     page,
		DictName: q.Query("dictName"),
		DictType: q.Query("dictType"),
	}
	var msgs []string
	if pageErr != nil {
		se := errs.As(pageErr)
		if se == nil {
			return TypeQuery{}, pageErr
		}
		msgs = append(msgs, se.Msg)
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
	dateField("beginTime", &out.BeginTime)
	dateField("endTime", &out.EndTime)
	if len(msgs) > 0 {
		return TypeQuery{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return out, nil
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
// ⚠️ dictSort 是 `int | None` 查询参数：显式出现即由 Pydantic lax int 校验
// （接受 "3.0"→3、"1_0"→10），非法（含空串）→ 400 int_parsing 原文；缺省才是 nil。
// 分页错误与 dictSort 错误按声明序拼接（pageNum/pageSize 在前）。
func BindDataQuery(q crudx.Querier) (DataQuery, error) {
	page, pageErr := crudx.BindPageQuery(q)
	out := DataQuery{Page: page, DictLabel: q.Query("dictLabel"), DictType: q.Query("dictType")}
	var msgs []string
	if pageErr != nil {
		se := errs.As(pageErr)
		if se == nil {
			return DataQuery{}, pageErr
		}
		msgs = append(msgs, se.Msg)
	}
	if raw, ok := q.GetQuery("dictSort"); ok {
		n, valid := bindx.PyQueryInt(raw)
		if !valid {
			msgs = append(msgs, msgValidInt)
		} else {
			v := int(n)
			out.DictSort = &v
		}
	}
	if len(msgs) > 0 {
		return DataQuery{}, errs.BadRequest(strings.Join(msgs, ", "))
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

// Pydantic v2 类型错误原文（BindDataQuery 与 handler 的 ID 串解析仍在用）。
const msgValidInt = bindx.MsgValidInt

// dictTypeRe 对应 RegexConstants.DICTIONARY_TYPE = ^[a-z][a-z0-9_]*$。
//
// ⚠️ Python re.match 的 $ 也匹配末尾换行之前，补 NL 分支（同 crudx orderBy 处理）。
var (
	dictTypeRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	dictTypeReNL = regexp.MustCompile(`^[a-z][a-z0-9_]*\n$`)
)

func init() {
	// pydicttype：逐字复刻 RegexConstants.DICTIONARY_TYPE（含末尾换行分支）。
	// 仅字典**类型**组校验正则；数据组的 dictType 只校验非空+长度（与旧实现一致）。
	bindx.RegisterTag("pydicttype", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return dictTypeRe.MatchString(s) || dictTypeReNL.MatchString(s)
	})
}

// ---- 阶段 4 惯用绑定：validate tag + bindx.BindJSON + i18n 文案 ----
//
// 相对手写 Decode* 的文案演进（接受语义冻结于旧 Go 行为，不收紧）：
//   - lax 强转退役（"3.0"/"1_0" 不再接受），严格 JSON 类型
//   - 类型错误文案走 i18n（不再是 Pydantic 英文原文）
//   - JSON 类型错误短路（encoding/json 限制），校验级错误仍全量按声明序收集
//
// 接受语义（与 HEAD 逐字镜像）：缺省跳过字段 *string + omitnil 前缀（nil 跳过
// 全部 tag）；显式 null 由 Decode* 后置 presence 检查补 HEAD 原文「不能为空」；
// update 的 id 缺省即必填（required 保留）。null 后置消息追加在 tag 错误之后，
// 多字段同时 null+校验错的拼接序与旧实现可能不同（排序边缘让步，见测试注释）。

// TypeCreateReq 新增字典类型请求体，对应 DictTypeCreateSchema。
// dictName/dictType 缺省跳过（omitnil），显式 null 由 DecodeTypeCreate 后置报错。
type TypeCreateReq struct {
	DictName *string `json:"dictName" validate:"omitnil,notblank,max=100" label:"字典名称"`
	DictType *string `json:"dictType" validate:"omitnil,notblank,max=100,pydicttype" label:"字典类型"`
	Remark   *string `json:"remark" label:"备注"`
}

// TypeUpdateReq 修改字典类型请求体（id 必填，追加在父类字段之后）。
type TypeUpdateReq struct {
	TypeCreateReq
	ID *int64 `json:"id" validate:"required" label:"字典主键"`
}

// DataCreateReq 新增字典数据请求体，对应 DictDataCreateSchema。
// dictLabel/dictValue/dictType 缺省跳过（omitnil），显式 null 由 DecodeDataCreate 后置报错。
type DataCreateReq struct {
	DictSort  *int    `json:"dictSort" label:"字典排序"`
	DictLabel *string `json:"dictLabel" validate:"omitnil,notblank,max=100" label:"字典标签"`
	DictValue *string `json:"dictValue" validate:"omitnil,notblank,max=100" label:"字典键值"`
	DictType  *string `json:"dictType" validate:"omitnil,notblank,max=100" label:"字典类型"`
	CssClass  *string `json:"cssClass" validate:"omitnil,max=100" label:"样式属性"`
	ListClass *string `json:"listClass" label:"表格回显样式"`
	IsDefault *string `json:"isDefault" label:"是否默认"`
	Remark    *string `json:"remark" label:"备注"`
}

// DataUpdateReq 修改字典数据请求体（id 必填，追加在父类字段之后）。
type DataUpdateReq struct {
	DataCreateReq
	ID *int64 `json:"id" validate:"required" label:"字典编码"`
}

func (r TypeCreateReq) toIn() TypeCreateIn {
	return TypeCreateIn{DictName: r.DictName, DictType: r.DictType, Remark: r.Remark}
}

func (r DataCreateReq) toIn() DataCreateIn {
	return DataCreateIn{
		DictSort:  r.DictSort,
		DictLabel: r.DictLabel,
		DictValue: r.DictValue,
		DictType:  r.DictType,
		CssClass:  r.CssClass,
		ListClass: r.ListClass,
		IsDefault: r.IsDefault,
		Remark:    r.Remark,
	}
}

// nullMsg 后置 presence 检查：键存在且原始值为显式 null（指针解为 nil）时，
// 追加 HEAD 原文「不能为空」。*string 无法区分缺省与 null（都 nil），故用
// bindx.Fields 的键存在性 + IsNull 精确定位 null；缺省（键不存在）跳过。
// ⚠️ IsNull 守卫确保 JSON 类型错误字段（present 非 null 但解为 nil）不误报。
func nullMsg(msgs []string, fields map[string]json.RawMessage, key string, isNil bool, msg string) []string {
	if raw, ok := fields[key]; ok && bindx.IsNull(raw) && isNil {
		return append(msgs, msg)
	}
	return msgs
}

// bindMsgs 提取 BindJSON 错误信封文案（nil 错误返回空切片）。
func bindMsgs(err error) ([]string, error) {
	if err == nil {
		return nil, nil
	}
	se := errs.As(err)
	if se == nil {
		return nil, err
	}
	return []string{se.Msg}, nil
}

// DecodeTypeCreate 解析并校验新增字典类型入参（service 消费 TypeCreateIn，零改动）。
func DecodeTypeCreate(ctx context.Context, body []byte) (TypeCreateIn, error) {
	var req TypeCreateReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return TypeCreateIn{}, err
	}
	fields, _ := bindx.Fields(body)
	// 显式 null 后置（声明序：dictName → dictType），文案逐字 HEAD 原文。
	msgs = nullMsg(msgs, fields, "dictName", req.DictName == nil, "字典名称不能为空")
	msgs = nullMsg(msgs, fields, "dictType", req.DictType == nil, "字典类型不能为空")
	if len(msgs) > 0 {
		return TypeCreateIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return req.toIn(), nil
}

// DecodeTypeUpdate 解析修改字典类型入参（id 必填，缺省/null 均由 required tag 报错）。
func DecodeTypeUpdate(ctx context.Context, body []byte) (TypeUpdateIn, error) {
	var req TypeUpdateReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return TypeUpdateIn{}, err
	}
	fields, _ := bindx.Fields(body)
	msgs = nullMsg(msgs, fields, "dictName", req.DictName == nil, "字典名称不能为空")
	msgs = nullMsg(msgs, fields, "dictType", req.DictType == nil, "字典类型不能为空")
	if len(msgs) > 0 {
		return TypeUpdateIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return TypeUpdateIn{TypeCreateIn: req.TypeCreateReq.toIn(), ID: req.ID}, nil
}

// DecodeDataCreate 解析并校验新增字典数据入参。
func DecodeDataCreate(ctx context.Context, body []byte) (DataCreateIn, error) {
	var req DataCreateReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return DataCreateIn{}, err
	}
	fields, _ := bindx.Fields(body)
	// 显式 null 后置（声明序：dictLabel → dictValue → dictType），文案逐字 HEAD 原文。
	msgs = nullMsg(msgs, fields, "dictLabel", req.DictLabel == nil, "字典标签不能为空")
	msgs = nullMsg(msgs, fields, "dictValue", req.DictValue == nil, "字典键值不能为空")
	msgs = nullMsg(msgs, fields, "dictType", req.DictType == nil, "字典类型不能为空")
	if len(msgs) > 0 {
		return DataCreateIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return req.toIn(), nil
}

// DecodeDataUpdate 解析修改字典数据入参（id 必填，缺省/null 均由 required tag 报错）。
func DecodeDataUpdate(ctx context.Context, body []byte) (DataUpdateIn, error) {
	var req DataUpdateReq
	msgs, err := bindMsgs(bindx.BindJSON(ctx, body, &req))
	if err != nil {
		return DataUpdateIn{}, err
	}
	fields, _ := bindx.Fields(body)
	msgs = nullMsg(msgs, fields, "dictLabel", req.DictLabel == nil, "字典标签不能为空")
	msgs = nullMsg(msgs, fields, "dictValue", req.DictValue == nil, "字典键值不能为空")
	msgs = nullMsg(msgs, fields, "dictType", req.DictType == nil, "字典类型不能为空")
	if len(msgs) > 0 {
		return DataUpdateIn{}, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return DataUpdateIn{DataCreateIn: req.DataCreateReq.toIn(), ID: req.ID}, nil
}
