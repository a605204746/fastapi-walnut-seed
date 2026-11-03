"""反向校验：用 Python 解 Go 生成的密文，并逐字节比对 AES 密文。

前置：Go 侧互通测试已运行并写出 `go_output.json`（P1 落地 `test/cryptointerop/`）。

用法：
    cd walnut-backend-python
    .venv\\Scripts\\python.exe ..\\walnut-backend-go\\test\\cryptointerop\\verify_go_output.py
"""

import base64
import json
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPO / "walnut-backend-python"))

from app.core.encrypt import decrypt_by_aes, decrypt_by_rsa, encrypt_by_aes  # noqa: E402

HERE = Path(__file__).parent
GO_OUTPUT = HERE / "go_output.json"

if not GO_OUTPUT.exists():
    raise SystemExit(f"未找到 {GO_OUTPUT.name}，请先运行 Go 侧互通测试")

go_out = json.loads(GO_OUTPUT.read_text(encoding="utf-8"))
priv = go_out["privateKey"]
fail = 0

# ---- AES：Go 密文 vs Python 密文，逐字节 ----
same = 0
for v in go_out["aes"]:
    if encrypt_by_aes(v["plaintext"], v["key"]) == v["ciphertext"]:
        same += 1
    else:
        fail += 1
        print(f"[FAIL] AES-{v['keyBits']} 密文不一致 明文={v['plaintext']!r}")
        print(f"       Go = {v['ciphertext']}")
print(f"[{'OK' if same == len(go_out['aes']) else 'FAIL'}] AES 逐字节相同：{same}/{len(go_out['aes'])}")

# ---- RSA：Python 解 Go 的密文 ----
ok = 0
for v in go_out["rsa"]:
    try:
        got = decrypt_by_rsa(v["ciphertext"], priv)
    except Exception as e:  # noqa: BLE001
        fail += 1
        print(f"[FAIL] Python 解 Go 的 RSA 密文异常: {e}")
        continue
    if got == v["plaintext"]:
        ok += 1
    else:
        fail += 1
        print(f"[FAIL] RSA 还原不一致\n  期望={v['plaintext']!r}\n  实得={got!r}")
print(f"[{'OK' if ok == len(go_out['rsa']) else 'FAIL'}] RSA 双向可解：{ok}/{len(go_out['rsa'])}")

# ---- RSA 分段块结构核验（1024 位密钥 → 每块 128 字节）----
for v in go_out["rsa"]:
    raw = base64.b64decode(v["ciphertext"]) if v["ciphertext"] else b""
    if raw and len(raw) % 128 != 0:
        fail += 1
        print(f"[FAIL] RSA 密文长度 {len(raw)} 非 128 整数倍，分段逻辑有误")
if not fail:
    print("[OK] RSA 密文分块均为 128 字节整数倍（1024 位密钥）")

print("=" * 60)
if fail:
    print(f"{fail} 项失败")
    sys.exit(1)
print("反向校验全部通过：Python↔Go 加解密完全互通")
