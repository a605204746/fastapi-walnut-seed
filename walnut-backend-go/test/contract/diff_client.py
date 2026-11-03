"""客户端管理模块（/system/client，7 条路由）的双端契约比对。

覆盖重点：
  1. 列表 id 主排序 + del_flag 过滤；全等值查询参数
  2. 新增时 client_id = md5(clientKey+clientSecret)（两端必须算出同一个 hex）
  3. grantTypeList 出参恒为数组（由 grant_type 逗号串拆出）；导出时转回逗号串
  4. key 重复是 raise 500（不是 ErrorResponse 信封的 code 语义差异，文案要对）
  5. changeStatus 按 client_id 更新；删除是逻辑删除（删后 detail 为 null）
  6. ID 串文案「主键格式有误 / 主键不能为空」
  7. 导出走 form 且 pop pageSize

用法：
    python test/contract/diff_client.py [--py ...] [--go ...]
"""

import argparse
import hashlib
import io
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

import diff_system as ds

LIST = "/system/client/list"


def q(value):
    return urllib.parse.quote(value)


def authed_call(base, token, method, path, body=None):
    return ds.call(base, method, path, body=body, headers=ds.authed(token))


def cmp(name, method, path, body=None, form=None):
    """两端发同一请求比对；form 非空时以表单体发送。"""
    results = []
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        if form is not None:
            data = urllib.parse.urlencode(form).encode()
            req = urllib.request.Request(base.rstrip("/") + path, data=data, method=method)
            req.add_header("Content-Type", "application/x-www-form-urlencoded")
            req.add_header("clientid", ds.CLIENT_ID)
            req.add_header("Authorization", f"Bearer {tok}")
            try:
                with urllib.request.urlopen(req, timeout=60) as r:
                    results.append((r.status, r.read().decode("utf-8", "replace")))
            except urllib.error.HTTPError as e:
                results.append((e.code, e.read().decode("utf-8", "replace")))
        else:
            s, _, b = authed_call(base, tok, method, path, body=body)
            results.append((s, b))
    (py_s, py_b), (go_s, go_b) = results
    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    if ds.canon(py_b) != ds.canon(go_b):
        diffs.append(f"响应体:\n           py = {ds.canon(py_b)}\n           go = {ds.canon(go_b)}")
    ds.record(name, not diffs, "\n           ".join(diffs))
    return py_b, go_b


def strip_volatile(obj):
    if isinstance(obj, dict):
        return {
            k: ("<ID>" if k == "id" else "<TS>" if k == "createTime" else strip_volatile(v))
            for k, v in obj.items()
        }
    if isinstance(obj, list):
        return [strip_volatile(i) for i in obj]
    return obj


def compare_authed_norm(name, path):
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", path)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", path)
    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    try:
        py_n = json.dumps(strip_volatile(json.loads(py_b)), sort_keys=True, ensure_ascii=False)
        go_n = json.dumps(strip_volatile(json.loads(go_b)), sort_keys=True, ensure_ascii=False)
    except Exception as e:  # noqa: BLE001
        diffs.append(f"响应体无法解析: {e}")
    else:
        if py_n != go_n:
            diffs.append(f"响应体（已归一化）:\n           py = {py_n}\n           go = {go_n}")
    ds.record(name, not diffs, "\n           ".join(diffs))


