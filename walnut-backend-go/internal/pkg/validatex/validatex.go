// Package validatex 把 go-playground/validator 的错误翻译成与 Pydantic v2 **逐字一致**的消息。
//
// 为什么值得做这件事：参数校验失败的 msg 会被前端**直接展示给用户**，
// 属于契约的一部分。Python 端 validation_exception_handler 的逻辑是
//
//	msgs = [e["msg"] for e in exc.errors()]
//	return ", ".join(msgs) or "请求参数验证失败"
//
// 即把 Pydantic 的英文消息用 ", " 拼接。若 Go 侧统一返回"请求参数验证失败"，
// 同一个错误请求在两个后端上给用户的提示就完全不同——契约比对实测抓到了这个偏差：
//
//	py = {"code":400,"msg":"Field required, Field required, Field required, Field required"}
//	go = {"code":400,"msg":"请求参数验证失败"}
package validatex

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// 与 Python 端完全一致的固定文案。
const (
	// MsgFallback 没有任何错误明细时的兜底（对应 _validation_msg 的 else 分支）
	MsgFallback = "请求参数验证失败"
	// MsgJSONInvalid JSON 解析失败（对应 Python 端判断 json_invalid 类型的分支）
	MsgJSONInvalid = "请求数据格式错误（JSON 解析失败）"
)

// Format 把绑定/校验错误转成与 Pydantic v2 一致的 msg 字符串。
//
// 返回的字符串可直接作为 body.msg，无需再加工。
func Format(err error) string {
	if err == nil {
		return ""
	}

	// ---- JSON 语法错误 ----
	// Python 端：exc.errors() 里出现 type == "json_invalid" 时返回固定文案
	if isJSONSyntaxError(err) {
		return MsgJSONInvalid
	}

	// ---- validator 的字段级错误 ----
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		msgs := make([]string, 0, len(verrs))
		for _, fe := range verrs {
			msgs = append(msgs, fieldMessage(fe))
		}
		if len(msgs) > 0 {
			return strings.Join(msgs, ", ")
		}
	}

	return MsgFallback
}

// isJSONSyntaxError 判断是否为 JSON 解析/类型错误（而非字段校验错误）。
//
// Python 端在 exc.errors() 里出现 type == "json_invalid" 时返回固定文案。
// Go 侧对应 encoding/json 的两类错误：
//   - *json.SyntaxError          —— JSON 语法本身不合法
//   - *json.UnmarshalTypeError   —— 语法合法但类型不匹配（如给 int 字段传字符串）
//
// 后者在 Pydantic 里其实是校验错误（消息形如 "Input should be a valid integer"），
// 但形态与 validator 的字段错误完全不同，且前端提交错类型本质是请求体格式问题，
// 故统一归入 JSON 格式错误，给用户看同一句文案更有意义。
func isJSONSyntaxError(err error) bool {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return true
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return true
	}
	// 兜底：gin 有时会把错误包装成纯文本
	msg := err.Error()
	return strings.Contains(msg, "invalid character") ||
		strings.Contains(msg, "unexpected end of JSON input")
}

// fieldMessage 把单个字段错误翻译成 Pydantic v2 的消息文本。
//
// 映射表基于 Pydantic v2 的实际输出（已用契约比对逐条核对）：
//
//	缺失必填        → "Field required"
//	字符串过短      → "String should have at least N character(s)"
//	字符串过长      → "String should have at most N character(s)"
//	数值过小        → "Input should be greater than or equal to N"
//	数值过大        → "Input should be less than or equal to N"
//	其余            → 回退到 validator 的通用描述
func fieldMessage(fe validator.FieldError) string {
	param := fe.Param()

	switch fe.Tag() {
	case "required":
		return "Field required"

	case "min":
		if isStringKind(fe) {
			return pluralCount("String should have at least %d character", param)
		}
		return "Input should be greater than or equal to " + param

	case "max":
		if isStringKind(fe) {
			return pluralCount("String should have at most %d character", param)
		}
		return "Input should be less than or equal to " + param

	case "len":
		if isStringKind(fe) {
			return fmt.Sprintf("String should have exactly %s characters", param)
		}
		return "Input should have exactly " + param + " items"

	case "oneof":
		// Pydantic 对 Literal/Enum 的消息是 "Input should be 'a', 'b' or 'c'"
		return "Input should be one of: " + param

	case "email":
		return "value is not a valid email address"
	case "omitempty":
		return ""
	}

	// 未覆盖的 tag：给出可诊断的通用描述，而不是静默返回空串
	return fmt.Sprintf("Value error, field %s failed on %s validation", fe.Field(), fe.Tag())
}

// pluralCount 处理英文单复数：N=1 时用 "character"，否则 "characters"。
//
// Pydantic v2 确实会区分单复数（"at least 1 character" vs "at least 2 characters"），
// 不做这个处理会在 min=1 的字段上产生差异。
func pluralCount(tmpl string, param string) string {
	n, err := strconv.Atoi(param)
	if err != nil {
		return fmt.Sprintf(tmpl+"s", 0)
	}
	if n == 1 {
		return fmt.Sprintf(tmpl, n)
	}
	return fmt.Sprintf(tmpl+"s", n)
}

// isStringKind 判断字段是否为字符串类型。
//
// validator 的 min/max 对字符串是「长度」、对数值是「大小」，
// 而 Pydantic 的消息也据此不同（"String should have at least..." vs
// "Input should be greater than or equal to..."），故必须区分。
func isStringKind(fe validator.FieldError) bool {
	// Kind() 是字段声明的反射种类；对 *string 这类指针字段，
	// Kind 是 Ptr，需要再看 Type().Elem()
	if fe.Kind() == reflect.String {
		return true
	}
	t := fe.Type()
	if t == nil {
		return false
	}
	if t.Kind() == reflect.String {
		return true
	}
	// 指针字段：解一层看元素类型
	if t.Kind() == reflect.Pointer && t.Elem() != nil {
		return t.Elem().Kind() == reflect.String
	}
	return false
}
