"""P0 修复回归 diff：覆盖既有脚本盲区（file 参数/形状、user 出参/校验、
resource sse-close/WS、monitor 操作日志副作用、unlock Redis 键）。

用法：
    python test/contract/diff_p0_fixes.py [--py http://127.0.0.1:8012] --go http://127.0.0.1:8011]

前置：两端运行中，MySQL/Redis 可达（docker exec walnut-redis 可用）。
退出码：全过 0，有差异 1。
"""

import argparse
import io
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid

import diff_system as ds

# 1x1 透明 PNG（73 字节）
PNG = bytes.fromhex(
    "89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489"
    "0000000d49444154789c626001000000ffff03000006000557bfabd40000000049454e44ae426082"
)


def hget(hd, key):
    """uvicorn 发小写头、gin 发规范头，统一按小写取。"""
    for k, v in hd.items():
        if k.lower() == key.lower():
            return v
    return None

RESULTS = []


def record(name, ok, detail=""):
    RESULTS.append((name, ok))
    print(f"  [{'OK' if ok else 'FAIL'}] {name}" + (f"\n         {detail}" if not ok and detail else ""))


def call(base, tok, method, path, body=None, headers=None, raw_body=None, content_type=None):
    data = raw_body if raw_body is not None else (json.dumps(body).encode() if body is not None else None)
    req = urllib.request.Request(base.rstrip("/") + path, data=data, method=method)
    if data is not None:
        req.add_header("Content-Type", content_type or "application/json")
    if tok:
        req.add_header("Authorization", "Bearer " + tok)
    req.add_header("clientid", ds.CLIENT_ID)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, dict(r.headers), r.read()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read()
    except Exception as e:  # noqa: BLE001
        return None, {}, f"<连接失败: {e}>".encode()


def jbody(raw):
    try:
        return json.loads(raw)
    except Exception:  # noqa: BLE001
        return None


