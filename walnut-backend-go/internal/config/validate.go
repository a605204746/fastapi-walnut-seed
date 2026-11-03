package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// RuoYi 生态公开的出厂默认 RSA 密钥对前缀（base64 DER）。
// 与 Python 端 init_app.py 的同名常量逐字一致：命中即视为已知坏密钥。
const (
	knownBadRSAPublicPrefix  = "MFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAKoR8mX0rGKLqzcWmOzbfj64K8ZIgOdHnzkXSOVOZbFu"
	knownBadRSAPrivatePrefix = "MIIBVAIBADANBgkqhkiG9w0BAQEFAASCAT4wggE6AgEAAoEAmc3CuPiGL"
)

// rsaMinKeySize RSA 密钥最小可接受位数（低于该长度视为无效）。
const rsaMinKeySize = 1024

// secretMinBytes JWT 签名密钥最小字节数。
const secretMinBytes = 32

// Warning 收集校验过程中产生的非致命告警，由调用方统一写日志。
// 之所以不在此处直接打日志：config 包不应依赖日志实现（避免循环依赖），
// 且启动期告警需要与 banner 一起集中输出。
type Warning struct {
	Code    string
	Message string
}

// ValidationResult 校验结果。
type ValidationResult struct {
	Warnings []Warning
}

func (r *ValidationResult) warn(code, format string, args ...any) {
	r.Warnings = append(r.Warnings, Warning{Code: code, Message: fmt.Sprintf(format, args...)})
}

// ValidateSecurity 复刻 Python 端 validate_security_settings()：
// 必须在服务对外可用前、且在注册加解密中间件之前执行（因为校验可能把加解密关掉）。
//
//   - SECRET_KEY：prod 下为空 / 含 change-me / 长度 <32 字节 → 返回 error 拒绝启动；dev 仅告警
//   - RSA 密钥：prod 下命中 RuoYi 出厂密钥 → 返回 error；
//     密钥为空或无效（任何环境）→ 将 APIDecryptEnabled 置 false 并告警，安全降级为明文。
//     **绝不带着假密钥运行。**
func (c *Config) ValidateSecurity() (*ValidationResult, error) {
	res := &ValidationResult{}
	isProd := c.Environment.IsProd()

	// ---- JWT 签名密钥 ----
	weak := c.SecretKey == "" ||
		strings.Contains(c.SecretKey, "change-me") ||
		len(c.SecretKey) < secretMinBytes
	if weak {
		if isProd {
			return nil, errors.New("SECRET_KEY 未配置或不安全（为空/含 change-me/长度不足 32 字节），生产环境拒绝启动")
		}
		res.warn("WEAK_SECRET_KEY", "⚠️ SECRET_KEY 为开发占位值（为空/含 change-me/长度不足 32 字节），生产环境必须更换")
	}

	// ---- 接口加解密 RSA 密钥对 ----
	pub, priv := c.APIDecryptPublicKey, c.APIDecryptPrivateKey
	knownBad := strings.HasPrefix(pub, knownBadRSAPublicPrefix) ||
		strings.HasPrefix(priv, knownBadRSAPrivatePrefix)
	if knownBad && isProd {
		return nil, errors.New("检测到 RuoYi 生态公开的默认 RSA 密钥，公开密钥严禁用于生产环境，请重新生成密钥对")
	}

	if !c.APIDecryptEnabled {
		return res, nil
	}
	if pub == "" || priv == "" {
		c.APIDecryptEnabled = false
		c.APIDecryptDisabledReason = "RSA 密钥未配置"
		res.warn("API_DECRYPT_DISABLED", "⚠️ 接口加解密 RSA 密钥未配置，已自动停用接口加解密（降级为明文）")
		return res, nil
	}
	if err := checkRSAKeyPair(pub, priv); err != nil {
		c.APIDecryptEnabled = false
		if knownBad {
			c.APIDecryptDisabledReason = "命中已知公开密钥"
		} else {
			c.APIDecryptDisabledReason = err.Error()
		}
		res.warn("API_DECRYPT_DISABLED", "⚠️ 接口加解密 RSA 密钥无效（%s），已自动停用接口加解密（降级为明文）", c.APIDecryptDisabledReason)
	}
	return res, nil
}

// checkRSAKeyPair 校验两把密钥能否按 base64 DER 加载且位数达标。
// 公钥为 X.509/SubjectPublicKeyInfo，私钥为 PKCS#8 —— 与 Python 端
// load_der_public_key / load_der_private_key 一致。
func checkRSAKeyPair(pubB64, privB64 string) error {
	pubDER, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil {
		return fmt.Errorf("公钥 Base64 不可解析: %w", err)
	}
	pubAny, err := x509.ParsePKIXPublicKey(pubDER)
	if err != nil {
		return fmt.Errorf("公钥非 X.509/SPKI DER: %w", err)
	}
	pubBits, err := rsaBitLen(pubAny)
	if err != nil {
		return fmt.Errorf("公钥: %w", err)
	}
	if pubBits < rsaMinKeySize {
		return fmt.Errorf("公钥长度 %d 位 < %d 位", pubBits, rsaMinKeySize)
	}

	privDER, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privB64))
	if err != nil {
		return fmt.Errorf("私钥 Base64 不可解析: %w", err)
	}
	privAny, err := x509.ParsePKCS8PrivateKey(privDER)
	if err != nil {
		return fmt.Errorf("私钥非 PKCS#8 DER: %w", err)
	}
	privBits, err := rsaBitLen(privAny)
	if err != nil {
		return fmt.Errorf("私钥: %w", err)
	}
	if privBits < rsaMinKeySize {
		return fmt.Errorf("私钥长度 %d 位 < %d 位", privBits, rsaMinKeySize)
	}
	return nil
}

// rsaBitLen 从 x509 解析结果取 RSA 模数位数。
// ParsePKIXPublicKey 返回 *rsa.PublicKey，ParsePKCS8PrivateKey 返回 *rsa.PrivateKey；
// 其他算法（ECDSA/Ed25519）一律视为无效，因为对端协议是 RSA/PKCS1v15。
func rsaBitLen(key any) (int, error) {
	switch k := key.(type) {
	case *rsa.PublicKey:
		return k.N.BitLen(), nil
	case *rsa.PrivateKey:
		return k.N.BitLen(), nil
	default:
		return 0, fmt.Errorf("不是 RSA 密钥（实际类型 %T）", key)
	}
}
