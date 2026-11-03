"""P3 窄切片（认证中间件 + getInfo + getRouters）的双端契约比对。

与 diff_auth.py 的分工：那个覆盖 /auth/* 的登录链路，本脚本覆盖**登录之后**的第一跳——
认证中间件（Python 的 get_current_user / Go 的 auth.NewMiddleware）与它守卫的头两个端点。

覆盖三类用例：
  1. **未认证路径**：无 token、scheme 不匹配、空 token、结构非法、签名被篡改、
     尾斜杠 307、方法不匹配 405。这一类的响应文案是契约的一部分（前端直接展示），
     必须逐字一致。
  2. **clientid 一致性**：token 合法但请求携带的 clientid 与之不符 → 拒绝。
  3. **成功路径**：两端各自登录后调用 getInfo / getRouters，比对响应体。
     getRouters 完全由种子数据决定，做**严格**比对；
     getInfo 只有 loginDate 因两次登录时刻不同而需归一化。

用法：
    python test/contract/diff_system.py [--py http://127.0.0.1:8012] [--go http://127.0.0.1:8011]

前置：两端都在运行，且中间件（MySQL/Redis）可达。
"""

import argparse
import base64
import json
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

CLIENT_ID = "e5cd7e4891bf95d1d19206ce24a7b32e"  # 种子数据里的 pc 客户端
ADMIN_USER = "admin"
ADMIN_PWD = "admin123"

GET_INFO = "/system/user/getInfo"
GET_ROUTERS = "/system/menu/getRouters"

# 各端的 Redis 键前缀（三端隔离的硬约束）
REDIS_PREFIX = {"py": "walnut-seed-python", "go": "walnut-seed-go"}
REDIS_CONTAINER = "walnut-redis"

# 必然不同、需归一化的字段
VOLATILE = {
    "uuid": "<UUID>",
    "img": "<IMG>",
    "access_token": "<TOKEN>",
    # loginDate 由「本端最近一次登录」写入，两端登录时刻必然差几秒
    "loginDate": "<TS>",
}


def call(base, method, path, body=None, headers=None, send_clientid=True):
    """发请求，返回 (status, headers, raw_text)。"""
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base.rstrip("/") + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if send_clientid:
        req.add_header("clientid", CLIENT_ID)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
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
    """解出 JWT 的 claims（不验签，只看内容）。"""
    parts = token.split(".")
    if len(parts) != 3:
        return None

    def b64url(s):
        s += "=" * (-len(s) % 4)
        return json.loads(base64.urlsafe_b64decode(s))

    try:
        return b64url(parts[1])
    except Exception:  # noqa: BLE001
        return None


def tamper_signature(token):
    """保留结构、破坏签名：用于验证「签名错误」分支的文案。"""
    parts = token.split(".")
    if len(parts) != 3 or not parts[2]:
        return token
    sig = parts[2]
    flipped = ("A" if sig[-1] != "A" else "B")
    parts[2] = sig[:-1] + flipped
    return ".".join(parts)


# ==================== 用例执行 ====================

RESULTS = []


def record(name, ok, detail=""):
    RESULTS.append((name, ok, detail))
    print(f"  {'[OK]  ' if ok else '[FAIL]'} {name}" + (f"\n         {detail}" if detail and not ok else ""))


def compare(name, method, path, body=None, headers=None, send_clientid=True):
    """对同一路径请求两端并比对（状态码 + 归一化后的响应体）。"""
    ps, _, pb = call(PY, method, path, body, headers, send_clientid)
    gs, _, gb = call(GO, method, path, body, headers, send_clientid)

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


def login(side, base):
    """在指定后端上走完一次成功登录，返回 (access_token, 错误信息)。"""
    st, _, raw = call(base, "GET", "/auth/code")
    if st != 200:
        return None, f"/auth/code 返回 HTTP {st}"
    data = (json.loads(raw).get("data") or {})
    if not data.get("captchaEnabled"):
        return None, "验证码未开启，无法测试该路径"
    uuid_str = data.get("uuid")
    if not uuid_str:
        return None, "/auth/code 未返回 uuid"

    answer = redis_get(side, f"global:captcha_codes:{uuid_str}")
    if answer is None:
        return None, f"Redis 中读不到验证码答案（key={REDIS_PREFIX[side]}:global:captcha_codes:{uuid_str}）"

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
    env = json.loads(raw)
    if env.get("code") != 200:
        return None, f"/auth/login 业务码 {env.get('code')}: {env.get('msg')}"
    token = (env.get("data") or {}).get("access_token")
    if not token:
        return None, "/auth/login 未返回 access_token"
    return token, None


