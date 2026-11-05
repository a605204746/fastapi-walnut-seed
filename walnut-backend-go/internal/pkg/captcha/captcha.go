// Package captcha 生成图形验证码，对齐 Python 端 module_web/auth/captcha.py。
//
// 两种模式（由 CAPTCHA_TYPE 决定）：
//   - math：形如 "3+5=?" 的算式，答案是计算结果
//   - char：N 位字母数字随机串（N=CAPTCHA_CHAR_LENGTH），答案就是该串本身
//     （**大小写不敏感**比对）
//
// 图片为 160×60 PNG，返回 **不带 data URI 前缀**的 base64（前端自行拼接
// `data:image/png;base64,`）——这与 Python 端一致，多带前缀会让前端图片裂开。
//
// # 为什么不再用 base64Captcha 的 driver 画图
//
// 早期实现用 base64Captcha 渲染，存在两个硬伤：
//  1. **char 模式会 panic**：它按名字加载字体，而旧配置里写了两个该库并不自带的
//     字体（chromohv-bold-z84.ttf / Apothem.ttf），loadFontByName 找不到就 panic。
//  2. **图片发虚**：库在 160×60 的极小画布上直接光栅化，前端又按 150×40 的
//     CSS 尺寸显示，高分屏（DPR≥2）一拉伸就糊。
//
// 现改为**自绘**：用 Go 官方 gobold 粗体字（清晰、无外部字体依赖），先在 2× 分辨率
// （320×120）上绘制，再用 CatmullRom 高质量降采样回 160×60，得到带抗锯齿的锐利图，
// 前端放大也不糊。答案仍必须存进项目自己的 Redis 键体系（见 service.GetCaptcha），
// 因此这里只管画图，不碰任何 store。
package captcha

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"strings"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/math/fixed"
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
// 注意比对是**大小写不敏感**的（见 EqualsIgnoreCase），故字符集含大小写只是增加
// 视觉熵，不会让用户因大小写输错而失败。
const charSource = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Result 验证码生成结果。
type Result struct {
	// Text 展示文本（math 模式是算式，char 模式是随机串），仅用于日志/调试
	Text string
	// Answer 正确答案，存入 Redis 供校验
	Answer string
	// ImageBase64 PNG 图片的 base64（**不含** data URI 前缀）
	ImageBase64 string
}

// goboldFont 是全局共享的 Go Bold 字体解析结果，只解析一次。
// gobold 数据是编译进二进制的常量，无外部文件依赖，不会像 base64Captcha 那样
// 因缺失字体文件而 panic。
var goboldFont = func() *truetype.Font {
	f, err := freetype.ParseFont(gobold.TTF)
	if err != nil {
		// gobold.TTF 是官方内嵌的合法字体，解析失败属编译期就能暴露的严重问题
		panic(fmt.Sprintf("解析内嵌 gobold 字体失败: %v", err))
	}
	return f
}()

// GenerateMath 生成算式验证码。
//
// numberLength 对应 CAPTCHA_NUMBER_LENGTH：参与运算的数的位数上限，
// 与 Python 端 `bound = 10**max(number_length, 1)` 语义一致
// （numberLength=1 → 两个 0-9 的数；=2 → 两个 0-99 的数）。
func GenerateMath(numberLength int) (*Result, error) {
	if numberLength < 1 {
		numberLength = 1
	}
	bound := pow10(numberLength)
	a := rand.Intn(bound)
	b := rand.Intn(bound)

	var content, answer string
	if rand.Intn(2) == 0 {
		content = fmt.Sprintf("%d+%d=?", a, b)
		answer = fmt.Sprintf("%d", a+b)
	} else {
		if a < b { // 保证结果非负，与 Python 端一致
			a, b = b, a
		}
		content = fmt.Sprintf("%d-%d=?", a, b)
		answer = fmt.Sprintf("%d", a-b)
	}
	return &Result{Text: content, Answer: answer, ImageBase64: render(content)}, nil
}

// GenerateChar 生成字符验证码。
//
// charLength 对应 CAPTCHA_CHAR_LENGTH（默认 1）。
func GenerateChar(charLength int) (*Result, error) {
	if charLength < 1 {
		charLength = 1
	}
	var sb strings.Builder
	sb.Grow(charLength)
	for i := 0; i < charLength; i++ {
		sb.WriteByte(charSource[rand.Intn(len(charSource))])
	}
	content := sb.String()
	return &Result{Text: content, Answer: content, ImageBase64: render(content)}, nil
}

// Generate 按模式分派。未知模式回退到 math（math 是配置默认值，
// 回退到它更符合"配置写错时仍可用"的预期）。
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

// ==================== 绘图内部实现 ====================

