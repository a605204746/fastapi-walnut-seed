"""菜单管理模块（/system/menu，8 条待铺路由 + 已铺 getRouters）的双端契约比对。

覆盖重点：
  1. 列表不分页数组 + parent_id,order_num 排序 + 非超管 menu_ids 收敛（admin 超管看全量）
  2. treeselect 节点键序（id/parentId/label/weight/menuType/icon/visible/status）
  3. roleMenuTreeselect 的 checkedKeys（menu_check_strictly 父子联动语义）
  4. 模型级 validate_default：空 body → 三条「不能为空」按声明序
  5. 新增三道前置：名称唯一 → 外链地址 http(s) → 路由名称/地址唯一（文案各不同）
  6. 删除两道 Warn(601)：存在子菜单 / 菜单已分配；cascade 批量级联
  7. queryParam 必须 JSON 对象、perms 正则格式

用法：
    python test/contract/diff_menu.py [--py ...] [--go ...]
"""

import argparse
import json
import sys
import time
import urllib.parse

import diff_system as ds

LIST = "/system/menu/list"


def q(value):
    return urllib.parse.quote(value)


def authed_call(base, token, method, path, body=None):
    return ds.call(base, method, path, body=body, headers=ds.authed(token))


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
            k: ("<ID>" if k in ("id", "parentId") else "<TS>" if k == "createTime"
                else strip_volatile(v))
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


def menu_rows(base, tok, query=""):
    _, _, raw = authed_call(base, tok, "GET", LIST + query)
    try:
        return json.loads(raw).get("data") or []
    except Exception:  # noqa: BLE001
        return []


def cleanup_leftovers():
    """清掉历次残留的测试菜单（cascade 删，叶子优先由 cascade 语义保证）。"""
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        rows = [r for r in menu_rows(base, tok, f"?menuName={q('契约测试')}")
                if (r.get("menuName") or "").startswith("契约测试")]
        ids = [r["id"] for r in rows]
        if ids:
            authed_call(base, tok, "DELETE",
                        "/system/menu/cascade/" + ",".join(str(i) for i in ids))


def find_id(base, tok, name):
    for r in menu_rows(base, tok, f"?menuName={q(name)}"):
        if r.get("menuName") == name:
            return r["id"]
    return None