def authed(token):
    return {"Authorization": f"Bearer {token}"}


def compare_authed(name, path, extra=None, send_clientid=True, strict=False):
    """两端各自用**自己的** token 请求同一端点并比对，返回 (py_body, go_body)。

    ⚠️ 必须各用各的 token：两端 SECRET_KEY 不同、Redis 会话也各自独立，
    把 py 的 token 发给 go 会先撞上"签名无效"，根本走不到要测的分支。
    （这个坑第一版脚本踩过：clientid 用例误用同一个 token，
    两端都返回"无效认证"，看着"一致"其实什么也没测到。）

    strict=True 时用**原始文本**比对（含键序、不做归一化），
    适用于输出完全由种子数据决定、不含易变字段的端点（如 getRouters）。
    """
    py_headers = {**authed(PY_TOKEN), **(extra or {})}
    go_headers = {**authed(GO_TOKEN), **(extra or {})}
    py_s, _, py_b = call(PY, "GET", path, headers=py_headers, send_clientid=send_clientid)
    go_s, _, go_b = call(GO, "GET", path, headers=go_headers, send_clientid=send_clientid)

    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    if strict:
        if py_b.rstrip("\n") != go_b.rstrip("\n"):
            diffs.append(
                "响应体不一致（原始文本比对，含键序）:\n"
                f"           py = {py_b[:2000]}\n"
                f"           go = {go_b[:2000]}"
            )
    elif canon(py_b) != canon(go_b):
        diffs.append(f"响应体:\n           py = {canon(py_b)}\n           go = {canon(go_b)}")
    record(name, not diffs, "\n           ".join(diffs))
    return py_b, go_b


def compare_authed_tampered(name):
    """比对「签名被篡改」分支：两端各自篡改自己的 token（结果都应是签名无效）。"""
    py_s, _, py_b = call(PY, "GET", GET_INFO,
                         headers={"Authorization": "Bearer " + tamper_signature(PY_TOKEN)})
    go_s, _, go_b = call(GO, "GET", GET_INFO,
                         headers={"Authorization": "Bearer " + tamper_signature(GO_TOKEN)})
    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    if canon(py_b) != canon(go_b):
        diffs.append(f"响应体:\n           py = {canon(py_b)}\n           go = {canon(go_b)}")
    record(name, not diffs, "\n           ".join(diffs))


def compare_authed_querystring(name):
    """比对 SSE 形态：token 与 clientid 都放查询参数，不带任何自定义请求头。"""
    def qs_call(base, token):
        q = urllib.parse.urlencode({"Authorization": f"Bearer {token}", "clientid": CLIENT_ID})
        return call(base, "GET", GET_INFO + "?" + q, send_clientid=False)

    py_s, _, py_b = qs_call(PY, PY_TOKEN)
    go_s, _, go_b = qs_call(GO, GO_TOKEN)
    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    if canon(py_b) != canon(go_b):
        diffs.append(f"响应体:\n           py = {canon(py_b)}\n           go = {canon(go_b)}")
    record(name, not diffs, "\n           ".join(diffs))