// supersample 超采样倍数。先在 supersample×supersample 的大图上绘制，
// 再高质量降采样回目标尺寸，得到锐利且带抗锯齿的成图。
const supersample = 2

// textColors 正文可选深色，保证在浅色背景上有足够对比度。
var textColors = []color.RGBA{
	{25, 90, 180, 255},  // 蓝
	{180, 40, 40, 255},  // 红
	{30, 130, 60, 255},  // 绿
	{120, 60, 170, 255}, // 紫
	{200, 110, 20, 255}, // 橙
}

// randLightBG 生成浅色背景，取值区间与 Python 端 _BG_RANGE 一致（230-250）。
func randLightBG() color.RGBA {
	r := uint8(230 + rand.Intn(21))
	g := uint8(230 + rand.Intn(21))
	b := uint8(230 + rand.Intn(21))
	return color.RGBA{r, g, b, 255}
}

// render 把文本渲染成 PNG base64（不含 data URI 前缀）。
func render(text string) string {
	W, H := Width*supersample, Height*supersample
	big := image.NewNRGBA(image.Rect(0, 0, W, H))
	draw.Draw(big, big.Bounds(), &image.Uniform{randLightBG()}, image.Point{}, draw.Src)

	drawNoise(big, W, H)
	drawText(big, text, W, H)

	// 高质量降采样回目标尺寸：CatmullRom 是双三次插值，边缘平滑不发虚。
	out := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	xdraw.CatmullRom.Scale(out, out.Bounds(), big, big.Bounds(), xdraw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		// png.Encode 对 NRGBA 不会失败，兜底返回空图不 panic
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// drawText 用 gobold 粗体把文本**整体水平居中、垂直居中**绘制。
//
// 字号随文本长度自适应：字符越多字号越小，始终留边距、不越界。
func drawText(img *image.NRGBA, text string, W, H int) {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return
	}

	// 目标：文本总宽不超过画布宽度的 88%。先按高度估一个字号，再按实测宽度收敛。
	fontSize := float64(H) * 0.62
	const maxWidthRatio = 0.88
	for i := 0; i < 6; i++ {
		face := truetype.NewFace(goboldFont, &truetype.Options{Size: fontSize, DPI: 72})
		w := font.MeasureString(face, text).Round()
		face.Close()
		if w <= int(float64(W)*maxWidthRatio) {
			break
		}
		fontSize *= 0.9
	}

	face := truetype.NewFace(goboldFont, &truetype.Options{Size: fontSize, DPI: 72})
	defer face.Close()

	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(goboldFont)
	c.SetFontSize(fontSize)
	c.SetDst(img)
	c.SetClip(img.Bounds())
	c.SetHinting(font.HintingFull)
	c.SetSrc(image.NewUniform(textColors[rand.Intn(len(textColors))]))

	textWidth := font.MeasureString(face, text).Round()
	x := (W - textWidth) / 2
	// 垂直居中：基线约在画布中线偏下 0.35 字号处（覆盖升部/降部的视觉中心）。
	y := H/2 + int(fontSize*0.35)

	if _, err := c.DrawString(text, fixed.P(x, y)); err != nil {
		return
	}
}

// drawNoise 画轻量干扰：几条淡色直线 + 少量浅色噪点，强度低于旧库
// （旧库干扰过强也是"看不清"的一部分）。
func drawNoise(img *image.NRGBA, W, H int) {
	lineColor := color.RGBA{
		uint8(150 + rand.Intn(60)),
		uint8(150 + rand.Intn(60)),
		uint8(150 + rand.Intn(60)),
		255,
	}
	for i := 0; i < 3; i++ {
		x1, y1 := rand.Intn(W), rand.Intn(H)
		x2, y2 := rand.Intn(W), rand.Intn(H)
		drawLine(img, x1, y1, x2, y2, lineColor)
	}
	for i := 0; i < 40; i++ {
		img.Set(rand.Intn(W), rand.Intn(H), lineColor)
	}
}

// drawLine Bresenham 画线（含 2px 粗细，超采样后仍清晰）。
func drawLine(img *image.NRGBA, x1, y1, x2, y2 int, c color.Color) {
	dx, dy := abs(x2-x1), abs(y2-y1)
	sx, sy := 1, 1
	if x1 >= x2 {
		sx = -1
	}
	if y1 >= y2 {
		sy = -1
	}
	err := dx - dy
	for {
		img.Set(x1, y1, c)
		img.Set(x1+1, y1, c)
		img.Set(x1, y1+1, c)
		img.Set(x1+1, y1+1, c)
		if x1 == x2 && y1 == y2 {
			return
		}
		e2 := err * 2
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func pow10(n int) int {
	p := 1
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}
