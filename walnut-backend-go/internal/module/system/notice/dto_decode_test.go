package notice

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
// ⚠️ notice_title 带 validate_default：缺省也跑校验器；update 的 id 同样。
// 脚本字符检查经 noscript tag 复刻，文案走中央映射的原始业务文案。

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
		{"空体触发validate_default", "", "公告标题不能为空"},
		{"空白体视为空对象", "   ", "公告标题不能为空"},
		{"null体视为空对象", "null", "公告标题不能为空"},
		{"非法JSON", "{", errs.MsgJSONInvalid},
		{"非对象JSON", `[1]`, errs.MsgJSONInvalid},
		{"合法最小集", `{"noticeTitle":"t"}`, ""},
		{"标题null跑校验器", `{"noticeTitle":null}`, "公告标题不能为空"},
		{"标题空白串", `{"noticeTitle":" "}`, "公告标题不能为空"},
		{"标题类型错短路", `{"noticeTitle":123}`, "公告标题必须是字符串"},
		{"标题超长", `{"noticeTitle":"` + strings.Repeat("字", 51) + `"}`, "公告标题长度不能超过50个字符"},
		{"标题脚本字符", `{"noticeTitle":"<script>x</script>"}`, "公告标题不能包含脚本字符"},
		{"标题超长优先于脚本", `{"noticeTitle":"<script>` + strings.Repeat("字", 51) + `"}`, "公告标题长度不能超过50个字符"},
		{"标题尖括号无闭合放行", `{"noticeTitle":"a<b"}`, ""},
		{"类型类型错", `{"noticeTitle":"t","noticeType":1}`, "noticeType必须是字符串"},
		{"类型错误短路", `{"noticeTitle":"","noticeType":1,"status":true}`, "noticeType必须是字符串"},
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
	in, err := DecodeCreate(context.Background(), []byte(`{"noticeTitle":"t","noticeType":"1","noticeContent":"c","status":"0","remark":"r"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.NoticeTitle == nil || *in.NoticeTitle != "t" || in.NoticeType == nil || *in.NoticeType != "1" ||
		in.NoticeContent == nil || *in.NoticeContent != "c" || in.Status == nil || *in.Status != "0" ||
		in.Remark == nil || *in.Remark != "r" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

func TestDecodeUpdateCharacterization(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"空体缺标题与id", `{}`, "公告标题不能为空, 公告ID不能为空"},
		{"id为null", `{"noticeTitle":"t","id":null}`, "公告ID不能为空"},
		{"id字符串非法", `{"noticeTitle":"t","id":"abc"}`, "公告ID必须是整数"},
		{"id布尔lax接受", `{"noticeTitle":"t","id":true}`, ""},
		{"id浮点非整值", `{"noticeTitle":"t","id":1.5}`, "公告ID必须是整数"},
		{"合法", `{"noticeTitle":"t","id":1}`, ""},
		{"id字符串lax接受", `{"noticeTitle":"t","id":"3.0"}`, ""},
		{"父类错误在前id在后", `{"noticeTitle":"","id":null}`, "公告标题不能为空, 公告ID不能为空"},
		{"空白体", "  ", "公告标题不能为空, 公告ID不能为空"},
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
	in, err := DecodeUpdate(context.Background(), []byte(`{"noticeTitle":"t","id":10,"status":"1"}`))
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if in.ID == nil || *in.ID != 10 || in.Status == nil || *in.Status != "1" {
		t.Errorf("字段解析错误: %+v", in)
	}
}

func TestDecodeLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	_, err := DecodeCreate(ctx, []byte(`{}`))
	if got := decodeMsg(t, err); !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}