def wait_visible(name, timeout=5):
    """py 端提交在响应之后（交接 §6）：写后立刻查可能查不到，
    进而让「同名重复」等断言落进竞态窗口（py 甚至会真建出重复行）。
    两端都等到按名称可查再继续。"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        if find_id(ds.PY, ds.PY_TOKEN, name) is not None \
                and find_id(ds.GO, ds.GO_TOKEN, name) is not None:
            return True
        time.sleep(0.1)
    return False


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"菜单管理契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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
    dir_name = f"契约测试目录{stamp}"
    menu_name = f"契约测试菜单{stamp}"
    dir_path = f"qwdir{stamp}"
    menu_path = f"qwmenu{stamp}"

    # ---- 1. 列表 ----
    print("\n--- 1. 列表与过滤 ---")
    ds.compare_authed("list · 全量（parent_id,order_num 排序 + children 恒 []）",
                      LIST, strict=True)
    ds.compare_authed("list · menuName like", f"{LIST}?menuName=%E7%94%A8", strict=True)
    ds.compare_authed("list · status 等值", f"{LIST}?status=0", strict=True)
    ds.compare_authed("list · menuType 等值 M", f"{LIST}?menuType=M", strict=True)
    ds.compare_authed("list · parentId 等值", f"{LIST}?parentId=1", strict=True)
    ds.compare_authed("list · parentId 非法整数 → 400 原文", f"{LIST}?parentId=abc")

    # ---- 2. 树 ----
    print("\n--- 2. 下拉树与角色菜单树 ---")
    ds.compare_authed("treeselect · 全量树（键序 + children 有无）",
                      "/system/menu/treeselect", strict=True)
    ds.compare_authed("treeselect · menuName 过滤后的多根形态",
                      f"/system/menu/treeselect?menuName={q('系统')}", strict=True)
    ds.compare_authed("roleMenuTreeselect · 角色 1（checkedKeys 联动语义）",
                      "/system/menu/roleMenuTreeselect/1", strict=True)
    ds.compare_authed("roleMenuTreeselect · 角色 2",
                      "/system/menu/roleMenuTreeselect/2", strict=True)
    ds.compare_authed("roleMenuTreeselect · 不存在角色 → checkedKeys 空",
                      "/system/menu/roleMenuTreeselect/999999999999", strict=True)

    # ---- 3. 详情 ----
    print("\n--- 3. 详情 ---")
    ds.compare_authed("detail · 已存在（id=1）", "/system/menu/1", strict=True)
    ds.compare_authed("detail · 不存在 → data=null", "/system/menu/999999999999")
    ds.compare_authed("detail · 路径参数非整数", "/system/menu/abc")

    # ---- 4. 校验 ----
    print("\n--- 4. 校验（模型级 validate_default）---")
    cmp("add · 空 body → 三条不能为空按声明序", "POST", "/system/menu", body={})
    cmp("add · queryParam 非 JSON 对象", "POST", "/system/menu",
        body={"menuName": "n", "orderNum": 1, "menuType": "M", "queryParam": "notjson"})
    cmp("add · queryParam 是 JSON 数组也不行", "POST", "/system/menu",
        body={"menuName": "n", "orderNum": 1, "menuType": "M", "queryParam": "[1,2]"})
    cmp("add · perms 格式不符", "POST", "/system/menu",
        body={"menuName": "n", "orderNum": 1, "menuType": "F", "perms": "badperms"})
    cmp("add · menuName 超长（>50）", "POST", "/system/menu",
        body={"menuName": "名" * 51, "orderNum": 1, "menuType": "M"})
    cmp("update · 缺 id（Field required）", "PUT", "/system/menu",
        body={"menuName": "n", "orderNum": 1, "menuType": "M"})

    # ---- 5. 新增三道前置 ----
    print("\n--- 5. 新增前置校验 ---")
    cmp("add · 目录正常新增", "POST", "/system/menu",
        body={"menuName": dir_name, "parentId": 0, "orderNum": 97, "path": dir_path,
              "menuType": "M", "isFrame": "1", "isCache": "0", "visible": "0", "status": "0"})
    wait_visible(dir_name)
    dir_py = find_id(ds.PY, ds.PY_TOKEN, dir_name)
    dir_go = find_id(ds.GO, ds.GO_TOKEN, dir_name)
    ds.record("add · 两端取回目录 id", dir_py is not None and dir_go is not None,
              f"py={dir_py} go={dir_go}")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "POST", "/system/menu",
                                body={"menuName": menu_name, "parentId": dir_py, "orderNum": 1,
                                      "path": menu_path, "component": "qw/index",
                                      "menuType": "C", "isFrame": "1",
                                      "perms": f"qw:test{stamp}:list"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "POST", "/system/menu",
                                body={"menuName": menu_name, "parentId": dir_go, "orderNum": 1,
                                      "path": menu_path, "component": "qw/index",
                                      "menuType": "C", "isFrame": "1",
                                      "perms": f"qw:test{stamp}:list"})
    ds.record("add · 菜单正常新增（两端各挂本端目录）",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    wait_visible(menu_name)
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "POST", "/system/menu",
                                body={"menuName": menu_name, "parentId": dir_py, "orderNum": 2,
                                      "path": menu_path + "x", "menuType": "C"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "POST", "/system/menu",
                                body={"menuName": menu_name, "parentId": dir_go, "orderNum": 2,
                                      "path": menu_path + "x", "menuType": "C"})
    ds.record("add · 同级同名 → 500「菜单名称已存在」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and "菜单名称已存在" in py_b,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    cmp("add · 外链 isFrame=0 但地址非 http → 500 文案", "POST", "/system/menu",
        body={"menuName": f"契约测试外链{stamp}", "parentId": 0, "orderNum": 98,
              "path": "notaurl", "menuType": "C", "isFrame": "0"})
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "POST", "/system/menu",
                                body={"menuName": f"契约测试冲突{stamp}", "parentId": dir_py,
                                      "orderNum": 3, "path": menu_path, "menuType": "C"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "POST", "/system/menu",
                                body={"menuName": f"契约测试冲突{stamp}", "parentId": dir_go,
                                      "orderNum": 3, "path": menu_path, "menuType": "C"})
    ds.record("add · 同级同 path → 500「路由名称或地址已存在」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and "路由名称或地址已存在" in py_b,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    # ---- 6. 修改 ----
    print("\n--- 6. 修改 ---")
    mid_py = find_id(ds.PY, ds.PY_TOKEN, menu_name)
    mid_go = find_id(ds.GO, ds.GO_TOKEN, menu_name)
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/menu",
                                body={"id": mid_py, "menuName": menu_name + "改",
                                      "parentId": dir_py, "orderNum": 1, "path": menu_path,
                                      "menuType": "C"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/menu",
                                body={"id": mid_go, "menuName": menu_name + "改",
                                      "parentId": dir_go, "orderNum": 1, "path": menu_path,
                                      "menuType": "C"})
    ds.record("update · 改名两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    wait_visible(menu_name + "改")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/menu",
                                body={"id": mid_py, "menuName": menu_name + "改",
                                      "parentId": mid_py, "orderNum": 1, "path": menu_path,
                                      "menuType": "C"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/menu",
                                body={"id": mid_go, "menuName": menu_name + "改",
                                      "parentId": mid_go, "orderNum": 1, "path": menu_path,
                                      "menuType": "C"})
    ds.record("update · 上级是自己 → 500 文案",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and "上级菜单不能选择自己" in py_b,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    compare_authed_norm("list · 改名后可查", f"{LIST}?menuName={q(menu_name + '改')}")

    # ---- 7. 删除 ----
    print("\n--- 7. 删除（两道 Warn + cascade）---")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/menu/{dir_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/menu/{dir_go}")
    ds.record("delete · 有子菜单 → 601「存在子菜单,不允许删除」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 601,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    cmp("delete · 已分配角色的种子菜单 → 601「菜单已分配,不允许删除」",
        "DELETE", "/system/menu/1")
    cmp("delete · 不存在的 id → 500「操作失败」", "DELETE", "/system/menu/999999999999")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE",
                                f"/system/menu/cascade/{dir_py},{mid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE",
                                f"/system/menu/cascade/{dir_go},{mid_go}")
    ds.record("cascade · 批量级联删除成功（含角色关联清理）",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 200,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", f"/system/menu/{dir_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", f"/system/menu/{dir_go}")
    ds.record("cascade · 删除后 detail 为 null",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("data") is None,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    # ---- 8. 框架级 ----
    print("\n--- 8. 框架级与收尾回归 ---")
    ds.compare_authed("list · 尾斜杠应 307", f"{LIST}/")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", LIST)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", LIST)
    ds.record("list · 方法不匹配应 405",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    ds.compare_authed("list · 清理后回到种子态（逐字节）", LIST, strict=True)
    ds.compare_authed("getRouters · 收尾回归（逐字节）", "/system/menu/getRouters", strict=True)

    return ds._summary()


if __name__ == "__main__":
    sys.exit(main())
