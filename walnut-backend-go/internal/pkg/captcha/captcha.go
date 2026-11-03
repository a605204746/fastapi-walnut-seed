// Package captcha 生成图形验证码，对齐 Python 端 module_web/auth/captcha.py。
//
// 两种模式（由 CAPTCHA_TYPE 决定）：
//   - math：形如 "3+5=?" 的算式，答案是计算结果
//   - char：4 位字母数字随机串，答案就是该串本身（**大小写不敏感**比对）
//
// 图片为 160×60 PNG，返回 **不带 data URI 前缀**的 base64（前端自行拼接
// `data:image/png;base64,`）——这与 Python 端一致，多带前缀会让前端图片裂开。
//
// ⚠️ 刻意**不使用** base64Captcha 自带的 store：答案必须存进项目自己的 Redis
// 键体系（`{REDIS_KEY_PREFIX}:global:captcha_codes:{uuid}`，TTL=CAPTCHA_EXPIRE_SECONDS），
// 否则三端的验证码缓存互不相通，且键名/前缀/过期策略都会与另两端不一致。
// 故这里只用它的 driver（画图），存储走 cachex。
package captcha

import (
	"fmt"
	"strings"

	base64captcha "github.com/mojocn/base64Captcha"
)

// 图片尺寸，与 Python 端 _WIDTH/_HEIGHT 一致。
const (
	Width  = 160
	Height = 60
)

// 模式取值，对应配置项 CAPTCHA_TYPE。
const (
	TypeMath = "math"
	TypeChar = "char"
)

// charSource 字符验证码的字符集。
//
// 与 Python 端 `string.ascii_letters + string.digits` 逐字一致（62 个字符）。
// 注意比对是**大小写不敏感**的（Python 用 equals_ignore_case），
// 故字符集含大小写只是增加视觉熵，不会让用户因大小写输错而失败。
const charSource = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// 干扰选项：base64Captcha 的 ShowLineOptions 是位掩码。
// Python 端画 4 条干扰线 + 3 个干扰圈，这里取「曲线 + 虚线」组合达到相近的干扰强度。
const noiseOptions = base64captcha.OptionShowSlimeLine | base64captcha.OptionShowHollowLine

// Result 验证码生成结果。
type Result struct {
	// Text 展示文本（math 模式是算式，char 模式是随机串），仅用于日志/调试
	Text string
	// Answer 正确答案，存入 Redis 供校验
	Answer string
	// ImageBase64 PNG 图片的 base64（**不含** data URI 前缀）
	ImageBase64 string
}

// GenerateMath 生成算式验证码。
//
// numberLength 对应 CAPTCHA_NUMBER_LENGTH：参与运算的数的位数上限，
// 与 Python 端 `bound = 10**max(number_length, 1)` 语义一致
// （numberLength=1 → 两个 0-9 的数；=2 → 两个 0-99 的数）。
func GenerateMath(numberLength int) (*Result, error) {
	if numberLength < 1 {
		numberLength = 1
	}
	driver := base64captcha.NewDriverMath(Height, Width, 6, noiseOptions, nil, nil)
	_, content, answer := driver.GenerateIdQuestionAnswer()

	item, err := driver.DrawCaptcha(content)
	if err != nil {
		return nil, fmt.Errorf("绘制算式验证码失败: %w", err)
	}
	return &Result{Text: content, Answer: answer, ImageBase64: item.EncodeB64string()}, nil
}

// GenerateChar 生成字符验证码。
//
// charLength 对应 CAPTCHA_CHAR_LENGTH（默认 4）。
func GenerateChar(charLength int) (*Result, error) {
	if charLength < 1 {
		charLength = 4
	}
	// 固定使用 charSource，不让库用它自己的默认字符集——
	// 默认集可能不含大写或含易混淆字符，与 Python 端不一致
	driver := base64captcha.NewDriverString(
		Height, Width, 6, noiseOptions, charLength, charSource, nil,
		[]string{"wqy-microhei.ttc", "chromohv-bold-z84.ttf", "Apothem.ttf"},
	)
	_, content, answer := driver.GenerateIdQuestionAnswer()

	item, err := driver.DrawCaptcha(content)
	if err != nil {
		return nil, fmt.Errorf("绘制字符验证码失败: %w", err)
	}
	return &Result{Text: content, Answer: answer, ImageBase64: item.EncodeB64string()}, nil
}

// Generate 按模式分派。未知模式回退到 math（与 Python 端 if/else 的 else 分支相反，
// 但 math 是配置默认值，回退到它更符合"配置写错时仍可用"的预期）。
func Generate(mode string, numberLength, charLength int) (*Result, error) {
	if strings.EqualFold(strings.TrimSpace(mode), TypeChar) {
		return GenerateChar(charLength)
	}
	return GenerateMath(numberLength)
}

// EqualsIgnoreCase 大小写不敏感比对，对应 Python 端 string_util.equals_ignore_case。
//
// ⚠️ 必须忽略大小写：Python 端就是这么比的，若 Go 改成区分大小写，
// 用 char 模式时用户输入 "ab12" 而答案是 "AB12" 就会失败——
// 同一个前端在两个后端上表现不同，属于契约偏差。
func EqualsIgnoreCase(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// 验证码的 uuid 用 idgen.UUID()（crypto/rand 生成的 32 位无横线十六进制串），
// 与 Python 端 uuid.uuid4().hex 等价。
//
// ⚠️ 不要用 math/rand 生成它：uuid 是验证码在 Redis 里的**唯一凭据**，
// 可预测就意味着攻击者能枚举/复用他人的验证码，绕过整个验证码机制。
