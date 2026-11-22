// Package bindx 集中托管各模块 DTO 的 Pydantic 兼容解码基座。
//
// 历史上 9 个模块各自复制 rawFields/decodeStr/decodeInt 等辅助，且已漂移出
// 多种语义（整数字符串回退有严格 ParseInt、Pydantic lax、纯数字循环三种）。
// 本包把每种语义显式命名、单点实现、单测钉住；模块按各自契约选用对应变体，
// 行为逐字不变。字段声明序与错误拼接逻辑仍留在各模块 Decode* 中——那是模块
// 契约的一部分，不是通用机制。
//
// 命名约定：OrNull 后缀 = 缺省/null 返回 nil 指针（*ptr, string 文案族）；
// 无后缀 = 值 + *string 文案族（调用方先以 IsNull 挡 null）。
package bindx

import (
	"encoding/json"
	"strconv"
	"strings"

	"walnut-seed-go/internal/pkg/errs"
)

// Pydantic v2 类型错误原文（各模块常量值已逐一核对一致）。
const (
	MsgValidInt    = "Input should be a valid integer, unable to parse string as an integer"
	MsgValidIntNum = "Input should be a valid integer, got a number with a fractional part"
	MsgValidIntT   = "Input should be a valid integer"
	MsgValidStr    = "Input should be a valid string"
	MsgValidList   = "Input should be a valid list"
	MsgValidBool   = "Input should be a valid boolean"
)

// ---- 顶层键值 ----

// Fields 解析 JSON 对象顶层键值，保留原始字节（client/config/dept/dict/menu/notice/post 同款）。
// 空白体视为空对象（触发各字段缺省校验）；null 体也归一为空对象。
func Fields(body []byte) (map[string]json.RawMessage, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, errs.BadRequest(errs.MsgJSONInvalid)
	}
	if fields == nil {
		return map[string]json.RawMessage{}, nil
	}
	return fields, nil
}

// FieldsStrict 是 user/role 同款：仅 len==0 视为空；空白体报 JSON 解析失败；
// null 体返回 nil map（键查找行为与空 map 等价）。与 Fields 的差异是契约的一部分。
func FieldsStrict(body []byte) (map[string]json.RawMessage, error) {
	if len(body) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, errs.BadRequest(errs.MsgJSONInvalid)
	}
	return m, nil
}

// IsNull 判断原始值是否为 JSON null。
func IsNull(raw json.RawMessage) bool { return string(raw) == "null" }

// ---- 值 + *string 文案族（user/role）----

// Str 严格字符串解码；类型错误返回 Pydantic 原文。null 由调用方先挡。
func Str(raw json.RawMessage) (string, *string) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		msg := MsgValidStr
		return "", &msg
	}
	return v, nil
}

// IntStrict 严格整型解码（user 同款）：JSON 整数，或字符串经 strconv.ParseInt
// （接受 "+5"，拒绝 "1_0"/"3.0"）。
func IntStrict(raw json.RawMessage) (int64, *string) {
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil {
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 == nil {
			n, err3 := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err3 == nil {
				return n, nil
			}
		}
		msg := MsgValidInt
		return 0, &msg
	}
	return v, nil
}

// IntDigits 整型解码（role 同款）：字符串回退走纯数字循环——只认数字与
// 前导 '-'（不接受 '+'/下划线/小数），保留历史怪癖（如 "-" 解析为 0）。
func IntDigits(raw json.RawMessage) (int64, *string) {
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil {
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 == nil {
			if n, ok := digitsInt(s); ok {
				return n, nil
			}
		}
		msg := MsgValidInt
		return 0, &msg
	}
	return v, nil
}

// digitsInt 逐字复刻 role 的历史实现（含 "-"→0、多个前导 '-' 的怪癖）。
func digitsInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	var v int64
	for _, c := range s {
		if c < '0' || c > '9' {
			if c == '-' && v == 0 {
				continue
			}
			return 0, false
		}
		v = v*10 + int64(c-'0')
	}
	if len(s) > 0 && s[0] == '-' {
		v = -v
	}
	return v, true
}

// SmallInt 小整型解码（role 同款）：浮点形态报 "valid integer"（非 fractional 文案）。
func SmallInt(raw json.RawMessage) (int, *string) {
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		var f float64
		if err2 := json.Unmarshal(raw, &f); err2 == nil {
			msg := MsgValidIntT
			return 0, &msg
		}
		msg := MsgValidInt
		return 0, &msg
	}
	return v, nil
}

// Bool 布尔解码（role 同款）。
func Bool(raw json.RawMessage) (bool, *string) {
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		msg := MsgValidBool
		return false, &msg
	}
	return v, nil
}

