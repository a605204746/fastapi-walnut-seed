"""校验前端与三个后端的 RSA 密钥配对关系。

配对规则（见各 env 文件的注释）：
    前端 VITE_GLOB_RSA_PUBLIC_KEY  （请求加密）↔ 后端 API_DECRYPT_PRIVATE_KEY（解请求）
    前端 VITE_GLOB_RSA_PRIVATE_KEY （响应解密）↔ 后端 API_DECRYPT_PUBLIC_KEY （加响应）

验证方法：从私钥**派生**公钥（私钥内部含公钥信息），再与对端声明的公钥比对模数 N。
模数相同 ⇒ 同一对密钥。这比肉眼比 base64 可靠——两者编码形式不同，
且 base64 前缀相同不代表模数相同。

同时检查：
  - 密钥位数（前端 jsencrypt 上限 1024；后端最小要求 1024）
  - 是否命中 RuoYi 出厂密钥黑名单（后端 prod 会拒绝启动）

用法：python scripts/verify_key_pairing.py
"""

import base64
import re
import sys
from pathlib import Path

from cryptography.hazmat.primitives import serialization

# 本文件位于 <repo>/walnut-backend-go/scripts/ ，故 parents[2] 是仓库根
REPO = Path(__file__).resolve().parents[2]

# RuoYi 生态公开的出厂默认密钥前缀（与后端 validate_security_settings 的黑名单一致）
KNOWN_BAD_PUBLIC_PREFIX = "MFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAKoR8mX0rGKLqzcWmOzbfj64K8ZIgOdHnzkXSOVOZbFu"
KNOWN_BAD_PRIVATE_PREFIX = "MIIBVAIBADANBgkqhkiG9w0BAQEFAASCAT4wggE6AgEAAoEAmc3CuPiGL"

FRONTEND_ENVS = {
    "development": REPO / "walnut-frontend/apps/web-antd/.env.development",
    "production": REPO / "walnut-frontend/apps/web-antd/.env.production",
    "test": REPO / "walnut-frontend/apps/web-antd/.env.test",
}

BACKEND_ENVS = {
    "python": REPO / "walnut-backend-python/env/.env.dev",
    "go": REPO / "walnut-backend-go/env/.env.dev",
}


def parse_env(path: Path) -> dict:
    """解析 KEY=VALUE 文件。"""
    out = {}
    if not path.exists():
        return out
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        out[k.strip()] = v.strip()
    return out


def load_public(b64: str):
    der = base64.b64decode(b64)
    return serialization.load_der_public_key(der)


def load_private(b64: str):
    der = base64.b64decode(b64)
    return serialization.load_der_private_key(der, password=None)


def modulus(key) -> int:
    """取 RSA 模数 N。公钥直接用 .n，私钥用 .private_numbers().public_numbers.n。"""
    pub = key.public_key() if hasattr(key, "private_numbers") else key
    return pub.public_numbers().n


def bits(key) -> int:
    return modulus(key).bit_length()


def describe(label: str, b64: str, kind: str) -> dict:
    """解析一把密钥并返回其指纹信息。"""
    info = {"label": label, "present": bool(b64), "kind": kind}
    if not b64:
        info["error"] = "未配置（空值）"
        return info
    try:
        key = load_public(b64) if kind == "public" else load_private(b64)
    except Exception as e:  # noqa: BLE001
        info["error"] = f"解析失败: {e}"
        return info
    info["bits"] = bits(key)
    info["n"] = modulus(key)
    # 派生出公钥指纹，用于配对判断
    info["pub_n"] = modulus(key)
    if kind == "private":
        info["derived_public_b64"] = base64.b64encode(
            key.public_key().public_bytes(
                encoding=serialization.Encoding.DER,
                format=serialization.PublicFormat.SubjectPublicKeyInfo,
            )
        ).decode()
    info["known_bad"] = b64.startswith(
        KNOWN_BAD_PUBLIC_PREFIX if kind == "public" else KNOWN_BAD_PRIVATE_PREFIX
    )
    return info


