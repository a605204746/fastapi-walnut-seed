package validatex

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	"walnut-seed-go/internal/pkg/i18nx"
)

// FormatI18n 把绑定/校验错误翻译成按 locale 本地化的文案（阶段 4 惯用模式）。
// labels 是结构体字段名 → 业务标签（bindx 从 label tag 提取）；缺省回退字段名。
// 与 Format（Pydantic 原文模式）并存：未迁移模块继续用 Format。
func FormatI18n(ctx context.Context, err error, labels map[string]string) string {
	if err == nil {
		return ""
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) || isJSONSyntaxText(err) {
		return i18nx.Message(ctx, "validation.json.invalid")
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return typeMessageI18n(ctx, typeErr, labels)
	}
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		msgs := make([]string, 0, len(verrs))
		for _, fe := range verrs {
			msgs = append(msgs, fieldMessageI18n(ctx, fe, labels))
		}
		if len(msgs) > 0 {
			return strings.Join(msgs, ", ")
		}
	}
	return i18nx.Message(ctx, "validation.failed")
}

// typeMessageI18n 按目标字段类型出本地化类型错误文案（对应 Format 的 typeErrorMessage）。
func typeMessageI18n(ctx context.Context, te *json.UnmarshalTypeError, labels map[string]string) string {
	// 非对象 JSON（数组/标量）打到结构体上：整体格式错误，不是字段级类型错
	if (te.Value == "array" || te.Value == "object") && te.Type.Kind() == reflect.Struct {
		return i18nx.Message(ctx, "validation.json.invalid")
	}
	label := labels[te.Field]
	if label == "" {
		label = te.Field
	}
	switch te.Type.Kind() {
	case reflect.String:
		return i18nx.Message(ctx, "validation.type.string", label)
	case reflect.Bool:
		return i18nx.Message(ctx, "validation.type.bool", label)
	case reflect.Float32, reflect.Float64:
		return i18nx.Message(ctx, "validation.type.number", label)
	case reflect.Slice, reflect.Array:
		return i18nx.Message(ctx, "validation.type.list", label)
	default:
		return i18nx.Message(ctx, "validation.type.int", label)
	}
}

func fieldMessageI18n(ctx context.Context, fe validator.FieldError, labels map[string]string) string {
	label := labels[fe.StructField()]
	if label == "" {
		label = fe.Field()
	}
	param := fe.Param()

	switch fe.Tag() {
	case "required", "notblank":
		return i18nx.Message(ctx, "validation.field.required", label)
	case "min":
		if fe.Kind() == reflect.Slice || fe.Kind() == reflect.Array {
			return i18nx.Message(ctx, "validation.field.required", label)
		}
		if isStringKind(fe) {
			return i18nx.Message(ctx, "validation.string.min", label, param)
		}
		return i18nx.Message(ctx, "validation.int.min", label, param)
	case "max":
		if fe.Kind() == reflect.Slice || fe.Kind() == reflect.Array {
			return i18nx.Message(ctx, "validation.list.max", label, param)
		}
		if isStringKind(fe) {
			return i18nx.Message(ctx, "validation.string.max", label, param)
		}
		return i18nx.Message(ctx, "validation.int.max", label, param)
	case "email", "pyemail", "pyemailOpt":
		return i18nx.Message(ctx, "validation.email", label)
	case "mobile":
		return i18nx.Message(ctx, "validation.mobile", label)
	case "password":
		return i18nx.Message(ctx, "user.password.format.valid")
	case "pyperms":
		return i18nx.Message(ctx, "validation.perms.format")
	case "jsonobj":
		return i18nx.Message(ctx, "validation.jsonobj")
	case "pydicttype":
		return i18nx.Message(ctx, "validation.dicttype")
	case "noscript":
		return i18nx.Message(ctx, "validation.noscript")
	case "oneof":
		return i18nx.Message(ctx, "validation.oneof", label)
	}
	return i18nx.Message(ctx, "validation.field.invalid", label)
}
