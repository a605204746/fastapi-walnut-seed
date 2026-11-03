// Package i18nx 实现国际化消息解析，对齐 Python 端 app/utils/i18n.py。
//
// 资源文件是 Python 端 app/i18n/messages_*.properties 的**逐字节副本**
// （SHA256 已校验一致），用 go:embed 打进二进制，避免运行期依赖外部目录。
//
// 与 Python 端的一处实现差异：locale 的传递方式。
//   - Python 用 ContextVar（asyncio 任务内自动传播）
//   - Go 用 context.Context 显式传递
//
// 二者语义等价，但 Go 侧**必须**把 ctx 一路传到需要文案的地方；
// 拿不到 ctx 时用 DefaultLocale（zh_CN）兜底，行为与 Python 的 ContextVar 默认值一致。
package i18nx

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

//go:embed messages/*.properties
var messagesFS embed.FS

// 受支持的语言环境与默认值（与 Python 端 _resolve_locale 的回退一致）。
const (
	LocaleZhCN = "zh_CN"
	LocaleEnUS = "en_US"

	// DefaultLocale 找不到匹配时的回退语言。
	DefaultLocale = LocaleZhCN
)

// ctxKey 是私有的 context key 类型，避免与其它包的 key 冲突。
type ctxKey struct{}

var (
	cacheMu sync.RWMutex
	// messagesCache: locale -> {key: value}，对应 Python 端的 _messages_cache
	messagesCache = make(map[string]map[string]string)
)

// WithLocale 把语言环境写入 ctx。由 Locale 中间件在请求开始时调用。
func WithLocale(ctx context.Context, locale string) context.Context {
	if locale == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, locale)
}

// LocaleFrom 从 ctx 取语言环境；未设置时返回 DefaultLocale。
// 对应 Python 端 get_locale()（ContextVar 的 default 就是 zh_CN）。
func LocaleFrom(ctx context.Context) string {
	if ctx == nil {
		return DefaultLocale
	}
	if v, ok := ctx.Value(ctxKey{}).(string); ok && v != "" {
		return v
	}
	return DefaultLocale
}

// Message 按 ctx 中的语言环境解析消息键，args 填充 {0}/{1} 占位符。
//
// 对应 Python 端 MessageUtils.message(code, *args)。**未命中时返回消息键本身**
// （而非空串或报错）——这是刻意的：让缺失的翻译在界面上暴露成
// "user.password.not.match" 这样的键名，比静默显示空白更容易被发现。
func Message(ctx context.Context, code string, args ...any) string {
	return MessageIn(LocaleFrom(ctx), code, args...)
}

// MessageIn 按显式指定的语言环境解析（不依赖 ctx）。
func MessageIn(locale, code string, args ...any) string {
	if code == "" {
		return ""
	}
	template, ok := messagesFor(locale)[code]
	if !ok {
		return code
	}
	if len(args) == 0 {
		return template
	}
	return format(template, args)
}

// format 把 {0}/{1}/... 占位符替换为参数。
//
// 对应 Python 的 template.format(*args)，但**不用 fmt.Sprintf**：
// properties 文件里的文案可能含裸 % 字符（如"密码长度至少 %d 位"之外的
// 百分号），Sprintf 会把它当动词解析并输出 %!d(MISSING) 之类的垃圾。
// 手动替换 {n} 既安全又与 Python 的 str.format 语义一致。
//
// 占位符序号越界时保留原样（Python 的 format 会抛 IndexError，
// 其调用方 except 后返回未格式化的 template —— 这里直接保留占位符，效果等价）。
func format(template string, args []any) string {
	var b strings.Builder
	b.Grow(len(template) + 16)
	for i := 0; i < len(template); i++ {
		if template[i] != '{' {
			b.WriteByte(template[i])
			continue
		}
		// 找到配对的 '}'
		end := strings.IndexByte(template[i+1:], '}')
		if end < 0 {
			b.WriteByte(template[i])
			continue
		}
		idxStr := template[i+1 : i+1+end]
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= len(args) {
			// 非法或越界的占位符原样保留
			b.WriteString(template[i : i+1+end+1])
			i += end + 1
			continue
		}
		b.WriteString(stringify(args[idx]))
		i += end + 1
	}
	return b.String()
}

// stringify 把参数转成字符串。
// error 单独处理：它的 %v 与 String() 一致，但显式分支更清晰。
func stringify(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case error:
		return val.Error()
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// messagesFor 返回指定 locale 的消息表（带缓存）。
//
// 回退链与 Python 端 _messages_for 完全一致，且**合并顺序**也必须一致：
//
//	candidates = [locale, locale.split("_")[0], "zh_CN"]
//	for cand in reversed(candidates): merged.update(load(cand))
//
// 逆序 update 意味着**越靠前的候选优先级越高**（后写入的覆盖先写入的）。
// 即：messages_zh_CN（默认）← messages_zh（语言级）← messages_zh_CN（精确）
// 精确匹配的最终生效。
func messagesFor(locale string) map[string]string {
	cacheMu.RLock()
	cached, ok := messagesCache[locale]
	cacheMu.RUnlock()
	if ok {
		return cached
	}

	prefix := locale
	if i := strings.IndexByte(locale, '_'); i > 0 {
		prefix = locale[:i]
	}
	candidates := []string{locale, prefix, DefaultLocale}

	merged := make(map[string]string, 64)
	// 逆序遍历，使靠前的候选获得更高优先级
	for i := len(candidates) - 1; i >= 0; i-- {
		for k, v := range loadProperties(candidates[i]) {
			merged[k] = v
		}
	}

	cacheMu.Lock()
	messagesCache[locale] = merged
	cacheMu.Unlock()
	return merged
}

// loadProperties 解析 .properties 资源（key=value，# 或 ! 开头为注释）。
// 规则与 Python 端 _load_properties 逐条对齐：
//   - 跳过空行、以 # 或 ! 开头的行
//   - 跳过不含 = 的行
//   - 按**第一个** = 分割（值里可以再出现 =）
//   - 键与值都 TrimSpace
func loadProperties(locale string) map[string]string {
	path := "messages/messages_" + locale + ".properties"
	raw, err := messagesFS.ReadFile(path)
	if err != nil {
		// 资源缺失是合法情况（如 locale=zh 没有对应文件），返回空表交给回退链处理
		return nil
	}

	out := make(map[string]string, 64)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// ResolveLocale 从请求头解析语言环境。
//
// 复刻 Python 端 _resolve_locale 的规则：
//   - 优先 Accept-Language，Content-Language 兜底
//   - 取首个语言项（按 , 分割），剥离 q 值（按 ; 分割）
//   - '-' 规范化为 '_'，再取语言前缀（按 _ 分割）小写匹配
//   - zh → zh_CN，en → en_US，其余 → zh_CN
func ResolveLocale(acceptLanguage, contentLanguage string) string {
	raw := acceptLanguage
	if raw == "" {
		raw = contentLanguage
	}
	if raw == "" {
		return DefaultLocale
	}
	primary := strings.TrimSpace(strings.Split(strings.Split(raw, ",")[0], ";")[0])
	primary = strings.ReplaceAll(primary, "-", "_")
	prefix := strings.ToLower(strings.Split(primary, "_")[0])
	switch prefix {
	case "zh":
		return LocaleZhCN
	case "en":
		return LocaleEnUS
	default:
		return DefaultLocale
	}
}

// KnownKeys 返回指定 locale 已加载的消息键数量，用于启动自检与测试。
func KnownKeys(locale string) int { return len(messagesFor(locale)) }
