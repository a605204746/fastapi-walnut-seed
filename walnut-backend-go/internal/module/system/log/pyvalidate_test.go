package log

import (
	"math"
	"testing"
	"time"

	"walnut-seed-go/internal/pkg/errs"
)

// 临时校验脚本：用例全部来自对 walnut-backend-python（pydantic 2.13.4）的实测输出。
func TestPyScratchInt(t *testing.T) {
	ok := map[string]int64{
		"+5": 5, "1_0": 10, " 12 ": 12, "\t12\n": 12, "007": 7, "-3": -3,
		"100.0": 100, "100.00": 100, "1_0.0": 10, "0.0": 0, "-0.0": 0, "1_0_0": 100,
		"99999999999999999999": math.MaxInt64, "-99999999999999999999": math.MinInt64,
	}
	for s, want := range ok {
		got, valid := pyParseInt(s)
		if !valid || got != want {
			t.Errorf("pyParseInt(%q) = %d,%v; want %d", s, got, valid, want)
		}
	}
	bad := []string{"0x10", "1e2", "1E2", "１２", "1__0", "_1", "1_", "", "  ", "5.", ".5",
		"100.5", "1_0.5", "1_00.0_0", "1 2", "+ 5", "-", "++5", "5-", "0b11", "0o7", "1,0", "+_1", "inf"}
	for _, s := range bad {
		if _, valid := pyParseInt(s); valid {
			t.Errorf("pyParseInt(%q) should fail", s)
		}
	}
	if n, valid := pyIntStrict("1_0"); !valid || n != 10 {
		t.Errorf("pyIntStrict(1_0) = %d,%v", n, valid)
	}
	if _, valid := pyIntStrict("100.0"); valid {
		t.Errorf("pyIntStrict(100.0) should fail")
	}
}

func TestPyScratchDate(t *testing.T) {
	okCases := map[string]string{
		"2024-01-01": "2024-01-01", "2024-02-29": "2024-02-29", "9999-12-31": "9999-12-31",
		"2024-01-01T00:00:00": "2024-01-01", "2024-01-01 00:00:00": "2024-01-01",
		"2024-01-01T00:00:00Z": "2024-01-01", "2024-01-01T00:00:00.000": "2024-01-01",
		"2024-01-01T00:00:00+08:00": "2024-01-01", "2024-01-01T00:00:00-00:00": "2024-01-01",
		"2024-01-01T00:00": "2024-01-01", "2024-01-01 00:00": "2024-01-01",
		"2024-01-01_00:00:00": "2024-01-01", "2024-01-01t00:00:00": "2024-01-01",
		"2024-01-01T00:00:00.000000": "2024-01-01",
		"0000":                       "1970-01-01", "0": "1970-01-01", "0.0": "1970-01-01",
	}
	for s, want := range okCases {
		got, derr := pyParseDate(s)
		if derr != nil || got.Format("2006-01-02") != want {
			t.Errorf("pyParseDate(%q) = %v,%+v; want %s", s, got, derr, want)
		}
	}
	const pfx = "Input should be a valid date or datetime, "
	badCases := map[string][2]string{
		"":                      {"date_from_datetime_parsing", pfx + "input is too short"},
		"2024-1-1":              {"date_from_datetime_parsing", pfx + "input is too short"},
		"2024-01-1":             {"date_from_datetime_parsing", pfx + "input is too short"},
		"2024-1-01":             {"date_from_datetime_parsing", pfx + "input is too short"},
		"abc":                   {"date_from_datetime_parsing", pfx + "input is too short"},
		"bad":                   {"date_from_datetime_parsing", pfx + "input is too short"},
		"2024-13-01":            {"date_from_datetime_parsing", pfx + "month value is outside expected range of 1-12"},
		"2024-00-01":            {"date_from_datetime_parsing", pfx + "month value is outside expected range of 1-12"},
		"2024-02-30":            {"date_from_datetime_parsing", pfx + "day value is outside expected range"},
		"2023-02-29":            {"date_from_datetime_parsing", pfx + "day value is outside expected range"},
		"2024-01-00":            {"date_from_datetime_parsing", pfx + "day value is outside expected range"},
		"2024-01-32":            {"date_from_datetime_parsing", pfx + "day value is outside expected range"},
		"2024/01/01":            {"date_from_datetime_parsing", pfx + "invalid date separator, expected `-`"},
		"10000-01-01":           {"date_from_datetime_parsing", pfx + "invalid date separator, expected `-`"},
		"abcd-01-01":            {"date_from_datetime_parsing", pfx + "invalid character in year"},
		" 2024-01-01":           {"date_from_datetime_parsing", pfx + "invalid character in year"},
		"-001-01-01":            {"date_from_datetime_parsing", pfx + "invalid character in year"},
		"2024-ab-01":            {"date_from_datetime_parsing", pfx + "invalid character in month"},
		"2024-01-ab":            {"date_from_datetime_parsing", pfx + "invalid character in day"},
		"2024-01-01 ":           {"date_from_datetime_parsing", pfx + "input is too short"},
		"2024-01-01T":           {"date_from_datetime_parsing", pfx + "input is too short"},
		"2024-01-01T00":         {"date_from_datetime_parsing", pfx + "input is too short"},
		"2024-01-011":           {"date_from_datetime_parsing", pfx + "invalid datetime separator, expected `T`, `t`, `_` or space"},
		"2024-01-01x":           {"date_from_datetime_parsing", pfx + "invalid datetime separator, expected `T`, `t`, `_` or space"},
		"2024-01-01\n":          {"date_from_datetime_parsing", pfx + "invalid datetime separator, expected `T`, `t`, `_` or space"},
		"0000-01-01":            {"date_parsing", "Input should be a valid date in the format YYYY-MM-DD, year 0 is out of range"},
		"0000-01-01T00:00:00":   {"date_parsing", "Input should be a valid date in the format YYYY-MM-DD, year 0 is out of range"},
		"2024":                  {"date_from_datetime_inexact", msgDateInexact},
		"20240101":              {"date_from_datetime_inexact", msgDateInexact},
		"123":                   {"date_from_datetime_inexact", msgDateInexact},
		"123456789":             {"date_from_datetime_inexact", msgDateInexact},
		"-1":                    {"date_from_datetime_inexact", msgDateInexact},
		"99999999999999":        {"date_from_datetime_inexact", msgDateInexact},
		"2024-01-01T01:00:00":   {"date_from_datetime_inexact", msgDateInexact},
		"2024-01-01 01:00:00":   {"date_from_datetime_inexact", msgDateInexact},
		"2024-01-01T00:00:00.5": {"date_from_datetime_inexact", msgDateInexact},
		"2024-01-01Txx:00:00":   {"date_from_datetime_parsing", pfx + "invalid character in hour"},
		"2024-01-01T0:00:00":    {"date_from_datetime_parsing", pfx + "invalid character in hour"},
		"2024-01-01T25:00:00":   {"date_from_datetime_parsing", pfx + "hour value is outside expected range of 0-23"},
		"2024-01-01T24:00:00":   {"date_from_datetime_parsing", pfx + "hour value is outside expected range of 0-23"},
		"2024-01-01T00:60:00":   {"date_from_datetime_parsing", pfx + "minute value is outside expected range of 0-59"},
		"2024-01-01T00:00:60":   {"date_from_datetime_parsing", pfx + "second value is outside expected range of 0-59"},
		"2024-01-01T0000:00":    {"date_from_datetime_parsing", pfx + "invalid time separator, expected `:`"},
		"2024-01-01T00:00:00x":  {"date_from_datetime_parsing", pfx + "invalid timezone sign"},
		"2024-01-01T00:00:00+":  {"date_from_datetime_parsing", pfx + "invalid timezone hour"},
		"2024-01-01T00:00:00.":  {"date_from_datetime_parsing", pfx + "second fraction digits missing after `.`"},
		"2024-01-01T00:00:00Zx": {"date_from_datetime_parsing", pfx + "unexpected extra characters at the end of the input"},
		"1_0":                   {"date_from_datetime_parsing", pfx + "input is too short"},
		"00:00":                 {"date_from_datetime_parsing", pfx + "input is too short"},
		"12:30":                 {"date_from_datetime_parsing", pfx + "input is too short"},
	}
	for s, want := range badCases {
		_, derr := pyParseDate(s)
		if derr == nil || derr.typ != want[0] || derr.msg != want[1] {
			t.Errorf("pyParseDate(%q) err = %+v; want %v", s, derr, want)
		}
	}
}

