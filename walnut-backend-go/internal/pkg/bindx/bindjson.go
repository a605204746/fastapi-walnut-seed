package bindx

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/validatex"
)

// validate 是阶段 4 惯用绑定的校验器实例。validator 全量收集字段错误
// （按声明序），经 validatex.FormatI18n 出本地化文案。
var validate = validator.New()

func init() {
	// notblank：TrimSpace 后非空。validator 的 required 放行纯空白串，
	// 而业务契约（Python strip 校验）拒绝，故补此 tag。
	RegisterTag("notblank", func(fl validator.FieldLevel) bool {
		return strings.TrimSpace(fl.Field().String()) != ""
	})
}

// RegisterTag 注册自定义校验 tag（如 password 复杂度），供业务规则复用。
func RegisterTag(tag string, fn validator.Func) {
	_ = validate.RegisterValidation(tag, fn)
}

var labelCache sync.Map // reflect.Type -> map[string]string

// labelsOf 提取 label tag（结构体字段名 → 业务标签），按类型缓存。
// 匿名嵌入字段递归展开（UpdateReq 嵌入 CreateReq 的场景）。
func labelsOf(dst any) map[string]string {
	t := reflect.TypeOf(dst)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v, ok := labelCache.Load(t); ok {
		return v.(map[string]string)
	}
	labels := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if l := f.Tag.Get("label"); l != "" {
			labels[f.Name] = l
			// encoding/json 的类型错误用 JSON 名，validator 用 Go 字段名——双键注册
			if jt := f.Tag.Get("json"); jt != "" {
				if name := strings.Split(jt, ",")[0]; name != "" && name != "-" {
					labels[name] = l
				}
			}
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for k, v := range labelsOf(reflect.New(f.Type).Interface()) {
				labels[k] = v
			}
		}
	}
	labelCache.Store(t, labels)
	return labels
}

// BindJSON 阶段 4 惯用绑定入口：JSON 解码 → validator 全量校验 → i18n 文案 → 400 信封。
//
// 与手写 Decode* 的语义差异（刻意，属契约演进）：
//   - 错误文案走 i18nx（Accept-Language），不再是 Pydantic 英文原文
//   - required 对空切片/空串一律失败（比 nil-only 检查更贴近 Python falsy 语义）
//
// 整型字段保留 Pydantic JSON 模式 lax 强转（laxJSONBody）：雪花 ID 回传是
// 字符串，Python model_validate_json 接受，Go 必须对等，否则更新类接口全 400。
func BindJSON(ctx context.Context, body []byte, dst any) error {
	// 空/空白体视为空对象（与 bindx.Fields 及 Python 缺省语义一致），
	// 让必填校验报错而不是 JSON 解析错误。
	if len(strings.TrimSpace(string(body))) == 0 {
		body = []byte("{}")
	}
	body = laxJSONBody(body, dst)
	if err := json.Unmarshal(body, dst); err != nil {
		return errs.BadRequest(validatex.FormatI18n(ctx, err, labelsOf(dst)))
	}
	if err := validate.Struct(dst); err != nil {
		return errs.BadRequest(validatex.FormatI18n(ctx, err, labelsOf(dst)))
	}
	return nil
}
