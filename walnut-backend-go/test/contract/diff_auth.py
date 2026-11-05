"""认证链路（P2）的双端契约比对。

与 diff_contract.py 的分工：那个覆盖无状态的形状比对，本脚本专攻**认证链路**，
因为它需要额外能力——从 Redis 读验证码答案，才能走完一次**成功登录**。

覆盖三类用例：
  1. 无请求体的端点：/auth/tenant/list、/auth/code、/auth/logout（幂等）
  2. 带请求体的失败路径：参数校验、未知 clientId、不支持的授权类型、验证码失效、注册未开启
  3. **成功登录全链路**：取验证码 → 从 Redis 读答案 → 登录 → 比对响应形状与 JWT claims

第 3 类是关键：它验证 Go 端能真实签发可用 token，而不只是错误路径对齐。

用法：
    python test/contract/diff_auth.py [--py http://127.0.0.1:8012] [--go http://127.0.0.1:8011]

前置：两端都在运行，且中间件（MySQL/Redis）可达。
"""

import argparse
import base64
import json
import subprocess
import sys
import urllib.error
import urllib.request

CLIENT_ID = "e5cd7e4891bf95d1d19206ce24a7b32e"  # 种子数据里的 pc 客户端
ADMIN_USER = "admin"
ADMIN_PWD = "admin123"

# 各端的 Redis 键前缀（三端隔离的硬约束）
REDIS_PREFIX = {"py": "walnut-seed-python", "go": "walnut-seed-go"}
REDIS_CONTAINER = "walnut-redis"

# 必然不同、需归一化的字段
VOLATILE = {
    "uuid": "<UUID>",       # 验证码标识，每次随机
    "img": "<IMG>",         # 验证码图片，每次随机
    "access_token": "<TOKEN>",  # 含随机 session_id，且两端 JWT header 键序不同
    "timestamp": "<TS>",
    "uptime_seconds": "<UPTIME>",
    "latency_ms": "<LATENCY>",
}


def call(base, method, path, body=None, headers=None):
    """发请求，返回 (status, headers, raw_text)。"""
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base.rstrip("/") + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    req.add_header("clientid", CLIENT_ID)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, dict(r.headers), r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read().decode("utf-8", "replace")
    except Exception as e:  # noqa: BLE001
        return None, {}, f"<连接失败: {e}>"


def normalize(obj):
    if isinstance(obj, dict):
        return {
            k: (None if v is None else VOLATILE[k]) if k in VOLATILE else normalize(v)
            for k, v in obj.items()
        }
    if isinstance(obj, list):
        return [normalize(i) for i in obj]
    return obj


