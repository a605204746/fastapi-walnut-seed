// 本文件托管查询串的 Pydantic 兼容解析器（原 dept/req.go 逐字迁入）。
// dept/config/dict/post/user 的查询绑定共用；逻辑与文案一字不动。

package bindx

import (
	"math"
	"strconv"
	"time"

	"walnut-seed-go/internal/pkg/errs"
)

// ParseDateQuery 镜像 Pydantic v2（speedate）对 DateStr 查询串的解析。
// 成功返回规范化 YYYY-MM-DD；失败返回 Pydantic 原文（code=400）。
//
// ⚠️ GET 查询串先经 Pydantic 的 date 类型强转：非法值报 date_from_datetime_parsing
// 原文（如 "input is too short"），**不是** validator 的「无效的日期格式」——
// 后者只在强转成功后的值上运行，查询场景不可达（已用 Python 实测钉住）。
// ⚠️ 纯数字串走 Unix 时间戳路径且优先于日期格式（'0000000000' → 1970-01-01）；
// 仅当时间戳恰好是整天时才是合法 date，否则报 inexact 原文。
func ParseDateQuery(raw string) (string, error) {
	const prefix = "Input should be a valid date or datetime, "
	const inexact = "Datetimes provided to dates should have zero time - e.g. be exact dates"
	bad := func(msg string) (string, error) { return "", errs.BadRequest(prefix + msg) }

	if isPlainNumeric(raw) {
		f, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) &&
			f == math.Trunc(f) && f >= math.MinInt64 && f <= math.MaxInt64 && int64(f)%86400 == 0 {
			return time.Unix(int64(f), 0).UTC().Format("2006-01-02"), nil
		}
		return "", errs.BadRequest(inexact)
	}
	if len(raw) < 10 {
		return bad("input is too short")
	}
	for i := 0; i < 4; i++ {
		if !isDigitByte(raw[i]) {
			return bad("invalid character in year")
		}
	}
	if raw[4] != '-' {
		return bad("invalid date separator, expected `-`")
	}
	if !isDigitByte(raw[5]) || !isDigitByte(raw[6]) {
		return bad("invalid character in month")
	}
	if raw[7] != '-' {
		return bad("invalid date separator, expected `-`")
	}
	if !isDigitByte(raw[8]) || !isDigitByte(raw[9]) {
		return bad("invalid character in day")
	}
	year := int(raw[0]-'0')*1000 + int(raw[1]-'0')*100 + int(raw[2]-'0')*10 + int(raw[3]-'0')
	month := int(raw[5]-'0')*10 + int(raw[6]-'0')
	day := int(raw[8]-'0')*10 + int(raw[9]-'0')
	if year == 0 {
		return "", errs.BadRequest("Input should be a valid date in the format YYYY-MM-DD, year 0 is out of range")
	}
	if month < 1 || month > 12 {
		return bad("month value is outside expected range of 1-12")
	}
	if day < 1 || day > daysInMonth(year, month) {
		return bad("day value is outside expected range")
	}
	if len(raw) == 10 {
		return raw, nil
	}
	// datetime 后缀：分隔符 + 时间（仅零时间合法）+ 可选时区
	switch raw[10] {
	case 'T', 't', '_', ' ':
	default:
		return bad("invalid datetime separator, expected `T`, `t`, `_` or space")
	}
	tp := raw[11:]
	if len(tp) < 5 {
		return bad("input is too short")
	}
	if !isDigitByte(tp[0]) || !isDigitByte(tp[1]) {
		return bad("invalid character in hour")
	}
	hour := int(tp[0]-'0')*10 + int(tp[1]-'0')
	if hour > 23 {
		return bad("hour value is outside expected range of 0-23")
	}
	if tp[2] != ':' {
		return bad("invalid time separator, expected `:`")
	}
	if !isDigitByte(tp[3]) || !isDigitByte(tp[4]) {
		return bad("invalid character in minute")
	}
	minute := int(tp[3]-'0')*10 + int(tp[4]-'0')
	if minute > 59 {
		return bad("minute value is outside expected range of 0-59")
	}
	second, fracZero := 0, true
	rest := tp[5:]
	if len(rest) > 0 && rest[0] == ':' {
		if len(rest) < 3 || !isDigitByte(rest[1]) || !isDigitByte(rest[2]) {
			return bad("invalid character in second")
		}
		second = int(rest[1]-'0')*10 + int(rest[2]-'0')
		if second > 59 {
			return bad("second value is outside expected range of 0-59")
		}
		rest = rest[3:]
		if len(rest) > 0 && rest[0] == '.' {
			rest = rest[1:]
			n := 0
			for n < len(rest) && isDigitByte(rest[n]) {
				if rest[n] != '0' {
					fracZero = false
				}
				n++
			}
			if n == 0 {
				return bad("second fraction digits missing after `.`")
			}
			rest = rest[n:]
		}
	}
	// 时区：Z/z 或 ±HH:MM（偏移不改变「零时间」判定，Python 实测 +08:00 仍合法）
	if len(rest) > 0 && rest != "Z" && rest != "z" {
		if (rest[0] != '+' && rest[0] != '-') || len(rest) != 6 ||
			!isDigitByte(rest[1]) || !isDigitByte(rest[2]) || rest[3] != ':' ||
			!isDigitByte(rest[4]) || !isDigitByte(rest[5]) {
			return bad("invalid timezone sign")
		}
		oh := int(rest[1]-'0')*10 + int(rest[2]-'0')
		om := int(rest[4]-'0')*10 + int(rest[5]-'0')
		if oh*60+om >= 24*60 {
			return bad("timezone offset must be less than 24 hours")
		}
	}
	if hour != 0 || minute != 0 || second != 0 || !fracZero {
		return "", errs.BadRequest(inexact)
	}
	return raw[:10], nil
}

func isDigitByte(ch byte) bool { return ch >= '0' && ch <= '9' }

// isPlainNumeric 判定 Pydantic 时间戳路径接受的数字串：
// 可选符号 + 数字 + 至多一个小数点（'.5'、'5.' 均可），**不含下划线/指数**
// （Go 的 ParseFloat 接受 '1_0'，Pydantic 不接受，须先过滤）。
func isPlainNumeric(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	digits, dot := 0, false
	for ; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			digits++
			continue
		}
		if ch == '.' && !dot {
			dot = true
			continue
		}
		return false
	}
	return digits > 0
}

func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	default:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	}
}
