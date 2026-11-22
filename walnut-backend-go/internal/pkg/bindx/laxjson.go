package bindx

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// laxJSONBody 在 BindJSON 原生 json.Unmarshal 之前，按 Pydantic v2 **JSON 模式
// lax** 语义预强转整型字段的原始值（2026-11-21 实测 Python golden 钉住）：
//
//	字符串：PyQueryInt（首尾空白/+−号/数字间下划线/全零小数部分；拒绝指数形式）
//	布尔：  true→1、false→0
//	浮点：  整值→整数（3.0→3）；带小数部分保持原样（交给 json.Unmarshal 报类型错）
//
// 背景：雪花 ID 超出 JS 安全整数范围时两端都序列化为字符串（idtype.JSInt /
// Python _encode_int），前端回传必然是字符串；阶段 4 惯用绑定改用原生
// json.Unmarshal 后 string→int64 被 encoding/json 直接拒绝，导致全部更新类
// 接口 400「xxID必须是整数」。Python _body_ 走 model_validate_json 的 lax 强转
// 接受这些形态，Go 必须对等（docs/go/00-实施计划 §1.12 亦要求接受字符串 ID）。
//
// 实现约束：只改写「目标字段为整型/整型切片/其指针」的顶层键值，其余字节原样
// 保留；键序按原文顺序重建（encoding/json 类型错误短路取首个错误，键序即契约）；
// 解析中途任何错误都退回原文，让 json.Unmarshal 产出与既往完全一致的错误文案。
func laxJSONBody(body []byte, dst any) []byte {
	kinds := intKindsOf(dst)
	if len(kinds) == 0 {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return body
	}
	var out bytes.Buffer
	out.WriteByte('{')
	first := true
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return body
		}
		key, ok := keyTok.(string)
		if !ok {
			return body
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return body
		}
		if kind, hit := kinds[key]; hit {
			raw = coerceIntRaw(raw, kind)
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(key)
		out.Write(k)
		out.WriteByte(':')
		out.Write(raw)
	}
	if _, err := dec.Token(); err != nil { // 消费 '}'
		return body
	}
	out.WriteByte('}')
	// 尾随多余内容：退回原文，保留 json.Unmarshal 的既有报错
	if dec.Decode(new(json.RawMessage)) != io.EOF {
		return body
	}
	return out.Bytes()
}

// intKindsOf 收集 dst 顶层（含匿名嵌入）json 名 → 整型类别。
// 0=非整型不登记；1=整型标量；2=整型切片。指针按 elem 归类。
func intKindsOf(dst any) map[string]int {
	t := reflect.TypeOf(dst)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	out := make(map[string]int)
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			switch ft.Kind() {
			case reflect.Int, reflect.Int64:
				out[name] = 1
			case reflect.Slice:
				et := ft.Elem()
				for et.Kind() == reflect.Pointer {
					et = et.Elem()
				}
				if et.Kind() == reflect.Int || et.Kind() == reflect.Int64 {
					out[name] = 2
				}
			}
		}
	}
	walk(t)
	return out
}

// coerceIntRaw 按上述 lax 语义改写单个原始值；不可强转时原样返回。
func coerceIntRaw(raw json.RawMessage, kind int) json.RawMessage {
	if kind == 2 {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return raw
		}
		changed := false
		for i, item := range items {
			if next := coerceIntRaw(item, 1); !bytes.Equal(next, item) {
				items[i] = next
				changed = true
			}
		}
		if !changed {
			return raw
		}
		out, err := json.Marshal(items)
		if err != nil {
			return raw
		}
		return out
	}
	text := strings.TrimSpace(string(raw))
	switch {
	case text == "null" || text == "":
		return raw
	case text == "true":
		return json.RawMessage("1")
	case text == "false":
		return json.RawMessage("0")
	case strings.HasPrefix(text, `"`):
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return raw
		}
		if n, ok := PyQueryInt(s); ok {
			return json.RawMessage(strconv.FormatInt(n, 10))
		}
		return raw
	default:
		// 数字形态：整值浮点收窄为整数字面量；其余（整数/带小数/非法）原样保留
		if !strings.ContainsAny(text, ".eE") {
			return raw
		}
		f, err := strconv.ParseFloat(text, 64)
		if err != nil || f != float64(int64(f)) {
			return raw
		}
		return json.RawMessage(strconv.FormatInt(int64(f), 10))
	}
}
