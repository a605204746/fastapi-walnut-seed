"""角色管理模块（/system/role，15 条路由）的双端契约比对。

覆盖：
  1. 列表：默认分页、条件过滤
  2. 详情：存在 / 不存在 / 路径参数非整数
  3. optionselect
  4. 写入闭环：新增 / 修改 / 删除 + 唯一性校验
  5. changeStatus / dataScope
  6. authUser cancel / cancelAll / selectAll
  7. 导出（响应头形态）
  8. 框架级：尾斜杠 307、方法不匹配 405

用法：
    python test/contract/diff_role.py [--py ...] [--go ...]
"""

import argparse
import json
import sys
import time
import urllib.parse

import diff_system as ds

LIST = "/system/role/list"


def authed_call(base, token, method, path, body=None, headers=None, raw_body=None,
                content_type="application/json"):
    hdrs = {**ds.authed(token), **(headers or {})}
    if raw_body is not None:
        hdrs["Content-Type"] = content_type
        import urllib.request
        import urllib.error
        req = urllib.request.Request(base.rstrip("/") + path,
                                     data=raw_body.encode(), method=method)
        for k, v in hdrs.items():
            req.add_header(k, v)
        req.add_header("clientid", ds.CLIENT_ID)
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.status, dict(r.headers), r.read().decode("utf-8", "replace")
        except urllib.error.HTTPError as e:
            return e.code, dict(e.headers), e.read().decode("utf-8", "replace")
    return ds.call(base, method, path, body=body, headers=hdrs)


def cmp(name, method, path, body=None):
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, method, path, body=body)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, method, path, body=body)
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
            k: ("<ID>" if k in ("id", "createBy", "updateBy", "createDept")
                else "<TS>" if k in ("createTime", "updateTime")
                else strip_volatile(v))
            for k, v in obj.items()
        }
    if isinstance(obj, list):
        return [strip_volatile(i) for i in obj]
    return obj


def compare_list_norm(name, path):
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", path)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", path)
    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    try:
        py_n = json.dumps(strip_volatile(json.loads(py_b)), sort_keys=True, ensure_ascii=False)
        go_n = json.dumps(strip_volatile(json.loads(go_b)), sort_keys=True, ensure_ascii=False)
    except Exception as e:
        diffs.append(f"响应体无法解析: {e}")
    else:
        if py_n != go_n:
            diffs.append(f"响应体(归一化): 不一致")
    ds.record(name, not diffs, "\n           ".join(diffs))


def first_role_id(body):
    try:
        rows = (json.loads(body).get("data") or {}).get("rows") or []
        return rows[0]["id"] if rows else None
    except Exception:
        return None


