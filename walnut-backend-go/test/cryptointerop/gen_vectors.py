"""互通测试向量生成器（Python 侧）。

用 Python 的 `cryptography` 生成 AES/RSA 测试向量，供 Go 侧解密并**反向比对密文**。

判据强度：AES/ECB/PKCS7 是**确定性算法**（无随机填充），因此 Go 的加密结果必须与
Python 密文**逐字节相同**——这比「能解开」强得多，任何模式、填充、编码差异都会立刻暴露。
RSA/PKCS1v15 含随机填充，密文不可比对，只验双向可解。

用法：
    cd walnut-backend-python
    .venv\\Scripts\\python.exe ..\\walnut-backend-go\\test\\cryptointerop\\gen_vectors.py

随后：
    cd walnut-backend-go
    go test ./test/cryptointerop/... -v      # Go 侧解密 + 逐字节比对（P1 落地）
    .venv\\Scripts\\python.exe ..\\walnut-backend-go\\test\\cryptointerop\\verify_go_output.py

背景见 docs/go/00-Go后端实施计划.md §9.2。
"""

import base64
import json
import sys
from pathlib import Path

# 本文件位于 <repo>/walnut-backend-go/test/cryptointerop/
REPO = Path(__file__).resolve().parents[3]
PY_BACKEND = REPO / "walnut-backend-python"
sys.path.insert(0, str(PY_BACKEND))

from app.core.encrypt import (  # noqa: E402
    decrypt_by_aes,
    decrypt_by_rsa,
    encrypt_by_aes,
    encrypt_by_rsa,
)

HERE = Path(__file__).parent
OUT = HERE / "vectors.json"

# .env.dev 里的 PUBLIC / PRIVATE 属于**两对不同密钥**（请求对 / 响应对），
# 直接配对使用会失败。故从私钥派生出与之严格配对的公钥来构造合法的 RSA 往返。
ENV_DEV = PY_BACKEND / "env" / ".env.dev"


def load_env_key(name: str) -> str:
    for line in ENV_DEV.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        if k.strip() == name:
            return v.strip()
    raise SystemExit(f".env.dev 中未找到 {name}")


def derive_public_b64(private_b64: str) -> str:
    from cryptography.hazmat.primitives import serialization

    priv = serialization.load_der_private_key(base64.b64decode(private_b64), password=None)
    der = priv.public_key().public_bytes(
        encoding=serialization.Encoding.DER,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    )
    return base64.b64encode(der).decode()


def main() -> None:
    priv_b64 = load_env_key("API_DECRYPT_PRIVATE_KEY")
    pub_b64 = derive_public_b64(priv_b64)

    aes_keys = {"16": "k" * 16, "24": "k" * 24, "32": "k" * 32}
    payloads = [
        "",
        "a",
        "x" * 16,  # 整块，验证 PKCS7 补满一整块（16B 明文 → 32B 密文）
        "y" * 17,  # 跨块
        '{"username":"admin","password":"admin123"}',
        "中文多字节载荷：核桃种子脚手架 WalnutSeed",
        "混排 mixed 中文 and ASCII 12345 !@#$%",
    ]

    aes_vectors = [
        {"keyBits": int(bits), "key": key, "plaintext": p, "ciphertext": encrypt_by_aes(p, key)}
        for bits, key in aes_keys.items()
        for p in payloads
    ]

    rsa_plaintexts = [
        "32位AES密钥abcdef0123456789xyz",
        # 真实协议里被 RSA 包裹的就是「Base64(AES密钥)」这个形状
        base64.b64encode(b"0123456789abcdef0123456789abcdef").decode(),
        "长载荷分段测试 " * 20,
        "中文 RSA 分段：核桃种子 " * 8,
        "",  # 空输入走 Python 的 `chunks = [...] or [b""]` 分支
    ]
    rsa_vectors = [{"plaintext": p, "ciphertext": encrypt_by_rsa(p, pub_b64)} for p in rsa_plaintexts]

    vectors = {
        "privateKey": priv_b64,
        "publicKey": pub_b64,
        "keyBits": 1024,
        "aes": aes_vectors,
        "rsa": rsa_vectors,
    }
    OUT.write_text(json.dumps(vectors, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"[OK] 已写出 {len(aes_vectors)} 条 AES 向量 + {len(rsa_vectors)} 条 RSA 向量 -> {OUT}")
    print(f"     配对公钥由私钥派生，DER 长度 {len(base64.b64decode(pub_b64))} 字节")

    # 自校验：确保向量本身有效，否则 Go 侧的失败无法归因
    for v in aes_vectors:
        assert decrypt_by_aes(v["ciphertext"], v["key"]) == v["plaintext"], "AES 自校验失败"
    for v in rsa_vectors:
        assert decrypt_by_rsa(v["ciphertext"], priv_b64) == v["plaintext"], "RSA 自校验失败"
    print("[OK] Python 侧自校验通过（向量本身有效）")
    print()
    print("下一步：在 walnut-backend-go 下运行 Go 侧互通测试，再执行 verify_go_output.py 做反向校验")


if __name__ == "__main__":
    main()
