package bindx

import (
	"context"
	"strings"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

type demoReq struct {
	Name  string   `json:"name" validate:"required,min=2,max=5" label:"名称"`
	Email string   `json:"email" validate:"omitempty,email" label:"邮箱"`
	Tags  []string `json:"tags" validate:"required,min=1" label:"标签"`
	Age   *int     `json:"age"`
}

type demoUpdate struct {
	demoReq
	ID *int64 `json:"id" validate:"required" label:"id"`
}

func bindMsg(t *testing.T, err error) string {
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

func TestBindJSONCollectsAllInDeclarationOrder(t *testing.T) {
	err := BindJSON(context.Background(), []byte(`{}`), &demoReq{})
	if got := bindMsg(t, err); got != "名称不能为空, 标签不能为空" {
		t.Errorf("msg = %q", got)
	}
}

func TestBindJSONEmbeddedLabels(t *testing.T) {
	err := BindJSON(context.Background(), []byte(`{}`), &demoUpdate{})
	if got := bindMsg(t, err); got != "名称不能为空, 标签不能为空, id不能为空" {
		t.Errorf("嵌入结构 label/序: %q", got)
	}
}

func TestBindJSONLengthAndEmail(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"name":"a","tags":["x"]}`, "名称长度不能少于2个字符"},
		{`{"name":"abcdef","tags":["x"]}`, "名称长度不能超过5个字符"},
		{`{"name":"ab","tags":["x"],"email":"bad"}`, "邮箱格式不正确"},
		{`{"name":"ab","tags":[]}`, "标签不能为空"},
	}
	for _, tc := range cases {
		if got := bindMsg(t, BindJSON(context.Background(), []byte(tc.body), &demoReq{})); got != tc.want {
			t.Errorf("BindJSON(%s) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestBindJSONLaxIntSemantics(t *testing.T) {
	// Pydantic JSON 模式 lax 对等（2026-11-21 实测 Python golden）：
	// 整数字符串/下划线/全零小数/整值浮点/布尔 均强转；指数/非法串/带小数拒绝。
	cases := []struct {
		body string
		want int
	}{
		{`{"name":"ab","tags":["x"],"age":"5"}`, 5},
		{`{"name":"ab","tags":["x"],"age":" 5 "}`, 5},
		{`{"name":"ab","tags":["x"],"age":"+5"}`, 5},
		{`{"name":"ab","tags":["x"],"age":"1_0"}`, 10},
		{`{"name":"ab","tags":["x"],"age":"3.0"}`, 3},
		{`{"name":"ab","tags":["x"],"age":3.0}`, 3},
		{`{"name":"ab","tags":["x"],"age":true}`, 1},
		{`{"name":"ab","tags":["x"],"age":false}`, 0},
	}
	for _, tc := range cases {
		var req demoReq
		if err := BindJSON(context.Background(), []byte(tc.body), &req); err != nil {
			t.Errorf("BindJSON(%s) = %v, want 接受", tc.body, err)
			continue
		}
		if req.Age == nil || *req.Age != tc.want {
			t.Errorf("BindJSON(%s) age = %v, want %d", tc.body, req.Age, tc.want)
		}
	}
	for _, body := range []string{`{"name":"ab","tags":["x"],"age":"abc"}`, `{"name":"ab","tags":["x"],"age":"1e3"}`, `{"name":"ab","tags":["x"],"age":2.5}`, `{"name":"ab","tags":["x"],"age":""}`} {
		if got := bindMsg(t, BindJSON(context.Background(), []byte(body), &demoReq{})); got != "age必须是整数" {
			t.Errorf("BindJSON(%s) = %q, want age必须是整数", body, got)
		}
	}
	if got := bindMsg(t, BindJSON(context.Background(), []byte(`{`), &demoReq{})); got != "请求数据格式错误（JSON 解析失败）" {
		t.Errorf("非法 JSON = %q", got)
	}
}

// TestBindJSONSnowflakeStringID 回归锁：雪花 ID 超 JS 安全范围时两端序列化为
// 字符串，前端回传字符串必须精确解析（不得经 float64 丢精度）。
func TestBindJSONSnowflakeStringID(t *testing.T) {
	var req demoUpdate
	body := `{"name":"ab","tags":["x"],"id":"911952916879929344"}`
	if err := BindJSON(context.Background(), []byte(body), &req); err != nil {
		t.Fatalf("雪花字符串 ID 被拒: %v", err)
	}
	if req.ID == nil || *req.ID != 911952916879929344 {
		t.Errorf("ID = %v, want 911952916879929344（精确）", req.ID)
	}
}

func TestBindJSONLocale(t *testing.T) {
	ctx := i18nx.WithLocale(context.Background(), i18nx.LocaleEnUS)
	err := BindJSON(ctx, []byte(`{}`), &demoReq{})
	got := bindMsg(t, err)
	if !strings.Contains(got, "is required") {
		t.Errorf("en locale 未生效: %q", got)
	}
}

func TestBindJSONValid(t *testing.T) {
	var req demoReq
	if err := BindJSON(context.Background(), []byte(`{"name":"ab","tags":["x"],"age":5}`), &req); err != nil {
		t.Errorf("合法输入: %v", err)
	}
	if req.Age == nil || *req.Age != 5 {
		t.Errorf("age 解析: %+v", req)
	}
}