def export_call(base, token, form=None):
    data = urllib.parse.urlencode(form or {}).encode()
    req = urllib.request.Request(base.rstrip("/") + "/system/client/export", data=data,
                                 method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    req.add_header("clientid", ds.CLIENT_ID)
    req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


def parse_xlsx(data):
    from openpyxl import load_workbook

    wb = load_workbook(io.BytesIO(data), read_only=False, data_only=True)
    ws = wb.active
    rows = [["" if c is None else str(c) for c in row] for row in ws.iter_rows(values_only=True)]
    title = ws.title
    wb.close()
    return title, rows


def client_rows(base, tok, query=""):
    _, _, raw = authed_call(base, tok, "GET", LIST + query)
    try:
        return (json.loads(raw).get("data") or {}).get("rows") or []
    except Exception:  # noqa: BLE001
        return []


def cleanup_leftovers():
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        for r in client_rows(base, tok, "?clientKey=qw_cli_&pageSize=1000"):
            authed_call(base, tok, "DELETE", f"/system/client/{r['id']}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"客户端管理契约比对：Python({ds.PY})  vs  Go({ds.GO})")
    print("=" * 82)

    for side, base in (("py", ds.PY), ("go", ds.GO)):
        token, err = ds.login(side, base)
        if err:
            print(f"[FATAL] {side} 端登录失败: {err}")
            return 2
        if side == "py":
            ds.PY_TOKEN = token
        else:
            ds.GO_TOKEN = token

    cleanup_leftovers()
    stamp = str(int(time.time()))[-6:]
    ckey = f"qw_cli_{stamp}"
    csecret = "secret" + stamp
    want_client_id = hashlib.md5((ckey + csecret).encode()).hexdigest()

    # ---- 1. 列表 ----
    print("\n--- 1. 列表与过滤 ---")
    ds.compare_authed("list · 默认分页（id 主排序 + del_flag 过滤）", LIST, strict=True)
    ds.compare_authed("list · clientKey 等值", f"{LIST}?clientKey=e5cd7e4891bf95d1d19206ce24a7b32e",
                      strict=True)
    ds.compare_authed("list · status 等值", f"{LIST}?status=0", strict=True)
    ds.compare_authed("list · 显式分页", f"{LIST}?pageNum=1&pageSize=2", strict=True)

    # ---- 2. 详情 ----
    print("\n--- 2. 详情 ---")
    seed_id = next((r["id"] for r in client_rows(ds.PY, ds.PY_TOKEN)), None)
    ds.compare_authed(f"detail · 种子 id={seed_id}", f"/system/client/{seed_id}", strict=True)
    ds.compare_authed("detail · 不存在 → data=null", "/system/client/999999999999")
    ds.compare_authed("detail · 路径参数非整数", "/system/client/abc")

    # ---- 3. 校验 ----
    print("\n--- 3. 校验（validate_default）---")
    cmp("add · 空 body → 三条缺省校验按声明序", "POST", "/system/client", body={})
    cmp("add · grantTypeList 传字符串 → list 原文", "POST", "/system/client",
        body={"clientKey": "k", "clientSecret": "s", "grantTypeList": "password"})
    cmp("update · 缺 id → 「id不能为空」", "PUT", "/system/client",
        body={"clientKey": "k", "clientSecret": "s", "grantTypeList": []})
    cmp("changeStatus · 缺 status → 「状态不能为空」", "PUT", "/system/client/changeStatus",
        body={"clientId": "x"})

    # ---- 4. 增改删闭环 ----
    print("\n--- 4. 增改删闭环 ---")
    cmp("add · 正常新增", "POST", "/system/client",
        body={"clientKey": ckey, "clientSecret": csecret, "grantTypeList": ["password", "sms"],
              "deviceType": "pc", "activeTimeout": 60, "timeout": 120, "status": "0"})
    cmp("add · 同 key 重复 → 500 文案", "POST", "/system/client",
        body={"clientKey": ckey, "clientSecret": "other", "grantTypeList": ["password"]})
    compare_authed_norm("list · 新增可查且 clientId=md5(key+secret)、grantTypeList 拆出",
                        f"{LIST}?clientKey={q(ckey)}")
    row_py = next((r for r in client_rows(ds.PY, ds.PY_TOKEN, f"?clientKey={q(ckey)}")), {})
    row_go = next((r for r in client_rows(ds.GO, ds.GO_TOKEN, f"?clientKey={q(ckey)}")), {})
    ds.record("add · clientId 两端均为 md5(key+secret)",
              row_py.get("clientId") == want_client_id == row_go.get("clientId"),
              f"py={row_py.get('clientId')} go={row_go.get('clientId')} want={want_client_id}")
    ds.record("add · grantTypeList 拆为数组且 grantType 原串并存",
              row_py.get("grantTypeList") == ["password", "sms"]
              and row_py.get("grantType") == "password,sms"
              and row_go.get("grantTypeList") == ["password", "sms"]
              and row_go.get("grantType") == "password,sms",
              f"py={row_py.get('grantTypeList')}/{row_py.get('grantType')}")

    cid_py = row_py.get("id")
    cid_go = row_go.get("id")
    cmp("changeStatus · 停用", "PUT", "/system/client/changeStatus",
        body={"clientId": want_client_id, "status": "1"})
    compare_authed_norm("list · 停用后 status=1", f"{LIST}?clientKey={q(ckey)}")
    cmp("changeStatus · 恢复启用", "PUT", "/system/client/changeStatus",
        body={"clientId": want_client_id, "status": "0"})

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/client",
                                body={"id": cid_py, "clientKey": ckey, "clientSecret": csecret,
                                      "grantTypeList": ["password"], "deviceType": "android"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/client",
                                body={"id": cid_go, "clientKey": ckey, "clientSecret": csecret,
                                      "grantTypeList": ["password"], "deviceType": "android"})
    ds.record("update · 改 deviceType 两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    compare_authed_norm("list · 更新后 deviceType=android、grantTypeList 单元素",
                        f"{LIST}?clientKey={q(ckey)}")

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/client/{cid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/client/{cid_go}")
    ds.record("delete · 逻辑删除成功",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 200,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", f"/system/client/{cid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", f"/system/client/{cid_go}")
    ds.record("delete · 逻辑删除后 detail 为 null（del_flag 过滤）",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("data") is None,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/client/{cid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/client/{cid_go}")
    ds.record("delete · 再删同一 id → 500「操作失败」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 500,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    cmp("delete · ID 串含非数字 → 400「主键格式有误」", "DELETE", "/system/client/1,abc")
    cmp("delete · 空串 → 400「主键不能为空」", "DELETE", "/system/client/%20")

    # ---- 5. 导出（form）----
    print("\n--- 5. 导出（form + grantTypeList 转逗号串）---")
    py_s, py_b = export_call(ds.PY, ds.PY_TOKEN, {"status": "0"})
    go_s, go_b = export_call(ds.GO, ds.GO_TOKEN, {"status": "0"})
    ds.record("export · HTTP 200", (py_s, go_s) == (200, 200), f"py={py_s} go={go_s}")
    try:
        py_t, py_rows = parse_xlsx(py_b)
        go_t, go_rows = parse_xlsx(go_b)
    except Exception as e:  # noqa: BLE001
        ds.record("export · 工作簿可解析", False, f"{e}")
        py_rows = go_rows = []
        py_t = go_t = ""
    else:
        ds.record("export · 工作簿可解析", True)
    ds.record("export · 表头与 sheet 名一致",
              py_t == go_t == "客户端管理"
              and py_rows[:1] == go_rows[:1]
              == [["id", "客户端id", "客户端key", "客户端秘钥", "授权类型",
                   "token活跃超时时间", "token固定超时时间", "状态"]],
              f"py={py_t}/{py_rows[:1]}\n           go={go_t}/{go_rows[:1]}")
    ds.record("export · 数据行一致（id 归一化）",
              [[("<ID>" if i == 0 else v) for i, v in enumerate(r)] for r in py_rows[1:]]
              == [[("<ID>" if i == 0 else v) for i, v in enumerate(r)] for r in go_rows[1:]],
              f"py={py_rows[1:3]}\n           go={go_rows[1:3]}")

    # ---- 6. 框架级 ----
    print("\n--- 6. 框架级 ---")
    ds.compare_authed("list · 尾斜杠应 307", f"{LIST}/")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", LIST)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", LIST)
    ds.record("list · 方法不匹配应 405",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    ds.compare_authed("list · 清理后回到种子态（逐字节）", LIST, strict=True)

    return ds._summary()


if __name__ == "__main__":
    sys.exit(main())
