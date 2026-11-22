package post

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
	"walnut-seed-go/internal/pkg/validatex"
)

// 特征测试（阶段 4 版）：结构语义（必填/长度/字段存在）冻结不变；
// 文案期望更新为 i18n 中文——类型错误不再复刻 Pydantic 英文，lax 强转退役。
// 必填类中文文案与旧实现逐字一致（label 机制复刻）。
//
// ⚠️ 导出 form 校验的共享件（ValidationErrorText/PyRepr 等）已迁至 validatex，
// bindExportForm 留 post；其特征测试保持 Pydantic 原文期望一字不动。

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

// postValid 是满足全部必填校验的最小合法字段集（不含外层花括号）。
const postValid = `"deptId":100,"postCode":"ceo","postName":"董事长","postSort":1`

func TestDecodeCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体", "", "部门id不能为空, 岗位编码不能为空, 岗位名称不能为空, 显示顺序不能为空"},
		{"空白体视为空对象", "   ", "部门id不能为空, 岗位编码不能为空, 岗位名称不能为空, 显示顺序不能为空"},
		{"null体视为空对象", "null", "部门id不能为空, 岗位编码不能为空, 岗位名称不能为空, 显示顺序不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", "[1]", errs.MsgJSONInvalid},
		{"合法最小集", "{" + postValid + "}", ""},
		{"deptId下划线lax接受", `{"deptId":"1_0","postCode":"c","postName":"n","postSort":1}`, ""},
		{"deptId小数字符串lax接受", `{"deptId":"3.0","postCode":"c","postName":"n","postSort":1}`, ""},
		{"deptId非法字符串", `{"deptId":"abc","postCode":"c","postName":"n","postSort":1}`, "部门id必须是整数"},
		{"deptId为null", `{"deptId":null,"postCode":"c","postName":"n","postSort":1}`, "部门id不能为空"},
		{"deptId布尔lax接受", `{"deptId":true,"postCode":"c","postName":"n","postSort":1}`, ""},
		{"deptId带小数拒绝", `{"deptId":2.5,"postCode":"c","postName":"n","postSort":1}`, "部门id必须是整数"},
		{"deptId整值浮点lax接受", `{"deptId":2.0,"postCode":"c","postName":"n","postSort":1}`, ""},
		{"postCode类型错误短路", `{"deptId":1,"postCode":123,"postName":"n","postSort":1}`, "岗位编码必须是字符串"},
		{"postCode空串", `{"deptId":1,"postCode":"","postName":"n","postSort":1}`, "岗位编码不能为空"},
		{"postCode空白串", `{"deptId":1,"postCode":"  ","postName":"n","postSort":1}`, "岗位编码不能为空"},
		{"postCode为null", `{"deptId":1,"postCode":null,"postName":"n","postSort":1}`, "岗位编码不能为空"},
		{"postCode超长", `{"deptId":1,"postCode":"` + strings.Repeat("a", 65) + `","postName":"n","postSort":1}`,
			"岗位编码长度不能超过64个字符"},
		{"postName超长", `{"deptId":1,"postCode":"c","postName":"` + strings.Repeat("字", 51) + `","postSort":1}`,
			"岗位名称长度不能超过50个字符"},
		{"postCategory超长", "{" + postValid + `,"postCategory":"` + strings.Repeat("a", 101) + `"}`,
			"类别编码长度不能超过100个字符"},
		{"postCategory为null跳过", "{" + postValid + `,"postCategory":null}`, ""},
		{"postCategory空串放行", "{" + postValid + `,"postCategory":""}`, ""},
		{"postSort为null", `{"deptId":1,"postCode":"c","postName":"n","postSort":null}`, "显示顺序不能为空"},
		{"postSort下划线lax接受", `{"deptId":1,"postCode":"c","postName":"n","postSort":"1_0"}`, ""},
		{"postSort带小数拒绝", `{"deptId":1,"postCode":"c","postName":"n","postSort":2.5}`, "显示顺序必须是整数"},
		{"status类型错误", "{" + postValid + `,"status":123}`, "status必须是字符串"},
		{"校验级多错误按声明序", `{"deptId":null,"postCode":"","postName":" ","postSort":null}`,
			"部门id不能为空, 岗位编码不能为空, 岗位名称不能为空, 显示顺序不能为空"},
		{"类型错误短路", `{"deptId":null,"postCode":"","postName":123,"postSort":null}`, "岗位名称必须是字符串"},
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
	in, err := DecodeCreate(context.Background(), []byte(`{"deptId":10,"postCode":"ceo","postName":"董事长","postCategory":"cat","postSort":3,"status":"0","remark":"r"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.DeptID == nil || *in.DeptID != 10 {
		t.Errorf("deptId 解析错误: %+v", in)
	}
	if in.PostSort == nil || *in.PostSort != 3 {
		t.Errorf("postSort 解析错误: %+v", in)
	}
	if in.PostCode == nil || *in.PostCode != "ceo" || in.Status == nil || *in.Status != "0" {
		t.Errorf("字段解析错误: %+v", in)
	}
	if in.PostCategory == nil || *in.PostCategory != "cat" || in.Remark == nil || *in.Remark != "r" {
		t.Errorf("可选字段解析错误: %+v", in)
	}
}

func TestDecodeCreateZeroValuesValid(t *testing.T) {
	// deptId=0 / postSort=0 是合法值（旧实现仅查 nil，不查零值）
	in, err := DecodeCreate(context.Background(), []byte(`{"deptId":0,"postCode":"c","postName":"n","postSort":0}`))
	if err != nil {
		t.Fatalf("零值应合法: %v", err)
	}
	if in.DeptID == nil || *in.DeptID != 0 || in.PostSort == nil || *in.PostSort != 0 {
		t.Errorf("零值解析错误: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体_基类错误后追加id缺省", "", "部门id不能为空, 岗位编码不能为空, 岗位名称不能为空, 显示顺序不能为空, 岗位ID不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"合法", "{" + postValid + `,"id":5}`, ""},
		{"缺id", "{" + postValid + "}", "岗位ID不能为空"},
		{"id为null报非空而非类型错", "{" + postValid + `,"id":null}`, "岗位ID不能为空"},
		{"id下划线lax接受", "{" + postValid + `,"id":"1_0"}`, ""},
		{"id非法字符串", "{" + postValid + `,"id":"abc"}`, "岗位ID必须是整数"},
		{"id布尔lax接受", "{" + postValid + `,"id":true}`, ""},
		{"基类错误排在id错误前", `{"postCode":"","id":null}`,
			"部门id不能为空, 岗位编码不能为空, 岗位名称不能为空, 显示顺序不能为空, 岗位ID不能为空"},
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
	in, err := DecodeUpdate(context.Background(), []byte("{"+postValid+`,"id":10}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 10 {
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

// TestBindExportFormCharacterization 钉住导出 form 校验机制（导出 API，
// user 模块复用）的 ValidationError 原文排版——迁移文件时签名与文案一字不动。
func TestBindExportFormCharacterization(t *testing.T) {
	_, err := bindExportForm(map[string]string{"deptId": "abc"})
	want := "1 validation error for PostQueryParam\n" +
		"deptId\n" +
		"  Input should be a valid integer, unable to parse string as an integer [type=int_parsing, input_value='abc', input_type=str]\n" +
		"    For further information visit https://errors.pydantic.dev/2.13/v/int_parsing"
	if got := decodeMsg(t, err); got != want {
		t.Errorf("msg = %q, want %q", got, want)
	}

	_, err = bindExportForm(map[string]string{"pageNum": "0"})
	want = "1 validation error for PostQueryParam\n" +
		"pageNum\n" +
		"  Input should be greater than or equal to 1 [type=greater_than_equal, input_value='0', input_type=str]\n" +
		"    For further information visit https://errors.pydantic.dev/2.13/v/greater_than_equal"
	if got := decodeMsg(t, err); got != want {
		t.Errorf("pageNum msg = %q, want %q", got, want)
	}

	_, err = bindExportForm(map[string]string{"deptId": "abc", "belongDeptId": "xyz"})
	got := decodeMsg(t, err)
	if !strings.HasPrefix(got, "2 validation errors for PostQueryParam\ndeptId\n") ||
		!strings.Contains(got, "\nbelongDeptId\n") {
		t.Errorf("多错误排版/声明序错误: %q", got)
	}

	q, err := bindExportForm(map[string]string{"pageNum": "2", "deptId": "1_0", "postName": "x"})
	if err != nil {
		t.Fatalf("合法 form 不应报错: %v", err)
	}
	if q.DeptID == nil || *q.DeptID != 10 || q.PostName != "x" {
		t.Errorf("form 解析错误: %+v", q)
	}
}

func TestPyReprCharacterization(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", `'abc'`},
		{"a'b", `"a'b"`},
		{`a"b`, `'a"b'`},
		{"a\nb", `'a\nb'`},
		{"中文", `'中文'`},
	}
	for _, tc := range cases {
		if got := validatex.PyRepr(tc.in); got != tc.want {
			t.Errorf("PyRepr(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
