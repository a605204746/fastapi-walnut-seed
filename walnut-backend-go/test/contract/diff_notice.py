"""通知公告模块（/system/notice，5 条路由）的双端契约比对。

覆盖重点：
  1. 列表 id 主排序在前、用户排序在后；createByName 反查用户（查不到 → 空结果）
  2. notice_content 是 BLOB：出参转回字符串（种子 hex 内容逐字节对齐）
  3. 标题校验带 validate_default：空 body → 400「公告标题不能为空」；脚本字符拦截
  4. update 的 id 缺省报「公告ID不能为空」（不是 Field required）
  5. 删除不存在的 id → 500「操作失败」（rowcount=0）；ID 串文案「公告ID格式有误/不能为空」
  6. createByName 回填（admin）

用法：
    python test/contract/diff_notice.py [--py ...] [--go ...]
"""

import argparse
import json
import sys
import time

import diff_system as ds

LIST = "/system/notice/list"


def q(value):
    return urllib_parse_quote(value)


def urllib_parse_quote(value):
    import urllib.parse
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


def notice_rows(base, tok, query=""):
    _, _, raw = authed_call(base, tok, "GET", LIST + query)
    try:
        return (json.loads(raw).get("data") or {}).get("rows") or []
    except Exception:  # noqa: BLE001
        return []


def cleanup_leftovers():
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        for r in notice_rows(base, tok, f"?noticeTitle={q('契约测试公告')}&pageSize=1000"):
            authed_call(base, tok, "DELETE", f"/system/notice/{r['id']}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"通知公告契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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
    title = f"契约测试公告{stamp}"

    # ---- 1. 列表 ----
    print("\n--- 1. 列表与过滤 ---")
    ds.compare_authed("list · 默认分页（id 主排序 + createByName 回填）", LIST, strict=True)
    ds.compare_authed("list · noticeTitle like", f"{LIST}?noticeTitle=%E7%BB%B4%E6%8A%A4",
                      strict=True)
    ds.compare_authed("list · noticeType 等值", f"{LIST}?noticeType=1", strict=True)
    ds.compare_authed("list · createByName=admin（反查用户ID）",
                      f"{LIST}?createByName=admin", strict=True)
    ds.compare_authed("list · createByName 不存在用户 → 空结果",
                      f"{LIST}?createByName=qw_no_such_user", strict=True)

    # ---- 2. 详情 ----
    print("\n--- 2. 详情（BLOB 转字符串）---")
    ds.compare_authed("detail · 种子 id=1（content 为 BLOB 解码串）",
                      "/system/notice/1", strict=True)
    ds.compare_authed("detail · 不存在 → data=null", "/system/notice/999999999999")
    ds.compare_authed("detail · 路径参数非整数（Pydantic 原文）", "/system/notice/abc")

    # ---- 3. 校验 ----
    print("\n--- 3. 校验 ---")
    compare_mutate("add · 空 body → 400「公告标题不能为空」（validate_default）",
                   "POST", "/system/notice", body={})
    compare_mutate("add · 标题超长（>50）", "POST", "/system/notice",
                   body={"noticeTitle": "标" * 51, "noticeType": "1"})
    compare_mutate("add · 标题含脚本字符", "POST", "/system/notice",
                   body={"noticeTitle": "a<script>b", "noticeType": "1"})
    compare_mutate("add · 缺 noticeType → 409「数据已存在或违反完整性约束」",
                   "POST", "/system/notice", body={"noticeTitle": "无类型公告"})
    compare_mutate("update · 缺 id → 「公告ID不能为空」", "PUT", "/system/notice",
                   body={"noticeTitle": "x"})

    # ---- 4. 增改删闭环 ----
    print("\n--- 4. 增改删闭环 ---")
    compare_mutate("add · 正常新增（content 含 HTML，仅标题拦脚本）", "POST", "/system/notice",
                   body={"noticeTitle": title, "noticeType": "1",
                         "noticeContent": "<p>契约内容</p>", "status": "0", "remark": "diff"})
    nid_py = next((r["id"] for r in notice_rows(ds.PY, ds.PY_TOKEN,
                   f"?noticeTitle={q(title)}")), None)
    nid_go = next((r["id"] for r in notice_rows(ds.GO, ds.GO_TOKEN,
                   f"?noticeTitle={q(title)}")), None)
    ds.record("add · 两端取回新建 id", nid_py is not None and nid_go is not None,
              f"py={nid_py} go={nid_go}")
    compare_authed_norm("detail · 新增后 content 原样回显 + createByName 回填",
                        f"{LIST}?noticeTitle={q(title)}")

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/notice",
                                body={"id": nid_py, "noticeTitle": title + "改",
                                      "noticeType": "2"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/notice",
                                body={"id": nid_go, "noticeTitle": title + "改",
                                      "noticeType": "2"})
    ds.record("update · 改标题/类型两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    compare_authed_norm("detail · 修改后标题/类型生效、content 保留",
                        f"{LIST}?noticeTitle={q(title + '改')}")

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/notice/{nid_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/notice/{nid_go}")
    ds.record("delete · 删除成功",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 200,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    compare_mutate("delete · 再删同一 id → 500「操作失败」（rowcount=0）",
                   "DELETE", f"/system/notice/{nid_py}")
    compare_mutate("delete · ID 串含非数字 → 400「公告ID格式有误」",
                   "DELETE", "/system/notice/1,abc")
    compare_mutate("delete · 空串 → 400「公告ID不能为空」", "DELETE", "/system/notice/%20")

    # ---- 5. 框架级 ----
    print("\n--- 5. 框架级 ---")
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
