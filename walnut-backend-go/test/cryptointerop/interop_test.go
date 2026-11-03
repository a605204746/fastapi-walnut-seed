// Package cryptointerop 验证 Go 的加解密实现与 Python 端**二进制级互通**。
//
// 运行前置：先用 Python 生成测试向量
//
//	cd walnut-backend-python
//	.venv/Scripts/python ../walnut-backend-go/test/cryptointerop/gen_vectors.py
//
// 随后本测试会：
//  1. 解 Python 生成的 AES/RSA 密文，断言还原一致
//  2. ★ 用同一明文+密钥重新加密，断言 AES 密文与 Python **逐字节相同**
//     （AES-ECB 无随机填充，是确定性算法，故这是比"能解开"强得多的判据）
//  3. 把 Go 的密文写回 go_output.json，供 verify_go_output.py 做反向校验
//
// vectors.json / go_output.json 均已加入 .gitignore（含开发私钥且可复现）。
package cryptointerop

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"walnut-seed-go/internal/pkg/cryptox"
)

const vectorsFile = "vectors.json"

type aesVector struct {
	KeyBits    int    `json:"keyBits"`
	Key        string `json:"key"`
	Plaintext  string `json:"plaintext"`
	Ciphertext string `json:"ciphertext"`
}

type rsaVector struct {
	Plaintext  string `json:"plaintext"`
	Ciphertext string `json:"ciphertext"`
}

type vectors struct {
	PrivateKey string      `json:"privateKey"`
	PublicKey  string      `json:"publicKey"`
	KeyBits    int         `json:"keyBits"`
	AES        []aesVector `json:"aes"`
	RSA        []rsaVector `json:"rsa"`
}

