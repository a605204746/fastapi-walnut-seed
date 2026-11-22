package role

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

// 特征测试：接受语义冻结于旧 Go 行为（git HEAD）——roleName/roleKey/roleSort
// 缺省跳过（空体合法）、显式 null 报「X不能为空」（HEAD 原文）、空串报错、在场跑长度。
// Update/DataScope/Status/Cancel 的 id 缺省跳过（无 else，由 controller 报 500），
// 显式 null 报「角色ID/用户ID不能为空」。
// 文案演进：类型错误走 i18n 中文（不复刻 Pydantic 英文），lax 强转退役。
//
// role 旧整数字符串回退是**纯数字循环**（bindx.IntDigits）：拒 "+5"/"1_0"，
// 含 "-"→0 的历史怪癖——阶段 4 起统一为严格 JSON 类型，怪癖退役。
//
// ⚠️ 排序边缘让步（唯一允许）：显式 null 的后置消息追加在 tag/类型错误之后，
// 多字段同时 null+校验错时拼接序可能与旧实现不同（见对应用例注释）；
// 单字段用例与接受语义（缺省/null/空串/在场）逐字镜像 HEAD。

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

// roleValid 是满足全部必填校验的最小合法字段集（不含外层花括号）。
const roleValid = `"roleName":"管理员","roleKey":"common","roleSort":1`

func TestDecodeCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体合法_缺省跳过", "", ""},
		{"空白体视为空对象_缺省跳过", "   ", ""},
		{"null体视为空对象_缺省跳过", `null`, ""},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", "{" + roleValid + "}", ""},
		{"roleName缺省跳过", `{"roleKey":"common","roleSort":1}`, ""},
		{"roleKey缺省跳过", `{"roleName":"管理员","roleSort":1}`, ""},
		{"roleSort缺省跳过", `{"roleName":"管理员","roleKey":"common"}`, ""},
		{"roleName为null报空", `{"roleName":null,"roleKey":"common","roleSort":1}`, "角色名称不能为空"},
		{"roleName空串", `{"roleName":"","roleKey":"common","roleSort":1}`, "角色名称不能为空"},
		{"roleName纯空白", `{"roleName":"   ","roleKey":"common","roleSort":1}`, "角色名称不能为空"},
		{"roleName类型错", `{"roleName":123,"roleKey":"common","roleSort":1}`, "角色名称必须是字符串"},
		{"roleName超长", `{"roleName":"` + strings.Repeat("字", 31) + `","roleKey":"common","roleSort":1}`, "角色名称长度不能超过30个字符"},
		{"roleKey为null报空", `{"roleName":"管理员","roleKey":null,"roleSort":1}`, "角色权限字符串不能为空"},
		{"roleKey超长", `{"roleName":"管理员","roleKey":"` + strings.Repeat("k", 101) + `","roleSort":1}`, "角色权限字符串长度不能超过100个字符"},
		{"roleSort为null报空", `{"roleName":"管理员","roleKey":"common","roleSort":null}`, "显示顺序不能为空"},
		{"roleSort零值合法", `{"roleName":"n","roleKey":"k","roleSort":0}`, ""},
		{"roleSort浮点拒绝", `{"roleName":"管理员","roleKey":"common","roleSort":1.5}`, "显示顺序必须是整数"},
		{"roleSort整值浮点lax接受", `{"roleName":"管理员","roleKey":"common","roleSort":3.0}`, ""},
		{"roleSort字符串lax接受", `{"roleName":"管理员","roleKey":"common","roleSort":"3"}`, ""},
		{"roleSort布尔lax接受", `{"roleName":"管理员","roleKey":"common","roleSort":true}`, ""},
		{"menuCheckStrictly字符串拒绝", "{" + roleValid + `,"menuCheckStrictly":"true"}`, "菜单树选择项是否关联显示必须是布尔值"},
		{"deptCheckStrictly数字拒绝", "{" + roleValid + `,"deptCheckStrictly":1}`, "部门树选择项是否关联显示必须是布尔值"},
		{"dataScope类型错", "{" + roleValid + `,"dataScope":5}`, "数据范围必须是字符串"},
		{"menuIds元素字符串lax接受", "{" + roleValid + `,"menuIds":["1"]}`, ""},
		{"menuIds非列表", "{" + roleValid + `,"menuIds":"x"}`, "菜单组必须是列表"},
		{"deptIds元素布尔lax接受", "{" + roleValid + `,"deptIds":[true]}`, ""},
		{"可选字段null视为缺省", "{" + roleValid + `,"dataScope":null,"menuIds":null,"deptIds":null,"status":null,"remark":null}`, ""},
		{"校验级多错误按声明序", `{"roleName":" ","roleKey":"","roleSort":null}`, "角色名称不能为空, 角色权限字符串不能为空, 显示顺序不能为空"},
		// 排序边缘让步：roleKey 空串先跑 notblank（tag），roleName null 后置——
		// 拼接序「权限, 名称」与 HEAD 声明序「名称, 权限」不同（唯一允许的让步）。
		{"排序边缘让步_null后置", `{"roleName":null,"roleKey":""}`, "角色权限字符串不能为空, 角色名称不能为空"},
		{"类型错误短路", `{"roleName":123,"roleKey":456}`, "角色名称必须是字符串"},
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
	in, err := DecodeCreate(context.Background(), []byte(`{"roleName":"管理员","roleKey":"common","roleSort":2,"dataScope":"2","menuCheckStrictly":true,"deptCheckStrictly":false,"status":"0","remark":"r","menuIds":[1,2],"deptIds":[100]}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.RoleName == nil || *in.RoleName != "管理员" || in.RoleKey == nil || *in.RoleKey != "common" {
		t.Errorf("字符串字段解析错误: %+v", in)
	}
	if in.RoleSort == nil || *in.RoleSort != 2 || in.DataScope == nil || *in.DataScope != "2" {
		t.Errorf("排序/数据范围解析错误: %+v", in)
	}
	if in.MenuCheckStrictly == nil || !*in.MenuCheckStrictly || in.DeptCheckStrictly == nil || *in.DeptCheckStrictly {
		t.Errorf("布尔字段解析错误: %+v", in)
	}
	if len(in.MenuIDs) != 2 || in.MenuIDs[1] != 2 || len(in.DeptIDs) != 1 || in.DeptIDs[0] != 100 {
		t.Errorf("列表解析错误: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体合法_缺省跳过", "", ""},
		{"空白体缺省跳过", "  ", ""},
		{"id缺省跳过", "{" + roleValid + "}", ""},
		{"id为null报空", `{"id":null,` + roleValid + `}`, "角色ID不能为空"},
		{"id字符串非数字", `{"id":"abc",` + roleValid + `}`, "角色ID必须是整数"},
		{"id空串", `{"id":"",` + roleValid + `}`, "角色ID必须是整数"},
		{"id浮点", `{"id":1.5,` + roleValid + `}`, "角色ID必须是整数"},
		{"id布尔lax接受", `{"id":true,` + roleValid + `}`, ""},
		{"roleName为null_id合法", `{"id":1,"roleName":null,"roleKey":"common","roleSort":1}`, "角色名称不能为空"},
		{"父类错误在前id在后", `{"roleName":null,"id":null}`, "角色名称不能为空, 角色ID不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"合法", `{"id":1,` + roleValid + `}`, ""},
		{"id字符串lax接受", `{"id":"3.0",` + roleValid + `}`, ""},
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

// TestDecodeUpdateLaxIntSemantics 钉住 Pydantic JSON 模式 lax 整型语义（实测 golden）：
// 整数字符串/符号/空白/下划线/全零小数 接受；旧 IntDigits 怪癖（"-"→0、"--5"→-5）退役。
func TestDecodeUpdateLaxIntSemantics(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr string
		wantID  int64
	}{
		{"纯数字字符串接受", `{"id":"5",` + roleValid + `}`, "", 5},
		{"负数字符串接受", `{"id":"-5",` + roleValid + `}`, "", -5},
		{"首尾空白字符串接受", `{"id":" 5 ",` + roleValid + `}`, "", 5},
		{"加号接受", `{"id":"+5",` + roleValid + `}`, "", 5},
		{"下划线接受", `{"id":"1_0",` + roleValid + `}`, "", 10},
		{"小数字符串接受", `{"id":"3.0",` + roleValid + `}`, "", 3},
		{"单减号怪癖退役", `{"id":"-",` + roleValid + `}`, "角色ID必须是整数", 0},
		{"双前导减号怪癖退役", `{"id":"--5",` + roleValid + `}`, "角色ID必须是整数", 0},
		{"JSON整数", `{"id":42,` + roleValid + `}`, "", 42},
		{"JSON负整数", `{"id":-5,` + roleValid + `}`, "", -5},
		{"JSON零值", `{"id":0,` + roleValid + `}`, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, err := DecodeUpdate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.wantErr {
				t.Fatalf("msg = %q, want %q", got, tc.wantErr)
			}
			if tc.wantErr != "" {
				return
			}
			if in.ID == nil || *in.ID != tc.wantID {
				t.Errorf("ID = %v, want %d", in.ID, tc.wantID)
			}
		})
	}
}

func TestDecodeDataScopeCharacterization(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"空体合法_id缺省跳过", "", ""},
		{"空对象id缺省跳过", `{}`, ""},
		{"id缺省跳过", `{"dataScope":"2"}`, ""},
		{"id为null报空", `{"id":null}`, "角色ID不能为空"},
		{"id字符串非数字", `{"id":"abc"}`, "角色ID必须是整数"},
		{"合法", `{"id":1,"dataScope":"2","deptCheckStrictly":true,"deptIds":[100]}`, ""},
		{"dataScope类型错", `{"id":1,"dataScope":5}`, "数据范围必须是字符串"},
		{"deptCheckStrictly类型错", `{"id":1,"deptCheckStrictly":1}`, "部门树选择项是否关联显示必须是布尔值"},
		{"deptIds非列表", `{"id":1,"deptIds":"x"}`, "部门组必须是列表"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeDataScope(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeStatusCharacterization(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"空体合法_id缺省跳过", "", ""},
		{"id缺省跳过", `{"status":"1"}`, ""},
		{"id为null报空", `{"id":null}`, "角色ID不能为空"},
		{"status缺省不校验", `{"id":1}`, ""},
		{"status为null不校验", `{"id":1,"status":null}`, ""},
		{"status类型错", `{"id":1,"status":5}`, "角色状态必须是字符串"},
		{"合法", `{"id":1,"status":"1"}`, ""},
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

func TestDecodeCancelCharacterization(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"空体合法_缺省跳过", "", ""},
		{"roleId缺省跳过", `{"userId":2}`, ""},
		{"userId缺省跳过", `{"roleId":1}`, ""},
		{"双null按声明序", `{"roleId":null,"userId":null}`, "角色ID不能为空, 用户ID不能为空"},
		{"合法", `{"roleId":1,"userId":2}`, ""},
		{"加号与下划线均lax接受", `{"roleId":"+5","userId":"1_0"}`, ""},
		// 排序边缘让步：userId 类型错短路在先，roleId null 后置追加——
		// 拼接序「用户ID类型错, 角色ID空」与 HEAD 声明序「角色ID空, 用户ID类型错」不同。
		{"null与类型错_null后置", `{"roleId":null,"userId":"abc"}`, "用户ID必须是整数, 角色ID不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeCancel(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	// 空体已合法（缺省跳过），改用空串触发 notblank tag 错误验证 en locale。
	_, err := DecodeCreate(ctx, []byte(`{"roleName":""}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}