def cleanup_leftovers():
    """清掉历次运行残留的测试角色。"""
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        for prefix in ("test_role_", "renamed_"):
            _, _, raw = authed_call(base, tok, "GET", f"{LIST}?roleName={prefix}&pageSize=1000")
            try:
                rows = (json.loads(raw).get("data") or {}).get("rows") or []
            except Exception:
                continue
            for r in rows:
                name = r.get("roleName", "")
                if name.startswith("test_role_") or name.startswith("renamed_"):
                    authed_call(base, tok, "DELETE", f"/system/role/{r['id']}")
                    time.sleep(0.1)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    ds.PY, ds.GO = args.py, args.go

    print("=" * 82)
    print(f"角色模块契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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

    # ---- 1. 列表与过滤 ----
    print("\n--- 1. 列表与过滤 ---")
    compare_list_norm("list · 默认分页", LIST)
    compare_list_norm("list · roleName like", LIST + "?roleName=" + urllib.parse.quote("管理"))
    compare_list_norm("list · status 等值", LIST + "?status=0")
    compare_list_norm("list · roleKey like", LIST + "?roleKey=" + urllib.parse.quote("admin"))

    # ---- 2. 详情 ----
    print("\n--- 2. 详情 ---")
    cmp("detail · 种子 id=1", "GET", "/system/role/1")
    cmp("detail · 不存在 → data=null", "GET", "/system/role/999999")
    cmp("detail · 路径参数非整数", "GET", "/system/role/abc")

    # ---- 3. optionselect ----
    print("\n--- 3. optionselect ---")
    cmp("optionselect · 全量", "GET", "/system/role/optionselect")

    # ---- 4. 校验 ----
    print("\n--- 4. 校验 ---")
    cmp("add · 空 body → 三条不能为空", "POST", "/system/role", body={})
    cmp("add · roleName 超长", "POST", "/system/role",
        body={"roleName": "x" * 31, "roleKey": "test", "roleSort": 1})
    cmp("update · 缺 id", "PUT", "/system/role",
        body={"roleName": "test", "roleKey": "test", "roleSort": 1})

    # ---- 5. 增改删闭环 ----
    print("\n--- 5. 增改删闭环 ---")
    ts = str(int(time.time()))
    rname = f"test_role_{ts}"
    rkey = f"test_key_{ts}"

    cmp("add · 正常新增", "POST", "/system/role",
        body={"roleName": rname, "roleKey": rkey, "roleSort": 99})

    time.sleep(1)
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", LIST + f"?roleName={urllib.parse.quote(rname)}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", LIST + f"?roleName={urllib.parse.quote(rname)}")
    py_id = first_role_id(py_b)
    go_id = first_role_id(go_b)
    ds.record("add · 两端各取新增角色 id", True, f"py={py_id} go={go_id}")

    if py_id and go_id:
        cmp("add · 同名重复 → 500", "POST", "/system/role",
            body={"roleName": rname, "roleKey": f"dup_{ts}", "roleSort": 1})
        cmp("add · 同权限重复 → 500", "POST", "/system/role",
            body={"roleName": f"dup_{ts}", "roleKey": rkey, "roleSort": 1})

        cmp("update · 改名", "PUT", "/system/role",
            body={"id": py_id, "roleName": f"renamed_{ts}", "roleKey": rkey, "roleSort": 99})
        time.sleep(0.5)
        compare_list_norm("list · 改名后可查", LIST + f"?roleName={urllib.parse.quote('renamed_' + ts)}")

        cmp("changeStatus · 停用", "PUT", "/system/role/changeStatus",
            body={"id": py_id, "status": "1"})
        cmp("changeStatus · 恢复", "PUT", "/system/role/changeStatus",
            body={"id": py_id, "status": "0"})

        cmp("dataScope · 改数据范围", "PUT", "/system/role/dataScope",
            body={"id": py_id, "dataScope": "3"})

    # ---- 6. 删除 ----
    print("\n--- 6. 删除 ---")
    if py_id and go_id:
        cmp("delete · 正常删除", "DELETE", f"/system/role/{py_id}")
        cmp("delete · 不存在的 id → 500", "DELETE", "/system/role/999999")
        cmp("delete · 空串 → 400", "DELETE", "/system/role/ ")
        cmp("delete · ID 串含非数字 → 400", "DELETE", "/system/role/abc")

    # ---- 7. authUser ----
    print("\n--- 7. authUser ---")
    cmp("allocatedList · 种子角色 1", "GET", "/system/role/authUser/allocatedList?roleId=1")
    cmp("unallocatedList · 种子角色 1", "GET", "/system/role/authUser/unallocatedList?roleId=1")
    cmp("cancel · 缺 roleId → Field required", "PUT", "/system/role/authUser/cancel", body={})

    # ---- 8. 导出 ----
    print("\n--- 8. 导出 ---")
    py_s, py_h, py_b = authed_call(ds.PY, ds.PY_TOKEN, "POST", "/system/role/export",
                                    raw_body="roleName=&status=", content_type="application/x-www-form-urlencoded")
    go_s, go_h, go_b = authed_call(ds.GO, ds.GO_TOKEN, "POST", "/system/role/export",
                                    raw_body="roleName=&status=", content_type="application/x-www-form-urlencoded")
    ds.record("export · HTTP 200", py_s == go_s == 200, f"py={py_s} go={go_s}")
    py_ct = py_h.get("Content-Type", py_h.get("content-type", ""))
    go_ct = go_h.get("Content-Type", go_h.get("content-type", ""))
    ds.record("export · 两端均为 xlsx",
              "spreadsheet" in py_ct.lower() and "spreadsheet" in go_ct.lower(),
              f"py={py_ct} go={go_ct}")

    # ---- 9. 框架级 ----
    print("\n--- 9. 框架级 ---")
    py_s, _, _ = authed_call(ds.PY, ds.PY_TOKEN, "GET", "/system/role/list/")
    go_s, _, _ = authed_call(ds.GO, ds.GO_TOKEN, "GET", "/system/role/list/")
    ds.record("list · 尾斜杠两端一致", py_s == go_s, f"py={py_s} go={go_s}")
    py_s, _, _ = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", LIST)
    go_s, _, _ = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", LIST)
    ds.record("list · 方法不匹配两端一致", py_s == go_s, f"py={py_s} go={go_s}")

    return ds._summary()


if __name__ == "__main__":
    sys.exit(main())
