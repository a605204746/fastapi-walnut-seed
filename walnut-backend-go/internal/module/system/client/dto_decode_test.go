package client

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

// 特征测试（阶段 4 版）：结构语义（必填/长度/code/字段存在）冻结不变；
// 文案期望更新为 i18n 中文——类型错误不再复刻 Pydantic 英文；整型 lax 强转与 Pydantic JSON 模式对等。
// 必填类中文文案与旧实现逐字一致（label 机制复刻）。

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
		{"空体触发必填", "", "客户端key不能为空, 客户端秘钥不能为空, 授权类型不能为空"},
		{"空白体视为空对象", "   ", "客户端key不能为空, 客户端秘钥不能为空, 授权类型不能为空"},
		{"null体视为空对象", "null", "客户端key不能为空, 客户端秘钥不能为空, 授权类型不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", `{"clientKey":"k","clientSecret":"s","grantTypeList":["password"]}`, ""},
		{"key为null跑校验器", `{"clientKey":null,"clientSecret":"s","grantTypeList":["p"]}`, "客户端key不能为空"},
		{"key类型错", `{"clientKey":123,"clientSecret":"s","grantTypeList":["p"]}`, "客户端key必须是字符串"},
		{"key纯空白拒绝", `{"clientKey":" ","clientSecret":"s","grantTypeList":["p"]}`, "客户端key不能为空"},
		{"授权列表null", `{"clientKey":"k","clientSecret":"s","grantTypeList":null}`, "授权类型不能为空"},
		{"授权列表空数组拒绝", `{"clientKey":"k","clientSecret":"s","grantTypeList":[]}`, "授权类型不能为空"},
		{"授权列表非列表", `{"clientKey":"k","clientSecret":"s","grantTypeList":"p"}`, "授权类型必须是列表"},
		{"授权列表元素类型错", `{"clientKey":"k","clientSecret":"s","grantTypeList":[1]}`, "授权类型必须是字符串"},
		{"整数字符串lax接受", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"activeTimeout":"3.0"}`, ""},
		{"整数布尔lax接受", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"activeTimeout":true}`, ""},
		{"整数浮点非整值", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"timeout":3.5}`, "超时时间必须是整数"},
		{"校验级多错误按声明序", `{"clientKey":" ","clientSecret":"s","grantTypeList":null}`, "客户端key不能为空, 授权类型不能为空"},
		{"类型错误短路", `{"clientKey":"","clientSecret":123,"grantTypeList":null}`, "客户端秘钥必须是字符串"},
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
	in, err := DecodeCreate(context.Background(), []byte(`{"clientKey":"k","clientSecret":"s","grantTypeList":["password","sms"],"deviceType":"pc","activeTimeout":3,"timeout":604800,"status":"0"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ClientKey == nil || *in.ClientKey != "k" || in.ClientSecret == nil || *in.ClientSecret != "s" {
		t.Errorf("字段解析错误: %+v", in)
	}
	if len(in.GrantTypeList) != 2 || in.GrantTypeList[1] != "sms" {
		t.Errorf("列表解析错误: %+v", in.GrantTypeList)
	}
	if in.ActiveTimeout == nil || *in.ActiveTimeout != 3 || in.Timeout == nil || *in.Timeout != 604800 {
		t.Errorf("整数解析错误: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体缺id", `{}`, "客户端key不能为空, 客户端秘钥不能为空, 授权类型不能为空, id不能为空"},
		{"id为null", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"id":null}`, "id不能为空"},
		{"id类型错", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"id":"abc"}`, "id必须是整数"},
		{"id布尔lax接受", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"id":true}`, ""},
		{"合法", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"id":1}`, ""},
		{"id字符串lax接受", `{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"id":"3.0"}`, ""},
		{"clientId类型错短路", `{"clientKey":"","clientSecret":"s","grantTypeList":["p"],"id":1,"clientId":5}`, "客户端id必须是字符串"},
		{"空白体", "  ", "客户端key不能为空, 客户端秘钥不能为空, 授权类型不能为空, id不能为空"},
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
	in, err := DecodeUpdate(context.Background(), []byte(`{"clientKey":"k","clientSecret":"s","grantTypeList":["p"],"id":10,"clientId":"cid"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 10 || in.ClientID == nil || *in.ClientID != "cid" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

func TestDecodeStatusCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体两字段必填", "", "客户端id不能为空, 状态不能为空"},
		{"null体", "null", "客户端id不能为空, 状态不能为空"},
		{"合法", `{"clientId":"x","status":"0"}`, ""},
		{"clientId为null", `{"clientId":null,"status":"0"}`, "客户端id不能为空"},
		{"status空白串", `{"clientId":"x","status":" "}`, "状态不能为空"},
		{"status类型错", `{"clientId":"x","status":123}`, "状态必须是字符串"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeStatus(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	_, err := DecodeCreate(ctx, []byte(`{}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}
