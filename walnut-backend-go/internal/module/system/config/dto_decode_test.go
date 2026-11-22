package config

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

// 特征测试（阶段 4 版）：结构语义（必填/长度/字段存在）冻结不变；
// 文案期望更新为 i18n 中文——类型错误不再复刻 Pydantic 英文；整型 lax 强转与 Pydantic JSON 模式对等。
// 必填类中文文案与旧实现逐字一致（label 机制复刻）。
//
// ⚠️ 本域校验器无 validate_default：字段缺省时不跑校验（空体合法）；
// 但显式 null 触发校验器（nullToEmpty 归一复刻）——两者必须区分钉住。

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

func TestDecodeCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体合法", "", ""},
		{"空白体视为空对象", "   ", ""},
		{"null体视为空对象", "null", ""},
		{"空对象合法", `{}`, ""},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", `{"configName":"n","configKey":"k","configValue":"v"}`, ""},
		{"名称缺省不校验", `{"configKey":"k","configValue":"v"}`, ""},
		{"名称空串", `{"configName":""}`, "参数名称不能为空"},
		{"名称null跑校验器", `{"configName":null}`, "参数名称不能为空"},
		{"名称类型错短路", `{"configName":123}`, "参数名称必须是字符串"},
		{"名称超长", `{"configName":"` + strings.Repeat("字", 101) + `"}`, "参数名称长度不能超过100个字符"},
		{"键名超长", `{"configKey":"` + strings.Repeat("k", 101) + `"}`, "参数键名长度不能超过100个字符"},
		{"键值超长", `{"configValue":"` + strings.Repeat("v", 501) + `"}`, "参数键值长度不能超过500个字符"},
		{"键值空白串", `{"configValue":" "}`, "参数键值不能为空"},
		{"键值null跑校验器", `{"configValue":null}`, "参数键值不能为空"},
		{"configType为null跳过", `{"configType":null}`, ""},
		{"remark为null跳过", `{"remark":null}`, ""},
		{"校验级多错误按声明序", `{"configName":"","configKey":null,"configValue":" "}`,
			"参数名称不能为空, 参数键名不能为空, 参数键值不能为空"},
		{"类型错误短路", `{"configName":"","configKey":123,"configValue":" "}`, "参数键名必须是字符串"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeCreate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeCreateValidFields(t *testing.T) {
	in, err := DecodeCreate(context.Background(), []byte(`{"configName":"n","configKey":"k","configValue":"v","configType":"Y","remark":"r"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ConfigName == nil || *in.ConfigName != "n" || in.ConfigKey == nil || *in.ConfigKey != "k" ||
		in.ConfigValue == nil || *in.ConfigValue != "v" || in.ConfigType == nil || *in.ConfigType != "Y" ||
		in.Remark == nil || *in.Remark != "r" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

func TestDecodeCreateAbsentFieldsStayNil(t *testing.T) {
	// 缺省字段留 nil（service 按列默认值镜像），不与显式 null 混淆
	in, err := DecodeCreate(context.Background(), []byte(`{"configKey":"k","configValue":"v"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ConfigName != nil || in.ConfigType != nil || in.Remark != nil {
		t.Errorf("缺省字段应留 nil: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺id报i18n必填", `{}`, "id不能为空"},
		{"空体", "", "id不能为空"},
		{"id为null报必填", `{"id":null}`, "id不能为空"},
		{"id布尔lax接受", `{"id":true}`, ""},
		{"id字符串非法", `{"id":"abc"}`, "id必须是整数"},
		{"id浮点非整值", `{"id":1.5}`, "id必须是整数"},
		{"合法仅id", `{"id":1}`, ""},
		{"id字符串lax接受", `{"id":"3.0"}`, ""},
		{"父类错误在前id在后", `{"configName":"","id":null}`, "参数名称不能为空, id不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeUpdate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeUpdateValidFields(t *testing.T) {
	in, err := DecodeUpdate(context.Background(), []byte(`{"id":10,"configKey":"k"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 10 || in.ConfigKey == nil || *in.ConfigKey != "k" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

func TestDecodeUpdateByKeyCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体合法", "", ""},
		{"空对象合法", `{}`, ""},
		{"名称空串不校验", `{"configName":""}`, ""},
		{"名称null不校验", `{"configName":null}`, ""},
		{"id可选lax接受", `{"id":"1_0"}`, ""},
		{"id布尔lax接受", `{"id":true}`, ""},
		{"id为null跳过", `{"id":null}`, ""},
		{"键名空串", `{"configKey":""}`, "参数键名不能为空"},
		{"键名null跑校验器", `{"configKey":null}`, "参数键名不能为空"},
		{"键名超长不校验_无长度上限", `{"configKey":"` + strings.Repeat("k", 101) + `"}`, ""},
		{"键值空串", `{"configValue":""}`, "参数键值不能为空"},
		{"校验级多错误按声明序", `{"configKey":"","configValue":null}`,
			"参数键名不能为空, 参数键值不能为空"},
		{"强转后落校验级拼接", `{"id":true,"configKey":"","configValue":null}`,
			"参数键名不能为空, 参数键值不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeUpdateByKey(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeUpdateByKeyValidFields(t *testing.T) {
	in, err := DecodeUpdateByKey(context.Background(), []byte(`{"id":10,"configName":"n","configKey":"k","configValue":"v","configType":"Y","remark":"r"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 10 || in.ConfigName == nil || *in.ConfigName != "n" ||
		in.ConfigKey == nil || *in.ConfigKey != "k" || in.ConfigValue == nil || *in.ConfigValue != "v" ||
		in.ConfigType == nil || *in.ConfigType != "Y" || in.Remark == nil || *in.Remark != "r" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

func TestDecodeLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	_, err := DecodeUpdate(ctx, []byte(`{}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}