// IntSlice 整型列表解码（user/role 同款）：元素类型错误整体报 list 原文。
func IntSlice(raw json.RawMessage) ([]int64, *string) {
	var v []int64
	if err := json.Unmarshal(raw, &v); err != nil {
		msg := MsgValidList
		return nil, &msg
	}
	return v, nil
}

// ---- *ptr + string 文案族（client/config/dept/dict/menu/notice/post）----

// StrOrNull 可空字符串解码；Pydantic lax 不把数字强转字符串。
func StrOrNull(raw json.RawMessage) (*string, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, MsgValidStr
	}
	return &s, ""
}

// StrList 字符串列表解码（client 同款）：逐元素校验，空列表返回非 nil 空切片。
func StrList(raw json.RawMessage) ([]string, string) {
	if string(raw) == "null" {
		return nil, ""
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, MsgValidList
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		var s string
		if err := json.Unmarshal(item, &s); err != nil {
			return nil, MsgValidStr
		}
		out = append(out, s)
	}
	return out, ""
}

// IntOrNull 可空整型解码（client/config/dept/dict/notice/post 同款）：
// 字符串走 Pydantic lax（"3.0"→3、"1_0"→10），布尔拒绝，浮点须整值。
func IntOrNull(raw json.RawMessage) (*int64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	n, ok, terr := looseInt(raw, true)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, ""
	}
	return &n, ""
}

// IntOrNullStrict 可空整型解码（menu 同款）：字符串走严格 ParseInt，
// 拒绝 "1_0"/"3.0"——与 IntOrNull 的差异是 menu 契约的一部分。
func IntOrNullStrict(raw json.RawMessage) (*int64, string) {
	n, ok, terr := looseInt(raw, false)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, ""
	}
	return &n, ""
}

// RequiredInt 必填整型解码（dept/config/dict 同款，lax）：缺省/null 报 int_type 原文。
func RequiredInt(raw json.RawMessage) (*int64, string) {
	if string(raw) == "null" {
		return nil, MsgValidIntT
	}
	n, ok, terr := looseInt(raw, true)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, MsgValidIntT
	}
	return &n, ""
}

// RequiredIntStrict 必填整型解码（menu 同款，严格字符串回退）。
func RequiredIntStrict(raw json.RawMessage) (*int64, string) {
	if string(raw) == "null" {
		return nil, MsgValidIntT
	}
	n, ok, terr := looseInt(raw, false)
	if terr != "" {
		return nil, terr
	}
	if !ok {
		return nil, MsgValidIntT
	}
	return &n, ""
}

// SmallIntOrNull 可空小整型解码（post 同款）：经 IntOrNull 后收窄为 int。
func SmallIntOrNull(raw json.RawMessage) (*int, string) {
	v, terr := IntOrNull(raw)
	if terr != "" {
		return nil, terr
	}
	if v == nil {
		return nil, ""
	}
	n := int(*v)
	return &n, ""
}

// looseInt 按 Pydantic lax 模式解析整数。pyStr=true 时字符串走 PyQueryInt，
// false 走严格 ParseInt。返回：值、是否非 null、类型错误文案。
func looseInt(raw json.RawMessage, pyStr bool) (int64, bool, string) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, false, ""
	}
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, false, MsgValidStr
		}
		if pyStr {
			n, valid := PyQueryInt(s)
			if !valid {
				return 0, false, MsgValidInt
			}
			return n, true, ""
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return 0, false, MsgValidInt
		}
		return n, true, ""
	}
	if text == "true" || text == "false" {
		return 0, false, MsgValidIntT
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false, MsgValidInt
	}
	if f != float64(int64(f)) {
		return 0, false, MsgValidIntNum
	}
	return int64(f), true, ""
}

// ---- Python int 语义 ----

// PyQueryInt 镜像 Pydantic v2 的 str→int（查询参数 lax 模式）：
// 允许首尾空白、+/- 号、数字间单个下划线、".全零" 小数（'5.00'→5、'1_0'→10）；
// 不允许指数形式。失败返回 false（调用方报 int_parsing 原文）。
// 原 dept.ParsePyQueryInt 逐字迁入；dept 包装已退役，查询绑定调用方直接用本函数。
func PyQueryInt(raw string) (int64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	neg := false
	if s[0] == '+' || s[0] == '-' {
		neg = s[0] == '-'
		s = s[1:]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		frac := s[i+1:]
		if frac == "" {
			return 0, false
		}
		for _, ch := range frac {
			if ch != '0' {
				return 0, false
			}
		}
		s = s[:i]
	}
	var b strings.Builder
	prevDigit := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= '0' && ch <= '9':
			b.WriteByte(ch)
			prevDigit = true
		case ch == '_' && prevDigit && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			prevDigit = false
		default:
			return 0, false
		}
	}
	if b.Len() == 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(b.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}
