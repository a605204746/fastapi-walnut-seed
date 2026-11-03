"""字典模块（/system/dict/type 8 条 + /system/dict/data 7 条）的双端契约比对。

覆盖重点（都是实测容易写错的契约点）：
  1. type/data 列表的分页与默认排序（type: id；data: dict_sort,id）
  2. 校验器**只对显式提供的键**生效：空 body 新增 type 不报 400 而是 500「字典类型不能为空」
  3. 新增 type 缺省字段插 NULL；新增 data 缺省字段走列默认值（0/""/"N"）——两者语义相反
  4. 缓存语义：新增 type 后按类型查为 []（空集合防穿透）；改/删数据后按类型查立即反映
  5. 删除 type 前的占用检查（有数据 → 500「xx已分配,不能删除」，整体不删）
  6. refreshCache 是 DELETE 方法；optionselect / data/type 只挂登录不挂权限串
  7. 导出表头顺序 + data 导出 isDefault 的 Y/N→是/否 标签转换
  8. ID 串解析：非整数 → 400「参数格式错误」；空串 → 空列表（删除 0 条也成功）

用法：
    python test/contract/diff_dict.py [--py http://127.0.0.1:8012] [--go http://127.0.0.1:8011]
"""

import argparse
import io
import json
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

import diff_system as ds

TYPE_LIST = "/system/dict/type/list"
DATA_LIST = "/system/dict/data/list"


def q(value):
    """查询串中文/特殊字符百分号编码（urllib 不隐式编码）。"""
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
        diffs.append(f"响应体无法解析: {e}\n           py={py_b[:400]}\n           go={go_b[:400]}")
    else:
        if py_n != go_n:
            diffs.append(f"响应体（已归一化）:\n           py = {py_n}\n           go = {go_n}")
    ds.record(name, not diffs, "\n           ".join(diffs))


def export_call(base, token, path, query=""):
    """POST 导出端点，返回 (status, headers, bytes)。xlsx 是 zip，必须原始字节。"""
    req = urllib.request.Request(base.rstrip("/") + path + query, data=b"", method="POST")
    req.add_header("clientid", ds.CLIENT_ID)
    req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, dict(r.headers), r.read()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read()


def hdr(headers, name):
    low = name.lower()
    for k, v in headers.items():
        if k.lower() == low:
            return v
    return None


def parse_xlsx(data):
    """解析工作簿 → (sheet名, [[单元格字符串,...],...])。read_only 必须 False（见 diff_post 注释）。"""
    from openpyxl import load_workbook

    wb = load_workbook(io.BytesIO(data), read_only=False, data_only=True)
    ws = wb.active
    rows = [["" if c is None else str(c) for c in row] for row in ws.iter_rows(values_only=True)]
    title = ws.title
    wb.close()
    return title, rows


def type_rows(base, tok, query=""):
    _, _, raw = authed_call(base, tok, "GET", TYPE_LIST + query)
    try:
        return (json.loads(raw).get("data") or {}).get("rows") or []
    except Exception:  # noqa: BLE001
        return []


def data_rows(base, tok, query=""):
    _, _, raw = authed_call(base, tok, "GET", DATA_LIST + query)
    try:
        return (json.loads(raw).get("data") or {}).get("rows") or []
    except Exception:  # noqa: BLE001
        return []


