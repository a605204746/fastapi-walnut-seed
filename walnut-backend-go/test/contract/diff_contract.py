"""契约 diff 跑分器（P0 骨架版）。

对同一路径分别请求 Python 与 Go 后端，归一化易变字段后**逐字节比对** JSON 响应。
这是 docs/go/00-Go后端实施计划.md §4.2 描述的核心工具的最小可用实现。

用法：
    python diff_contract.py [--py http://127.0.0.1:8012] [--go http://127.0.0.1:8011]

归一化规则（§4.3）：请求耗时、时间戳、uptime、latency 等**必然不同**的字段
替换为占位符，但保持「同值同占位」以便验证引用完整性。
"""

import argparse
import json
import sys
import urllib.error
import urllib.request

# 易变字段：值本身无意义，只验证「存在且类型正确」
VOLATILE_KEYS = {
    "timestamp": "<TS>",
    "uptime_seconds": "<UPTIME>",
    "latency_ms": "<LATENCY>",
}

# 容差字段：两端读的是**同一个**真实资源，数值应当接近，而不是无条件归一化。
#
# ⚠️ disk_usage 曾放在 VOLATILE_KEYS 里做无条件归一化，结果**掩盖了一个真实偏差**：
# Python 端 shutil.disk_usage("/") 在 Windows 上经 os.statvfs 模拟能返回真实百分比（实测 94.3），
# 而 Go 端 disk_other.go 当时对 Windows 直接返回 -1.0。归一化后两端都变成 <DISK>，diff 全绿。
#
# 教训：归一化只应用于「本质上不可能相同」的字段（时间戳、耗时、自增 ID）。
# 凡是两端读同一真实来源的数值，必须用容差比对——否则跑分器只会给出虚假的安全感。
TOLERANCE_KEYS = {
    "disk_usage": 1.0,  # 文件系统占用随时间漂移，且 Python 端 round(x, 1)，容 1 个百分点
}

# 待比对的路由清单（P0 只有健康探针与根路径；P2/P3 起逐步扩充）
CASES = [
    ("GET", "/common/health/check"),
    ("GET", "/common/health/live"),
    ("GET", "/common/health/ready"),
    ("GET", "/"),
    # 404 / 405 也是契约的一部分
    ("GET", "/nonexistent/path"),
    ("POST", "/common/health/check"),
    # R16：尾斜杠行为
    ("GET", "/common/health/check/"),
]


def fetch(base: str, method: str, path: str):
    """返回 (http_status, headers, raw_body)。连接失败返回 None。"""
    req = urllib.request.Request(base.rstrip("/") + path, method=method)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, dict(resp.headers), resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        # 4xx/5xx 也要比对：契约要求它们带信封体
        return e.code, dict(e.headers), e.read().decode("utf-8", "replace")
    except Exception as e:  # noqa: BLE001
        return None, {}, f"<连接失败: {e}>"


def normalize(obj):
    """递归把易变字段替换为占位符。

    容差字段也在此替换为占位符（让逐字节比对跳过它们），
    但**必须**同时由 check_tolerance 做数值容差校验，否则等于没检查。
    """
    if isinstance(obj, dict):
        out = {}
        for k, v in obj.items():
            if k in VOLATILE_KEYS:
                # 保留「是否为 null」这一信息：latency_ms=null 与 latency_ms=0.5 语义不同
                out[k] = None if v is None else VOLATILE_KEYS[k]
            elif k in TOLERANCE_KEYS:
                out[k] = None if v is None else f"<TOL:{k}>"
            else:
                out[k] = normalize(v)
        return out
    if isinstance(obj, list):
        return [normalize(i) for i in obj]
    return obj