def main() -> int:
    print("=" * 82)
    print("RSA 密钥配对校验：前端 ↔ Python 后端 ↔ Go 后端")
    print("=" * 82)

    # ---- 1. 后端密钥 ----
    backends = {}
    for name, path in BACKEND_ENVS.items():
        env = parse_env(path)
        pub = describe(f"{name}.API_DECRYPT_PUBLIC_KEY", env.get("API_DECRYPT_PUBLIC_KEY", ""), "public")
        priv = describe(f"{name}.API_DECRYPT_PRIVATE_KEY", env.get("API_DECRYPT_PRIVATE_KEY", ""), "private")
        backends[name] = {
            "path": path,
            "enabled": env.get("API_DECRYPT_ENABLED", ""),
            "public": pub,
            "private": priv,
        }
        print(f"\n[{name}] {path.relative_to(REPO)}")
        print(f"  API_DECRYPT_ENABLED = {env.get('API_DECRYPT_ENABLED', '(未设置)')}")
        for k in ("public", "private"):
            d = backends[name][k]
            if "error" in d:
                print(f"  {k:8s}: {d['error']}")
            else:
                flag = "  ⚠️ 命中 RuoYi 出厂密钥黑名单" if d.get("known_bad") else ""
                print(f"  {k:8s}: {d['bits']} 位{flag}")

    # ---- 2. 前端各环境 ----
    frontends = {}
    for name, path in FRONTEND_ENVS.items():
        env = parse_env(path)
        pub = describe(f"{name}.VITE_GLOB_RSA_PUBLIC_KEY", env.get("VITE_GLOB_RSA_PUBLIC_KEY", ""), "public")
        priv = describe(f"{name}.VITE_GLOB_RSA_PRIVATE_KEY", env.get("VITE_GLOB_RSA_PRIVATE_KEY", ""), "private")
        frontends[name] = {
            "path": path,
            "enableEncrypt": env.get("VITE_GLOB_ENABLE_ENCRYPT", "(未设置)"),
            "public": pub,
            "private": priv,
        }
        print(f"\n[frontend:{name}] {path.relative_to(REPO)}")
        print(f"  VITE_GLOB_ENABLE_ENCRYPT = {frontends[name]['enableEncrypt']}")
        for k in ("public", "private"):
            d = frontends[name][k]
            if "error" in d:
                print(f"  {k:8s}: {d['error']}")
            else:
                flag = "  ⚠️ 命中 RuoYi 出厂密钥黑名单" if d.get("known_bad") else ""
                print(f"  {k:8s}: {d['bits']} 位{flag}")

    # ---- 3. 配对校验 ----
    print("\n" + "=" * 82)
    print("配对校验（模数 N 相同即为同一对）")
    print("=" * 82)
    problems = []

    for fe_name, fe in frontends.items():
        encrypt_on = fe["enableEncrypt"] == "true"
        if not encrypt_on:
            print(f"\n[frontend:{fe_name}] 加密已关闭 → 不发送 encrypt-key 头，无需配对")
            # 仍需检查：若声明了密钥但开关关闭，属配置冗余（非错误）
            continue

        print(f"\n[frontend:{fe_name}] 加密**已开启** → 必须有配对的后端密钥")
        for be_name, be in backends.items():
            # 请求方向：前端 PUBLIC ↔ 后端 PRIVATE
            fe_pub, be_priv = fe["public"], be["private"]
            req_ok = (
                "n" in fe_pub and "pub_n" in be_priv and fe_pub["n"] == be_priv["pub_n"]
            )
            # 响应方向：前端 PRIVATE ↔ 后端 PUBLIC
            fe_priv, be_pub = fe["private"], be["public"]
            resp_ok = (
                "pub_n" in fe_priv and "n" in be_pub and fe_priv["pub_n"] == be_pub["n"]
            )

            status = "✅" if (req_ok and resp_ok) else "❌"
            print(f"  {status} {be_name:7s} 请求方向(前端PUB↔后端PRIV)={'配对' if req_ok else '不配对'}  "
                  f"响应方向(前端PRIV↔后端PUB)={'配对' if resp_ok else '不配对'}")
            if not (req_ok and resp_ok):
                missing = []
                if "error" in fe_pub or "error" in be_priv:
                    missing.append("密钥缺失或无法解析")
                problems.append(
                    f"frontend:{fe_name} ↔ {be_name}: 请求方向{'OK' if req_ok else 'FAIL'}、"
                    f"响应方向{'OK' if resp_ok else 'FAIL'}"
                    + (f"（{'; '.join(missing)}）" if missing else "")
                )

    # ---- 4. 后端之间的密钥一致性 ----
    print("\n" + "=" * 82)
    print("后端之间的密钥一致性（三端应使用同一对开发密钥，否则前端要按后端切换配置）")
    print("=" * 82)
    names = list(backends)
    for i in range(len(names)):
        for j in range(i + 1, len(names)):
            a, b = backends[names[i]], backends[names[j]]
            same_pub = a["public"].get("n") == b["public"].get("n") and "n" in a["public"]
            same_priv = a["private"].get("pub_n") == b["private"].get("pub_n") and "pub_n" in a["private"]
            mark = "✅" if (same_pub and same_priv) else "⚠️"
            print(f"  {mark} {names[i]} vs {names[j]}: PUBLIC={'相同' if same_pub else '不同'}  "
                  f"PRIVATE={'相同' if same_priv else '不同'}")
            if not (same_pub and same_priv):
                problems.append(f"{names[i]} 与 {names[j]} 的开发密钥不一致")

    # ---- 5. 位数与黑名单 ----
    print("\n" + "=" * 82)
    print("密钥强度与黑名单检查")
    print("=" * 82)
    for group, name in ((frontends, "frontend"), (backends, "backend")):
        for key, item in group.items():
            for side in ("public", "private"):
                d = item[side]
                if "error" in d:
                    continue
                if d.get("known_bad"):
                    msg = f"{name}:{key} 的 {side} 命中 RuoYi 出厂密钥黑名单"
                    print(f"  ⚠️ {msg}")
                    problems.append(msg)
                if d["bits"] < 1024:
                    msg = f"{name}:{key} 的 {side} 只有 {d['bits']} 位（后端最小要求 1024）"
                    print(f"  ⚠️ {msg}")
                    problems.append(msg)
                elif d["bits"] > 1024:
                    print(f"  ℹ️ {name}:{key} 的 {side} 是 {d['bits']} 位（> jsencrypt 的 1024 上限，前端可能无法使用）")

    print("\n" + "=" * 82)
    if problems:
        print(f"发现 {len(problems)} 项需关注：")
        for p in problems:
            print(f"  - {p}")
        return 1
    print("✅ 全部校验通过：加密开启的环境均有配对密钥，三端开发密钥一致，无黑名单/弱密钥")
    return 0


if __name__ == "__main__":
    sys.exit(main())
