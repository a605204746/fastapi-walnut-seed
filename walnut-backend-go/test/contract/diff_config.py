"""参数配置模块（/system/config，9 条路由）的双端契约比对。

覆盖重点：
  1. 列表分页与 id 兜底排序、config_type 等值 / name、key like
  2. /configKey/{key}：缓存优先、未找到返回**空串**（不是 null）、仅登录无权限串
  3. 校验器无 validate_default：空 body 新增 → 500「参数键名不能为空」
  4. updateByKey（无 id 批量 update 路径）与按 id 修改（含改键名删旧缓存）
  5. 内置参数（config_type=Y）禁删文案；refreshCache 是 DELETE
  6. 导出 configType 的 Y/N→是/否 转换
  7. ID 串：非整数 400「参数格式错误」、空串删 0 条也成功

用法：
    python test/contract/diff_config.py [--py ...] [--go ...]
"""

import argparse
import io
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

import diff_system as ds

LIST = "/system/config/list"


def q(value):
    return urllib.parse.quote(value)


def authed_call(base, token, method, path, body=None):
    return ds.call(base, method, path, body=body, headers=ds.authed(token))


def compare_mutate(name, method, path, body=None):
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


def export_call(base, token, path, query=""):
    req = urllib.request.Request(base.rstrip("/") + path + query, data=b"", method="POST")
    req.add_header("clientid", ds.CLIENT_ID)
    req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, dict(r.headers), r.read()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read()


def parse_xlsx(data):
    from openpyxl import load_workbook

    wb = load_workbook(io.BytesIO(data), read_only=False, data_only=True)
    ws = wb.active
    rows = [["" if c is None else str(c) for c in row] for row in ws.iter_rows(values_only=True)]
    title = ws.title
    wb.close()
    return title, rows


def config_rows(base, tok, query=""):
    _, _, raw = authed_call(base, tok, "GET", LIST + query)
    try:
        return (json.loads(raw).get("data") or {}).get("rows") or []
    except Exception:  # noqa: BLE001
        return []


