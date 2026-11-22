package user

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
// 刻意演进：空白体视为空对象；含类型错误的多错误短路为单条。
// 严格镜像旧语义：password/roleIds 缺省放行（password 缺省由 service 兜底 500，
// roleIds 缺省 service no-op）；仅在场且非 null 时后置校验（空密码/空数组拒绝）；
// phonenumber 仅个人中心校验，新增/修改不跑 mobile。

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

const pwdMsg = "密码必须包含大写字母、小写字母、数字和特殊字符"

func TestDecodeCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体触发必填", "", "用户账号不能为空, 用户昵称不能为空"},
		{"空白体视为空对象", "   ", "用户账号不能为空, 用户昵称不能为空"},
		{"null体视为空对象", "null", "用户账号不能为空, 用户昵称不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", `{"userName":"admin","nickName":"管理员","password":"Abc12345!","roleIds":[1]}`, ""},
		{"账号类型错", `{"userName":123,"nickName":"x","password":"Abc12345!","roleIds":[1]}`, "用户账号必须是字符串"},
		{"含类型错误的多错误短路", `{"userName":"","nickName":123,"email":"bad","password":"weak","roleIds":[1]}`, "用户昵称必须是字符串"},
		{"纯校验级多错误按声明序", `{"userName":"","nickName":" ","email":"bad","password":"weak","roleIds":[]}`,
			"用户账号不能为空, 用户昵称不能为空, 邮箱格式不正确"},
		{"deptId字符串数字lax接受", `{"deptId":"5","userName":"admin","nickName":"x","password":"Abc12345!","roleIds":[1]}`, ""},
		{"deptId下划线lax接受", `{"deptId":"1_0","userName":"admin","nickName":"x","password":"Abc12345!","roleIds":[1]}`, ""},
		{"deptId布尔lax接受", `{"deptId":true,"userName":"admin","nickName":"x","password":"Abc12345!","roleIds":[1]}`, ""},
		{"roleIds空数组", `{"userName":"admin","nickName":"x","password":"Abc12345!","roleIds":[]}`, "用户角色不能为空"},
		{"roleIds为null放行", `{"userName":"admin","nickName":"x","password":"Abc12345!","roleIds":null}`, ""},
		{"roleIds元素字符串lax接受", `{"userName":"admin","nickName":"x","password":"Abc12345!","roleIds":["1"]}`, ""},
		{"roleIds非列表", `{"userName":"admin","nickName":"x","password":"Abc12345!","roleIds":"1"}`, "用户角色必须是列表"},
		{"null字段视为缺省", `{"userName":null,"nickName":"x","password":"Abc12345!","roleIds":[1]}`, "用户账号不能为空"},
		{"账号纯空白拒绝", `{"userName":" ","nickName":"x","password":"Abc12345!","roleIds":[1]}`, "用户账号不能为空"},
		{"账号长度下界", `{"userName":"a","nickName":"x","password":"Abc12345!","roleIds":[1]}`, "用户账号长度不能少于2个字符"},
		{"账号长度上界", `{"userName":"` + strings.Repeat("a", 31) + `","nickName":"x","password":"Abc12345!","roleIds":[1]}`, "用户账号长度不能超过30个字符"},
		{"昵称超长", `{"userName":"admin","nickName":"` + strings.Repeat("字", 31) + `","password":"Abc12345!","roleIds":[1]}`, "用户昵称长度不能超过30个字符"},
		{"邮箱超长", `{"userName":"admin","nickName":"x","email":"` + strings.Repeat("a", 45) + `@b.com","password":"Abc12345!","roleIds":[1]}`, "邮箱长度不能超过50个字符"},
		{"邮箱格式", `{"userName":"admin","nickName":"x","email":"bad","password":"Abc12345!","roleIds":[1]}`, "邮箱格式不正确"},
		{"邮箱空串合法", `{"userName":"admin","nickName":"x","email":"","password":"Abc12345!","roleIds":[1]}`, ""},
		{"手机号不校验", `{"userName":"admin","nickName":"x","phonenumber":"123","password":"Abc12345!","roleIds":[1]}`, ""},
		{"弱密码", `{"userName":"admin","nickName":"x","password":"weak","roleIds":[1]}`, pwdMsg},
		{"密码空串报不能为空", `{"userName":"admin","nickName":"x","password":"","roleIds":[1]}`, "用户密码不能为空"},
		{"密码纯空白报不能为空", `{"userName":"admin","nickName":"x","password":" ","roleIds":[1]}`, "用户密码不能为空"},
		{"密码为null放行", `{"userName":"admin","nickName":"x","password":null,"roleIds":[1]}`, ""},
		{"缺密码放行service兜底", `{"userName":"admin","nickName":"x","roleIds":[1]}`, ""},
		{"缺roleIds放行", `{"userName":"admin","nickName":"x","password":"Abc12345!"}`, ""},
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
	in, err := DecodeCreate(context.Background(), []byte(`{"deptId":100,"userName":"admin","nickName":"管理员","email":"a@b.com","phonenumber":"13800138000","sex":"0","password":"Abc12345!","status":"0","remark":"r","roleIds":[1,2],"postIds":[3]}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.DeptID == nil || *in.DeptID != 100 || in.UserName == nil || *in.UserName != "admin" {
		t.Errorf("字段解析错误: %+v", in)
	}
	if in.Password == nil || *in.Password != "Abc12345!" || in.Email == nil || *in.Email != "a@b.com" {
		t.Errorf("字段解析错误: %+v", in)
	}
	if len(in.RoleIDs) != 2 || len(in.PostIDs) != 1 || in.PostIDs[0] != 3 {
		t.Errorf("列表解析错误: %+v", in)
	}
}

// 可选字段缺省必须映射为 nil（service 按 nil 判断是否更新/查重）。
func TestDecodeCreateOptionalNil(t *testing.T) {
	in, err := DecodeCreate(context.Background(), []byte(`{"userName":"admin","nickName":"x","password":"Abc12345!","roleIds":[1]}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.DeptID != nil || in.Email != nil || in.PhoneNumber != nil || in.Sex != nil || in.Status != nil || in.Remark != nil || in.PostIDs != nil {
		t.Errorf("可选字段缺省应为 nil: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺id", `{"userName":"admin","nickName":"x","roleIds":[1]}`, "用户ID不能为空"},
		{"id为null", `{"id":null,"userName":"admin","nickName":"x","roleIds":[1]}`, "用户ID不能为空"},
		{"合法", `{"id":1,"userName":"admin","nickName":"x","roleIds":[1]}`, ""},
		{"修改不校验密码复杂度", `{"id":1,"userName":"admin","nickName":"x","roleIds":[1],"password":"weak"}`, ""},
		{"修改password为null合法", `{"id":1,"userName":"admin","nickName":"x","roleIds":[1],"password":null}`, ""},
		{"id类型错短路", `{"userName":"","nickName":"x","id":"abc"}`, "用户ID必须是整数"},
		{"id字符串lax接受", `{"id":"3.0","userName":"admin","nickName":"x","roleIds":[1]}`, ""},
		{"雪花字符串ID精确", `{"id":"911952916879929344","userName":"admin","nickName":"x","roleIds":[1]}`, ""},
		{"纯校验多错误id在继承字段后", `{"userName":"","nickName":"x"}`, "用户账号不能为空, 用户ID不能为空"},
		{"空体", "", "用户账号不能为空, 用户昵称不能为空, 用户ID不能为空"},
		{"缺roleIds放行", `{"id":1,"userName":"admin","nickName":"x"}`, ""},
		{"roleIds空数组", `{"id":1,"userName":"admin","nickName":"x","roleIds":[]}`, "用户角色不能为空"},
		{"roleIds为null放行", `{"id":1,"userName":"admin","nickName":"x","roleIds":null}`, ""},
		{"手机号不校验", `{"id":1,"userName":"admin","nickName":"x","phonenumber":"123"}`, ""},
		{"邮箱空串合法", `{"id":1,"userName":"admin","nickName":"x","email":""}`, ""},
		{"邮箱格式仍拒", `{"id":1,"userName":"admin","nickName":"x","email":"bad"}`, "邮箱格式不正确"},
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
	in, err := DecodeUpdate(context.Background(), []byte(`{"id":10,"userName":"admin","nickName":"x","roleIds":[1,2],"postIds":[3],"password":"weak"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 10 || len(in.RoleIDs) != 2 || len(in.PostIDs) != 1 {
		t.Errorf("字段解析错误: %+v", in)
	}
	if in.Password == nil || *in.Password != "weak" {
		t.Errorf("修改应透传弱密码（不校验复杂度）: %+v", in)
	}
	// 缺省 password 必须为 nil（service 据此跳过密码更新）。
	in2, err := DecodeUpdate(context.Background(), []byte(`{"id":10,"userName":"admin","nickName":"x","roleIds":[1]}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in2.Password != nil {
		t.Errorf("缺省 password 应为 nil: %+v", in2)
	}
}

func TestDecodeResetPwdCharacterization(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"缺both", `{}`, "用户ID不能为空, 用户密码不能为空"},
		{"合法", `{"id":1,"password":"Abc12345!"}`, ""},
		{"弱密码", `{"id":1,"password":"weak"}`, pwdMsg},
		{"id类型错", `{"id":"abc","password":"Abc12345!"}`, "用户ID必须是整数"},
		{"密码为null", `{"id":1,"password":null}`, "用户密码不能为空"},
		{"密码纯空白走复杂度文案", `{"id":1,"password":" "}`, pwdMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeResetPwd(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeChangeStatusCharacterization(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"缺id", `{}`, "用户ID不能为空"},
		{"合法", `{"id":1,"status":"1"}`, ""},
		{"status可选缺省", `{"id":1}`, ""},
		{"id类型错", `{"id":"x","status":"1"}`, "用户ID必须是整数"},
		{"status类型错", `{"id":1,"status":1}`, "账号状态必须是字符串"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeChangeStatus(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeProfileUpdateCharacterization(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"空体合法", `{}`, ""},
		{"昵称超长", `{"nickName":"` + strings.Repeat("字", 31) + `"}`, "用户昵称长度不能超过30个字符"},
		{"昵称空串合法", `{"nickName":""}`, ""},
		{"邮箱格式", `{"email":"bad"}`, "邮箱格式不正确"},
		{"邮箱空串合法", `{"email":""}`, ""},
		{"手机号空串合法", `{"phonenumber":""}`, ""},
		{"邮箱超长", `{"email":"` + strings.Repeat("a", 45) + `@b.com"}`, "邮箱长度不能超过50个字符"},
		{"手机号格式", `{"phonenumber":"123"}`, "手机号格式不正确"},
		{"昵称类型错", `{"nickName":1}`, "用户昵称必须是字符串"},
		{"合法全字段", `{"nickName":"n","email":"a@b.com","phonenumber":"13800138000","sex":"0"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeProfileUpdate(context.Background(), []byte(tc.body))
			if got := decodeMsg(t, err); got != tc.want {
				t.Errorf("msg = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeUpdatePwdCharacterization(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"缺both", `{}`, "旧密码不能为空, 新密码不能为空"},
		{"旧密码空白", `{"oldPassword":" ","newPassword":"Abc12345!"}`, "旧密码不能为空"},
		{"旧密码为null", `{"oldPassword":null,"newPassword":"Abc12345!"}`, "旧密码不能为空"},
		{"新密码弱", `{"oldPassword":"old","newPassword":"weak"}`, pwdMsg},
		{"新密码类型错", `{"oldPassword":"old","newPassword":123}`, "新密码必须是字符串"},
		{"合法", `{"oldPassword":"old","newPassword":"Abc12345!"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeUpdatePwd(context.Background(), []byte(tc.body))
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
	_, err = DecodeCreate(ctx, []byte(`{"userName":123}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "must be a string") {
		t.Errorf("en locale 类型文案未生效: %q", got)
	}
}