def canon(raw):
    text = raw.rstrip("\n")
    try:
        return json.dumps(normalize(json.loads(text)), ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    except json.JSONDecodeError:
        return text


def redis_get(side, key):
    """从 Redis 读取键值（用于取验证码答案）。"""
    full = f"{REDIS_PREFIX[side]}:{key}"
    proc = subprocess.run(
        ["docker", "exec", REDIS_CONTAINER, "redis-cli", "GET", full],
        capture_output=True, text=True, encoding="utf-8",
    )
    if proc.returncode != 0:
        return None
    out = proc.stdout.strip()
    return None if out in ("", "(nil)") else out


def decode_jwt(token):
    """解出 JWT 的 header 与 claims（不验签，只看形状）。"""
    parts = token.split(".")
    if len(parts) != 3:
        return None, None

    def b64url(s):
        s += "=" * (-len(s) % 4)
        return json.loads(base64.urlsafe_b64decode(s))

    try:
        return b64url(parts[0]), b64url(parts[1])
    except Exception:  # noqa: BLE001
        return None, None


# ==================== 用例执行 ====================

RESULTS = []


def record(name, ok, detail=""):
    RESULTS.append((name, ok, detail))
    print(f"  {'[OK]  ' if ok else '[FAIL]'} {name}" + (f"\n         {detail}" if detail and not ok else ""))


def compare(name, method, path, body=None, headers=None):
    """对同一路径请求两端并比对。"""
    ps, ph, pb = call(PY, method, path, body, headers)
    gs, gh, gb = call(GO, method, path, body, headers)

    if ps is None or gs is None:
        record(name, False, f"有一端不可达 (py={ps}, go={gs})")
        return None, None

    diffs = []
    if ps != gs:
        diffs.append(f"HTTP 状态码: py={ps} go={gs}")
    pc, gc = canon(pb), canon(gb)
    if pc != gc:
        diffs.append(f"响应体:\n           py = {pc}\n           go = {gc}")
    record(name, not diffs, "\n           ".join(diffs))
    return (json.loads(pb) if pb.startswith("{") else None), (json.loads(gb) if gb.startswith("{") else None)


def login_flow(side, base):
    """在指定后端上走完一次成功登录，返回 (响应 dict, 错误信息)。"""
    # 1. 取验证码
    st, _, raw = call(base, "GET", "/auth/code")
    if st != 200:
        return None, f"/auth/code 返回 HTTP {st}"
    env = json.loads(raw)
    data = env.get("data") or {}
    if not data.get("captchaEnabled"):
        return None, "验证码未开启，无法测试该路径"
    uuid_str = data.get("uuid")
    if not uuid_str:
        return None, "/auth/code 未返回 uuid"

    # 2. 从 Redis 读答案（验证码图片是随机的，只能这样拿到正确答案）
    answer = redis_get(side, f"global:captcha_codes:{uuid_str}")
    if answer is None:
        return None, f"Redis 中读不到验证码答案（key={REDIS_PREFIX[side]}:global:captcha_codes:{uuid_str}）"

    # 3. 登录
    st, _, raw = call(base, "POST", "/auth/login", {
        "clientId": CLIENT_ID,
        "grantType": "password",
        "username": ADMIN_USER,
        "password": ADMIN_PWD,
        "code": answer,
        "uuid": uuid_str,
    })
    if st != 200:
        return None, f"/auth/login 返回 HTTP {st}: {raw[:200]}"
    return json.loads(raw), None


def main():
    global PY, GO
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    PY, GO = args.py, args.go

    print("=" * 82)
    print(f"认证链路契约比对：Python({PY})  vs  Go({GO})")
    print("=" * 82)

    print("\n--- 1. 无请求体端点 ---")
    compare("GET /auth/tenant/list", "GET", "/auth/tenant/list")
    compare("GET /auth/code（归一化 uuid/img）", "GET", "/auth/code")
    # ⚠️ img 必须是**裸 base64**（不带 data URI 前缀）：前端自行拼接
    # `data:image/png;base64,`。多带前缀会让前端裂图（2026-11-04 实测踩过）。
    # compare() 把 img 归一化成 <IMG> 会掩盖这个偏差，故单独断言。
    for side, base in (("py", PY), ("go", GO)):
        _, _, raw = call(base, "GET", "/auth/code")
        try:
            img = (json.loads(raw).get("data") or {}).get("img") or ""
        except Exception:  # noqa: BLE001
            img = ""
        record(f"/auth/code img 裸 base64（{side}，无 data: 前缀）",
               not img.startswith("data:"),
               f"img 前 30 字符: {img[:30]}")
    compare("POST /auth/logout（无 token，应幂等成功）", "POST", "/auth/logout")
    compare("GET /auth/login（方法不匹配 → 405）", "GET", "/auth/login")
    compare("GET /auth/code/（尾斜杠 307）", "GET", "/auth/code/")

    print("\n--- 2. 失败路径（带请求体）---")
    failure_cases = [
        ("登录-空请求体", {}),
        ("登录-缺 grantType", {"clientId": CLIENT_ID}),
        ("登录-用户名过短(min=2)", {"clientId": CLIENT_ID, "grantType": "password", "username": "a", "password": "12345"}),
        ("登录-密码过短(min=5)", {"clientId": CLIENT_ID, "grantType": "password", "username": "admin", "password": "123"}),
        ("登录-未知 clientId", {"clientId": "nonexistent-client", "grantType": "password", "username": ADMIN_USER, "password": ADMIN_PWD}),
        ("登录-不支持的授权类型", {"clientId": CLIENT_ID, "grantType": "sms", "username": ADMIN_USER, "password": ADMIN_PWD}),
        ("登录-验证码已失效", {"clientId": CLIENT_ID, "grantType": "password", "username": ADMIN_USER, "password": ADMIN_PWD, "code": "0", "uuid": "not-a-real-uuid"}),
    ]
    for name, body in failure_cases:
        compare(name, "POST", "/auth/login", body)

    compare("注册-开关未开启", "POST", "/auth/register", {
        "clientId": CLIENT_ID, "grantType": "password",
        "username": "probeuser", "password": "Probe@123",
    })

    print("\n--- 3. 成功登录全链路 ---")
    py_resp, py_err = login_flow("py", PY)
    go_resp, go_err = login_flow("go", GO)

    if py_err or go_err:
        record("成功登录", False, f"py={py_err} go={go_err}")
    else:
        # 3.1 信封与响应形状
        diffs = []
        if py_resp.get("code") != go_resp.get("code"):
            diffs.append(f"body.code: py={py_resp.get('code')} go={go_resp.get('code')}")
        pd, gd = py_resp.get("data") or {}, go_resp.get("data") or {}
        if sorted(pd.keys()) != sorted(gd.keys()):
            diffs.append(f"data 字段集不同:\n           py={sorted(pd.keys())}\n           go={sorted(gd.keys())}")
        # ⚠️ 登录响应是 snake_case（Python 端 do_login 返回裸 dict，无 alias 转换）
        for k in ("access_token", "refresh_token", "expire_in", "refresh_expire_in", "client_id", "scope", "openid"):
            if k not in gd:
                diffs.append(f"go 的 data 缺少 {k}（必须是 snake_case）")
        for k in ("refresh_token", "refresh_expire_in", "scope", "openid"):
            if gd.get(k) is not None:
                diffs.append(f"go 的 {k} 应为 null，实际 {gd.get(k)!r}")
        if pd.get("expire_in") != gd.get("expire_in"):
            diffs.append(f"expire_in: py={pd.get('expire_in')} go={gd.get('expire_in')}")
        if pd.get("client_id") != gd.get("client_id"):
            diffs.append(f"client_id: py={pd.get('client_id')} go={gd.get('client_id')}")
        record("登录响应形状（snake_case + null 字段）", not diffs, "\n           ".join(diffs))

        # 3.2 JWT claims 比对（token 字符串必然不同，但 claims 形状必须一致）
        ph, pcl = decode_jwt(pd.get("access_token", ""))
        gh, gcl = decode_jwt(gd.get("access_token", ""))
        jwt_diffs = []
        if pcl is None or gcl is None:
            jwt_diffs.append(f"token 解析失败 py={pcl is not None} go={gcl is not None}")
        else:
            if sorted(pcl.keys()) != sorted(gcl.keys()):
                jwt_diffs.append(f"claims 字段集不同:\n           py={sorted(pcl.keys())}\n           go={sorted(gcl.keys())}")
            # 逐字段比对（sub/exp 是随机的，只验证存在且类型正确）
            for k in ("user_name", "clientid", "is_refresh", "dept_name", "dept_category"):
                if pcl.get(k) != gcl.get(k):
                    jwt_diffs.append(f"claims.{k}: py={pcl.get(k)!r} go={gcl.get(k)!r}")
            for k in ("user_id", "dept_id"):
                if type(pcl.get(k)) is not type(gcl.get(k)):
                    jwt_diffs.append(f"claims.{k} 类型不同: py={type(pcl.get(k)).__name__} go={type(gcl.get(k)).__name__}")
            if pcl.get("user_name") != ADMIN_USER:
                jwt_diffs.append(f"py 的 claims.user_name = {pcl.get('user_name')!r}，期望 {ADMIN_USER!r}")
            if not isinstance(pcl.get("exp"), int) or not isinstance(gcl.get("exp"), int):
                jwt_diffs.append("exp 必须是整数时间戳")
            # ⚠️ Go 侧不得添加 PyJWT 不会自动生成的注册声明
            for forbidden in ("iat", "nbf", "iss", "aud", "jti"):
                if forbidden in gcl and forbidden not in pcl:
                    jwt_diffs.append(f"go 的 claims 多出 {forbidden}（PyJWT 不会自动添加该声明）")
            # header 键序不同是**已接受的差异**（见实施计划 §12.7），此处只验证 alg 一致
            if (ph or {}).get("alg") != (gh or {}).get("alg"):
                jwt_diffs.append(f"JWT header.alg: py={(ph or {}).get('alg')} go={(gh or {}).get('alg')}")
        record("JWT claims 形状与内容", not jwt_diffs, "\n           ".join(jwt_diffs))

        # 3.3 登录后 Redis 会话键是否都写入了
        sess_diffs = []
        for side, resp in (("py", pd), ("go", gd)):
            token = (resp.get("access_token") or "")
            header, claims = decode_jwt(token)
            if not claims:
                sess_diffs.append(f"{side}: 无法解析 token")
                continue
            sid = claims.get("sub", "")
            for key in (f"user_session:{sid}", f"access_token:{sid}", f"online_tokens:{token}"):
                if redis_get(side, key) is None:
                    sess_diffs.append(f"{side}: Redis 缺少键 {REDIS_PREFIX[side]}:{key}")
        record("登录后 Redis 会话键（user_session/access_token/online_tokens）", not sess_diffs,
               "\n           ".join(sess_diffs))

    print("\n" + "=" * 82)
    passed = sum(1 for _, ok, _ in RESULTS if ok)
    failed = len(RESULTS) - passed
    print(f"通过 {passed} / 失败 {failed}  （共 {len(RESULTS)} 项）")
    print("=" * 82)
    return 1 if failed else 0


PY = GO = ""

if __name__ == "__main__":
    sys.exit(main())
