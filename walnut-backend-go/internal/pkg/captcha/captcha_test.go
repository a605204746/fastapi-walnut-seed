package captcha

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
)

func decodePNG(t *testing.T, b64 string) image.Image {
	t.Helper()
	if strings.Contains(b64, "data:") {
		t.Fatal("base64 不应带 data URI 前缀（前端自行拼接，带前缀会裂图）")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("PNG 解码失败: %v", err)
	}
	return img
}

func TestGenerateMathContract(t *testing.T) {
	for i := 0; i < 200; i++ {
		r, err := GenerateMath(1)
		if err != nil {
			t.Fatalf("GenerateMath 失败: %v", err)
		}
		if !strings.HasSuffix(r.Text, "=?") {
			t.Fatalf("算式格式错误: %q", r.Text)
		}
		expr := strings.TrimSuffix(r.Text, "=?")
		var a, b, want int
		if idx := strings.Index(expr, "+"); idx >= 0 {
			a = atoi(t, expr[:idx])
			b = atoi(t, expr[idx+1:])
			want = a + b
		} else if idx := strings.Index(expr, "-"); idx >= 0 {
			a = atoi(t, expr[:idx])
			b = atoi(t, expr[idx+1:])
			want = a - b
		} else {
			t.Fatalf("算式缺少运算符: %q", r.Text)
		}
		if a < 0 || a > 9 || b < 0 || b > 9 {
			t.Fatalf("numberLength=1 时运算数应在 0-9: %q", r.Text)
		}
		if want < 0 {
			t.Fatalf("减法结果不应为负: %q", r.Text)
		}
		if r.Answer != itoa(want) {
			t.Fatalf("答案不匹配: text=%q answer=%q want=%d", r.Text, r.Answer, want)
		}
	}
}

func TestGenerateMathNumberLength(t *testing.T) {
	for i := 0; i < 200; i++ {
		r, err := GenerateMath(2)
		if err != nil {
			t.Fatalf("GenerateMath 失败: %v", err)
		}
		expr := strings.TrimSuffix(r.Text, "=?")
		parts := splitOp(expr)
		for _, p := range parts {
			v := atoi(t, p)
			if v < 0 || v > 99 {
				t.Fatalf("numberLength=2 时运算数应在 0-99: %q", r.Text)
			}
		}
	}
}

func TestGenerateCharLength(t *testing.T) {
	for _, n := range []int{1, 2, 4} {
		r, err := GenerateChar(n)
		if err != nil {
			t.Fatalf("GenerateChar(%d) 失败: %v", n, err)
		}
		if len(r.Answer) != n {
			t.Fatalf("GenerateChar(%d) 长度 = %d", n, len(r.Answer))
		}
		if r.Text != r.Answer {
			t.Fatalf("char 模式 Text 应等于 Answer: %q vs %q", r.Text, r.Answer)
		}
		if !strings.ContainsAny(r.Answer, charSource) {
			t.Fatalf("字符超出字符集: %q", r.Answer)
		}
	}
	// 下限保护：0/负数按 1 处理
	if r, _ := GenerateChar(0); len(r.Answer) != 1 {
		t.Fatalf("GenerateChar(0) 应下限保护为 1 位, got %d", len(r.Answer))
	}
}

func TestImageIsPNGWithExpectedSize(t *testing.T) {
	for _, mode := range []string{TypeMath, TypeChar} {
		r, err := Generate(mode, 1, 1)
		if err != nil {
			t.Fatalf("Generate(%s) 失败: %v", mode, err)
		}
		img := decodePNG(t, r.ImageBase64)
		b := img.Bounds()
		if b.Dx() != Width || b.Dy() != Height {
			t.Fatalf("图片尺寸 = %dx%d, 期望 %dx%d", b.Dx(), b.Dy(), Width, Height)
		}
	}
}

func TestEqualsIgnoreCase(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"ab12", "AB12", true},
		{" 5 ", "5", true},
		{"5", "6", false},
		{"", "", true},
	}
	for _, c := range cases {
		if got := EqualsIgnoreCase(c.a, c.b); got != c.want {
			t.Errorf("EqualsIgnoreCase(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestGenerateUnknownModeFallsBackToMath(t *testing.T) {
	r, err := Generate("whatever", 1, 1)
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	if !strings.HasSuffix(r.Text, "=?") {
		t.Fatalf("未知模式应回退 math: %q", r.Text)
	}
}

// ---- helpers ----

func atoi(t *testing.T, s string) int {
	t.Helper()
	var v int
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("非数字字符: %q in %q", c, s)
		}
		v = v*10 + int(c-'0')
	}
	return v
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func splitOp(expr string) []string {
	if idx := strings.Index(expr, "+"); idx >= 0 {
		return []string{expr[:idx], expr[idx+1:]}
	}
	idx := strings.Index(expr, "-")
	return []string{expr[:idx], expr[idx+1:]}
}
