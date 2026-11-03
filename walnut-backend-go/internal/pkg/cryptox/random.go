package cryptox

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"unicode/utf8"
)

// randomAlphabet 一次性 AES 密钥的字符集，与 Python 端 random_string 逐字一致。
//
// ⚠️ 只有小写字母 + 数字（36 个字符），**不含大写与符号**。
// 前端 crypto-js 侧按同样字符集生成，改动会导致两端密钥空间不一致。
const randomAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// DefaultAESKeyLength 一次性 AES 密钥长度。
// 对应 Python 端 random_string(32) 的默认值，同时也是 AES-256 的密钥长度。
const DefaultAESKeyLength = 32

// RandomString 生成 n 位随机字符串（用于一次性 AES 密钥）。
// 对应 Python 端 random_string。
//
// 必须用 crypto/rand 而非 math/rand：这是加密密钥，可预测等于没有加密。
// 用 rand.Int 做取模而非「取随机字节再 %36」——后者会引入模偏差
// （256 不是 36 的整数倍，前 4 个字符出现概率偏高），虽然对 32 位密钥
// 的实际熵损失很小，但没有理由接受它。
func RandomString(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("长度必须为正数，当前 %d", n)
	}
	max := big.NewInt(int64(len(randomAlphabet)))
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("生成随机密钥失败: %w", err)
		}
		out[i] = randomAlphabet[idx.Int64()]
	}
	return string(out), nil
}

// RandomAESKey 生成一次性 AES 密钥（32 位，AES-256）。
func RandomAESKey() (string, error) {
	return RandomString(DefaultAESKeyLength)
}

// EncodeBase64 / DecodeBase64 对应 Python 端 encrypt_by_base64 / decrypt_by_base64。
//
// 协议里被 RSA 包裹的是 **Base64(AES密钥字符串)**，不是裸密钥字节：
//
//	前端 → 后端：encrypt-key = RSA_encrypt(base64(aesKey), 后端公钥)
//	后端 → 前端：encrypt-key = RSA_encrypt(base64(aesKey), 前端公钥)
//
// 漏掉这层 Base64 是最常见的对接失败原因——RSA 解出来的是 Base64 文本，
// 直接当密钥用会因长度不对（44 而非 32）而报「AES秘钥长度要求为16位、24位、32位」。

// EncodeBase64Str 把字符串编码为 Base64（对应 encrypt_by_base64）。
func EncodeBase64Str(data string) string {
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// DecodeBase64Str 把 Base64 解码回字符串（对应 decrypt_by_base64）。
//
// 解码结果必须是合法 UTF-8：AES 密钥是 random_string 生成的 ASCII，
// 若解出乱码说明密钥不配对或密文被篡改，此时应报错而不是拿着垃圾当密钥。
func DecodeBase64Str(data string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("Base64 解码失败: %w", err)
	}
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("Base64 解码结果不是合法 UTF-8（密钥不配对或密文被篡改）")
	}
	return string(raw), nil
}
