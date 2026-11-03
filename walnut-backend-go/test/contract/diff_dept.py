"""部门模块（/system/dept，8 条路由）的双端契约比对。

复用 diff_system.py 的登录与比对机制（两端 SECRET_KEY 不同、Redis 会话独立，
必须各用各的 token）。覆盖：

  1. 列表：不分页数组形态、四级复合排序、各过滤条件、belongDeptId 子树
  2. list/exclude：排除节点及其子孙（元素级匹配）、非法路径参数
  3. treeselect：下拉树形态（children 键有无、排序、disabled）
  4. optionselect：无参 / deptIds / 非法 deptIds 的 400 文案
  5. 详情：存在 / 不存在（超管跳过数据权限 → data=null）/ 非法路径参数
  6. 写入校验：Pydantic 缺省校验器文案与顺序、长度/邮箱格式
  7. 增改删闭环：ancestors 维护、移动子孙同步、停用前置校验、删除四道 Warn
  8. 框架级：尾斜杠 307、方法不匹配 405

⚠️ 种子数据的部门 ID 是固定字面量（100/101/…），故种子态响应可以 strict 逐字节；
测试自建行的雪花 ID 两端必然不同，一律走 compare_authed_norm 归一化。

用法：
    python test/contract/diff_dept.py [--py http://127.0.0.1:8012] [--go http://127.0.0.1:8011]
"""

import argparse
import json
import sys
import time
import urllib.parse

import diff_system as ds

LIST = "/system/dept/list"


def q(value):
    """查询串里的中文必须百分号编码：urllib 不对 URL 做隐式编码，
    裸中文会直接抛 'ascii' codec can't encode（2026-11-03 实测）。"""
    return urllib.parse.quote(value)


def authed_call(base, token, method, path, body=None):
    """带 token 发任意方法的请求，返回 (status, headers, raw_text)。"""
    return ds.call(base, method, path, body=body, headers=ds.authed(token))


def compare_mutate(name, method, path, body=None):
    """两端各自发同一个写请求并比对响应。返回 (py_body, go_body)。"""
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
    """把「两端必然不同」的字段归一化（雪花 id / createTime）。"""
    if isinstance(obj, dict):
        return {
            k: ("<ID>" if k == "id" else "<TS>" if k == "createTime" else strip_volatile(v))
            for k, v in obj.items()
        }
    if isinstance(obj, list):
        return [strip_volatile(i) for i in obj]
    return obj


def compare_authed_norm(name, path):
    """带 token 比对 GET，但先归一化易变字段（测试自建行用）。"""
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", path)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", path)
    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    try:
        py_n = json.dumps(strip_volatile(json.loads(py_b)), sort_keys=True, ensure_ascii=False)
        go_n = json.dumps(strip_volatile(json.loads(go_b)), sort_keys=True, ensure_ascii=False)
    except Exception as e:  # noqa: BLE001
        diffs.append(f"响应体无法解析: {e}\n           py={py_b[:500]}\n           go={go_b[:500]}")
    else:
        if py_n != go_n:
            diffs.append(f"响应体（已归一化 id/createTime）:\n           py = {py_n}\n           go = {go_n}")
    ds.record(name, not diffs, "\n           ".join(diffs))


def list_rows(base, tok, path):
    _, _, raw = authed_call(base, tok, "GET", path)
    try:
        return (json.loads(raw).get("data") or [])
    except Exception:  # noqa: BLE001
        return []


def find_id(base, tok, name):
    """按名称取测试自建部门的 id（两端各取各的）。"""
    for row in list_rows(base, tok, f"{LIST}?deptName={q(name)}"):
        if row.get("deptName") == name:
            return row["id"]
    return None


