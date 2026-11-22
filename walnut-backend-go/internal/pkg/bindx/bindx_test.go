package bindx

import (
	"encoding/json"
	"testing"

	"walnut-seed-go/internal/pkg/errs"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func msgOf(m *string) string {
	if m == nil {
		return ""
	}
	return *m
}

func TestFields(t *testing.T) {
	cases := []struct {
		body    string
		wantErr bool
		wantLen int
	}{
		{"", false, 0},
		{"   ", false, 0},
		{"null", false, 0},
		{`{"a":1}`, false, 1},
		{"{", true, 0},
		{"[1]", true, 0},
	}
	for _, tc := range cases {
		m, err := Fields([]byte(tc.body))
		if tc.wantErr {
			se := errs.As(err)
			if se == nil || se.Msg != errs.MsgJSONInvalid {
				t.Errorf("Fields(%q) err = %v, want JSONInvalid 信封", tc.body, err)
			}
			continue
		}
		if err != nil || len(m) != tc.wantLen {
			t.Errorf("Fields(%q) = %v, %v; want len %d", tc.body, m, err, tc.wantLen)
		}
	}
}

func TestFieldsStrict(t *testing.T) {
	// 与 Fields 的契约差异：空白体报错、null 体返回 nil map
	if _, err := FieldsStrict([]byte("  ")); err == nil {
		t.Error("FieldsStrict 空白体应报 JSON 解析失败")
	}
	m, err := FieldsStrict([]byte("null"))
	if err != nil || m != nil {
		t.Errorf("FieldsStrict(null) = %v, %v; want nil map, nil", m, err)
	}
	if m, err := FieldsStrict(nil); err != nil || len(m) != 0 {
		t.Errorf("FieldsStrict(nil) = %v, %v; want 空 map", m, err)
	}
}

func TestStr(t *testing.T) {
	if v, m := Str(raw(`"x"`)); v != "x" || m != nil {
		t.Errorf(`Str("x") = %q, %v`, v, m)
	}
	if _, m := Str(raw(`123`)); msgOf(m) != MsgValidStr {
		t.Errorf("Str(123) msg = %v", m)
	}
}

func TestStrOrNull(t *testing.T) {
	if v, m := StrOrNull(nil); v != nil || m != "" {
		t.Errorf("StrOrNull(nil) = %v, %q", v, m)
	}
	if v, m := StrOrNull(raw("null")); v != nil || m != "" {
		t.Errorf("StrOrNull(null) = %v, %q", v, m)
	}
	if v, m := StrOrNull(raw(`"x"`)); v == nil || *v != "x" || m != "" {
		t.Errorf(`StrOrNull("x") = %v, %q`, v, m)
	}
	if _, m := StrOrNull(raw(`123`)); m != MsgValidStr {
		t.Errorf("StrOrNull(123) msg = %q", m)
	}
}

func TestStrList(t *testing.T) {
	if v, m := StrList(raw("null")); v != nil || m != "" {
		t.Errorf("StrList(null) = %v, %q", v, m)
	}
	v, m := StrList(raw(`["a","b"]`))
	if m != "" || len(v) != 2 || v[0] != "a" || v[1] != "b" {
		t.Errorf(`StrList(["a","b"]) = %v, %q`, v, m)
	}
	if v, m := StrList(raw(`[]`)); v == nil || len(v) != 0 || m != "" {
		t.Errorf("StrList([]) 应返回非 nil 空切片, got %v", v)
	}
	if _, m := StrList(raw(`[1]`)); m != MsgValidStr {
		t.Errorf("StrList([1]) msg = %q", m)
	}
	if _, m := StrList(raw(`"x"`)); m != MsgValidList {
		t.Errorf(`StrList("x") msg = %q`, m)
	}
}

func TestIntStrict(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		msg  string
	}{
		{`5`, 5, ""},
		{`"5"`, 5, ""},
		{`"+5"`, 5, ""},
		{`" 5 "`, 5, ""},
		{`"1_0"`, 0, MsgValidInt},
		{`"3.0"`, 0, MsgValidInt},
		{`true`, 0, MsgValidInt},
		{`"abc"`, 0, MsgValidInt},
	}
	for _, tc := range cases {
		v, m := IntStrict(raw(tc.in))
		if v != tc.want || msgOf(m) != tc.msg {
			t.Errorf("IntStrict(%s) = %d, %q; want %d, %q", tc.in, v, msgOf(m), tc.want, tc.msg)
		}
	}
}

func TestIntDigits(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		msg  string
	}{
		{`5`, 5, ""},
		{`"5"`, 5, ""},
		{`"-5"`, -5, ""},
		{`"-"`, 0, ""},    // 历史怪癖：纯负号解析为 0
		{`"--5"`, -5, ""}, // 历史怪癖：多个前导负号
		{`"+5"`, 0, MsgValidInt},
		{`"1_0"`, 0, MsgValidInt},
		{`"3.0"`, 0, MsgValidInt},
	}
	for _, tc := range cases {
		v, m := IntDigits(raw(tc.in))
		if v != tc.want || msgOf(m) != tc.msg {
			t.Errorf("IntDigits(%s) = %d, %q; want %d, %q", tc.in, v, msgOf(m), tc.want, tc.msg)
		}
	}
}

func TestSmallInt(t *testing.T) {
	if v, m := SmallInt(raw(`5`)); v != 5 || m != nil {
		t.Errorf("SmallInt(5) = %d, %v", v, m)
	}
	if _, m := SmallInt(raw(`5.0`)); msgOf(m) != MsgValidIntT {
		t.Errorf("SmallInt(5.0) msg = %v, want valid integer", m)
	}
	if _, m := SmallInt(raw(`"5"`)); msgOf(m) != MsgValidInt {
		t.Errorf(`SmallInt("5") msg = %v`, m)
	}
}

