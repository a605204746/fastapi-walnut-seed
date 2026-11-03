package timex

import (
	"encoding/json"
	"testing"
	"time"
)

// ref 是构造测试时间的基准，固定本地时区避免受运行环境影响。
func ref(y int, m time.Month, d, hh, mm, ss, nsec int) time.Time {
	return time.Date(y, m, d, hh, mm, ss, nsec, time.Local)
}

// TestISOTimestamp 验证与 Python datetime.isoformat() 的逐字符一致。
//
// 两个易错点：
//  1. 微秒为 0 时**整体省略**小数部分（不能用 ".000000" 布局）
//  2. 微秒非 0 时**恒定 6 位、不裁剪末尾的 0**（不能用 ".999999" 布局，
//     它会把 .691200 输出成 .6912）
func TestISOTimestamp(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"无小数", ref(2026, 11, 2, 11, 53, 34, 0), "2026-11-02T11:53:34"},
		{"完整 6 位微秒", ref(2026, 11, 2, 11, 53, 34, 691234000), "2026-11-02T11:53:34.691234"},
		{"末尾有零不可裁剪", ref(2026, 11, 2, 11, 53, 34, 691200000), "2026-11-02T11:53:34.691200"},
		{"仅 1 位有效微秒", ref(2026, 11, 2, 11, 53, 34, 100000000), "2026-11-02T11:53:34.100000"},
		{"纳秒被截断为微秒", ref(2026, 11, 2, 11, 53, 34, 691234999), "2026-11-02T11:53:34.691234"},
		{"午夜", ref(2026, 1, 1, 0, 0, 0, 0), "2026-01-01T00:00:00"},
		{"个位数月日补零", ref(2026, 3, 5, 7, 8, 9, 0), "2026-03-05T07:08:09"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ISOTimestamp(tc.in); got != tc.want {
				t.Errorf("ISOTimestamp() = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestJSONTimeMarshal 验证展示格式与零值 → null。
//
// 零值必须输出 null：Go 的 time.Time 零值是 0001-01-01，若直接格式化会让前端
// 收到 "0001-01-01 00:00:00"，而 Python 端 None 输出的是 null。
func TestJSONTimeMarshal(t *testing.T) {
	cases := []struct {
		name string
		in   JSONTime
		want string
	}{
		{"正常值", JSONTime(ref(2026, 11, 2, 11, 53, 34, 0)), `"2026-11-02 11:53:34"`},
		{"零值输出 null", JSONTime(time.Time{}), `null`},
		{"含纳秒被截断", JSONTime(ref(2026, 11, 2, 11, 53, 34, 691234000)), `"2026-11-02 11:53:34"`},
		{"Unix 纪元", JSONTime(ref(1970, 1, 1, 0, 0, 0, 0)), `"1970-01-01 00:00:00"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("Marshal 失败: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Marshal = %s，期望 %s", got, tc.want)
			}
		})
	}
}

func TestJSONDateMarshal(t *testing.T) {
	got, err := json.Marshal(JSONDate(ref(2026, 11, 2, 11, 53, 34, 0)))
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	if want := `"2026-11-02"`; string(got) != want {
		t.Errorf("JSONDate = %s，期望 %s（时分秒必须被裁掉）", got, want)
	}

	got, _ = json.Marshal(JSONDate(time.Time{}))
	if string(got) != `null` {
		t.Errorf("JSONDate 零值 = %s，期望 null", got)
	}
}

func TestJSONClockMarshal(t *testing.T) {
	got, err := json.Marshal(JSONClock(ref(2026, 11, 2, 11, 53, 34, 0)))
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	if want := `"11:53:34"`; string(got) != want {
		t.Errorf("JSONClock = %s，期望 %s", got, want)
	}
}

// TestJSONTimeUnmarshal 验证宽松接收：前端提交日期时可能带 T 分隔符或时区偏移。
func TestJSONTimeUnmarshal(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string // 期望再序列化后的结果
		wantNil bool
	}{
		{"展示格式", `"2026-11-02 11:53:34"`, `"2026-11-02 11:53:34"`, false},
		{"RFC3339 带 T", `"2026-11-02T11:53:34+08:00"`, `"2026-11-02 11:53:34"`, false},
		{"null", `null`, `null`, true},
		{"空字符串", `""`, `null`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v JSONTime
			if err := json.Unmarshal([]byte(tc.in), &v); err != nil {
				t.Fatalf("Unmarshal(%s) 失败: %v", tc.in, err)
			}
			if tc.wantNil && !v.IsZero() {
				t.Errorf("Unmarshal(%s) 应得到零值，却得到 %v", tc.in, v.Time())
			}
			got, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("再序列化失败: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("往返结果 = %s，期望 %s", got, tc.want)
			}
		})
	}
}

func TestJSONTimeUnmarshalRejectsGarbage(t *testing.T) {
	var v JSONTime
	if err := json.Unmarshal([]byte(`"不是时间"`), &v); err == nil {
		t.Error("非法时间字符串应当报错，而不是静默变成零值")
	}
}

// TestRoundTripInStruct 验证嵌入结构体后的行为（健康探针载荷就是这种形状）。
func TestRoundTripInStruct(t *testing.T) {
	type row struct {
		Created JSONTime `json:"createTime"`
		Updated JSONTime `json:"updateTime"`
	}
	in := row{Created: JSONTime(ref(2026, 11, 2, 11, 53, 34, 0))}
	got, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	// UpdateTime 未设置 → 必须输出 null，而不是 "0001-01-01 00:00:00"
	want := `{"createTime":"2026-11-02 11:53:34","updateTime":null}`
	if string(got) != want {
		t.Errorf("结构体序列化\n  got  = %s\n  want = %s", got, want)
	}
}

// TestScanValue 验证 GORM / database/sql 互操作。
func TestScanValue(t *testing.T) {
	src := JSONTime(ref(2026, 11, 2, 11, 53, 34, 0))

	v, err := src.Value()
	if err != nil {
		t.Fatalf("Value() 失败: %v", err)
	}
	if v == nil {
		t.Fatal("非零值 Value() 不应返回 nil")
	}

	var zero JSONTime
	zv, err := zero.Value()
	if err != nil {
		t.Fatalf("零值 Value() 失败: %v", err)
	}
	if zv != nil {
		t.Errorf("零值 Value() 应返回 nil（存进 DB 为 NULL），却得到 %v", zv)
	}

	// 从 time.Time 扫描回来
	var back JSONTime
	if err := back.Scan(ref(2026, 11, 2, 11, 53, 34, 0)); err != nil {
		t.Fatalf("Scan(time.Time) 失败: %v", err)
	}
	if !back.Time().Equal(src.Time()) {
		t.Errorf("Scan 往返不一致: %v vs %v", back.Time(), src.Time())
	}

	// 从 []byte 扫描（某些驱动会这样返回）
	var fromBytes JSONTime
	if err := fromBytes.Scan([]byte("2026-11-02 11:53:34")); err != nil {
		t.Fatalf("Scan([]byte) 失败: %v", err)
	}
	if !fromBytes.Time().Equal(src.Time()) {
		t.Errorf("Scan([]byte) 往返不一致: %v", fromBytes.Time())
	}

	// 从 nil 扫描（DB 里的 NULL）
	var fromNil JSONTime
	if err := fromNil.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) 失败: %v", err)
	}
	if !fromNil.IsZero() {
		t.Errorf("Scan(nil) 应得到零值，却得到 %v", fromNil.Time())
	}
}