def cleanup_leftovers():
    """清掉历次运行残留的测试部门（叶子优先，否则删除会被「存在下级」拦下）。

    "n" 是校验用例曾误建过的名称（email 长度边界写错的那次），一并清理。
    """
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        rows = [r for r in list_rows(base, tok, LIST)
                if (r.get("deptName") or "").startswith("契约测试部")
                or r.get("deptName") == "n"]
        rows.sort(key=lambda r: len(r.get("ancestors") or ""), reverse=True)
        for r in rows:
            authed_call(base, tok, "DELETE", f"/system/dept/{r['id']}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()

    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"部门模块契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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

    # ---- 1. 列表（种子态可逐字节）----
    print("\n--- 1. 列表与过滤 ---")
    ds.compare_authed("list · 全量（不分页数组 + 四级复合排序）", LIST, strict=True)
    ds.compare_authed("list · deptName 过滤", f"{LIST}?deptName=%E7%A0%94", strict=True)
    ds.compare_authed("list · status 过滤", f"{LIST}?status=1", strict=True)
    ds.compare_authed("list · parentId 过滤", f"{LIST}?parentId=100", strict=True)
    ds.compare_authed("list · id 过滤", f"{LIST}?id=100", strict=True)
    ds.compare_authed("list · belongDeptId 子树（含自身）", f"{LIST}?belongDeptId=101", strict=True)
    ds.compare_authed("list · begin/end 上界为当日 00:00（种子创建时间均晚于该日 → 空）",
                      f"{LIST}?beginTime=2020-01-01&endTime=2020-01-02", strict=True)

    # ---- 2. list/exclude ----
    print("\n--- 2. 排除节点及其子孙 ---")
    ds.compare_authed("exclude · 排除根 100（其余全是其子孙 → 空数组）",
                      "/system/dept/list/exclude/100", strict=True)
    ds.compare_authed("exclude · 排除 101（其子孙一并排除）",
                      "/system/dept/list/exclude/101", strict=True)
    ds.compare_authed("exclude · 路径参数非整数（Pydantic 文案）",
                      "/system/dept/list/exclude/abc")

    # ---- 3. treeselect ----
    print("\n--- 3. 下拉树 ---")
    ds.compare_authed("treeselect · 全量树（children 键有无 + 排序）",
                      "/system/dept/treeselect", strict=True)
    ds.compare_authed("treeselect · deptName 过滤（多根形态）",
                      "/system/dept/treeselect?deptName=%E7%A0%94", strict=True)

    # ---- 4. optionselect ----
    print("\n--- 4. 选择框 ---")
    ds.compare_authed("optionselect · 无参数（全部正常部门，仅三字段有值）",
                      "/system/dept/optionselect", strict=True)
    ds.compare_authed("optionselect · deptIds=100,101",
                      "/system/dept/optionselect?deptIds=100,101", strict=True)
    ds.compare_authed("optionselect · deptIds 含非数字（400 文案）",
                      "/system/dept/optionselect?deptIds=1,abc")

    # ---- 5. 详情 ----
    print("\n--- 5. 详情 ---")
    ds.compare_authed("detail · 已存在的部门（parentName/leaderName 回填）",
                      "/system/dept/100", strict=True)
    ds.compare_authed("detail · 不存在的部门（超管跳过数据权限 → data=null）",
                      "/system/dept/999999999999")
    ds.compare_authed("detail · 路径参数非整数（Pydantic 文案）", "/system/dept/abc")

    # ---- 6. 写入校验 ----
    print("\n--- 6. 新增/修改校验（文案与顺序）---")
    compare_mutate("add · 空对象（Field required + 两个缺省校验器按声明序）",
                   "POST", "/system/dept", body={})
    compare_mutate("add · 缺 parentId（Field required）",
                   "POST", "/system/dept", body={"deptName": "n", "orderNum": 1})
    compare_mutate("add · 缺 orderNum（显示顺序不能为空）",
                   "POST", "/system/dept", body={"parentId": 100, "deptName": "n"})
    compare_mutate("add · deptName 空白串",
                   "POST", "/system/dept", body={"parentId": 100, "deptName": "   ", "orderNum": 1})
    compare_mutate("add · deptName 超长（>30 字符，按字符数）",
                   "POST", "/system/dept", body={"parentId": 100, "deptName": "部" * 31, "orderNum": 1})
    compare_mutate("add · deptCategory 超长（>100 字符）",
                   "POST", "/system/dept",
                   body={"parentId": 100, "deptName": "n", "deptCategory": "x" * 101, "orderNum": 1})
    compare_mutate("add · phone 超长（>11 字符）",
                   "POST", "/system/dept",
                   body={"parentId": 100, "deptName": "n", "orderNum": 1, "phone": "1" * 12})
    compare_mutate("add · email 格式不正确",
                   "POST", "/system/dept",
                   body={"parentId": 100, "deptName": "n", "orderNum": 1, "email": "bad"})
    compare_mutate("add · email 超长（>50 字符；51 个字符才超，50 整是合法边界）",
                   "POST", "/system/dept",
                   body={"parentId": 100, "deptName": "n", "orderNum": 1,
                         "email": "a" * 46 + "@b.co"})
    compare_mutate("add · parentId 类型错误（Pydantic 原文）",
                   "POST", "/system/dept", body={"parentId": "abc", "deptName": "n", "orderNum": 1})
    compare_mutate("add · 多字段同时非法（按声明序拼接）",
                   "POST", "/system/dept", body={"deptName": "", "orderNum": None, "email": 3})
    compare_mutate("update · 缺 id（Field required）",
                   "PUT", "/system/dept", body={"parentId": 100, "deptName": "n", "orderNum": 1})

    # ---- 7. 增改删闭环 ----
    print("\n--- 7. 增改删闭环（ancestors 维护 + 删除四道 Warn）---")
    stamp = str(int(time.time()))[-6:]
    name_a = f"契约测试部A{stamp}"
    name_b = f"契约测试部B{stamp}"
    name_c = f"契约测试部C{stamp}"

    compare_mutate("add · 正常新增 A（挂 100 下）", "POST", "/system/dept",
                   body={"parentId": 100, "deptName": name_a, "orderNum": 88, "status": "0"})
    compare_mutate("add · 正常新增 B（挂 100 下）", "POST", "/system/dept",
                   body={"parentId": 100, "deptName": name_b, "orderNum": 89, "status": "0"})

    id_a_py = find_id(ds.PY, ds.PY_TOKEN, name_a)
    id_a_go = find_id(ds.GO, ds.GO_TOKEN, name_a)
    id_b_py = find_id(ds.PY, ds.PY_TOKEN, name_b)
    id_b_go = find_id(ds.GO, ds.GO_TOKEN, name_b)
    ds.record("add · 两端均可按名称取回新建 id",
              None not in (id_a_py, id_a_go, id_b_py, id_b_go),
              f"py A={id_a_py} B={id_b_py}; go A={id_a_go} B={id_b_go}")

    # ancestors = 父.ancestors + "," + 父id = "0,100"；parentName 回填「核桃科技」
    compare_authed_norm("list · 新增 A 可查且 ancestors=0,100、parentName 回填",
                        f"{LIST}?deptName={q(name_a)}")

    compare_mutate("add · 同父级同名应报「部门名称已存在」", "POST", "/system/dept",
                   body={"parentId": 100, "deptName": name_a, "orderNum": 90})

    # C 的 parentId 用两端各自的 A.id（两端不同），故逐端发送：
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "POST", "/system/dept",
                                body={"parentId": id_a_py, "deptName": name_c,
                                      "orderNum": 1, "status": "0"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "POST", "/system/dept",
                                body={"parentId": id_a_go, "deptName": name_c,
                                      "orderNum": 1, "status": "0"})
    ds.record("add · 新增 C（挂 A 下）两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    id_c_py = find_id(ds.PY, ds.PY_TOKEN, name_c)
    id_c_go = find_id(ds.GO, ds.GO_TOKEN, name_c)

    # C 的 ancestors 两端各自含本端 A.id，故逐端断言形态而不是互相比对
    def ancestors_of(base, tok, dept_id):
        for row in list_rows(base, tok, f"{LIST}?id={dept_id}"):
            return row.get("ancestors")
        return None

    anc_py = ancestors_of(ds.PY, ds.PY_TOKEN, id_c_py)
    anc_go = ancestors_of(ds.GO, ds.GO_TOKEN, id_c_go)
    ds.record("add · C 的 ancestors = 0,100,<本端A.id>",
              anc_py == f"0,100,{id_a_py}" and anc_go == f"0,100,{id_a_go}",
              f"py={anc_py} want=0,100,{id_a_py}; go={anc_go} want=0,100,{id_a_go}")

    # 停用 A 的 body 含本端 id，逐端发送：
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/dept",
                                body={"id": id_a_py, "parentId": 100, "deptName": name_a,
                                      "orderNum": 88, "status": "1"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/dept",
                                body={"id": id_a_go, "parentId": 100, "deptName": name_a,
                                      "orderNum": 88, "status": "1"})
    ds.record("update · 停用 A（含未停用子部门 C）应报「该部门包含未停用的子部门!」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and "该部门包含未停用的子部门!" in py_b,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/dept/{id_a_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/dept/{id_a_go}")
    ds.record("delete · 删除 A（存在下级 C）应 601「存在下级部门,不允许删除」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 601,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    # 移动 C 到 B 下：子孙 ancestors 同步替换（C 是叶子，只验自身）
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/dept",
                                body={"id": id_c_py, "parentId": id_b_py, "deptName": name_c,
                                      "orderNum": 1, "status": "0"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/dept",
                                body={"id": id_c_go, "parentId": id_b_go, "deptName": name_c,
                                      "orderNum": 1, "status": "0"})
    ds.record("update · 移动 C 到 B 下两端响应一致",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    anc_py = ancestors_of(ds.PY, ds.PY_TOKEN, id_c_py)
    anc_go = ancestors_of(ds.GO, ds.GO_TOKEN, id_c_go)
    ds.record("update · 移动后 C 的 ancestors = 0,100,<本端B.id>",
              anc_py == f"0,100,{id_b_py}" and anc_go == f"0,100,{id_b_go}",
              f"py={anc_py} want=0,100,{id_b_py}; go={anc_go} want=0,100,{id_b_go}")

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/dept",
                                body={"id": id_a_py, "parentId": id_a_py, "deptName": name_a,
                                      "orderNum": 88})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/dept",
                                body={"id": id_a_go, "parentId": id_a_go, "deptName": name_a,
                                      "orderNum": 88})
    ds.record("update · 上级部门是自己应报「上级部门不能是自己」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and "上级部门不能是自己" in py_b,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    # C 已移走，A 无子部门 → 停用成功（顺带覆盖「启用上级链」分支的反向路径）
    # ⚠️ orderNum 改成 87：RepeatSubmit 的防重键含请求体，与上面「停用被拦」那次
    # 同 body 会在窗口内被「不允许重复提交」拦下（两端行为一致，但会毁掉本断言）。
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/dept",
                                body={"id": id_a_py, "parentId": 100, "deptName": name_a,
                                      "orderNum": 87, "status": "1"})
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/dept",
                                body={"id": id_a_go, "parentId": 100, "deptName": name_a,
                                      "orderNum": 87, "status": "1"})
    ds.record("update · 停用 A（已无子部门）应成功",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("code") == 200,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    # 删除闭环：C → A → B（叶子优先）
    for label, id_py, id_go in (("C", id_c_py, id_c_go), ("A", id_a_py, id_a_go),
                                ("B", id_b_py, id_b_go)):
        py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/dept/{id_py}")
        go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/dept/{id_go}")
        ds.record(f"delete · 删除 {label} 应成功（逻辑删除）",
                  (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
                  and json.loads(py_b).get("code") == 200,
                  f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "GET", f"/system/dept/{id_c_py}")
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "GET", f"/system/dept/{id_c_go}")
    ds.record("delete · 逻辑删除后详情 data=null",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json.loads(py_b).get("data") is None,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    compare_mutate("delete · 默认部门 100 应 601「默认部门,不允许删除」",
                   "DELETE", "/system/dept/100")
    compare_mutate("delete · ID 非整数（Pydantic 文案）", "DELETE", "/system/dept/abc")

    # ---- 8. 框架级 + 收尾种子态回归 ----
    print("\n--- 8. 框架级与收尾回归 ---")
    ds.compare_authed("list · 尾斜杠应 307", f"{LIST}/")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", LIST)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", LIST)
    ds.record("list · 方法不匹配应 405",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    ds.compare_authed("treeselect · 清理后回到种子态（逐字节）",
                      "/system/dept/treeselect", strict=True)

    return ds._summary()


if __name__ == "__main__":
    sys.exit(main())