func TestPyScratchReprDump(t *testing.T) {
	cases := map[string]string{
		"abc":      "'abc'",
		"a'b":      `"a'b"`,
		`a"b`:      `'a"b'`,
		"a'b\"c":   `'a\'b"c'`,
		"a\\b":     `'a\\b'`,
		"a\nb":     `'a\nb'`,
		"":         "''",
		"2024-1-1": "'2024-1-1'",
	}
	for s, want := range cases {
		if got := pyRepr(s); got != want {
			t.Errorf("pyRepr(%q) = %s; want %s", s, got, want)
		}
	}
	if got := pyListRepr([]string{"1", "2"}); got != "['1', '2']" {
		t.Errorf("pyListRepr = %s", got)
	}
	dump := pyValidationDump("OperLogFilterSchema", []pyFieldErr{
		{"businessType", "int_parsing", msgIntParsing, "'abc'", "str"},
	})
	want := "1 validation error for OperLogFilterSchema\nbusinessType\n" +
		"  Input should be a valid integer, unable to parse string as an integer [type=int_parsing, input_value='abc', input_type=str]\n" +
		"    For further information visit https://errors.pydantic.dev/2.13/v/int_parsing"
	if dump != want {
		t.Errorf("dump =\n%s\nwant\n%s", dump, want)
	}
}

func TestPyScratchParseLogIDs(t *testing.T) {
	ids, err := parseLogIDs("")
	if err != nil || len(ids) != 0 {
		t.Errorf("empty: %v %v", ids, err)
	}
	ids, err = parseLogIDs(",,,")
	if err != nil || len(ids) != 0 {
		t.Errorf("commas: %v %v", ids, err)
	}
	ids, err = parseLogIDs("1_0, 2 ,3")
	if err != nil || len(ids) != 3 || ids[0] != 10 || ids[1] != 2 || ids[2] != 3 {
		t.Errorf("mixed: %v %v", ids, err)
	}
	_, err = parseLogIDs("1,abc")
	if err == nil {
		t.Fatalf("abc should fail")
	}
	se := errs.As(err)
	// 2026-11-20 两端同步：Python _parse_ids 改抛 ServiceException，不再透传 traceback
	if se == nil || se.Code != 400 || se.Msg != "日志ID格式有误" {
		t.Errorf("err = %+v", se)
	}
}

func TestPyScratchDateRange(t *testing.T) {
	b, e1 := pyParseDate("2024-01-01")
	en, e2 := pyParseDate("2024-01-02")
	if e1 != nil || e2 != nil {
		t.Fatal("parse failed")
	}
	from, to, ok := dateRange(&b, &en)
	if !ok || from.Format(time.RFC3339) != "2024-01-01T00:00:00Z" || to.Format(time.RFC3339) != "2024-01-02T23:59:59Z" {
		t.Errorf("range = %v %v", from, to)
	}
	if _, _, ok := dateRange(&b, nil); ok {
		t.Errorf("nil end should not range")
	}
}
