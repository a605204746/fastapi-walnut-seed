package cryptox

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// RSA 密钥最小可接受位数。
//
// ⚠️ 1024 是**前端 jsencrypt 的兼容上限**决定的，不是安全推荐值。
// 已实测 Go 1.24 / 1.25 / 1.26 均可正常对 1024 位密钥做 PKCS1v15 运算
// （见实施计划 §9.2），故沿用与另两端相同的开发密钥长度。
// 生产环境应使用 2048 位（需同步重新生成前端密钥对）。
const RSAMinKeySize = 1024

// PEM 头尾，用于容错剥离。
// 配置里理论上只放裸 Base64 DER，但运维手滑粘进 PEM 是常见事故，
// 与其报一个看不懂的解析错误，不如直接容错。
var pemArmor = []string{
	"-----BEGIN PUBLIC KEY-----", "-----END PUBLIC KEY-----",
	"-----BEGIN PRIVATE KEY-----", "-----END PRIVATE KEY-----",
	"-----BEGIN RSA PRIVATE KEY-----", "-----END RSA PRIVATE KEY-----",
}

// stripPEM 去掉 PEM 头尾与所有空白，返回纯 Base64 字符串。
func stripPEM(s string) string {
	s = strings.TrimSpace(s)
	for _, a := range pemArmor {
		s = strings.ReplaceAll(s, a, "")
	}
	// Base64 里的换行/空格（PEM 每 64 字符折行）必须清掉，否则解码失败
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', ' ', '\t':
			return -1
		}
		return r
	}, s)
}

// ParsePublicKey 解析 Base64 DER 的 X.509/SubjectPublicKeyInfo 公钥。
// 对应 Python 端 load_der_public_key(base64.b64decode(key))。
func ParsePublicKey(b64 string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(stripPEM(b64))
	if err != nil {
		return nil, fmt.Errorf("公钥 Base64 解码失败: %w", err)
	}
	pubAny, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("公钥不是合法的 X.509/SPKI DER: %w", err)
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("公钥不是 RSA（实际类型 %T）", pubAny)
	}
	if bits := pub.N.BitLen(); bits < RSAMinKeySize {
		return nil, fmt.Errorf("公钥长度 %d 位 < 最小要求 %d 位", bits, RSAMinKeySize)
	}
	return pub, nil
}

// ParsePrivateKey 解析 Base64 DER 的 PKCS#8 私钥。
// 对应 Python 端 load_der_private_key(base64.b64decode(key))。
func ParsePrivateKey(b64 string) (*rsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(stripPEM(b64))
	if err != nil {
		return nil, fmt.Errorf("私钥 Base64 解码失败: %w", err)
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("私钥不是合法的 PKCS#8 DER: %w", err)
	}
	priv, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("私钥不是 RSA（实际类型 %T）", keyAny)
	}
	if bits := priv.N.BitLen(); bits < RSAMinKeySize {
		return nil, fmt.Errorf("私钥长度 %d 位 < 最小要求 %d 位", bits, RSAMinKeySize)
	}
	return priv, nil
}