func TestBool(t *testing.T) {
	if v, m := Bool(raw(`true`)); !v || m != nil {
		t.Errorf("Bool(true) = %v, %v", v, m)
	}
	if _, m := Bool(raw(`1`)); msgOf(m) != MsgValidBool {
		t.Errorf("Bool(1) msg = %v", m)
	}
}

func TestIntSlice(t *testing.T) {
	v, m := IntSlice(raw(`[1,2]`))
	if m != nil || len(v) != 2 || v[0] != 1 || v[1] != 2 {
		t.Errorf("IntSlice([1,2]) = %v, %v", v, m)
	}
	if _, m := IntSlice(raw(`["1"]`)); msgOf(m) != MsgValidList {
		t.Errorf(`IntSlice(["1"]) msg = %v`, m)
	}
	if v, m := IntSlice(raw("null")); v != nil || m != nil {
		t.Errorf("IntSlice(null) = %v, %v", v, m)
	}
}

func TestIntOrNull(t *testing.T) {
	cases := []struct {
		in   string
		want *int64
		msg  string
	}{
		{`null`, nil, ""},
		{`5`, ptr64(5), ""},
		{`5.0`, ptr64(5), ""},
		{`5.5`, nil, MsgValidIntNum},
		{`"3.0"`, ptr64(3), ""},
		{`"1_0"`, ptr64(10), ""},
		{`"1e2"`, nil, MsgValidInt},
		{`true`, nil, MsgValidIntT},
		{`"abc"`, nil, MsgValidInt},
	}
	for _, tc := range cases {
		v, m := IntOrNull(raw(tc.in))
		if !eqPtr(v, tc.want) || m != tc.msg {
			t.Errorf("IntOrNull(%s) = %v, %q; want %v, %q", tc.in, v, m, tc.want, tc.msg)
		}
	}
	if v, m := IntOrNull(nil); v != nil || m != "" {
		t.Errorf("IntOrNull(nil) = %v, %q", v, m)
	}
}

func TestIntOrNullStrict(t *testing.T) {
	// menu 契约：字符串回退严格，拒绝 lax 形态
	if _, m := IntOrNullStrict(raw(`"1_0"`)); m != MsgValidInt {
		t.Errorf(`IntOrNullStrict("1_0") msg = %q, want %q`, m, MsgValidInt)
	}
	if _, m := IntOrNullStrict(raw(`"3.0"`)); m != MsgValidInt {
		t.Errorf(`IntOrNullStrict("3.0") msg = %q`, m)
	}
	if v, m := IntOrNullStrict(raw(`"5"`)); v == nil || *v != 5 || m != "" {
		t.Errorf(`IntOrNullStrict("5") = %v, %q`, v, m)
	}
	if v, m := IntOrNullStrict(raw("null")); v != nil || m != "" {
		t.Errorf("IntOrNullStrict(null) = %v, %q", v, m)
	}
}

func TestRequiredInt(t *testing.T) {
	if _, m := RequiredInt(nil); m != MsgValidIntT {
		t.Errorf("RequiredInt(缺省) msg = %q, want %q", m, MsgValidIntT)
	}
	if _, m := RequiredInt(raw("null")); m != MsgValidIntT {
		t.Errorf("RequiredInt(null) msg = %q", m)
	}
	if v, m := RequiredInt(raw(`"1_0"`)); v == nil || *v != 10 || m != "" {
		t.Errorf(`RequiredInt("1_0") = %v, %q`, v, m)
	}
}

func TestRequiredIntStrict(t *testing.T) {
	if _, m := RequiredIntStrict(nil); m != MsgValidIntT {
		t.Errorf("RequiredIntStrict(缺省) msg = %q", m)
	}
	// 严格回退的类型错误透传 int_parsing 原文（非 int_type）
	if _, m := RequiredIntStrict(raw(`"1_0"`)); m != MsgValidInt {
		t.Errorf(`RequiredIntStrict("1_0") msg = %q, want %q`, m, MsgValidInt)
	}
}

func TestSmallIntOrNull(t *testing.T) {
	if v, m := SmallIntOrNull(raw(`5`)); v == nil || *v != 5 || m != "" {
		t.Errorf("SmallIntOrNull(5) = %v, %q", v, m)
	}
	if v, m := SmallIntOrNull(raw("null")); v != nil || m != "" {
		t.Errorf("SmallIntOrNull(null) = %v, %q", v, m)
	}
	if _, m := SmallIntOrNull(raw(`5.5`)); m != MsgValidIntNum {
		t.Errorf("SmallIntOrNull(5.5) msg = %q", m)
	}
}

func TestPyQueryInt(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"5", 5, true},
		{" 7 ", 7, true},
		{"+5", 5, true},
		{"-5", -5, true},
		{"5.00", 5, true},
		{"1_0", 10, true},
		{"1e2", 0, false},
		{"5.5", 0, false},
		{"5.", 0, false},
		{"_1", 0, false},
		{"1_", 0, false},
		{"1__0", 0, false},
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, tc := range cases {
		n, ok := PyQueryInt(tc.in)
		if n != tc.want || ok != tc.ok {
			t.Errorf("PyQueryInt(%q) = %d, %v; want %d, %v", tc.in, n, ok, tc.want, tc.ok)
		}
	}
}

func ptr64(v int64) *int64 { return &v }

func eqPtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
