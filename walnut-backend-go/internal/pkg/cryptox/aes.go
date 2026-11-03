// Package cryptox 实现与 Python 端 app/core/encrypt.py 二进制级一致的加解密。
//
// 算法固定为：
//
//	AES/ECB/PKCS7Padding（128 位块）+ Base64       —— 报文体
//	RSA/ECB/PKCS1Padding（即 PKCS1v15）+ Base64    —— 包裹一次性 AES 密钥
//
// 对齐依据：docs/go/00-Go后端实施计划.md §9.2，已通过 Python↔Go **双向互通测试**，
// 其中 AES 密文经逐字节比对 21/21 完全相同（AES-ECB 是确定性算法，无随机填充，
// 因此"密文相同"是比"能解开"强得多的判据）。
//
// ⚠️ 移植时最容易错的三点：
//  1. Go 标准库**没有 ECB 模式**，必须手写逐块加解密
//  2. PKCS7 在明文长度恰为块长整数倍时**要补满一整块**（16B 明文 → 32B 密文）
//  3. RSA 分段参数 `-11` 是 PKCS1v15 的特征（OAEP 是 `-42`），用错填充算法就解不开
package cryptox

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"fmt"
)

// AES 密钥合法长度（字节）。对应 Python 端 _check_aes_key 的 (16, 24, 32)。
var validAESKeyLengths = map[int]struct{}{16: {}, 24: {}, 32: {}}

// 错误文案与 Python 端 _check_aes_key 逐字一致，保证两端报错信息相同。
const errAESKeyLength = "AES秘钥长度要求为16位、24位、32位"

// checkAESKey 校验密钥长度。
func checkAESKey(password string) ([]byte, error) {
	key := []byte(password)
	if _, ok := validAESKeyLengths[len(key)]; !ok {
		return nil, fmt.Errorf("%s（当前 %d）", errAESKeyLength, len(key))
	}
	return key, nil
}

// pkcs7Pad 补齐到 blockSize 的整数倍。
//
// ⚠️ PKCS7 的语义是**总会补**：明文长度恰为块长整数倍时补满一整块。
// 若写成"整除就不补"，密文会比 Python 端少 16 字节，对端解出来是多一块垃圾。
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

// pkcs7Unpad 去除填充并校验其合法性。
//
// 严格校验每一个填充字节：只检查最后一个字节的话，密钥不匹配时
// （解出的是乱码）有 1/256 概率"看起来合法"，会返回一段垃圾数据而不是报错，
// 那种故障极难定位。
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("密文为空")
	}
	if len(data)%blockSize != 0 {
		return nil, fmt.Errorf("密文长度 %d 非块长 %d 的整数倍", len(data), blockSize)
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("非法 PKCS7 填充值 %d（块长 %d）", padding, blockSize)
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, fmt.Errorf("PKCS7 填充字节不一致（密钥或密文有误）")
		}
	}
	return data[:len(data)-padding], nil
}

// EncryptByAES 用 AES/ECB/PKCS7 加密并返回 Base64。
// 对应 Python 端 encrypt_by_aes。
func EncryptByAES(data, password string) (string, error) {
	key, err := checkAESKey(password)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("初始化 AES 失败: %w", err)
	}

	bs := block.BlockSize()
	padded := pkcs7Pad([]byte(data), bs)
	out := make([]byte, len(padded))
	// ECB：逐块独立加密，无 IV、无链式依赖（这正是它被诟病的原因，
	// 但前端 crypto-js 侧就是这个模式，必须对齐）
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(out[i:i+bs], padded[i:i+bs])
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptByAES 解 Base64 的 AES/ECB/PKCS7 密文。
// 对应 Python 端 decrypt_by_aes。
func DecryptByAES(b64, password string) (string, error) {
	key, err := checkAESKey(password)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("Base64 解码失败: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("初始化 AES 失败: %w", err)
	}

	bs := block.BlockSize()
	if len(raw) == 0 || len(raw)%bs != 0 {
		return "", fmt.Errorf("密文长度 %d 非法（应为 %d 的整数倍）", len(raw), bs)
	}
	out := make([]byte, len(raw))
	for i := 0; i < len(raw); i += bs {
		block.Decrypt(out[i:i+bs], raw[i:i+bs])
	}
	plain, err := pkcs7Unpad(out, bs)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
