package dict

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/bindx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

// 特征测试：接受语义冻结于旧 Go 行为（git HEAD）——缺省跳过（空体合法）、
// 显式 null 报「X不能为空」（HEAD 原文）、空串报错、在场跑长度/正则。
// 文案演进：类型错误走 i18n 中文（不复刻 Pydantic 英文）；整型 lax 强转与 Pydantic JSON 模式对等；
// 长度/正则文案走 i18n（label 机制 / 中央 pydicttype 映射）。
//
// ⚠️ 排序边缘让步（唯一允许）：显式 null 的后置消息追加在 tag 错误之后，
// 多字段同时 null+校验错时拼接序可能与旧实现不同（见对应用例注释）；
// 单字段用例与接受语义（缺省/null/空串/在场）逐字镜像 HEAD。

// TestProbeNilPointerSkipsTags 探针：validator 对 nil 指针字段跳过全部校验 tag
// 的**前提**是 validate 链以 omitnil 打头——nil 指针在 omitnil 处直接 return，
// notblank/max/pydicttype 等后续 tag 不跑；非 nil 指针解引用后照常校验。
// ⚠️ 若不加 omitnil，nil 指针会在首个 tag（notblank）上失败（已实测），
// 故「缺省跳过」字段必须 *string + omitnil 前缀 + 去掉 required。
func TestProbeNilPointerSkipsTags(t *testing.T) {
	type probe struct {
		Name *string `json:"name" validate:"omitnil,notblank,max=100" label:"名称"`
		Type *string `json:"type" validate:"omitnil,notblank,max=100,pydicttype" label:"类型"`
		Sort *int64  `json:"sort" label:"排序"`
	}
	cases := []struct {
		name string
		body string
		want string
	}{
		{"nil指针跳过全部tag", `{}`, ""},
		{"显式null解为nil同样跳过", `{"name":null,"type":null,"sort":null}`, ""},
		{"非nil空串跑notblank", `{"name":""}`, "名称不能为空"},
		{"非nil超长跑max", `{"name":"` + strings.Repeat("字", 101) + `"}`, "名称长度不能超过100个字符"},
		{"非nil正则跑pydicttype", `{"type":"BAD"}`, "字典类型必须以字母开头，且只能为（小写字母，数字，下滑线）"},
		{"非nil合法值通过", `{"name":"n","type":"sys_ok","sort":0}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p probe
			err := bindx.BindJSON(context.Background(), []byte(tc.body), &p)
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func decodeMsg(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	se := errs.As(err)
	if se == nil {
		t.Fatalf("非信封错误: %v", err)
	}
	if se.Code != errs.CodeBadRequest {
		t.Fatalf("code = %d, want 400", se.Code)
	}
	return se.Msg
}

func TestDecodeTypeCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体合法_缺省跳过", "", ""},
		{"空白体视为空对象_缺省跳过", "   ", ""},
		{"null体视为空对象_缺省跳过", "null", ""},
		{"空对象缺省跳过", `{}`, ""},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", `{"dictName":"n","dictType":"sys_ok"}`, ""},
		{"名称缺省跳过", `{"dictType":"sys_ok"}`, ""},
		{"类型缺省跳过", `{"dictName":"n"}`, ""},
		{"名称空串", `{"dictName":"","dictType":"sys_ok"}`, "字典名称不能为空"},
		{"名称null报空", `{"dictName":null,"dictType":"sys_ok"}`, "字典名称不能为空"},
		{"名称类型错", `{"dictName":123,"dictType":"sys_ok"}`, "字典名称必须是字符串"},
		{"名称超长", `{"dictName":"` + strings.Repeat("字", 101) + `","dictType":"sys_ok"}`, "字典名称长度不能超过100个字符"},
		{"类型空串", `{"dictName":"n","dictType":" "}`, "字典类型不能为空"},
		{"类型null报空", `{"dictName":"n","dictType":null}`, "字典类型不能为空"},
		{"类型超长", `{"dictName":"n","dictType":"` + strings.Repeat("a", 101) + `"}`, "字典类型长度不能超过100个字符"},
		{"类型大写拒绝", `{"dictName":"n","dictType":"Abc"}`, "字典类型必须以字母开头，且只能为（小写字母，数字，下滑线）"},
		{"类型数字开头拒绝", `{"dictName":"n","dictType":"9abc"}`, "字典类型必须以字母开头，且只能为（小写字母，数字，下滑线）"},
		{"类型末尾换行接受", "{\"dictName\":\"n\",\"dictType\":\"sys_ok\\n\"}", ""},
		{"备注null不校验", `{"dictName":"n","dictType":"sys_ok","remark":null}`, ""},
		{"校验级多错误按声明序", `{"dictName":"","dictType":"BAD"}`, "字典名称不能为空, 字典类型必须以字母开头，且只能为（小写字母，数字，下滑线）"},
		{"null与校验错_tag在前null在后", `{"dictName":"","dictType":null}`, "字典名称不能为空, 字典类型不能为空"},
		// 排序边缘让步：dictType 空串先跑 notblank（tag），dictName null 后置——
		// 拼接序「类型, 名称」与 HEAD 声明序「名称, 类型」不同（唯一允许的让步）。
		{"排序边缘让步_null后置", `{"dictName":null,"dictType":""}`, "字典类型不能为空, 字典名称不能为空"},
		{"类型错误短路", `{"dictName":123,"dictType":"BAD"}`, "字典名称必须是字符串"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeTypeCreate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeTypeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺id报必填_父字段缺省跳过", `{}`, "字典主键不能为空"},
		{"空体只报id必填", "", "字典主键不能为空"},
		{"id为null报必填", `{"id":null,"dictName":"n","dictType":"sys_ok"}`, "字典主键不能为空"},
		{"id字符串非法", `{"id":"abc","dictName":"n","dictType":"sys_ok"}`, "字典主键必须是整数"},
		{"名称缺省跳过", `{"id":1,"dictType":"sys_ok"}`, ""},
		{"名称null报空", `{"id":1,"dictName":null,"dictType":"sys_ok"}`, "字典名称不能为空"},
		{"合法全字段", `{"id":1,"dictName":"n","dictType":"sys_ok"}`, ""},
		{"id字符串lax接受", `{"id":"1_0","dictName":"n","dictType":"sys_ok"}`, ""},
		{"父类错误在前id在后", `{"dictName":"","id":null}`, "字典名称不能为空, 字典主键不能为空"},
		{"id类型错短路", `{"dictName":"","id":"abc"}`, "字典主键必须是整数"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeTypeUpdate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeTypeUpdateValidFields(t *testing.T) {
	in, err := DecodeTypeUpdate(context.Background(), []byte(`{"id":3,"dictName":"n","dictType":"sys_ok","remark":"r"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 3 || in.DictName == nil || *in.DictName != "n" || in.Remark == nil || *in.Remark != "r" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

// dataValid 是满足字典数据全部必填校验的最小合法字段集（不含外层花括号）。
const dataValid = `"dictLabel":"l","dictValue":"v","dictType":"sys_ok"`

func TestDecodeDataCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体合法_缺省跳过", "", ""},
		{"空对象缺省跳过", `{}`, ""},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"合法最小集", "{" + dataValid + "}", ""},
		{"排序字符串lax接受", `{"dictSort":"3.0",` + dataValid + `}`, ""},
		{"排序下划线lax接受", `{"dictSort":"1_0",` + dataValid + `}`, ""},
		{"排序布尔lax接受", `{"dictSort":true,` + dataValid + `}`, ""},
		{"排序浮点非整值", `{"dictSort":2.5,` + dataValid + `}`, "字典排序必须是整数"},
		{"排序null跳过", `{"dictSort":null,` + dataValid + `}`, ""},
		{"排序缺省跳过", "{" + dataValid + "}", ""},
		{"标签缺省跳过", `{"dictValue":"v","dictType":"sys_ok"}`, ""},
		{"标签空串", `{"dictLabel":"","dictValue":"v","dictType":"sys_ok"}`, "字典标签不能为空"},
		{"标签null报空", `{"dictLabel":null,"dictValue":"v","dictType":"sys_ok"}`, "字典标签不能为空"},
		{"标签超长", `{"dictLabel":"` + strings.Repeat("字", 101) + `","dictValue":"v","dictType":"sys_ok"}`, "字典标签长度不能超过100个字符"},
		{"键值空串", `{"dictLabel":"l","dictValue":" ","dictType":"sys_ok"}`, "字典键值不能为空"},
		{"键值null报空", `{"dictLabel":"l","dictValue":null,"dictType":"sys_ok"}`, "字典键值不能为空"},
		{"键值超长", `{"dictLabel":"l","dictValue":"` + strings.Repeat("v", 101) + `","dictType":"sys_ok"}`, "字典键值长度不能超过100个字符"},
		{"类型空串", `{"dictLabel":"l","dictValue":"v","dictType":""}`, "字典类型不能为空"},
		{"类型null报空", `{"dictLabel":"l","dictValue":"v","dictType":null}`, "字典类型不能为空"},
		{"类型超长", `{"dictLabel":"l","dictValue":"v","dictType":"` + strings.Repeat("a", 101) + `"}`, "字典类型长度不能超过100个字符"},
		{"类型不校验正则", `{"dictLabel":"l","dictValue":"v","dictType":"ABC"}`, ""},
		{"样式超长", "{" + dataValid + `,"cssClass":"` + strings.Repeat("c", 101) + `"}`, "样式属性长度不能超过100个字符"},
		{"样式null不校验", "{" + dataValid + `,"cssClass":null}`, ""},
		{"校验级多错误按声明序", `{"dictLabel":"","dictValue":" ","dictType":""}`, "字典标签不能为空, 字典键值不能为空, 字典类型不能为空"},
		// 排序边缘让步：dictValue 空串先跑 notblank（tag），dictLabel null 后置——
		// 拼接序「键值, 标签」与 HEAD 声明序「标签, 键值」不同（唯一允许的让步）。
		{"排序边缘让步_null后置", `{"dictLabel":null,"dictValue":""}`, "字典键值不能为空, 字典标签不能为空"},
		{"类型错误短路_强转后", `{"dictSort":true,"dictLabel":"","dictValue":123,"dictType":" "}`, "字典键值必须是字符串"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeDataCreate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeDataCreateValidFields(t *testing.T) {
	in, err := DecodeDataCreate(context.Background(), []byte(`{"dictSort":10,"dictLabel":"l","dictValue":"v","dictType":"sys_ok","cssClass":"c","listClass":"lc","isDefault":"Y","remark":"r"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.DictSort == nil || *in.DictSort != 10 || in.DictLabel == nil || *in.DictLabel != "l" ||
		in.IsDefault == nil || *in.IsDefault != "Y" || in.ListClass == nil || *in.ListClass != "lc" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

func TestDecodeDataUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺id报必填_父字段缺省跳过", `{}`, "字典编码不能为空"},
		{"id为null报必填", `{"id":null,` + dataValid + `}`, "字典编码不能为空"},
		{"id布尔lax接受", `{"id":false,` + dataValid + `}`, ""},
		{"合法", `{"id":1,` + dataValid + `}`, ""},
		{"父类错误在前id在后", `{"dictLabel":"","id":null}`, "字典标签不能为空, 字典编码不能为空"},
		{"id类型错短路", `{"dictLabel":"","id":"abc"}`, "字典编码必须是整数"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeDataUpdate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	// 空体已合法（缺省跳过），改用空串触发 notblank tag 错误验证 en locale。
	_, err := DecodeTypeCreate(ctx, []byte(`{"dictName":""}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}