def canon(obj):
    return json.dumps(obj, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def norm_volatile(obj):
    if isinstance(obj, dict):
        out = {}
        for k, v in obj.items():
            if k in ("loginDate", "loginTime", "operTime", "createTime", "updateTime", "accessTime"):
                out[k] = "<TS>" if v is not None else None
            elif k in ("url", "imgUrl", "avatar") and isinstance(v, str) and "/upload/" in v:
                out[k] = "<URL>"
            elif k in ("id", "infoId", "operId", "userId", "deptId", "postId", "roleId") and isinstance(v, str) and v.isdigit() and len(v) > 15:
                out[k] = "<ID>"
            else:
                out[k] = norm_volatile(v)
        return out
    if isinstance(obj, list):
        return [norm_volatile(i) for i in obj]
    return obj


def compare_json(name, py_raw, go_raw, normalize=True):
    pj, gj = jbody(py_raw), jbody(go_raw)
    if pj is None or gj is None:
        record(name, False, f"py={py_raw[:200]!r} go={go_raw[:200]!r}")
        return
    if normalize:
        pj, gj = norm_volatile(pj), norm_volatile(gj)
    record(name, canon(pj) == canon(gj), f"py={canon(pj)[:400]}\n           go={canon(gj)[:400]}")


def multipart(fields, files):
    boundary = uuid.uuid4().hex
    buf = io.BytesIO()
    for k, v in fields.items():
        buf.write(f"--{boundary}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode())
    for k, (fname, content) in files.items():
        buf.write(f"--{boundary}\r\nContent-Disposition: form-data; name=\"{k}\"; filename=\"{fname}\"\r\nContent-Type: application/octet-stream\r\n\r\n".encode())
        buf.write(content)
        buf.write(b"\r\n")
    buf.write(f"--{boundary}--\r\n".encode())
    return buf.getvalue(), f"multipart/form-data; boundary={boundary}"


def redis_cmd(*args):
    p = subprocess.run(["docker", "exec", "walnut-redis", "redis-cli", *args],
                       capture_output=True, text=True, encoding="utf-8")
    return p.stdout.strip()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    ds.PY, ds.GO = args.py, args.go

    for side, base in (("py", args.py), ("go", args.go)):
        tok, err = ds.login(side, base)
        if err:
            print(f"[FATAL] {side} 登录失败: {err}")
            return 2
        if side == "py":
            ds.PY_TOKEN = tok
        else:
            ds.GO_TOKEN = tok

    print("\n--- 1. file 模块 ---")
    urls = {}
    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        body, ct = multipart({}, {"file": ("p0test.png", PNG)})
        st, hd, raw = call(base, tok, "POST", "/common/file/upload", raw_body=body, content_type=ct)
        j = jbody(raw) or {}
        data = j.get("data") or {}
        urls[side] = data.get("url", "")
        ok = st == 200 and j.get("code") == 200 and j.get("msg") == "上传成功" and "original_filename" in data and data.get("original_filename") == "p0test.png"
        record(f"upload · {side} 响应形状（original_filename + msg）", ok, f"HTTP {st}: {raw[:200]!r}")

    for params, label in (("", "缺参"), ("file_url=/upload/p0-nonexistent-xyz", "不存在")):
        pys, _, pyr = call(args.py, ds.PY_TOKEN, "GET", "/common/file/download" + ("?" + params if params else ""))
        gos, _, gor = call(args.go, ds.GO_TOKEN, "GET", "/common/file/download" + ("?" + params if params else ""))
        compare_json(f"download · {label} 两端一致", pyr, gor)
        record(f"download · {label} HTTP 状态一致", pys == gos, f"py={pys} go={gos}")

    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        st, hd, raw = call(base, tok, "GET", "/common/file/download?file_url=" + urllib.request.quote(urls[side], safe=""))
        ok = st == 200 and raw == PNG and hget(hd, "X-Content-Type-Options") == "nosniff" and "attachment" in (hget(hd, "Content-Disposition") or "")
        record(f"download · {side} 按 file_url 取回原文 + 安全头", ok, f"HTTP {st} len={len(raw)} hd={hd.get('Content-Disposition')}")

    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        st, _, raw = call(base, tok, "DELETE", "/common/file/delete?file_url=" + urllib.request.quote(urls[side], safe=""))
        j = jbody(raw) or {}
        record(f"delete · {side} msg=删除成功", st == 200 and j.get("code") == 200 and j.get("msg") == "删除成功", f"HTTP {st}: {raw[:150]!r}")

    for path in ("/upload/p0-nonexistent-xyz", "/upload/"):
        pys, _, pyr = call(args.py, None, "GET", path)
        gos, _, gor = call(args.go, None, "GET", path)
        record(f"{path} · HTTP 状态一致", pys == gos, f"py={pys} go={gos}")
        compare_json(f"{path} · 信封一致", pyr, gor)

    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        body, ct = multipart({}, {"file": ("evil.svg", b"<svg xmlns='http://www.w3.org/2000/svg'/>")})
        st, _, raw = call(base, tok, "POST", "/common/file/upload", raw_body=body, content_type=ct)
        j = jbody(raw) or {}
        record(f"upload · {side} svg 被拒（白名单对齐）", j.get("code") == 400 and "svg" in (j.get("msg") or ""), f"HTTP {st}: {raw[:150]!r}")

    print("\n--- 2. user 出参与校验 ---")
    for path in ("/system/user/", "/system/user/1", "/system/user/999999999", "/system/user/authRole/1",
                 "/system/user/profile", "/system/user/optionselect", "/system/user/deptTree"):
        pys, _, pyr = call(args.py, ds.PY_TOKEN, "GET", path)
        gos, _, gor = call(args.go, ds.GO_TOKEN, "GET", path)
        record(f"GET {path} · HTTP 一致", pys == gos, f"py={pys} go={gos}")
        compare_json(f"GET {path} · 响应体（易变字段归一化）", pyr, gor)

    pys, _, pyr = call(args.py, ds.PY_TOKEN, "GET", "/system/user/abc")
    gos, _, gor = call(args.go, ds.GO_TOKEN, "GET", "/system/user/abc")
    # abc 不匹配 {user_id:int} 但命中 DELETE /{user_ids} 路径 → Starlette 405
    record("GET /system/user/abc · 均 HTTP 405", pys == 405 and gos == 405, f"py={pys} go={gos}")
    compare_json("GET /system/user/abc · 405 信封", pyr, gor)
    pys, _, pyr = call(args.py, ds.PY_TOKEN, "GET", "/system/user/authRole/abc")
    gos, _, gor = call(args.go, ds.GO_TOKEN, "GET", "/system/user/authRole/abc")
    record("GET /system/user/authRole/abc · 均 HTTP 404", pys == 404 and gos == 404, f"py={pys} go={gos}")
    compare_json("GET /system/user/authRole/abc · 404 信封", pyr, gor)

    marker = str(int(time.time()))[-6:]
    for method, path, body, label in (
        ("POST", "/system/user", {}, "空 body 校验拼接"),
        ("PUT", "/system/user", {"remark": "p0fix" + marker}, "PUT 校验拼接（含 id）"),
        ("PUT", "/system/user", {"id": 1, "userName": "admin", "nickName": "n", "roleIds": [], "remark": "p0fix" + marker}, "roleIds 空数组"),
        ("POST", "/system/user", {"userName": "ab", "nickName": "n", "password": "weak", "remark": "p0fix" + marker}, "弱密码"),
        ("PUT", "/system/user/resetPwd", {"id": 1}, "resetPwd 缺密码"),
        ("PUT", "/system/user/profile/updatePwd", {}, "updatePwd 双缺"),
    ):
        pys, _, pyr = call(args.py, ds.PY_TOKEN, method, path, body)
        gos, _, gor = call(args.go, ds.GO_TOKEN, method, path, body)
        compare_json(f"{method} {path} · {label}", pyr, gor)
        time.sleep(0.1)

    print("\n--- 3. resource ---")
    for q in ("user_id=abc", "user_id=1&token=x", ""):
        pys, _, pyr = call(args.py, None, "GET", "/resource/sse/close" + ("?" + q if q else ""))
        gos, _, gor = call(args.go, None, "GET", "/resource/sse/close" + ("?" + q if q else ""))
        compare_json(f"sse/close?{q or '<无参>'} · 响应一致", pyr, gor)

    try:
        import asyncio

        import websockets

        async def ws_probe(base):
            url = base.replace("http", "ws") + "/resource/websocket?token=invalid"
            try:
                async with websockets.connect(url):
                    return 101, None
            except Exception as e:  # noqa: BLE001
                resp = getattr(e, "response", None)
                return (resp.status_code if resp is not None else None), type(e).__name__

        pys, pye = asyncio.run(ws_probe(args.py))
        gos, goe = asyncio.run(ws_probe(args.go))
        record("WS 无效 token · 均拒绝握手 HTTP 403", pys == 403 and gos == 403, f"py={pys}({pye}) go={gos}({goe})")
    except ImportError:
        print("  [SKIP] websockets 库不可用")

    print("\n--- 4. monitor 操作日志副作用 ---")
    # 两端各发一次同标题 notice，再比对各自 operlog 里该行的 title/business_type/method 列
    notice_title = "P0回归通知" + marker
    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        call(base, tok, "POST", "/system/notice", {"noticeTitle": notice_title, "noticeType": "1", "noticeContent": "x", "status": "0"})
    time.sleep(1.5)  # 等异步写入
    rows = {}
    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        st, _, raw = call(base, tok, "GET", "/monitor/operlog/list?pageSize=50")
        j = jbody(raw) or {}
        found = [r for r in (j.get("data") or {}).get("rows", []) if notice_title in json.dumps(r, ensure_ascii=False)]
        rows[side] = found[0] if found else None
    if rows["py"] and rows["go"]:
        keys = ("title", "businessType", "method", "requestMethod", "operUrl", "status", "operatorType")
        pyv = {k: rows["py"].get(k) for k in keys}
        gov = {k: rows["go"].get(k) for k in keys}
        record("operlog · notice 行的 title/businessType/method 等列一致", canon(pyv) == canon(gov), f"py={pyv}\n           go={gov}")
        record("operlog · jsonResult 非空且为 JSON 信封", bool(rows["go"].get("jsonResult")) and jbody(rows["go"]["jsonResult"].encode()) is not None, f"go jsonResult={str(rows['go'].get('jsonResult'))[:120]}")
    else:
        record("operlog · 两端均采集到 notice 行", False, f"py={bool(rows['py'])} go={bool(rows['go'])}")
    # 清理通知
    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        st, _, raw = call(base, tok, "GET", "/system/notice/list?noticeTitle=" + urllib.request.quote(notice_title))
        j = jbody(raw) or {}
        for r in (j.get("data") or {}).get("rows", []):
            call(base, tok, "DELETE", f"/system/notice/{r['id']}")

    # unlock：人工注入错误计数键 → 调解锁 → 键必须被删除（回归 global: 前缀错配 bug）
    for side, prefix in (("py", "walnut-seed-python"), ("go", "walnut-seed-go")):
        base, tok = (args.py, ds.PY_TOKEN) if side == "py" else (args.go, ds.GO_TOKEN)
        key = f"{prefix}:pwd_err_cnt:p0unlock{marker}:127.0.0.1"
        redis_cmd("SET", key, "3", "EX", "300")
        st, _, raw = call(base, tok, "GET", f"/monitor/logininfor/unlock/p0unlock{marker}")
        time.sleep(0.3)
        gone = redis_cmd("EXISTS", key) == "0"
        record(f"unlock · {side} 真实删除 pwd_err_cnt 键", gone and (jbody(raw) or {}).get("code") == 200, f"HTTP {st} exists={redis_cmd('EXISTS', key)}")

    print("\n--- 5. 导出 query 过滤 ---")
    exports = {}
    for side, base, tok in (("py", args.py, ds.PY_TOKEN), ("go", args.go, ds.GO_TOKEN)):
        st, hd, raw = call(base, tok, "POST", "/system/user/export?userName=admin", raw_body=b"", content_type="application/x-www-form-urlencoded")
        ok = st == 200 and "spreadsheet" in (hget(hd, "Content-Type") or "") and len(raw) > 2000
        record(f"user/export?userName=admin · {side} 返回 xlsx", ok, f"HTTP {st} ct={hget(hd, 'Content-Type')} len={len(raw)} body={raw[:120]!r}")
        exports[side] = raw
    try:
        from openpyxl import load_workbook

        def rows_of(blob):
            if not blob.startswith(b"PK"):
                return [("<非xlsx>", blob[:120])]
            # 不用 read_only：excelize 写的 dimension 元素会让 openpyxl
            # read_only 模式按 A1 截断单元格（内容本身正确，常规模式可验证）
            ws = load_workbook(io.BytesIO(blob), data_only=True).active
            out = []
            for row in ws.iter_rows(values_only=True):
                out.append(tuple("<V>" if isinstance(c, str) and (len(c) > 15 and c.isdigit() or "-" in c and ":" in c) else c for c in row))
            return out

        pr, gr = rows_of(exports["py"]), rows_of(exports["go"])
        record("user/export · 表头+数据行一致（id/时间归一化）", pr == gr, f"py={pr[:3]}\n           go={gr[:3]}")
    except ImportError:
        print("  [SKIP] openpyxl 不可用")

    print("\n--- 6. auth body 多错误收集 / export 校验原文 ---")
    for body, label in (
        ({"username": 123, "password": "x"}, "login 类型错误+缺参+长度（多错误拼接）"),
        ({"username": "admin", "password": 456}, "login password 类型错误"),
        ({}, "login 空 body"),
    ):
        pys, _, pyr = call(args.py, None, "POST", "/auth/login", body)
        gos, _, gor = call(args.go, None, "POST", "/auth/login", body)
        compare_json(f"POST /auth/login · {label}", pyr, gor)
    for q in ("deptId=abc", "beginTime=abc&deptId=xyz"):
        pys, _, pyr = call(args.py, ds.PY_TOKEN, "POST", "/system/user/export?" + q, raw_body=b"", content_type="application/x-www-form-urlencoded")
        gos, _, gor = call(args.go, ds.GO_TOKEN, "POST", "/system/user/export?" + q, raw_body=b"", content_type="application/x-www-form-urlencoded")
        compare_json(f"user/export?{q} · pydantic ValidationError 原文", pyr, gor)

    passed = sum(1 for _, ok in RESULTS if ok)
    failed = len(RESULTS) - passed
    print("\n" + "=" * 70)
    print(f"通过 {passed} / 失败 {failed}  （共 {len(RESULTS)} 项）")
    print("=" * 70)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())