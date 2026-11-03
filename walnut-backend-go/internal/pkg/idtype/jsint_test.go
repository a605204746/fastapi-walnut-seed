package idtype

import (
	"encoding/json"
	"testing"
)

// TestJSIntMarshal 的期望值全部来自 Python 端 _encode_int 的**实测输出**
// （见 docs/go/00-Go后端实施计划.md §9.1），不是推导出来的。
//
// 这组用例是整类回归的防线：若有人把 JSInt 换成 encoding/json 的 `,string` tag，
// 前 5 个「范围内保持数字」的用例会立刻失败——而那正是 blanket 转字符串会破坏
// 前端 `status === 0` 判断的地方。
func TestJSIntMarshal(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string // 期望的 JSON 字面量
	}{
		{"零", 0, `0`},
		{"正小整数", 1, `1`},
		{"负小整数", -1, `-1`},
		{"典型状态值", 100, `100`},
		{"MAX-1 仍是数字", 9007199254740990, `9007199254740990`},
		{"MAX 本身转字符串", 9007199254740991, `"9007199254740991"`},
		{"MAX+1 转字符串", 9007199254740992, `"9007199254740992"`},
		{"MIN 本身转字符串", -9007199254740991, `"-9007199254740991"`},
		{"MIN-1 转字符串", -9007199254740992, `"-9007199254740992"`},
		{"MIN+1 仍是数字", -9007199254740990, `-9007199254740990`},
		{"真实雪花 ID", 904941282285273088, `"904941282285273088"`},
		{"int64 上限", 9223372036854775807, `"9223372036854775807"`},
		{"int64 下限", -9223372036854775808, `"-9223372036854775808"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(JSInt(tc.in))
			if err != nil {
				t.Fatalf("Marshal 失败: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("JSInt(%d) = %s，期望 %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestJSIntUnmarshal 验证双向兼容：前端回传 ID 时用的是字符串
// （因为它收到的就是字符串），而 Pydantic 会自动强转、Go 不会。
// 少了字符串分支，所有带 ID 的写请求都会 400。
func TestJSIntUnmarshal(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int64
	}{
		{"数字字面量", `904941282285273088`, 904941282285273088},
		{"字符串形式", `"904941282285273088"`, 904941282285273088},
		{"小整数字符串", `"100"`, 100},
		{"负数字符串", `"-9007199254740992"`, -9007199254740992},
		{"零", `0`, 0},
		{"带空白", `  42  `, 42},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v JSInt
			if err := json.Unmarshal([]byte(tc.in), &v); err != nil {
				t.Fatalf("Unmarshal(%s) 失败: %v", tc.in, err)
			}
			if v.Int64() != tc.want {
				t.Errorf("Unmarshal(%s) = %d，期望 %d", tc.in, v.Int64(), tc.want)
			}
		})
	}
}

func TestJSIntUnmarshalRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"null", `null`},
		{"非数字字符串", `"abc"`},
		{"浮点", `1.5`},
		{"超出 int64", `"99999999999999999999"`},
		{"空串", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v JSInt
			if err := json.Unmarshal([]byte(tc.in), &v); err == nil {
				t.Errorf("Unmarshal(%s) 应当失败，却得到 %d", tc.in, v.Int64())
			}
		})
	}
}

// TestJSIntRoundTrip 验证「序列化后再反序列化」值不变，
// 且范围内的值不会被意外加上引号。
func TestJSIntRoundTrip(t *testing.T) {
	for _, in := range []int64{0, 1, -1, 9007199254740990, 9007199254740991, 904941282285273088} {
		blob, err := json.Marshal(JSInt(in))
		if err != nil {
			t.Fatalf("Marshal(%d) 失败: %v", in, err)
		}
		var out JSInt
		if err := json.Unmarshal(blob, &out); err != nil {
			t.Fatalf("Unmarshal(%s) 失败: %v", blob, err)
		}
		if out.Int64() != in {
			t.Errorf("往返不一致：in=%d out=%d (json=%s)", in, out.Int64(), blob)
		}
	}
}

// TestStructMixedFields 是 §9.1 要求的「一次性防住整类回归」的用例：
// 同一个响应里 status/total 必须是数字、id 必须是字符串。
//
// 期望的 JSON 形状来自 Python 端 _JSON_CUSTOM_ENCODER 的实际行为。
func TestStructMixedFields(t *testing.T) {
	type row struct {
		ID     JSInt `json:"id"`
		Status int   `json:"status"`
		Total  int64 `json:"total"`
		Sex    JSInt `json:"sex"`
	}

	got, err := json.Marshal(row{ID: 904941282285273088, Status: 0, Total: 5, Sex: 1})
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	// status/sex/total 保持数字，id 是字符串
	want := `{"id":"904941282285273088","status":0,"total":5,"sex":1}`
	if string(got) != want {
		t.Errorf("混合字段序列化不符\n  got  = %s\n  want = %s", got, want)
	}

	// 反序列化时前端回传的 id 是字符串，必须能收
	var back row
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if back.ID.Int64() != 904941282285273088 || back.Status != 0 || back.Total != 5 {
		t.Errorf("反序列化结果不符: %+v", back)
	}
}

// TestJSIntSlice 验证空值输出 [] 而非 null（对齐 Python 的 default_factory=list）。
func TestJSIntSlice(t *testing.T) {
	var nilSlice JSIntSlice
	got, err := json.Marshal(nilSlice)
	if err != nil {
		t.Fatalf("Marshal nil 切片失败: %v", err)
	}
	if string(got) != `[]` {
		t.Errorf("nil JSIntSlice = %s，期望 []（null 会让前端 v-for 崩）", got)
	}

	empty := JSIntSlice{}
	got, _ = json.Marshal(empty)
	if string(got) != `[]` {
		t.Errorf("空 JSIntSlice = %s，期望 []", got)
	}

	filled := JSIntSlice{904941282285273088, 1, 2}
	got, _ = json.Marshal(filled)
	if want := `["904941282285273088",1,2]`; string(got) != want {
		t.Errorf("JSIntSlice = %s，期望 %s", got, want)
	}

	var back JSIntSlice
	if err := json.Unmarshal([]byte(`["904941282285273088",1,2]`), &back); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if len(back) != 3 || back[0].Int64() != 904941282285273088 || back[1].Int64() != 1 {
		t.Errorf("反序列化结果不符: %v", back)
	}
}

func TestStringer(t *testing.T) {
	if got := JSInt(904941282285273088).String(); got != "904941282285273088" {
		t.Errorf("String() = %s", got)
	}
}