// load 读取向量文件。缺失时 Skip 而非 Fail——CI 若没有 Python 环境，
// 这组测试应当跳过而不是把整条流水线染红（Go 侧自身的正确性由
// internal/pkg/cryptox 的单测保证）。
func load(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile(vectorsFile)
	if err != nil {
		t.Skipf("未找到 %s，跳过互通测试。请先运行 gen_vectors.py（%v）", vectorsFile, err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("解析 %s 失败: %v", vectorsFile, err)
	}
	if len(v.AES) == 0 || len(v.RSA) == 0 {
		t.Fatalf("%s 内容为空，请重新生成", vectorsFile)
	}
	return v
}

// TestAESDecryptFromPython 解 Python 的 AES 密文。
func TestAESDecryptFromPython(t *testing.T) {
	v := load(t)
	for i, vec := range v.AES {
		got, err := cryptox.DecryptByAES(vec.Ciphertext, vec.Key)
		if err != nil {
			t.Errorf("[%d] AES-%d 解密失败: %v（明文长度=%d）", i, vec.KeyBits, err, len(vec.Plaintext))
			continue
		}
		if got != vec.Plaintext {
			t.Errorf("[%d] AES-%d 还原不一致\n  Python=%q\n  Go    =%q", i, vec.KeyBits, vec.Plaintext, got)
		}
	}
	t.Logf("AES 解密：%d 条向量全部与 Python 一致", len(v.AES))
}

// TestAESCiphertextByteIdenticalToPython 是最强判据：
// AES-ECB-PKCS7 无随机填充，Go 的加密结果必须与 Python **逐字节相同**。
//
// 任何模式（ECB↔CBC）、填充（PKCS5/7 边界）、编码（Base64 标准/URL-safe）
// 的差异都会在这里立刻暴露——而"能互相解开"的测试是抓不到这些的。
func TestAESCiphertextByteIdenticalToPython(t *testing.T) {
	v := load(t)
	mismatch := 0
	for i, vec := range v.AES {
		got, err := cryptox.EncryptByAES(vec.Plaintext, vec.Key)
		if err != nil {
			t.Errorf("[%d] AES-%d 加密失败: %v", i, vec.KeyBits, err)
			mismatch++
			continue
		}
		if got != vec.Ciphertext {
			mismatch++
			gotRaw, _ := base64.StdEncoding.DecodeString(got)
			pyRaw, _ := base64.StdEncoding.DecodeString(vec.Ciphertext)
			t.Errorf("[%d] AES-%d 密文不一致（明文=%q 长度=%d）\n  Python=%s（%dB）\n  Go    =%s（%dB）",
				i, vec.KeyBits, vec.Plaintext, len(vec.Plaintext),
				vec.Ciphertext, len(pyRaw), got, len(gotRaw))
		}
	}
	if mismatch == 0 {
		t.Logf("★ AES 密文逐字节相同：%d/%d 条（ECB 模式 + PKCS7 填充 + Base64 编码三项均一致）",
			len(v.AES), len(v.AES))
	}
}

// TestRSADecryptFromPython 解 Python 的 RSA 密文（1024 位 + 分段 + 中文多字节切断 + 空串）。
func TestRSADecryptFromPython(t *testing.T) {
	v := load(t)
	priv, err := cryptox.ParsePrivateKey(v.PrivateKey)
	if err != nil {
		t.Fatalf("解析 Python 侧私钥失败: %v", err)
	}
	bits := priv.N.BitLen()
	if bits != v.KeyBits {
		t.Errorf("私钥位数不符：向量声明 %d，实得 %d", v.KeyBits, bits)
	}
	t.Logf("私钥位数 = %d（确认 Go 可正常使用该长度的 RSA）", bits)

	for i, vec := range v.RSA {
		got, err := cryptox.DecryptByRSA(vec.Ciphertext, priv)
		if err != nil {
			t.Errorf("[%d] RSA 解密失败: %v（明文长度=%d）", i, err, len(vec.Plaintext))
			continue
		}
		if got != vec.Plaintext {
			t.Errorf("[%d] RSA 还原不一致\n  Python=%q\n  Go    =%q", i, vec.Plaintext, got)
		}
	}
	t.Logf("RSA 解密：%d 条向量全部与 Python 一致", len(v.RSA))
}

// TestRSAEncryptForPython 用 Go 加密并写出 go_output.json，
// 供 verify_go_output.py 做**反向**校验（Python 解 Go 的密文）。
func TestRSAEncryptForPython(t *testing.T) {
	v := load(t)

	// gen_vectors.py 里的 publicKey 是由 privateKey 派生的**配对**公钥
	// （.env.dev 里那两把属于不同的密钥对，不能混用）
	pub, err := cryptox.ParsePublicKey(v.PublicKey)
	if err != nil {
		t.Fatalf("解析配对公钥失败: %v", err)
	}

	type rsaOut struct {
		Plaintext  string `json:"plaintext"`
		Ciphertext string `json:"ciphertext"`
	}
	type aesOut struct {
		KeyBits    int    `json:"keyBits"`
		Key        string `json:"key"`
		Plaintext  string `json:"plaintext"`
		Ciphertext string `json:"ciphertext"`
	}

	rsaResults := make([]rsaOut, 0, len(v.RSA))
	for _, vec := range v.RSA {
		ct, err := cryptox.EncryptByRSA(vec.Plaintext, pub)
		if err != nil {
			t.Fatalf("Go RSA 加密失败: %v", err)
		}
		// 自检：Go 自己也要能解回来（分段逻辑双向一致）
		back, err := cryptox.DecryptByRSA(ct, mustPriv(t, v.PrivateKey))
		if err != nil || back != vec.Plaintext {
			t.Fatalf("Go RSA 自洽往返失败: err=%v got=%q want=%q", err, back, vec.Plaintext)
		}
		rsaResults = append(rsaResults, rsaOut{Plaintext: vec.Plaintext, Ciphertext: ct})
	}

	aesResults := make([]aesOut, 0, len(v.AES))
	for _, vec := range v.AES {
		ct, err := cryptox.EncryptByAES(vec.Plaintext, vec.Key)
		if err != nil {
			t.Fatalf("Go AES 加密失败: %v", err)
		}
		aesResults = append(aesResults, aesOut{
			KeyBits: vec.KeyBits, Key: vec.Key, Plaintext: vec.Plaintext, Ciphertext: ct,
		})
	}

	payload := map[string]any{
		"rsa":        rsaResults,
		"aes":        aesResults,
		"privateKey": v.PrivateKey,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	dst := filepath.Join(".", "go_output.json")
	if err := os.WriteFile(dst, blob, 0o600); err != nil {
		t.Fatalf("写出 go_output.json 失败: %v", err)
	}
	t.Logf("已写出 %d 条 RSA + %d 条 AES 的 Go 密文 -> %s（供 verify_go_output.py 反向校验）",
		len(rsaResults), len(aesResults), dst)
}

// TestRSASegmentBoundaries 验证分段参数与 Python 端一致：
// 加密块 = Size-11，解密块 = Size。用 1024 位密钥即 117 / 128 字节。
func TestRSASegmentBoundaries(t *testing.T) {
	v := load(t)
	priv := mustPriv(t, v.PrivateKey)
	pub := &priv.PublicKey

	if got, want := pub.Size()-11, 117; got != want {
		t.Errorf("加密分段大小 = %d，期望 %d（Size-11，PKCS1v15 特征）", got, want)
	}
	if got, want := priv.Size(), 128; got != want {
		t.Errorf("解密分段大小 = %d，期望 %d（Size）", got, want)
	}

	// 构造恰好跨块的载荷：117 字节（1 块）与 118 字节（2 块）等边界
	for _, n := range []int{116, 117, 118, 234, 235} {
		// 用可打印字符而非 \x00，便于失败时肉眼比对
		buf := make([]byte, n)
		for i := range buf {
			buf[i] = byte('a' + i%26)
		}
		payload := string(buf)

		ct, err := cryptox.EncryptByRSA(payload, pub)
		if err != nil {
			t.Fatalf("n=%d 加密失败: %v", n, err)
		}
		raw, _ := base64.StdEncoding.DecodeString(ct)
		// 块数 = ceil(n / 117)
		wantBlocks := (n + 116) / 117
		if len(raw) != wantBlocks*128 {
			t.Errorf("n=%d 密文 %dB，期望 %d 块 × 128 = %dB", n, len(raw), wantBlocks, wantBlocks*128)
		}
		back, err := cryptox.DecryptByRSA(ct, priv)
		if err != nil || back != payload {
			t.Errorf("n=%d 往返失败: err=%v 一致=%v", n, err, back == payload)
		}
	}
}

func mustPriv(t *testing.T, b64 string) *rsa.PrivateKey {
	t.Helper()
	priv, err := cryptox.ParsePrivateKey(b64)
	if err != nil {
		t.Fatalf("解析私钥失败: %v", err)
	}
	return priv
}