// EncodePublicKey / EncodePrivateKey 把密钥编码回配置里用的 Base64 DER 形式。
func EncodePublicKey(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

func EncodePrivateKey(priv *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// GenerateKeyPair 生成 RSA 密钥对并返回 Base64 DER 形式，供 `genkey` 子命令使用。
func GenerateKeyPair(bits int) (publicB64, privateB64 string, err error) {
	if bits < RSAMinKeySize {
		return "", "", fmt.Errorf("密钥长度 %d 位 < 最小要求 %d 位", bits, RSAMinKeySize)
	}
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", fmt.Errorf("生成 RSA 密钥失败: %w", err)
	}
	pub, err := EncodePublicKey(&priv.PublicKey)
	if err != nil {
		return "", "", err
	}
	privB64, err := EncodePrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	return pub, privB64, nil
}

// EncryptByRSA 用 RSA/PKCS1v15 分段加密并返回 Base64。
// 对应 Python 端 encrypt_by_rsa。
//
// 分段参数：max_chunk = key_size//8 - 11。这个 -11 是 PKCS1v15 的填充开销特征
// （OAEP-SHA1 是 -42），因此它本身就证明了对端用的是 PKCS1v15。
//
// ⚠️ 对 **UTF-8 字节流**分段，会把多字节字符从中间切断——这是刻意且正确的：
// 解密侧先拼接全部明文字节、最后才转字符串，故切断不影响结果。
// 若改成按字符分段，密文长度会与 Python 端不同，互通直接失败。
func EncryptByRSA(data string, pub *rsa.PublicKey) (string, error) {
	if pub == nil {
		return "", errors.New("RSA 公钥为空")
	}
	maxChunk := pub.Size() - 11
	if maxChunk <= 0 {
		return "", fmt.Errorf("RSA 密钥过短（%d 位），无法容纳 PKCS1v15 填充", pub.N.BitLen())
	}

	raw := []byte(data)
	var out []byte

	if len(raw) == 0 {
		// Python: chunks = [raw[i:i+max_chunk] for i in range(0, len(raw), max_chunk)] or [b""]
		// 空输入时列表推导得到 []，`or [b""]` 兜底成"一个空块"，故仍要加密一次
		enc, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte{}) //nolint:staticcheck // 与前端 jsencrypt / Python 端契约固定为 PKCS#1 v1.5，不可换 OAEP
		if err != nil {
			return "", fmt.Errorf("RSA 加密失败: %w", err)
		}
		out = enc
	} else {
		for i := 0; i < len(raw); i += maxChunk {
			end := min(i+maxChunk, len(raw))
			enc, err := rsa.EncryptPKCS1v15(rand.Reader, pub, raw[i:end]) //nolint:staticcheck // 与前端 jsencrypt / Python 端契约固定为 PKCS#1 v1.5，不可换 OAEP
			if err != nil {
				return "", fmt.Errorf("RSA 分段加密失败（第 %d 块）: %w", i/maxChunk+1, err)
			}
			out = append(out, enc...)
		}
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptByRSA 解 Base64 的 RSA/PKCS1v15 分段密文。
// 对应 Python 端 decrypt_by_rsa。
//
// 分段参数：chunk_size = key_size//8（密文块长恒等于密钥字节长，与填充无关）。
func DecryptByRSA(b64 string, priv *rsa.PrivateKey) (string, error) {
	if priv == nil {
		return "", errors.New("RSA 私钥为空")
	}
	chunkSize := priv.Size()
	raw, err := base64.StdEncoding.DecodeString(stripPEM(b64))
	if err != nil {
		return "", fmt.Errorf("Base64 解码失败: %w", err)
	}
	if len(raw) == 0 {
		return "", errors.New("密文为空")
	}
	if len(raw)%chunkSize != 0 {
		return "", fmt.Errorf("密文长度 %d 非密钥字节长 %d 的整数倍（密钥不匹配或密文被截断）",
			len(raw), chunkSize)
	}

	// ⚠️ 必须先把所有分段解密结果**拼成字节**，最后才 string()。
	// 逐块 string() 再拼接会在多字节字符被切断处产生非法 UTF-8。
	//
	// 另注：DecryptPKCS1v15 的第一个参数是 io.Reader（用于 RSA blinding 抗时序攻击），
	// 不是 (priv, ciphertext) 两参形式——传错会编译失败。
	var out []byte
	for i := 0; i < len(raw); i += chunkSize {
		dec, err := rsa.DecryptPKCS1v15(rand.Reader, priv, raw[i:i+chunkSize]) //nolint:staticcheck // 与前端 jsencrypt / Python 端契约固定为 PKCS#1 v1.5，不可换 OAEP
		if err != nil {
			return "", fmt.Errorf("RSA 分段解密失败（第 %d 块）: %w", i/chunkSize+1, err)
		}
		out = append(out, dec...)
	}
	return string(out), nil
}
