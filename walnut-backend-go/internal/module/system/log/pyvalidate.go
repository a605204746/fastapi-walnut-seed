package log

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// 本文件复刻 Python 端 pydantic 2.13 / speedate 的入参解析与错误文案（均已对
// walnut-backend-python 实际输出逐条实测）：
//   - GET list 校验失败 → RequestValidationError，各 msg 以 ", " 拼接，code=400；
//   - export model_validate 失败 → ValidationError 是 ValueError 子类，ValueError
//     处理器透传 str(exc) 多行 dump，code=400。
//
// ⚠️ DateStr 的 AfterValidator「无效的日期格式」对字符串入参永远不会触发：
// pydantic 核心校验（speedate）先行拒绝非法串（含 "2024-1-1" 这类单位数日期），
// 故文案以 speedate 为准，而不是 validator.py 里的 strptime 分支。

const pydanticErrURL = "https://errors.pydantic.dev/2.13/v/"

const (
	msgIntParsing   = "Input should be a valid integer, unable to parse string as an integer"
	msgIntType      = "Input should be a valid integer"
	msgIntFromFloat = "Input should be a valid integer, got a number with a fractional part"
	msgIntSize      = "Unable to parse input string as an integer, exceeded maximum size"
	msgListType     = "Input should be a valid list"
	msgStringType   = "Input should be a valid string"
	msgDateType     = "Input should be a valid date"
	msgDateInexact  = "Datetimes provided to dates should have zero time - e.g. be exact dates"
)

// pyFieldErr 对应 pydantic ValidationError.errors() 的一条（export dump 用）。
type pyFieldErr struct {
	loc       string
	typ       string
	msg       string
	inputRepr string
	inputType string
}

