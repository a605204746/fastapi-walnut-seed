// Package timex 提供与 Python 端一致的日期时间 JSON 序列化类型。
//
// 契约（app/common/constant.py）：
//
//	DATE_DISPLAY_FMT     = "%Y-%m-%d"
//	TIME_DISPLAY_FMT     = "%H:%M:%S"
//	DATETIME_DISPLAY_FMT = "%Y-%m-%d %H:%M:%S"
//
// 三个类型的零值统一序列化为 null —— Go 的 time.Time 零值是 0001-01-01，
// 若直接输出会让前端收到 "0001-01-01 00:00:00"，而 Python 端 None 输出 null。
package timex

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Go 的时间布局常量（参考时间必须是 2006-01-02 15:04:05）。
const (
	LayoutDateTime = "2006-01-02 15:04:05"
	LayoutDate     = "2006-01-02"
	LayoutClock    = "15:04:05"
)

// nullJSON 零值的统一输出。
const nullJSON = "null"

// JSONTime 对应 Python 的 datetime，输出 DATETIME_DISPLAY_FMT。
type JSONTime time.Time

// JSONDate 对应 Python 的 date，输出 DATE_DISPLAY_FMT。
type JSONDate time.Time

// JSONClock 对应 Python 的 time，输出 TIME_DISPLAY_FMT。
type JSONClock time.Time

func (t JSONTime) IsZero() bool { return time.Time(t).IsZero() }
func (t JSONDate) IsZero() bool { return time.Time(t).IsZero() }
func (t JSONClock) IsZero() bool {
	// time.Time 零值是 0001-01-01 00:00:00；对纯时刻类型，判据取「日期部分为零值」
	return time.Time(t).IsZero()
}

func (t JSONTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte(nullJSON), nil
	}
	return strconv.AppendQuote(nil, time.Time(t).Format(LayoutDateTime)), nil
}

func (t JSONDate) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte(nullJSON), nil
	}
	return strconv.AppendQuote(nil, time.Time(t).Format(LayoutDate)), nil
}

func (t JSONClock) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte(nullJSON), nil
	}
	return strconv.AppendQuote(nil, time.Time(t).Format(LayoutClock)), nil
}

// UnmarshalJSON 接受展示格式字符串、RFC3339 与 null。
// 宽松接收是必要的：前端提交日期时可能带 T 分隔符或时区偏移。
func (t *JSONTime) UnmarshalJSON(data []byte) error {
	parsed, ok, err := parseTimeBytes(data, LayoutDateTime, time.RFC3339)
	if err != nil {
		return err
	}
	if !ok {
		*t = JSONTime(time.Time{})
		return nil
	}
	*t = JSONTime(parsed)
	return nil
}

func (t *JSONDate) UnmarshalJSON(data []byte) error {
	parsed, ok, err := parseTimeBytes(data, LayoutDate, LayoutDateTime, time.RFC3339)
	if err != nil {
		return err
	}
	if !ok {
		*t = JSONDate(time.Time{})
		return nil
	}
	*t = JSONDate(parsed)
	return nil
}

func (t *JSONClock) UnmarshalJSON(data []byte) error {
	parsed, ok, err := parseTimeBytes(data, LayoutClock)
	if err != nil {
		return err
	}
	if !ok {
		*t = JSONClock(time.Time{})
		return nil
	}
	*t = JSONClock(parsed)
	return nil
}

// parseTimeBytes 依次尝试给定布局。ok=false 表示输入是 null/空串（合法的「无值」）。
func parseTimeBytes(data []byte, layouts ...string) (time.Time, bool, error) {
	s := strings.TrimSpace(string(data))
	if s == "" || s == nullJSON || s == `""` {
		return time.Time{}, false, nil
	}
	unquoted, err := strconv.Unquote(s)
	if err != nil {
		unquoted = strings.Trim(s, `"`)
	}
	unquoted = strings.TrimSpace(unquoted)
	if unquoted == "" {
		return time.Time{}, false, nil
	}
	for _, layout := range layouts {
		if parsed, perr := time.ParseInLocation(layout, unquoted, time.Local); perr == nil {
			return parsed, true, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("无法解析时间 %q，接受格式：%s", unquoted, strings.Join(layouts, " / "))
}

// Value / Scan 让三个类型可直接用于 GORM 与 database/sql。

func (t JSONTime) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return time.Time(t), nil
}

func (t *JSONTime) Scan(v any) error {
	return scanTime(v, func(parsed time.Time) { *t = JSONTime(parsed) })
}

func (t JSONDate) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return time.Time(t), nil
}

func (t *JSONDate) Scan(v any) error {
	return scanTime(v, func(parsed time.Time) { *t = JSONDate(parsed) })
}

func scanTime(v any, assign func(time.Time)) error {
	switch val := v.(type) {
	case nil:
		return nil
	case time.Time:
		assign(val)
		return nil
	case []byte:
		parsed, ok, err := parseTimeBytes(val, LayoutDateTime, LayoutDate, time.RFC3339)
		if err != nil {
			return err
		}
		if ok {
			assign(parsed)
		}
		return nil
	case string:
		return scanTime([]byte(val), assign)
	default:
		return fmt.Errorf("无法把 %T 扫描为时间类型", v)
	}
}

// Time 转出标准 time.Time。
func (t JSONTime) Time() time.Time { return time.Time(t) }

// FromTime 从标准 time.Time 构造。
func FromTime(t time.Time) JSONTime { return JSONTime(t) }

// ISOTimestamp 复刻 Python 的 datetime.isoformat() 输出格式。
//
// 语义必须精确对齐（健康检查的 timestamp 字段用它）：
//   - 微秒为 0 → 不带小数部分：      2026-11-02T11:53:34
//   - 微秒非 0 → 恒定 6 位，不裁剪：  2026-11-02T11:53:34.691200
//
// ⚠️ 不能用 Go 的 ".999999" 布局：它会裁掉末尾的 0，把 .691200 输出成 .6912。
// 也不能用 ".000000"：它在微秒为 0 时仍输出 .000000，而 Python 会整体省略。
func ISOTimestamp(t time.Time) string {
	base := t.Format("2006-01-02T15:04:05")
	micro := t.Nanosecond() / 1000
	if micro == 0 {
		return base
	}
	return base + "." + fmt.Sprintf("%06d", micro)
}
