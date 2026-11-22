package dept

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

// 特征测试（阶段 4 版）：结构语义（必填/长度/邮箱格式/字段存在）冻结不变；
// 文案期望更新为 i18n 中文——类型错误不再复刻 Pydantic 英文，字符串回退退役。
// 必填类中文文案与旧实现逐字一致（label 机制复刻）。
//
// ⚠️ dept 旧整型字符串回退是**严格 ParseInt**（接受 "5"、拒绝 "1_0"/"3.0"）——
// 阶段 4 起统一为 Pydantic JSON 模式 lax（bindx.laxJSONBody，实测 golden 对等）。

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

// deptValid 是满足全部必填校验的最小合法字段集（不含外层花括号）。
const deptValid = `"parentId":0,"deptName":"x","orderNum":1`

func TestDecodeCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体", "", "父部门ID不能为空, 部门名称不能为空, 显示顺序不能为空"},
		{"空白体视为空对象", "   ", "父部门ID不能为空, 部门名称不能为空, 显示顺序不能为空"},
		{"null体视为空对象", "null", "父部门ID不能为空, 部门名称不能为空, 显示顺序不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", "[1]", errs.MsgJSONInvalid},
		{"合法最小集", "{" + deptValid + "}", ""},
		{"缺parentId", `{"deptName":"x","orderNum":1}`, "父部门ID不能为空"},
		{"parentId为null", `{"parentId":null,"deptName":"x","orderNum":1}`, "父部门ID不能为空"},
		{"parentId零值合法", `{"parentId":0,"deptName":"x","orderNum":1}`, ""},
		{"parentId字符串数字lax接受", `{"parentId":"5","deptName":"x","orderNum":1}`, ""},
		{"parentId下划线lax接受", `{"parentId":"1_0","deptName":"x","orderNum":1}`, ""},
		{"parentId小数字符串lax接受", `{"parentId":"3.0","deptName":"x","orderNum":1}`, ""},
		{"parentId布尔lax接受", `{"parentId":true,"deptName":"x","orderNum":1}`, ""},
		{"parentId带小数拒绝", `{"parentId":3.5,"deptName":"x","orderNum":1}`, "父部门ID必须是整数"},
		{"parentId整值浮点lax接受", `{"parentId":3.0,"deptName":"x","orderNum":1}`, ""},
		{"deptName类型错", `{"parentId":0,"deptName":123,"orderNum":1}`, "部门名称必须是字符串"},
		{"deptName空串", `{"parentId":0,"deptName":"","orderNum":1}`, "部门名称不能为空"},
		{"deptName空白串", `{"parentId":0,"deptName":"   ","orderNum":1}`, "部门名称不能为空"},
		{"deptName为null触发必填", `{"parentId":0,"deptName":null,"orderNum":1}`, "部门名称不能为空"},
		{"deptName超长", `{"parentId":0,"deptName":"` + strings.Repeat("字", 31) + `","orderNum":1}`,
			"部门名称长度不能超过30个字符"},
		{"deptCategory超长", "{" + deptValid + `,"deptCategory":"` + strings.Repeat("a", 101) + `"}`,
			"部门类别编码长度不能超过100个字符"},
		{"orderNum为null", `{"parentId":0,"deptName":"x","orderNum":null}`, "显示顺序不能为空"},
		{"orderNum下划线lax接受", `{"parentId":0,"deptName":"x","orderNum":"1_0"}`, ""},
		{"orderNum字符串数字lax接受", `{"parentId":0,"deptName":"x","orderNum":"2"}`, ""},
		{"leader下划线lax接受", "{" + deptValid + `,"leader":"1_0"}`, ""},
		{"phone超长", "{" + deptValid + `,"phone":"123456789012"}`, "联系电话长度不能超过11个字符"},
		{"email格式错误", "{" + deptValid + `,"email":"bad"}`, "邮箱格式不正确"},
		{"email空串格式错误", "{" + deptValid + `,"email":""}`, "邮箱格式不正确"},
		{"email末尾换行合法", "{" + deptValid + `,"email":"a@b.co\n"}`, ""},
		{"email超长优先于格式", "{" + deptValid + `,"email":"` + strings.Repeat("a", 45) + `@b.com"}`,
			"邮箱长度不能超过50个字符"},
		{"status类型错误", "{" + deptValid + `,"status":123}`, "部门状态必须是字符串"},
		{"校验级多错误按声明序", `{"parentId":null,"deptName":"","orderNum":null,"phone":"123456789012","email":"bad"}`,
			"父部门ID不能为空, 部门名称不能为空, 显示顺序不能为空, 联系电话长度不能超过11个字符, 邮箱格式不正确"},
		{"类型错误短路", `{"deptName":123,"deptCategory":456}`, "部门名称必须是字符串"},
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
	in, err := DecodeCreate(context.Background(), []byte(`{"parentId":100,"deptName":"研发部门","deptCategory":"dev","orderNum":3,"leader":7,"phone":"13800138000","email":"a@b.com","status":"0"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ParentID == nil || *in.ParentID != 100 || in.DeptName == nil || *in.DeptName != "研发部门" {
		t.Errorf("字段解析错误: %+v", in)
	}
	if in.OrderNum == nil || *in.OrderNum != 3 || in.Leader == nil || *in.Leader != 7 {
		t.Errorf("整型字段解析错误: %+v", in)
	}
	if in.Status == nil || *in.Status != "0" {
		t.Errorf("status 解析错误: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体_基类错误后追加id缺省", "", "父部门ID不能为空, 部门名称不能为空, 显示顺序不能为空, 部门ID不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"合法", "{" + deptValid + `,"id":5}`, ""},
		{"缺id报必填", "{" + deptValid + "}", "部门ID不能为空"},
		{"id为null", "{" + deptValid + `,"id":null}`, "部门ID不能为空"},
		{"id下划线lax接受", "{" + deptValid + `,"id":"1_0"}`, ""},
		{"id字符串数字lax接受", "{" + deptValid + `,"id":"7"}`, ""},
		{"基类错误排在id错误前", `{"deptName":"","id":null}`,
			"父部门ID不能为空, 部门名称不能为空, 显示顺序不能为空, 部门ID不能为空"},
		{"id类型错短路", `{"deptName":"","id":"abc"}`, "部门ID必须是整数"},
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
	in, err := DecodeUpdate(context.Background(), []byte("{"+deptValid+`,"id":9}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 9 {
		t.Errorf("id 解析错误: %+v", in)
	}
}

func TestDecodeLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	_, err := DecodeCreate(ctx, []byte(`{}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}