// pyValidationDump 复刻 str(pydantic.ValidationError)。
func pyValidationDump(model string, errs []pyFieldErr) string {
	noun := "validation errors"
	if len(errs) == 1 {
		noun = "validation error"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s for %s", len(errs), noun, model)
	for _, e := range errs {
		fmt.Fprintf(&b, "\n%s\n  %s [type=%s, input_value=%s, input_type=%s]\n    For further information visit %s%s",
			e.loc, e.msg, e.typ, e.inputRepr, e.inputType, pydanticErrURL, e.typ)
	}
	return b.String()
}

// pyRepr 复刻 Python str 的 repr（单引号优先；含单引号且不含双引号时改用双引号）。
func pyRepr(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote):
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func pyListRepr(items []string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = pyRepr(s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// pyParseInt 按 pydantic str→int 规则解析：允许首尾空白、正负号、数字间单个下划线、
// 全零小数后缀（"100.0"→100）。溢出时饱和到 int64 边界——Python int 无上限，
// 日志过滤列均为小枚举值，饱和保持「查不到」语义。
func pyParseInt(s string) (int64, bool) { return pyInt(s, true) }

// pyIntStrict 对应 Python 内置 int()（批量删除 _parse_ids 用）：不接受小数后缀。
func pyIntStrict(s string) (int64, bool) { return pyInt(s, false) }

func pyInt(s string, allowFloatSuffix bool) (int64, bool) {
	t := strings.TrimSpace(s)
	neg := false
	if len(t) > 0 && (t[0] == '+' || t[0] == '-') {
		neg = t[0] == '-'
		t = t[1:]
	}
	digits, rest, ok := pyIntDigits(t)
	if !ok {
		return 0, false
	}
	if rest != "" {
		if !allowFloatSuffix || rest[0] != '.' || len(rest) < 2 {
			return 0, false
		}
		for i := 1; i < len(rest); i++ {
			if rest[i] != '0' {
				return 0, false
			}
		}
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		if neg {
			return math.MinInt64, true
		}
		return math.MaxInt64, true
	}
	if neg {
		n = -n
	}
	return n, true
}

// pyIntDigits 消费「数字(_数字)*」，返回去掉下划线的数字串与剩余部分。
// 下划线只能出现在两个数字之间（"_1"/"1_"/"1__0" 均非法）。
func pyIntDigits(s string) (digits, rest string, ok bool) {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			b.WriteByte(c)
			i++
		case c == '_' && b.Len() > 0 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			i++
		default:
			return b.String(), s[i:], b.Len() > 0
		}
	}
	return b.String(), "", b.Len() > 0
}

// pyDateErr 日期解析错误：typ 用于 dump 的 [type=...] 与 URL，msg 用于 list 拼接。
type pyDateErr struct{ typ, msg string }

func pyDateParseErr(detail string) *pyDateErr {
	return &pyDateErr{"date_from_datetime_parsing", "Input should be a valid date or datetime, " + detail}
}

// pyParseDate 按 pydantic(DateStr)/speedate 规则解析日期字符串：
// 先走 YYYY-MM-DD[_time] 文法；文法失败再回退 Unix 时间戳（整数按 u64  wrapping
// 累加后限 i64::MAX，浮点须含 '.'；|ts|>2e10 按毫秒），全部失败才报文法错误。
func pyParseDate(s string) (time.Time, *pyDateErr) {
	if len(s) >= 10 {
		d, gerr := pyDateGrammar(s)
		if gerr == nil {
			return d, nil
		}
		if ts, ok := pySpeedateInt(s); ok {
			return pyDateFromTS(ts)
		}
		if f, ok := pyFloatTimestamp(s); ok {
			return pyDateFromFloat(f)
		}
		return time.Time{}, gerr
	}
	if ts, ok := pySpeedateInt(s); ok {
		return pyDateFromTS(ts)
	}
	if f, ok := pyFloatTimestamp(s); ok {
		return pyDateFromFloat(f)
	}
	return time.Time{}, pyDateParseErr("input is too short")
}

// pyDateGrammar 解析 YYYY-MM-DD 及可选的零时刻时间后缀。
func pyDateGrammar(s string) (time.Time, *pyDateErr) {
	for i := 0; i < 4; i++ {
		if !pyIsDigit(s[i]) {
			return time.Time{}, pyDateParseErr("invalid character in year")
		}
	}
	if s[4] != '-' {
		return time.Time{}, pyDateParseErr("invalid date separator, expected `-`")
	}
	if !pyIsDigit(s[5]) || !pyIsDigit(s[6]) {
		return time.Time{}, pyDateParseErr("invalid character in month")
	}
	if s[7] != '-' {
		return time.Time{}, pyDateParseErr("invalid date separator, expected `-`")
	}
	if !pyIsDigit(s[8]) || !pyIsDigit(s[9]) {
		return time.Time{}, pyDateParseErr("invalid character in day")
	}
	year, _ := strconv.Atoi(s[0:4])
	month, _ := strconv.Atoi(s[5:7])
	day, _ := strconv.Atoi(s[8:10])
	if month < 1 || month > 12 {
		return time.Time{}, pyDateParseErr("month value is outside expected range of 1-12")
	}
	if day < 1 || day > pyDaysInMonth(year, month) {
		return time.Time{}, pyDateParseErr("day value is outside expected range")
	}
	if year == 0 {
		return time.Time{}, &pyDateErr{"date_parsing", "Input should be a valid date in the format YYYY-MM-DD, year 0 is out of range"}
	}
	if len(s) > 10 {
		switch s[10] {
		case 'T', 't', '_', ' ':
		default:
			return time.Time{}, pyDateParseErr("invalid datetime separator, expected `T`, `t`, `_` or space")
		}
		if err := pyTimeZeroCheck(s[11:]); err != nil {
			return time.Time{}, err
		}
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// pySpeedateInt 复刻 speedate int_parse_bytes：可选符号 + 纯数字，u64 wrapping 累加，
// 结果（含负数）不得超过 i64::MAX（故 i64::MIN 不可表示；2^64 回绕为 0）。
func pySpeedateInt(s string) (int64, bool) {
	neg := false
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if len(s) == 0 {
		return 0, false
	}
	var acc uint64
	for i := 0; i < len(s); i++ {
		if !pyIsDigit(s[i]) {
			return 0, false
		}
		acc = acc*10 + uint64(s[i]-'0')
	}
	if acc > math.MaxInt64 {
		return 0, false
	}
	v := int64(acc)
	if neg {
		v = -v
	}
	return v, true
}

// pyFloatTimestamp 复刻 speedate float_parse_bytes 的回退条件：整数解析须在 '.' 处
// 失败才尝试浮点（故 "1e2"/"1_0.0" 不行，"1.5"/".0"/"1.8e19" 可以）。
func pyFloatTimestamp(s string) (float64, bool) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 || strings.ContainsAny(s, "_xX") {
		return 0, false
	}
	p := s[:dot]
	if len(p) > 0 && (p[0] == '+' || p[0] == '-') {
		p = p[1:]
	}
	for i := 0; i < len(p); i++ {
		if !pyIsDigit(p[i]) {
			return 0, false
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

// 时间戳边界：0000-01-01 与 9999-12-31T23:59:59 的 Unix 秒；|ts|>2e10 视为毫秒。
const (
	pyUnix0000     int64 = -62167219200
	pyUnix9999     int64 = 253402300799
	pyMSWatershed  int64 = 20000000000
	msgTSBefore0000      = "Input should be a valid date or datetime, dates before 0000 are not supported as unix timestamps"
	msgTSAfter9999       = "Input should be a valid date or datetime, dates after 9999 are not supported as unix timestamps"
)

func pyDateFromTS(ts int64) (time.Time, *pyDateErr) {
	abs := ts
	if abs < 0 {
		abs = -abs
	}
	var sec, us int64
	if abs <= pyMSWatershed {
		sec, us = ts, 0
	} else {
		sec = ts / 1000
		us = (ts % 1000) * 1000
		if us < 0 {
			sec--
			us += 1000000
		}
	}
	return pyDateFromSeconds(sec, us)
}

func pyDateFromFloat(f float64) (time.Time, *pyDateErr) {
	if math.Abs(f) > float64(pyMSWatershed) {
		f /= 1000
	}
	if f < float64(pyUnix0000) {
		return time.Time{}, &pyDateErr{"date_from_datetime_parsing", msgTSBefore0000}
	}
	if f > float64(pyUnix9999) {
		return time.Time{}, &pyDateErr{"date_from_datetime_parsing", msgTSAfter9999}
	}
	sec, frac := math.Modf(f)
	us := int64(frac * 1e6)
	if us < 0 {
		sec--
		us += 1000000
	}
	return pyDateFromSeconds(int64(sec), us)
}

func pyDateFromSeconds(sec, us int64) (time.Time, *pyDateErr) {
	if sec < pyUnix0000 {
		return time.Time{}, &pyDateErr{"date_from_datetime_parsing", msgTSBefore0000}
	}
	if sec > pyUnix9999 {
		return time.Time{}, &pyDateErr{"date_from_datetime_parsing", msgTSAfter9999}
	}
	t := time.Unix(sec, us*1000).UTC()
	if t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0 || us != 0 {
		return time.Time{}, &pyDateErr{"date_from_datetime_inexact", msgDateInexact}
	}
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if d.Year() == 0 {
		return time.Time{}, &pyDateErr{"date_parsing", "Input should be a valid date in the format YYYY-MM-DD, year 0 is out of range"}
	}
	return d, nil
}

// pyTimeZeroCheck 解析日期后的时间后缀（HH:MM[:SS[.frac]][tz]）：
// 时刻为零返回 nil（日期可用），非零返回 inexact，语法错误返回对应 speedate 文案。
// 时区偏移不影响零时刻判定（"+08:00" 的零点仍算零点，与 Python 一致）。
func pyTimeZeroCheck(s string) *pyDateErr {
	if len(s) < 2 {
		return pyDateParseErr("input is too short")
	}
	if !pyIsDigit(s[0]) || !pyIsDigit(s[1]) {
		return pyDateParseErr("invalid character in hour")
	}
	hour := int(s[0]-'0')*10 + int(s[1]-'0')
	if hour > 23 {
		return pyDateParseErr("hour value is outside expected range of 0-23")
	}
	if len(s) < 3 {
		return pyDateParseErr("input is too short")
	}
	if s[2] != ':' {
		return pyDateParseErr("invalid time separator, expected `:`")
	}
	if len(s) < 5 {
		return pyDateParseErr("input is too short")
	}
	if !pyIsDigit(s[3]) || !pyIsDigit(s[4]) {
		return pyDateParseErr("invalid character in minute")
	}
	minute := int(s[3]-'0')*10 + int(s[4]-'0')
	if minute > 59 {
		return pyDateParseErr("minute value is outside expected range of 0-59")
	}
	pos := 5
	second := 0
	if pos < len(s) && s[pos] == ':' {
		pos++
		if len(s) < pos+2 {
			return pyDateParseErr("input is too short")
		}
		if !pyIsDigit(s[pos]) || !pyIsDigit(s[pos+1]) {
			return pyDateParseErr("invalid character in second")
		}
		second = int(s[pos]-'0')*10 + int(s[pos+1]-'0')
		if second > 59 {
			return pyDateParseErr("second value is outside expected range of 0-59")
		}
		pos += 2
	}
	fracZero := true
	if pos < len(s) && s[pos] == '.' {
		pos++
		start := pos
		// speedate 只保留微秒（6 位），更低位直接截断（".0000001" 仍算零点）。
		for n := 0; pos < len(s) && pyIsDigit(s[pos]); n++ {
			if n < 6 && s[pos] != '0' {
				fracZero = false
			}
			pos++
		}
		if pos == start {
			return pyDateParseErr("second fraction digits missing after `.`")
		}
	}
	if pos < len(s) {
		switch s[pos] {
		case 'Z', 'z':
			pos++
		case '+', '-':
			pos++
			if len(s) < pos+2 || !pyIsDigit(s[pos]) || !pyIsDigit(s[pos+1]) {
				return pyDateParseErr("invalid timezone hour")
			}
			pos += 2
			if pos >= len(s) || s[pos] != ':' {
				return pyDateParseErr("invalid timezone minute")
			}
			pos++
			if len(s) < pos+2 || !pyIsDigit(s[pos]) || !pyIsDigit(s[pos+1]) {
				return pyDateParseErr("invalid timezone minute")
			}
			pos += 2
		default:
			return pyDateParseErr("invalid timezone sign")
		}
	}
	if pos < len(s) {
		return pyDateParseErr("unexpected extra characters at the end of the input")
	}
	if hour == 0 && minute == 0 && second == 0 && fracZero {
		return nil
	}
	return &pyDateErr{"date_from_datetime_inexact", msgDateInexact}
}

func pyIsDigit(c byte) bool { return c >= '0' && c <= '9' }

func pyDaysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	}
	return 30
}