def check_tolerance(py_obj, go_obj, path="$", out=None):
    """递归校验容差字段：两端数值差必须在允许范围内。

    与 normalize 配套使用——normalize 把这些字段从逐字节比对里摘出去，
    本函数负责把它们真正比一遍。缺了这一步，容差字段就等于完全不校验。
    """
    if out is None:
        out = []

    if isinstance(py_obj, dict) and isinstance(go_obj, dict):
        for k in TOLERANCE_KEYS:
            if k in py_obj or k in go_obj:
                pv, gv = py_obj.get(k), go_obj.get(k)
                loc = f"{path}.{k}"
                if (pv is None) != (gv is None):
                    out.append(f"{loc}: 一端为 null 另一端不是 (py={pv!r} go={gv!r})")
                elif pv is not None:
                    try:
                        delta = abs(float(pv) - float(gv))
                    except (TypeError, ValueError):
                        out.append(f"{loc}: 非数值 (py={pv!r} go={gv!r})")
                        continue
                    allowed = TOLERANCE_KEYS[k]
                    if delta > allowed:
                        out.append(
                            f"{loc}: py={pv} go={gv} 差 {delta:.2f} 超出容差 {allowed}"
                            f"（两端读同一文件系统，不应差这么多）"
                        )
        for k in py_obj.keys() & go_obj.keys():
            if k not in TOLERANCE_KEYS:
                check_tolerance(py_obj[k], go_obj[k], f"{path}.{k}", out)
    elif isinstance(py_obj, list) and isinstance(go_obj, list):
        for i, (pi, gi) in enumerate(zip(py_obj, go_obj)):
            check_tolerance(pi, gi, f"{path}[{i}]", out)
    return out


def canon(raw: str) -> str:
    """把响应体规范化为可逐字节比对的字符串。

    非 JSON（如根路径的 JSON 字符串、错误页）按原文比对，仅去掉末尾换行。
    """
    text = raw.rstrip("\n")
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError:
        return text
    return json.dumps(normalize(parsed), ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012", help="Python 后端基址")
    ap.add_argument("--go", default="http://127.0.0.1:8011", help="Go 后端基址")
    args = ap.parse_args()

    print("=" * 78)
    print(f"契约 diff：Python({args.py})  vs  Go({args.go})")
    print("=" * 78)

    passed = failed = skipped = 0
    for method, path in CASES:
        py_status, py_headers, py_body = fetch(args.py, method, path)
        go_status, go_headers, go_body = fetch(args.go, method, path)

        if py_status is None or go_status is None:
            print(f"[SKIP] {method:5s} {path:28s} 有一端不可达 (py={py_status}, go={go_status})")
            skipped += 1
            continue

        diffs = []
        if py_status != go_status:
            diffs.append(f"HTTP 状态码: py={py_status} go={go_status}")

        py_canon, go_canon = canon(py_body), canon(go_body)
        if py_canon != go_canon:
            diffs.append(f"响应体:\n         py = {py_canon}\n         go = {go_canon}")

        # 容差字段单独校验（normalize 已把它们从逐字节比对中摘出，不补这一步等于没检查）
        try:
            py_parsed = json.loads(py_body)
            go_parsed = json.loads(go_body)
            diffs.extend(check_tolerance(py_parsed, go_parsed))
        except json.JSONDecodeError:
            pass  # 非 JSON 响应（如根路径的 JSON 字符串）无容差字段可言

        # Content-Type 也属于契约（前端拦截器会看）
        py_ct = (py_headers.get("Content-Type") or "").split(";")[0].strip()
        go_ct = (go_headers.get("Content-Type") or "").split(";")[0].strip()
        if py_ct and go_ct and py_ct != go_ct:
            diffs.append(f"Content-Type: py={py_ct!r} go={go_ct!r}")

        # encrypt-key 必须在 CORS expose headers 里，否则前端跨域读不到
        for name, hdrs in (("py", py_headers), ("go", go_headers)):
            expose = hdrs.get("Access-Control-Expose-Headers", "")
            if expose and "encrypt-key" not in expose:
                diffs.append(f"{name} 的 Access-Control-Expose-Headers 缺少 encrypt-key: {expose!r}")

        if diffs:
            failed += 1
            print(f"[FAIL] {method:5s} {path:28s}")
            for d in diffs:
                print(f"         {d}")
        else:
            passed += 1
            print(f"[OK]   {method:5s} {path:28s} HTTP {py_status}  响应体逐字节一致")

    print("-" * 78)
    print(f"通过 {passed} / 失败 {failed} / 跳过 {skipped}  （共 {len(CASES)} 个用例）")
    print("=" * 78)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