def cleanup_leftovers():
    """清掉历次残留的测试类型及其数据（先数据后类型）。"""
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        for t in type_rows(base, tok, "?dictType=qw_test_&pageSize=1000"):
            dtype = t.get("dictType") or ""
            codes = [r["id"] for r in data_rows(base, tok, f"?dictType={q(dtype)}&pageSize=1000")]
            if codes:
                authed_call(base, tok, "DELETE",
                            "/system/dict/data/" + ",".join(str(c) for c in codes))
            authed_call(base, tok, "DELETE", f"/system/dict/type/{t['id']}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()

    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"字典模块契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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
    tname = f"契约测试类型{stamp}"
    ttype = f"qw_test_{stamp}"

    # ---- 1. 类型列表 ----
    print("\n--- 1. 类型列表与过滤 ---")
    ds.compare_authed("type list · 默认分页（id 兜底排序）", TYPE_LIST, strict=True)
    ds.compare_authed("type list · dictName 过滤", f"{TYPE_LIST}?dictName=%E7%94%A8", strict=True)
    ds.compare_authed("type list · dictType 过滤（like）", f"{TYPE_LIST}?dictType=sys_", strict=True)
    ds.compare_authed("type list · 显式分页", f"{TYPE_LIST}?pageNum=2&pageSize=3", strict=True)
    ds.compare_authed("type list · pageSize 超限 1000 校验", f"{TYPE_LIST}?pageSize=1001")

    # ---- 2. optionselect / 详情 ----
    print("\n--- 2. optionselect 与详情 ---")
    ds.compare_authed("type optionselect · 全量（仅登录无权限串）",
                      "/system/dict/type/optionselect", strict=True)
    ds.compare_authed("type detail · 已存在（id=1）", "/system/dict/type/1", strict=True)
    ds.compare_authed("type detail · 不存在 → data=null", "/system/dict/type/999999999999")
    ds.compare_authed("type detail · 路径参数非整数", "/system/dict/type/abc")

    # ---- 3. 类型校验 ----
    print("\n--- 3. 类型校验（缺省不跑校验器）---")
    compare_mutate("type add · 空 body → 500「字典类型不能为空」（非 400）",
                   "POST", "/system/dict/type", body={})
    compare_mutate("type add · dictName 空白串",
                   "POST", "/system/dict/type", body={"dictName": "  ", "dictType": "a_b"})
    compare_mutate("type add · dictName 超长（>100）",
                   "POST", "/system/dict/type", body={"dictName": "名" * 101, "dictType": "a_b"})
    compare_mutate("type add · dictType 大写字母（正则拦）",
                   "POST", "/system/dict/type", body={"dictName": "n", "dictType": "Abc"})
    compare_mutate("type add · dictType 数字开头（正则拦）",
                   "POST", "/system/dict/type", body={"dictName": "n", "dictType": "1abc"})
    compare_mutate("type add · 显式 null dictName（校验器跑）",
                   "POST", "/system/dict/type", body={"dictName": None, "dictType": "a_b"})
    compare_mutate("type update · 缺 id（Field required）",
                   "PUT", "/system/dict/type", body={"dictName": "n", "dictType": "a_b"})

    # ---- 4. 类型增改删 + 缓存空集合 ----
    print("\n--- 4. 类型增改删与缓存语义 ---")
    compare_mutate("type add · 正常新增", "POST", "/system/dict/type",
                   body={"dictName": tname, "dictType": ttype, "remark": "contract-diff"})
    compare_mutate("type add · 同 dictType 重复 → 500 文案", "POST", "/system/dict/type",
                   body={"dictName": tname + "二", "dictType": ttype})
    # 新增类型下无数据：空集合缓存 → 按类型查为 []
    ds.compare_authed("data type · 新类型按类型查为 []（空集合防穿透）",
                      f"/system/dict/data/type/{ttype}", strict=True)
    compare_authed_norm("type list · 新增可查（remark 回填）", f"{TYPE_LIST}?dictType={q(ttype)}")

    tid_py = next((r["id"] for r in type_rows(ds.PY, ds.PY_TOKEN, f"?dictType={q(ttype)}")), None)
    tid_go = next((r["id"] for r in type_rows(ds.GO, ds.GO_TOKEN, f"?dictType={q(ttype)}")), None)
    ds.record("type add · 两端取回新建 id", tid_py is not None and tid_go is not None,
              f"py={tid_py} go={tid_go}")

    new_type = ttype + "b"
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/dict/type",
                                body={"id": tid_py, "dictName": tname, "dictType": new_type})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/dict/type",
                                body={"id": tid_go, "dictName": tname, "dictType": new_type})
    ds.record("type update · 改 dictType 两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    ds.compare_authed("data type · 旧类型缓存已失效仍为 []",
                      f"/system/dict/data/type/{ttype}", strict=True)

    # ---- 5. 数据增改删 + 缺省值语义 ----
    print("\n--- 5. 数据增改删与缺省值语义 ---")
    compare_mutate("data add · 全字段新增", "POST", "/system/dict/data",
                   body={"dictSort": 7, "dictLabel": "契约标签", "dictValue": "qv1",
                         "dictType": new_type, "cssClass": "cc", "listClass": "primary",
                         "isDefault": "Y", "remark": "contract-diff"})
    compare_mutate("data add · 仅必填三字段（其余走列默认值 0/''/N/NULL）",
                   "POST", "/system/dict/data",
                   body={"dictLabel": "缺省标签", "dictValue": "qv2", "dictType": new_type})
    compare_mutate("data add · 同类型同键值重复 → 500 文案", "POST", "/system/dict/data",
                   body={"dictLabel": "x", "dictValue": "qv1", "dictType": new_type})
    compare_mutate("data add · 缺 dictType → 500「字典类型不能为空」",
                   "POST", "/system/dict/data", body={"dictLabel": "x", "dictValue": "qv9"})
    compare_mutate("data update · 缺 id（Field required）",
                   "PUT", "/system/dict/data", body={"dictLabel": "x", "dictType": new_type})

    compare_authed_norm("data list · 按类型查两条（dict_sort,id 排序 + 缺省值形态）",
                        f"{DATA_LIST}?dictType={q(new_type)}")
    # ⚠️ 测试自建行的雪花 id 两端必然不同，不能 strict（下同）
    compare_authed_norm("data type · 按类型查走缓存且与列表一致",
                        f"/system/dict/data/type/{q(new_type)}")

    codes_py = [r["id"] for r in data_rows(ds.PY, ds.PY_TOKEN, f"?dictType={q(new_type)}")]
    codes_go = [r["id"] for r in data_rows(ds.GO, ds.GO_TOKEN, f"?dictType={q(new_type)}")]
    ds.record("data add · 两端各取回两条 id", len(codes_py) == 2 and len(codes_go) == 2,
              f"py={codes_py} go={codes_go}")

    # 改标签后按类型查必须立即反映（写后刷缓存）。
    # ⚠️ codes 按 dict_sort,id 排序 → codes[0] 是 qv2 行；body 的 dictValue 必须
    # 保持 qv2，否则撞同类型键值唯一性变成 500（第一版脚本就写错了）。
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/dict/data",
                                body={"id": codes_py[0], "dictLabel": "改后标签",
                                      "dictValue": "qv2", "dictType": new_type, "dictSort": 0})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/dict/data",
                                body={"id": codes_go[0], "dictLabel": "改后标签",
                                      "dictValue": "qv2", "dictType": new_type, "dictSort": 0})
    ds.record("data update · 改标签两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    compare_authed_norm("data type · 改后缓存立即反映新标签",
                        f"/system/dict/data/type/{q(new_type)}")

    # ---- 6. 占用检查与删除 ----
    print("\n--- 6. 占用检查与删除 ---")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/dict/type/{tid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/dict/type/{tid_go}")
    ds.record("type delete · 有数据应 500「xx已分配,不能删除」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and f"{tname}已分配,不能删除" in py_b,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    compare_mutate("type delete · ID 串含非数字 → 400「参数格式错误」",
                   "DELETE", "/system/dict/type/1,abc")
    compare_mutate("type delete · 空串 → 删除 0 条也成功", "DELETE", "/system/dict/type/%20")

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE",
                                "/system/dict/data/" + ",".join(str(c) for c in codes_py))
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE",
                                "/system/dict/data/" + ",".join(str(c) for c in codes_go))
    ds.record("data delete · 批量删除两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    # ⚠️ py 端提交在响应之后（交接 §6）：删除响应返回时提交可能未落库，紧跟的
    # 按类型查会重载到旧行并把旧结果**写进缓存**（缓存污染）。Go 端提交在响应前
    # 无此窗口。用产品自带的 refreshCache 中和竞态后再断言，不改任何一端实现。
    compare_mutate("refreshCache · 中和 py 写后提交竞态的缓存污染",
                   "DELETE", "/system/dict/type/refreshCache")
    ds.compare_authed("data type · 删空后按类型查为 []",
                      f"/system/dict/data/type/{q(new_type)}", strict=True)

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/dict/type/{tid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/dict/type/{tid_go}")
    ds.record("type delete · 无数据后删除成功",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 200,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", f"/system/dict/type/{tid_py}")
    # ⚠️ py 提交在响应之后（交接 §6）：删除响应返回时提交可能未落库，紧跟的详情
    # 查询会读到旧行。给 py 一个 5 秒重试窗口（语义=最终一致），Go 端无需重试。
    deadline = time.time() + 5
    while json.loads(py_b).get("data") is not None and time.time() < deadline:
        time.sleep(0.1)
        py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", f"/system/dict/type/{tid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", f"/system/dict/type/{tid_go}")
    ds.record("type detail · 删除后 data=null",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("data") is None,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    # ---- 7. refreshCache / 数据列表 ----
    print("\n--- 7. refreshCache 与数据列表 ---")
    compare_mutate("refreshCache · DELETE 方法成功", "DELETE", "/system/dict/type/refreshCache")
    ds.compare_authed("data type · 种子类型刷新后仍正确（sys_user_sex）",
                      "/system/dict/data/type/sys_user_sex", strict=True)
    ds.compare_authed("data list · 种子全量第一页（dict_sort,id 排序）", DATA_LIST, strict=True)
    ds.compare_authed("data list · dictType 等值过滤", f"{DATA_LIST}?dictType=sys_user_sex",
                      strict=True)
    ds.compare_authed("data list · dictSort 过滤", f"{DATA_LIST}?dictSort=1", strict=True)
    ds.compare_authed("data list · dictSort 非法 → 400 原文", f"{DATA_LIST}?dictSort=abc")
    ds.compare_authed("data detail · 已存在（id=1）", "/system/dict/data/1", strict=True)
    ds.compare_authed("data detail · 不存在 → data=null", "/system/dict/data/999999999999")

    # ---- 8. 导出 ----
    print("\n--- 8. 导出（响应头 + 单元格）---")
    py_s, py_h, py_b = export_call(ds.PY, ds.PY_TOKEN, "/system/dict/type/export")
    go_s, go_h, go_b = export_call(ds.GO, ds.GO_TOKEN, "/system/dict/type/export")
    ds.record("type export · HTTP 200", (py_s, go_s) == (200, 200), f"py={py_s} go={go_s}")
    try:
        py_t, py_rows = parse_xlsx(py_b)
        go_t, go_rows = parse_xlsx(go_b)
    except Exception as e:  # noqa: BLE001
        ds.record("type export · 工作簿可解析", False, f"{e}")
        py_rows = go_rows = []
        py_t = go_t = ""
    else:
        ds.record("type export · 工作簿可解析", True)
    ds.record("type export · 表头与 sheet 名一致",
              py_t == go_t == "字典类型"
              and py_rows[:1] == go_rows[:1]
              == [["字典主键", "字典名称", "字典类型", "备注", "创建时间"]],
              f"py={py_t}/{py_rows[:1]}\n           go={go_t}/{go_rows[:1]}")
    ds.record("type export · 数据行一致（id/createTime 归一化）",
              [[("<ID>" if i == 0 else "<TS>" if i == 4 else v) for i, v in enumerate(r)]
               for r in py_rows[1:]]
              == [[("<ID>" if i == 0 else "<TS>" if i == 4 else v) for i, v in enumerate(r)]
                  for r in go_rows[1:]],
              f"py={py_rows[1:3]}\n           go={go_rows[1:3]}")

    py_s, py_h, py_b = export_call(ds.PY, ds.PY_TOKEN, "/system/dict/data/export")
    go_s, go_h, go_b = export_call(ds.GO, ds.GO_TOKEN, "/system/dict/data/export")
    ds.record("data export · HTTP 200", (py_s, go_s) == (200, 200), f"py={py_s} go={go_s}")
    try:
        py_t, py_drows = parse_xlsx(py_b)
        go_t, go_drows = parse_xlsx(go_b)
    except Exception as e:  # noqa: BLE001
        ds.record("data export · 工作簿可解析", False, f"{e}")
        py_drows = go_drows = []
        py_t = go_t = ""
    else:
        ds.record("data export · 工作簿可解析", True)
    ds.record("data export · 表头与 sheet 名一致",
              py_t == go_t == "字典数据"
              and py_drows[:1] == go_drows[:1]
              == [["字典编码", "字典排序", "字典标签", "字典键值",
                   "字典类型", "是否默认", "备注", "创建时间"]],
              f"py={py_t}/{py_drows[:1]}\n           go={go_t}/{go_drows[:1]}")
    # isDefault 列必须已是「是/否」标签而不是 Y/N
    ds.record("data export · isDefault 已转 是/否 标签",
              any(len(r) > 5 and r[5] in ("是", "否") for r in py_drows[1:])
              and any(len(r) > 5 and r[5] in ("是", "否") for r in go_drows[1:])
              and not any(len(r) > 5 and r[5] in ("Y", "N") for r in py_drows[1:] + go_drows[1:]),
              f"py 样本={py_drows[1:3]}\n           go 样本={go_drows[1:3]}")
    ds.record("data export · 数据行一致（id/createTime 归一化）",
              [[("<ID>" if i == 0 else "<TS>" if i == 7 else v) for i, v in enumerate(r)]
               for r in py_drows[1:]]
              == [[("<ID>" if i == 0 else "<TS>" if i == 7 else v) for i, v in enumerate(r)]
                  for r in go_drows[1:]],
              f"py={py_drows[1:3]}\n           go={go_drows[1:3]}")

    # ---- 9. 框架级 ----
    print("\n--- 9. 框架级 ---")
    ds.compare_authed("type list · 尾斜杠应 307", f"{TYPE_LIST}/")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", TYPE_LIST)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", TYPE_LIST)
    ds.record("type list · 方法不匹配应 405",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    ds.compare_authed("type list · 清理后回到种子态（逐字节）", TYPE_LIST, strict=True)

    return ds._summary()


if __name__ == "__main__":
    sys.exit(main())