def cleanup_leftovers():
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        for r in config_rows(base, tok, "?configKey=qw_cfg_&pageSize=1000"):
            authed_call(base, tok, "DELETE", f"/system/config/{r['id']}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"参数配置契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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
    key = f"qw_cfg_{stamp}"

    # ---- 1. 列表 ----
    print("\n--- 1. 列表与过滤 ---")
    ds.compare_authed("list · 默认分页（id 兜底排序）", LIST, strict=True)
    ds.compare_authed("list · configName like", f"{LIST}?configName=%E5%AF%86", strict=True)
    ds.compare_authed("list · configKey like", f"{LIST}?configKey=sys.user", strict=True)
    ds.compare_authed("list · configType 等值 Y", f"{LIST}?configType=Y", strict=True)
    ds.compare_authed("list · 显式分页", f"{LIST}?pageNum=1&pageSize=3", strict=True)

    # ---- 2. configKey / 详情 ----
    print("\n--- 2. configKey 与详情 ---")
    ds.compare_authed("configKey · 种子键取值（仅登录无权限串）",
                      "/system/config/configKey/sys.user.initPassword", strict=True)
    ds.compare_authed("configKey · 未知键 → 空串（不是 null）",
                      "/system/config/configKey/qw_no_such_key", strict=True)
    ds.compare_authed("detail · 已存在（id=1）", "/system/config/1", strict=True)
    ds.compare_authed("detail · 不存在 → data=null", "/system/config/999999999999")
    ds.compare_authed("detail · 路径参数非整数", "/system/config/abc")

    # ---- 3. 校验 ----
    print("\n--- 3. 校验（缺省不跑校验器）---")
    compare_mutate("add · 空 body → 500「参数键名不能为空」", "POST", "/system/config", body={})
    compare_mutate("add · configName 空白串", "POST", "/system/config",
                   body={"configName": "  ", "configKey": "k", "configValue": "v"})
    compare_mutate("add · configValue 超长（>500）", "POST", "/system/config",
                   body={"configName": "n", "configKey": "k", "configValue": "v" * 501})
    compare_mutate("update · 缺 id（Field required）", "PUT", "/system/config",
                   body={"configName": "n", "configKey": "k", "configValue": "v"})
    compare_mutate("updateByKey · 缺 configKey（校验器跑）", "PUT", "/system/config/updateByKey",
                   body={"configValue": "v"})

    # ---- 4. 增改与缓存 ----
    print("\n--- 4. 增改与缓存语义 ---")
    compare_mutate("add · 正常新增", "POST", "/system/config",
                   body={"configName": "契约测试参数", "configKey": key,
                         "configValue": "v1", "configType": "N", "remark": "contract-diff"})
    compare_mutate("add · 同键名重复 → 500 文案", "POST", "/system/config",
                   body={"configName": "契约测试参数二", "configKey": key, "configValue": "v2"})
    ds.compare_authed("configKey · 新增后取到 v1（写后回填缓存）",
                      f"/system/config/configKey/{key}", strict=True)
    compare_mutate("updateByKey · 改值为 v2", "PUT", "/system/config/updateByKey",
                   body={"configKey": key, "configValue": "v2"})
    ds.compare_authed("configKey · updateByKey 后取到 v2",
                      f"/system/config/configKey/{key}", strict=True)

    cid_py = next((r["id"] for r in config_rows(ds.PY, ds.PY_TOKEN, f"?configKey={q(key)}")), None)
    cid_go = next((r["id"] for r in config_rows(ds.GO, ds.GO_TOKEN, f"?configKey={q(key)}")), None)
    ds.record("add · 两端取回新建 id", cid_py is not None and cid_go is not None,
              f"py={cid_py} go={cid_go}")

    key2 = key + "b"
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/config",
                                body={"id": cid_py, "configName": "契约测试参数",
                                      "configKey": key2, "configValue": "v2"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/config",
                                body={"id": cid_go, "configName": "契约测试参数",
                                      "configKey": key2, "configValue": "v2"})
    ds.record("update · 按 id 改键名两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    ds.compare_authed("configKey · 旧键缓存已删 → 空串",
                      f"/system/config/configKey/{key}", strict=True)
    ds.compare_authed("configKey · 新键取到 v2",
                      f"/system/config/configKey/{key2}", strict=True)
    compare_authed_norm("list · 改键名后可查（remark 回填）", f"{LIST}?configKey={q(key2)}")

    # ---- 5. 删除与内置保护 ----
    print("\n--- 5. 删除与内置保护 ---")
    builtin = next((r for r in config_rows(ds.PY, ds.PY_TOKEN, "?configType=Y&pageSize=1")
                    if r.get("configType") == "Y"), None)
    if builtin is not None:
        compare_mutate(f"delete · 内置参数禁删文案（{builtin['configKey']}）",
                       "DELETE", f"/system/config/{builtin['id']}")
    else:
        ds.record("delete · 内置参数禁删文案", False, "种子数据里没有 config_type=Y 的参数")
    compare_mutate("delete · ID 串含非数字 → 400「参数格式错误」",
                   "DELETE", "/system/config/1,abc")
    compare_mutate("delete · 空串 → 删除 0 条也成功", "DELETE", "/system/config/%20")

    cid_py = next((r["id"] for r in config_rows(ds.PY, ds.PY_TOKEN, f"?configKey={q(key2)}")), None)
    cid_go = next((r["id"] for r in config_rows(ds.GO, ds.GO_TOKEN, f"?configKey={q(key2)}")), None)
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/config/{cid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/config/{cid_go}")
    ds.record("delete · 删除自建参数成功",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 200,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    ds.compare_authed("configKey · 删除后 → 空串",
                      f"/system/config/configKey/{key2}", strict=True)
    compare_mutate("refreshCache · DELETE 方法成功", "DELETE", "/system/config/refreshCache")
    ds.compare_authed("configKey · 刷新后种子键仍正确",
                      "/system/config/configKey/sys.user.initPassword", strict=True)

    # ---- 6. 导出 ----
    print("\n--- 6. 导出 ---")
    py_s, _, py_b = export_call(ds.PY, ds.PY_TOKEN, "/system/config/export")
    go_s, _, go_b = export_call(ds.GO, ds.GO_TOKEN, "/system/config/export")
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
              py_t == go_t == "参数数据"
              and py_rows[:1] == go_rows[:1]
              == [["参数主键", "参数名称", "参数键名", "参数键值", "系统内置", "备注", "创建时间"]],
              f"py={py_t}/{py_rows[:1]}\n           go={go_t}/{go_rows[:1]}")
    ds.record("export · configType 已转 是/否",
              any(len(r) > 4 and r[4] in ("是", "否") for r in py_rows[1:])
              and not any(len(r) > 4 and r[4] in ("Y", "N") for r in py_rows[1:] + go_rows[1:]),
              f"py 样本={py_rows[1:3]}")
    ds.record("export · 数据行一致（id/createTime 归一化）",
              [[("<ID>" if i == 0 else "<TS>" if i == 6 else v) for i, v in enumerate(r)]
               for r in py_rows[1:]]
              == [[("<ID>" if i == 0 else "<TS>" if i == 6 else v) for i, v in enumerate(r)]
                  for r in go_rows[1:]],
              f"py={py_rows[1:3]}\n           go={go_rows[1:3]}")

    # ---- 7. 框架级 ----
    print("\n--- 7. 框架级 ---")
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
