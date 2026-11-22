package menu

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

// 特征测试（阶段 4 版）：结构语义（必填/长度/queryParam/perms/字段存在）冻结不变；
// 文案期望更新为 i18n 中文——类型错误不再复刻 Pydantic 英文，字符串回退退役。
// 必填类中文文案与旧实现逐字一致（label 机制复刻）。
//
// ⚠️ menu 旧整型字符串回退是**严格 ParseInt**（接受 "5"、拒绝 "1_0"/"3.0"）——
// 阶段 4 起统一为 Pydantic JSON 模式 lax（bindx.laxJSONBody，实测 golden 对等）。
// 自定义规则 pyperms/jsonobj 经 bindx.RegisterTag 复刻，文案走 i18n 兜底
// （「权限标识必须符合 tool:build:list 格式」/「路由参数必须符合JSON格式」），不再逐字保留旧格式提示。

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

// menuValid 是满足全部必填校验的最小合法字段集（不含外层花括号）。
const menuValid = `"menuName":"x","orderNum":1,"menuType":"M"`

func TestDecodeCreateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体", "", "菜单名称不能为空, 显示顺序不能为空, 菜单类型不能为空"},
		{"空白体视为空对象", "   ", "菜单名称不能为空, 显示顺序不能为空, 菜单类型不能为空"},
		{"null体视为空对象", "null", "菜单名称不能为空, 显示顺序不能为空, 菜单类型不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", "[1]", errs.MsgJSONInvalid},
		{"合法最小集", "{" + menuValid + "}", ""},
		{"id下划线lax接受", "{" + menuValid + `,"id":"1_0"}`, ""},
		{"id字符串数字lax接受", "{" + menuValid + `,"id":"5"}`, ""},
		{"id为null视为缺省", "{" + menuValid + `,"id":null}`, ""},
		{"parentId下划线lax接受", "{" + menuValid + `,"parentId":"1_0"}`, ""},
		{"parentId小数字符串lax接受", "{" + menuValid + `,"parentId":"3.0"}`, ""},
		{"parentId带小数拒绝", "{" + menuValid + `,"parentId":2.5}`, "父菜单ID必须是整数"},
		{"parentId布尔lax接受", "{" + menuValid + `,"parentId":true}`, ""},
		{"parentId整值浮点lax接受", "{" + menuValid + `,"parentId":3.0}`, ""},
		{"menuName类型错", `{"menuName":123,"orderNum":1,"menuType":"M"}`, "菜单名称必须是字符串"},
		{"menuName空串", `{"menuName":"","orderNum":1,"menuType":"M"}`, "菜单名称不能为空"},
		{"menuName为null触发必填", `{"menuName":null,"orderNum":1,"menuType":"M"}`, "菜单名称不能为空"},
		{"menuName超长", `{"menuName":"` + strings.Repeat("字", 51) + `","orderNum":1,"menuType":"M"}`,
			"菜单名称长度不能超过50个字符"},
		{"orderNum为null", `{"menuName":"x","orderNum":null,"menuType":"M"}`, "显示顺序不能为空"},
		{"orderNum下划线lax接受", `{"menuName":"x","orderNum":"1_0","menuType":"M"}`, ""},
		{"orderNum字符串数字lax接受", `{"menuName":"x","orderNum":"2","menuType":"M"}`, ""},
		{"path超长", "{" + menuValid + `,"path":"` + strings.Repeat("a", 201) + `"}`, "路由地址长度不能超过200个字符"},
		{"component超长", "{" + menuValid + `,"component":"` + strings.Repeat("a", 201) + `"}`, "组件路径长度不能超过200个字符"},
		{"queryParam非JSON", "{" + menuValid + `,"queryParam":"notjson"}`, "路由参数必须符合JSON格式"},
		{"queryParam字面null拒绝", "{" + menuValid + `,"queryParam":"null"}`, "路由参数必须符合JSON格式"},
		{"queryParam合法对象", "{" + menuValid + `,"queryParam":"{\"a\":1}"}`, ""},
		{"queryParam空串跳过校验", "{" + menuValid + `,"queryParam":""}`, ""},
		{"queryParam空白串跳过校验", "{" + menuValid + `,"queryParam":"  "}`, ""},
		{"menuType空串", `{"menuName":"x","orderNum":1,"menuType":""}`, "菜单类型不能为空"},
		{"perms格式错误", "{" + menuValid + `,"perms":"bad"}`, "权限标识必须符合 tool:build:list 格式"},
		{"perms空串合法", "{" + menuValid + `,"perms":""}`, ""},
		{"perms超长优先于格式", "{" + menuValid + `,"perms":"` + strings.Repeat("a", 101) + `"}`,
			"权限标识长度不能超过100个字符"},
		{"perms末尾换行合法", "{" + menuValid + `,"perms":"system:user:list\n"}`, ""},
		{"isFrame类型错误", "{" + menuValid + `,"isFrame":123}`, "是否为外链必须是字符串"},
		{"校验级多错误按声明序", `{"menuName":"","orderNum":null,"path":"` + strings.Repeat("a", 201) + `","menuType":"","perms":"bad"}`,
			"菜单名称不能为空, 显示顺序不能为空, 路由地址长度不能超过200个字符, 菜单类型不能为空, 权限标识必须符合 tool:build:list 格式"},
		{"类型错误短路", `{"menuName":"","isFrame":123,"menuType":""}`, "是否为外链必须是字符串"},
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
	in, err := DecodeCreate(context.Background(), []byte(`{"id":3,"parentId":0,"menuName":"系统管理","orderNum":1,"path":"system","component":"system/index","queryParam":"{\"a\":1}","isFrame":"1","isCache":"0","menuType":"C","visible":"0","status":"0","perms":"system:user:list","icon":"#","remark":"r"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 3 || in.ParentID == nil || *in.ParentID != 0 {
		t.Errorf("整型字段解析错误: %+v", in)
	}
	if in.MenuName == nil || *in.MenuName != "系统管理" || in.OrderNum == nil || *in.OrderNum != 1 {
		t.Errorf("字段解析错误: %+v", in)
	}
	if in.Perms == nil || *in.Perms != "system:user:list" {
		t.Errorf("perms 解析错误: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体_基类错误后追加id缺省", "", "菜单名称不能为空, 显示顺序不能为空, 菜单类型不能为空, 菜单ID不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"合法", "{" + menuValid + `,"id":5}`, ""},
		{"缺id报必填", "{" + menuValid + "}", "菜单ID不能为空"},
		{"id为null", "{" + menuValid + `,"id":null}`, "菜单ID不能为空"},
		// 旧怪癖退役：create 基类不再重复解析 id，类型错误单条短路。
		{"id下划线lax接受_单条", "{" + menuValid + `,"id":"1_0"}`, ""},
		{"id字符串数字lax接受", "{" + menuValid + `,"id":"7"}`, ""},
		{"id类型错短路", `{"menuName":"","id":"abc"}`, "菜单ID必须是整数"},
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
	in, err := DecodeUpdate(context.Background(), []byte("{"+menuValid+`,"id":9}`))
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
