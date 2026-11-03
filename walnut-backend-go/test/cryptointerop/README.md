# 加解密互通测试（Python ↔ Go）

验证 Go 侧的 `AES/ECB/PKCS7` 与 `RSA/PKCS1v15 分段` 实现与 Python 端
`walnut-backend-python/app/core/encrypt.py` **二进制级一致**。

> 这是 P1 任务 1.16 的前置资产，已在 2026-11-02 完成验证。
> 背景与完整结论见 [`docs/go/00-Go后端实施计划.md`](../../../docs/go/00-Go后端实施计划.md) §9.2。

## 为什么判据是「逐字节相同」而不只是「能解开」

`AES/ECB/PKCS7` 是**确定性算法**（无随机填充），同一明文 + 同一密钥必得同一密文。
因此 Go 的加密结果必须与 Python 密文**完全相同**——任何模式、填充、Base64 变体的差异
都会立刻暴露。这比「互相能解开」强得多。

`RSA/PKCS1v15` 含随机填充，密文不可比对，故只验**双向可解**。

## 已验证结果（2026-11-02）

| 测试项 | 结果 |
|---|---|
| Go 解 Python 的 AES 密文 | ✅ 21/21 一致 |
| **Go 加密结果 vs Python 密文逐字节** | ✅ **21/21 完全相同** |
| Go 解 Python 的 RSA 密文 | ✅ 5/5 一致 |
| Python 解 Go 的 RSA 密文（反向） | ✅ 5/5 一致 |
| RSA 密文分块均为 128 字节整数倍 | ✅ |
| 1024 位 RSA @ Go 1.24.0 与 1.26.0 | ✅ 均可用 |
| AES-128/192/256 × {空串, 单字节, 整块 16B, 跨块 17B, JSON, 中文, 混排} | ✅ 全部往返一致 |
| PKCS7 整块补齐（16B 明文 → **32B** 密文） | ✅ 与 Python `PKCS7(128)` 一致 |

## 用法

```bash
# 1. 生成 Python 侧向量（写入 vectors.json，已被 .gitignore 忽略）
cd walnut-backend-python
.venv/Scripts/python.exe ../walnut-backend-go/test/cryptointerop/gen_vectors.py

# 2. Go 侧解密 + 逐字节比对 + 写出 go_output.json
cd walnut-backend-go
go test ./test/cryptointerop/... -v          # P1 落地该测试文件

# 3. Python 反向校验 Go 的密文
cd walnut-backend-python
.venv/Scripts/python.exe ../walnut-backend-go/test/cryptointerop/verify_go_output.py
```

## 算法规格（照此实现，勿凭记忆）

| 项 | 规格 | Go 对应 |
|---|---|---|
| AES 模式 | `AES/ECB/PKCS5Padding`（即 PKCS7，块长 128 位） | **标准库无 ECB，必须手写逐块** |
| AES 密钥 | password 字符串 UTF-8 编码，长度须 16/24/32 | `aes.NewCipher([]byte(password))` |
| AES 输出 | Base64 标准编码 | `base64.StdEncoding` |
| RSA 填充 | `RSA/ECB/PKCS1Padding` → **PKCS1v15**（非 OAEP） | `rsa.EncryptPKCS1v15` / `rsa.DecryptPKCS1v15(rand.Reader, ...)` |
| RSA 密钥格式 | Base64 DER：公钥 X.509/SPKI，私钥 PKCS#8 | `x509.ParsePKIXPublicKey` / `x509.ParsePKCS8PrivateKey` |
| RSA 加密分段 | `max_chunk = key_size//8 - 11`（1024 位 → **117 字节**） | `pub.Size() - 11` |
| RSA 解密分段 | `chunk_size = key_size//8`（1024 位 → **128 字节**） | `priv.Size()` |
| 空输入 | `chunks = [...] or [b""]` → 按**一个空块**处理 | 需显式分支 |
| 一次性 AES 密钥 | `random_string(32)`，字符集 `a-z0-9`（36 字符） | 同字符集 |

### 两个必须复刻的实现细节

1. **多字节字符切断**：Python 对 **UTF-8 字节流**分段（`raw[i:i+max_chunk]`），会把中文等
   多字节字符从中间切断。解密时**先拼接全部解密字节，最后才 `.decode("utf-8")`**。
   Go 必须同样先 `append` 拼完再 `string(out)`——逐块 `string()` 再拼接会产生非法 UTF-8。
   本目录的向量含 396 字节中文载荷（约 4 块）专门覆盖此场景。

2. **`-11` 是 PKCS1v15 的填充特征**：OAEP 是 `-42`。分段参数取 `-11` 本身就证明了
   对端是 PKCS1v15，Go 侧不可误用 `rsa.EncryptOAEP`。

## P1 待办

Go 侧的测试文件尚未落地（当前只有 Python 侧的向量生成与反向校验）。P1 任务 1.6–1.8 时：

1. 新建 `internal/pkg/cryptox/{aes,rsa}.go`，实现上表规格
2. 新建 `test/cryptointerop/interop_test.go`，读取 `vectors.json`：
   - `TestAESDecryptFromPython` — 解 Python 密文
   - `TestAESCiphertextByteIdenticalToPython` — ★ 逐字节比对
   - `TestRSADecryptFromPython` — 解 Python 密文（含分段与中文切断）
   - `TestRSAEncryptForPython` — Go 加密后写出 `go_output.json`
3. 把上述四个测试加进 CI（与 `make contract-diff` 同为契约门槛）

> ⚠️ `.env.dev` 里的 `API_DECRYPT_PUBLIC_KEY` 与 `API_DECRYPT_PRIVATE_KEY` 属于
> **两对不同密钥**（请求对 / 响应对），直接配对使用会失败。
> `gen_vectors.py` 从私钥**派生**出严格配对的公钥来构造合法的 RSA 往返。