def main():
    global PY, GO
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    PY, GO = args.py, args.go

    print("=" * 82)
    print(f"P3 窄切片契约比对：Python({PY})  vs  Go({GO})")
    print("=" * 82)

    # ---------- 1. 未认证路径（无需登录即可比对）----------
    print("\n--- 1. 认证中间件的失败分支 ---")
    unauth_cases = [
        ("无 Authorization 头", {}),
        ("scheme 不是 Bearer（裸 token）", {"Authorization": "abc"}),
        ("scheme 小写 bearer（应通过 scheme 校验）", {"Authorization": "bearer not.a.jwt"}),
        ("Bearer 后无 token", {"Authorization": "Bearer"}),
        ("结构非法：not.a.jwt", {"Authorization": "Bearer not.a.jwt"}),
        ("结构非法：单段", {"Authorization": "Bearer abc"}),
        ("结构非法：两段", {"Authorization": "Bearer a.b"}),
        ("header 段不是合法 base64", {"Authorization": "Bearer !!!.@@@.###"}),
        ("alg=none 的伪造 token", {"Authorization": "Bearer " + forged({"alg": "none", "typ": "JWT"})}),
        ("alg 未知（HS999）", {"Authorization": "Bearer " + forged({"alg": "HS999", "typ": "JWT"})}),
        ("header 缺 alg 字段", {"Authorization": "Bearer " + forged({"typ": "JWT"})}),
    ]
    for name, headers in unauth_cases:
        compare(f"getInfo · {name}", "GET", GET_INFO, headers=headers)

    compare("getRouters · 无 token", "GET", GET_ROUTERS)
    compare("getInfo · 尾斜杠（307 重定向）", "GET", GET_INFO + "/")
    compare("getInfo · 方法不匹配（405）", "POST", GET_INFO)

    # ---------- 2/3. 需要先登录 ----------
    global PY_TOKEN, GO_TOKEN
    print("\n--- 2. 登录并取真 token ---")
    PY_TOKEN, py_err = login("py", PY)
    GO_TOKEN, go_err = login("go", GO)
    if py_err or go_err:
        record("两端登录", False, f"py={py_err} go={go_err}")
        return _summary()
    record("两端登录（各自取得 access_token）", True)

    print("\n--- 3. clientid 一致性校验 ---")
    compare_authed("getInfo · 完全不带 clientid", GET_INFO, send_clientid=False)
    compare_authed("getInfo · clientid 值不匹配", GET_INFO, extra={"clientid": "wrong-client"})

    print("\n--- 4. 签名被篡改（结构合法，两端各自篡改自己的 token）---")
    compare_authed_tampered("getInfo · 签名篡改")

    print("\n--- 5. token 与 clientid 都走查询参数（SSE 形态，EventSource 设不了请求头）---")
    compare_authed_querystring("getInfo · token/clientid 走查询参数")

    # ---------- 6. 成功路径 ----------
    print("\n--- 6. 成功路径：getInfo ---")
    py_b, go_b = compare_authed("getInfo · 响应体逐字段一致", GET_INFO)

    # 额外的形状断言：不依赖两端一致，而是各自都要满足契约
    for side, raw in (("py", py_b), ("go", go_b)):
        shape = []
        try:
            env = json.loads(raw)
        except json.JSONDecodeError:
            record(f"getInfo · {side} 响应不是合法 JSON", False, raw[:200])
            continue
        data = env.get("data") or {}
        if sorted(data.keys()) != ["permissions", "roles", "user"]:
            shape.append(f"data 键集 = {sorted(data.keys())}，期望 ['permissions','roles','user']")
        u = data.get("user") or {}
        expect_user_keys = [
            "avatar", "createTime", "deptId", "deptName", "email", "id", "loginDate",
            "loginIp", "nickName", "phonenumber", "postIds", "remark", "roleId",
            "roleIds", "roles", "sex", "status", "userName", "userType",
        ]
        if sorted(u.keys()) != expect_user_keys:
            shape.append(f"user 键集不符:\n             实际 = {sorted(u.keys())}\n             期望 = {expect_user_keys}")
        # getInfo 里这三个字段恒为 null（只有 GET /user 与 /user/{userId} 才填）
        for k in ("roleIds", "postIds", "roleId"):
            if u.get(k) is not None:
                shape.append(f"user.{k} 应为 null，实际 {u.get(k)!r}")
        # ⚠️ JSInt 是**条件**序列化：|v| >= 2^53-1 才转字符串。
        # 种子数据里 admin 的 id=1、deptId=103 都是小整数，必须保持**数字**——
        # 若被 blanket 转成字符串，前端所有 `=== 0` / `=== 1` 判断都会失效。
        for k in ("id", "deptId"):
            v = u.get(k)
            if v is not None and not isinstance(v, int):
                shape.append(f"user.{k} 是小整数，应为数字而非字符串，实际 {v!r}")
        # 时间字段必须是 "yyyy-MM-dd HH:mm:ss" 字符串
        for k in ("createTime", "loginDate"):
            v = u.get(k)
            if v is not None and not re.fullmatch(r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}", v):
                shape.append(f"user.{k} 格式应为 'yyyy-MM-dd HH:mm:ss'，实际 {v!r}")
        if u.get("userName") != ADMIN_USER:
            shape.append(f"user.userName = {u.get('userName')!r}，期望 {ADMIN_USER!r}")
        if data.get("permissions") != ["*:*:*"]:
            shape.append(f"permissions = {data.get('permissions')!r}，admin 期望 ['*:*:*']")
        if data.get("roles") != ["superadmin"]:
            shape.append(f"roles = {data.get('roles')!r}，admin 期望 ['superadmin']")
        record(f"getInfo · {side} 响应形状符合契约", not shape, "\n           ".join(shape))

    print("\n--- 7. 成功路径：getRouters（严格比对，种子数据完全确定）---")
    # 输出不含任何易变字段，故用**原始文本**（不归一化、不排序）做最严格比对
    py_b, go_b = compare_authed("getRouters · 响应体逐字节一致", GET_ROUTERS, strict=True)

    for side, raw in (("py", py_b), ("go", go_b)):
        try:
            env = json.loads(raw)
        except json.JSONDecodeError:
            record(f"getRouters · {side} 响应不是合法 JSON", False, raw[:200])
            continue
        data = env.get("data")
        shape = []
        if not isinstance(data, list):
            shape.append(f"data 应为数组，实际 {type(data).__name__}")
        elif len(data) == 0:
            shape.append("data 为空数组：admin 是超管，应能取到全部菜单路由")
        else:
            # 顶级路由应当是目录形态：alwaysShow + redirect=noRedirect + 有 children
            tops = [r for r in data if isinstance(r, dict)]
            with_children = [r for r in tops if r.get("children")]
            if not with_children:
                shape.append("没有任何顶级路由带 children，菜单树构建可能有问题")
            for r in tops:
                if r.get("children") and r.get("alwaysShow") is not True:
                    shape.append(f"带 children 的目录路由 {r.get('name')!r} 缺 alwaysShow=true")
                if r.get("children") and r.get("redirect") != "noRedirect":
                    shape.append(f"带 children 的目录路由 {r.get('name')!r} 的 redirect 应为 noRedirect")
                # meta 必须全量输出 5 个键（与 Router 的空值过滤规则不同）
                meta = r.get("meta")
                if meta is not None and sorted(meta.keys()) != ["activeMenu", "icon", "link", "noCache", "title"]:
                    shape.append(f"路由 {r.get('name')!r} 的 meta 键集 = {sorted(meta.keys())}，应为全量 5 键")
            print(f"         （{side} 顶级路由 {len(tops)} 个，其中 {len(with_children)} 个带子路由）")
        record(f"getRouters · {side} 路由树形态符合契约", not shape, "\n           ".join(shape))

    return _summary()


def forged(header, payload=None):
    """构造一个 header 可自定义的伪造 token（签名段是占位值，本就验不过）。

    用于比对两端在「算法不被允许」这一类的文案归属：PyJWT 抛
    InvalidAlgorithmError（落第三个 except 分支），golang-jwt 却把它和
    「签名错误」报成同一个 ErrTokenSignatureInvalid —— 必须实测确认对齐。
    """
    def b64(obj):
        return base64.urlsafe_b64encode(json.dumps(obj, separators=(",", ":")).encode()).rstrip(b"=").decode()

    if payload is None:
        payload = {"sub": "x", "exp": 9999999999}
    return f"{b64(header)}.{b64(payload)}.{b64('sig')}"


def _summary():
    print("\n" + "=" * 82)
    passed = sum(1 for _, ok, _ in RESULTS if ok)
    failed = len(RESULTS) - passed
    print(f"通过 {passed} / 失败 {failed}  （共 {len(RESULTS)} 项）")
    print("=" * 82)
    return 1 if failed else 0


PY = GO = ""
PY_TOKEN = GO_TOKEN = ""

if __name__ == "__main__":
    sys.exit(main())
