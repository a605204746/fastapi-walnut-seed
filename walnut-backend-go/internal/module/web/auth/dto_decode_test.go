package auth

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

// 特征测试（阶段 4 版）：结构语义（必填/长度/字段存在）冻结不变；
// 文案期望更新为 i18n 中文——旧英文 "Field required"/Pydantic 原文退役。
//
// ⚠️ 空 body 语义刻意演进：旧单条 body 级 "Field required" →
// 新按字段 i18n 必填拼接（clientId → grantType → username → password 声明序）。
// clientId/grantType 补 notblank：纯空白拒绝（旧 min=1 放行，属刻意收紧）。

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

// loginValid 是满足全部必填校验的最小合法字段集（不含外层花括号）。
const loginValid = `"clientId":"cid","grantType":"password","username":"admin","password":"123456"`

// loginRequired 空 body 按字段必填拼接（声明序）。
const loginRequired = "客户端id不能为空, 授权类型不能为空, 用户名不能为空, 密码不能为空"

func TestDecodeLoginBodyCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体按字段必填拼接", "", loginRequired},
		{"空白体视为空对象", "   ", loginRequired},
		{"null体视为空对象", "null", loginRequired},
		{"空对象按字段必填拼接", `{}`, loginRequired},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", "{" + loginValid + "}", ""},
		{"缺username单条", `{"clientId":"cid","grantType":"password","password":"123456"}`, "用户名不能为空"},
		{"clientId空串", `{"clientId":"","grantType":"password","username":"admin","password":"123456"}`,
			"客户端id不能为空"},
		{"clientId纯空白拒绝", `{"clientId":" ","grantType":"password","username":"admin","password":"123456"}`,
			"客户端id不能为空"},
		{"grantType纯空白拒绝", `{"clientId":"cid","grantType":" ","username":"admin","password":"123456"}`,
			"授权类型不能为空"},
		{"username过短", `{"clientId":"cid","grantType":"password","username":"a","password":"123456"}`,
			"用户名长度不能少于2个字符"},
		{"username过长", `{"clientId":"cid","grantType":"password","username":"` + strings.Repeat("a", 31) + `","password":"123456"}`,
			"用户名长度不能超过30个字符"},
		{"password过短", `{"clientId":"cid","grantType":"password","username":"admin","password":"abcd"}`,
			"密码长度不能少于5个字符"},
		{"password过长", `{"clientId":"cid","grantType":"password","username":"admin","password":"` + strings.Repeat("a", 31) + `"}`,
			"密码长度不能超过30个字符"},
		{"username类型错短路", `{"clientId":"cid","grantType":"password","username":123,"password":"123456"}`,
			"用户名必须是字符串"},
		{"username为null按空串必填", `{"clientId":"cid","grantType":"password","username":null,"password":"123456"}`,
			"用户名不能为空"},
		{"可选字段类型错", "{" + loginValid + `,"tenantId":123}`, "tenantId必须是字符串"},
		{"可选字段null跳过", "{" + loginValid + `,"tenantId":null,"code":null,"uuid":null}`, ""},
		{"login忽略userType类型错", "{" + loginValid + `,"userType":123}`, ""},
		{"校验级多错误按声明序", `{"clientId":"","grantType":" ","username":"a","password":"x"}`,
			"客户端id不能为空, 授权类型不能为空, 用户名长度不能少于2个字符, 密码长度不能少于5个字符"},
		{"类型错误短路", `{"username":123,"password":"x"}`, "用户名必须是字符串"},
		{"长度按rune计", `{"clientId":"cid","grantType":"password","username":"汉字","password":"密码密码密"}`, ""},
		{"长度边界值合法", `{"clientId":"cid","grantType":"password","username":"` + strings.Repeat("a", 30) + `","password":"` + strings.Repeat("b", 30) + `"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeLoginBody(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeLoginBodyValidFields(t *testing.T) {
	req, err := decodeLoginBody(context.Background(), []byte("{"+loginValid+`,"tenantId":"t","code":"c","uuid":"u"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if req.ClientID != "cid" || req.GrantType != "password" || req.Username != "admin" || req.Password != "123456" {
		t.Errorf("字段解析错误: %+v", req)
	}
	if req.TenantID == nil || *req.TenantID != "t" || req.Code == nil || *req.Code != "c" || req.UUID == nil || *req.UUID != "u" {
		t.Errorf("可选字段解析错误: %+v", req)
	}
}

func TestDecodeRegisterBodyCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体按字段必填拼接", "", loginRequired},
		{"空对象按字段必填拼接", `{}`, loginRequired},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", "{" + loginValid + "}", ""},
		{"userType类型错", "{" + loginValid + `,"userType":123}`, "userType必须是字符串"},
		{"userType可选null", "{" + loginValid + `,"userType":null}`, ""},
		{"校验级多错误按声明序", `{"clientId":"","username":"a"}`,
			"客户端id不能为空, 授权类型不能为空, 用户名长度不能少于2个字符, 密码不能为空"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeRegisterBody(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeRegisterBodyValidFields(t *testing.T) {
	req, err := decodeRegisterBody(context.Background(), []byte("{"+loginValid+`,"userType":"sys_user"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if req.ClientID != "cid" || req.Username != "admin" || req.Password != "123456" {
		t.Errorf("字段解析错误: %+v", req)
	}
	if req.UserType == nil || *req.UserType != "sys_user" {
		t.Errorf("userType 解析错误: %+v", req)
	}
}

func TestDecodeLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	_, err := decodeLoginBody(ctx, []byte(`{}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}
