// Package idtype 提供 JSInt：复刻 Python 端 app/common/response.py 的 _encode_int 条件转换语义。
//
// ⚠️ 不可用 encoding/json 的 `,string` tag 替代。`,string` 是**无条件**转字符串，
// 会把 status/sex/total 等小整数也变成 "0"/"1"，导致前端所有 === 0 判断失效。
// 实测确认 Python 的规则是：|v| >= 9007199254740991 才转字符串（边界值本身也转）。
//
// 详见 docs/go/00-Go后端实施计划.md §9.1。
package idtype

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// JS 安全整数范围（Number.MAX_SAFE_INTEGER = 2^53 - 1）。
const (
	JSSafeMax int64 = 9007199254740991
	JSSafeMin int64 = -9007199254740991
)

// JSInt 是条件序列化的 int64：超出 JS 安全范围输出字符串，范围内输出数字。
//
// 典型用途：雪花 ID（实测 18-19 位，恒超范围 → 恒为字符串）；
// 同时可安全用于任何 int 字段，行为与 Python 端逐字节一致。
type JSInt int64

// MarshalJSON 复刻 _encode_int：严格不等式，边界值本身转字符串。
func (v JSInt) MarshalJSON() ([]byte, error) {
	n := int64(v)
	if n > JSSafeMin && n < JSSafeMax {
		// 范围内 → 数字字面量
		return strconv.AppendInt(nil, n, 10), nil
	}
	// 超范围 → 带引号字符串
	return strconv.AppendQuote(nil, strconv.FormatInt(n, 10)), nil
}

// UnmarshalJSON 同时接受数字与字符串。
//
// 必须两者都收：前端回传 ID 时用的是字符串（因为它收到的就是字符串），
// 而 Pydantic 会自动强转、Go 不会。少了字符串分支，所有带 ID 的写请求都会 400。
func (v *JSInt) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return fmt.Errorf("JSInt 不接受 null（可空 ID 请用 *JSInt）")
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("无法把字符串 %q 解析为整数: %w", s, err)
		}
		*v = JSInt(n)
		return nil
	}
	n, err := strconv.ParseInt(string(trimmed), 10, 64)
	if err != nil {
		return fmt.Errorf("无法解析整数 %q: %w", trimmed, err)
	}
	*v = JSInt(n)
	return nil
}

// Int64 返回底层值。
func (v JSInt) Int64() int64 { return int64(v) }

// String 实现 fmt.Stringer，便于日志输出。
func (v JSInt) String() string { return strconv.FormatInt(int64(v), 10) }

// JSIntSlice 是 []JSInt 的别名语义包装，确保空值序列化为 [] 而非 null。
type JSIntSlice []JSInt

func (s JSIntSlice) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	items := make([]json.RawMessage, len(s))
	for i, v := range s {
		blob, err := v.MarshalJSON()
		if err != nil {
			return nil, err
		}
		items[i] = blob
	}
	return json.Marshal(items)
}

func (s *JSIntSlice) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(JSIntSlice, 0, len(raw))
	for _, item := range raw {
		var v JSInt
		if err := v.UnmarshalJSON(item); err != nil {
			return err
		}
		out = append(out, v)
	}
	*s = out
	return nil
}
