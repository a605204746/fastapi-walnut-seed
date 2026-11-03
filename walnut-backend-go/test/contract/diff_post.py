"""岗位模块（/system/post，8 条路由）的双端契约比对。

复用 diff_system.py 的登录与比对机制（两端 SECRET_KEY 不同、Redis 会话独立，
必须各用各的 token）。本脚本聚焦 P3 铺开阶段的第一个业务模块，覆盖：

  1. 列表查询：默认分页、条件过滤、分页参数校验文案
  2. 排序：orderByColumn/isAsc 的多列展开、注入防护、参数不匹配
  3. optionselect 的三分支优先级、deptTree 的树形态（children 键的有无）
  4. 详情：存在 / 不存在 / 路径参数非整数 三种文案
  5. 写入：Pydantic 风格的校验错误拼接、唯一性冲突文案、增改删闭环
  6. 框架级：尾斜杠 307、方法不匹配 405
  7. 导出：响应头形态 + 解析后的单元格内容（**字节不可能一致**，见 excelx 注释）

用法：
    python test/contract/diff_post.py [--py http://127.0.0.1:8012] [--go http://127.0.0.1:8011]
"""

import argparse
import json
import sys
import time

import diff_system as ds

LIST = "/system/post/list"


def authed_call(base, token, method, path, body=None, headers=None, raw_body=None,
                content_type="application/json"):
    """带 token 发任意方法的请求，返回 (status, headers, raw_text)。"""
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


def compare_mutate(name, method, path, body=None, raw_body=None, content_type=None,
                   normalize_ids=True):
    """两端各自发同一个写请求并比对响应。返回 (py_body, go_body)。"""
    kw = {}
    if raw_body is not None:
        kw = {"raw_body": raw_body, "content_type": content_type}
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, method, path, body=body, **kw)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, method, path, body=body, **kw)

    diffs = []
    if py_s != go_s:
        diffs.append(f"HTTP 状态码: py={py_s} go={go_s}")
    if ds.canon(py_b) != ds.canon(go_b):
        diffs.append(f"响应体:\n           py = {ds.canon(py_b)}\n           go = {ds.canon(go_b)}")
    ds.record(name, not diffs, "\n           ".join(diffs))
    return py_b, go_b


def first_post_id(body):
    """从列表响应里取第一条岗位的 id（两端各取各的，仅用于构造详情请求）。"""
    try:
        rows = (json.loads(body).get("data") or {}).get("rows") or []
        return rows[0]["id"] if rows else None
    except Exception:
        return None


def strip_volatile(obj):
    """把「两端必然不同」的字段归一化。

    雪花 ID 由各自的生成器产出，两端库里同一条业务数据的 id 一定不同，
    这不是契约偏差；createTime 也因两次写入的时刻差而不同。
    """
    if isinstance(obj, dict):
        return {
            k: ("<ID>" if k == "id" else "<TS>" if k == "createTime" else strip_volatile(v))
            for k, v in obj.items()
        }
    if isinstance(obj, list):
        return [strip_volatile(i) for i in obj]
    return obj


def compare_authed_norm(name, path):
    """带 token 比对 GET，但先归一化易变字段。"""
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


def cleanup_leftovers():
    """清掉历次运行残留的测试岗位。

    ⚠️ 必须做：残留行的雪花 ID 两端不同，会让之后**每一个**列表比对都失败，
    而失败原因看起来像产品偏差，实际是测试污染（第一版脚本就踩了这个坑）。
    """
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        for prefix in ("c1", "gotest"):
            _, _, raw = authed_call(base, tok, "GET", f"{LIST}?postCode={prefix}&pageSize=1000")
            try:
                rows = (json.loads(raw).get("data") or {}).get("rows") or []
            except Exception:  # noqa: BLE001
                continue
            for r in rows:
                authed_call(base, tok, "DELETE", f"/system/post/{r['id']}")


def export_call(base, token, form_pairs):
    """POST 导出端点，返回 (status, headers, **bytes**)。

    ⚠️ 必须返回原始字节：xlsx 是 zip，按文本解码会损坏内容。
    """
    import urllib.error
    import urllib.parse
    import urllib.request

    body = urllib.parse.urlencode(form_pairs).encode()
    req = urllib.request.Request(base.rstrip("/") + "/system/post/export",
                                 data=body, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    req.add_header("clientid", ds.CLIENT_ID)
    req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, dict(r.headers), r.read()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read()


def parse_xlsx(data):
    """解析工作簿，返回 (sheet名, [[单元格字符串,...],...])。

    单元格统一转字符串比较：两端写入的类型可能不同（数字 vs 文本），
    而契约关心的是**用户看到的内容**。

    ⚠️ 必须 read_only=False：read_only 模式下 openpyxl 依赖工作表里的
    <dimension> 声明来确定范围，而 excelize 写出的 dimension 与 openpyxl
    的假设不兼容，会把 9 列截断成 1 列（实测踩过：go 表头只解析出「岗位序号」）。
    """
    import io

    from openpyxl import load_workbook

    wb = load_workbook(io.BytesIO(data), read_only=False, data_only=True)
    ws = wb.active
    rows = [["" if c is None else str(c) for c in row] for row in ws.iter_rows(values_only=True)]
    title = ws.title
    wb.close()
    return title, rows


def hdr(headers, name):
    """大小写不敏感地取响应头。

    ⚠️ 必须用这个而不是 headers.get(name)：Starlette 发出的是**全小写**头名
    （content-disposition），Go 的 net/http 会规范化成 **Content-Disposition**。
    HTTP 头名本就大小写不敏感（RFC 7230），两种写法都不是偏差；
    但 dict 查找是大小写敏感的，直接 get 会得到「一端有值一端 None」的假失败。
    """
    low = name.lower()
    for k, v in headers.items():
        if k.lower() == low:
            return v
    return None


def compare_export():
    """第 8 组：导出端点。

    ⚠️ xlsx **字节不可能逐字节一致**（openpyxl 与 excelize 是不同的 zip 实现，
    压缩级别、内部条目顺序、样式 XML 都不同）。故契约断言只覆盖：
    响应头形态 + 工作表名 + 表头行逐格一致 + 数据行逐格一致。
    """
    import re
    import urllib.parse

    print("\n--- 8. 导出（响应头 + 解析后的单元格内容）---")
    py_s, py_h, py_b = export_call(ds.PY, ds.PY_TOKEN, [("postName", ""), ("status", "")])
    go_s, go_h, go_b = export_call(ds.GO, ds.GO_TOKEN, [("postName", ""), ("status", "")])

    ds.record("export · HTTP 200", (py_s, go_s) == (200, 200), f"py={py_s} go={go_s}")

    ctype = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    ds.record("export · Content-Type 一致且为 xlsx",
              (hdr(py_h, "Content-Type") or "").split(";")[0] == ctype
              and (hdr(go_h, "Content-Type") or "").split(";")[0] == ctype,
              f"py={hdr(py_h, 'Content-Type')} go={hdr(go_h, 'Content-Type')}")

    # 文件名形如 岗位数据_{yyyy-MM-dd HH:mm:ss}.xlsx，时间部分需归一化
    pat = re.compile(r"^attachment;filename\*=utf-8''(.+)$")

    def enc_part(h):
        m = pat.match(hdr(h, "Content-Disposition") or "")
        return m.group(1) if m else None

    def norm_disp(h):
        part = enc_part(h)
        if part is None:
            return f"<形态不符: {hdr(h, 'Content-Disposition')}>"
        name = urllib.parse.unquote(part)
        return re.sub(r"_\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.xlsx$", "_<TS>.xlsx", name)

    py_d, go_d = norm_disp(py_h), norm_disp(go_h)
    ds.record("export · Content-Disposition 形态与文件名前缀一致",
              py_d == go_d == "岗位数据_<TS>.xlsx", f"py={py_d} go={go_d}")

    ds.record("export · download-filename 与 Content-Disposition 内编码串一致",
              enc_part(py_h) is not None and enc_part(py_h) == hdr(py_h, "download-filename")
              and enc_part(go_h) is not None and enc_part(go_h) == hdr(go_h, "download-filename"),
              f"py={hdr(py_h, 'download-filename')} go={hdr(go_h, 'download-filename')}")

    # 空格必须编码成 %20（Python urllib.parse.quote 的行为），不能是 +
    def space_ok(h):
        v = hdr(h, "download-filename") or ""
        return "%20" in v and "+" not in v

    ds.record("export · 文件名空格编码为 %20 而非 +",
              space_ok(py_h) and space_ok(go_h),
              f"py={hdr(py_h, 'download-filename')} go={hdr(go_h, 'download-filename')}")

    ds.record("export · Access-Control-Expose-Headers 一致",
              hdr(py_h, "Access-Control-Expose-Headers")
              == hdr(go_h, "Access-Control-Expose-Headers")
              == "Content-Disposition,download-filename",
              f"py={hdr(py_h, 'Access-Control-Expose-Headers')} "
              f"go={hdr(go_h, 'Access-Control-Expose-Headers')}")

    try:
        py_t, py_rows = parse_xlsx(py_b)
        go_t, go_rows = parse_xlsx(go_b)
    except Exception as e:  # noqa: BLE001
        ds.record("export · 工作簿可被解析", False, f"解析失败: {e}")
        return
    ds.record("export · 工作簿可被解析", True)
    ds.record("export · 工作表名一致（岗位数据）",
              py_t == go_t == "岗位数据", f"py={py_t} go={go_t}")

    # ★ 表头 + 数据行共同构成那个「6 列恒为空」缺陷的探针：
    # 键名对不上时表头文字仍然正确，但数据行会全空，故两处都要断言。
    want_header = ["岗位序号", "部门id", "岗位编码", "岗位名称", "类别编码",
                   "岗位排序", "状态", "备注", "创建时间"]
    ds.record("export · 表头行逐格一致（顺序即列序）",
              py_rows[0] == go_rows[0] == want_header,
              f"py={py_rows[0]}\n           go={go_rows[0]}\n           want={want_header}")

    def norm_data(rows):
        """归一化两端必然不同的列：雪花 id、createTime。"""
        out = []
        for r in rows[1:]:
            r = list(r)
            if r:
                r[0] = "<ID>"
                if len(r) > 8:
                    r[8] = "<TS>"
            out.append(r)
        return out

    ds.record("export · 数据行逐格一致（id/createTime 归一化）",
              norm_data(py_rows) == norm_data(go_rows),
              f"py={norm_data(py_rows)[:4]}\n           go={norm_data(go_rows)[:4]}")

    # ★ 缺陷回归断言：修复前这 6 列恒为空串
    nonempty_cols = [1, 2, 3, 4, 5, 8]  # 部门id/编码/名称/类别/排序/创建时间
    if len(go_rows) > 1:
        sample = go_rows[1]
        blank = [want_header[i] for i in nonempty_cols if i < len(sample) and sample[i] == ""]
        # 类别编码(postCategory) 在种子数据里本就可能是 NULL → 空，故单独豁免
        blank = [b for b in blank if b != "类别编码"]
        ds.record("export · 此前恒为空的列现在确有内容（缺陷已修）",
                  not blank, f"仍为空的列: {blank}；样本行={sample}")
    else:
        ds.record("export · 此前恒为空的列现在确有内容（缺陷已修）", False, "导出无数据行")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()

    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"岗位模块契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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

    # ---- 1. 列表与分页 ----
    print("\n--- 1. 列表查询与分页参数校验 ---")
    ds.compare_authed("list · 默认分页（pageSize 缺省应为 10）", LIST, strict=True)
    ds.compare_authed("list · 显式分页 pageNum=1&pageSize=5", f"{LIST}?pageNum=1&pageSize=5", strict=True)
    ds.compare_authed("list · pageNum=0 触发 ge=1 校验", f"{LIST}?pageNum=0")
    ds.compare_authed("list · pageSize=1001 触发 le=1000 校验", f"{LIST}?pageSize=1001")
    ds.compare_authed("list · pageSize 非整数", f"{LIST}?pageSize=abc")
    ds.compare_authed("list · pageNum 与 pageSize 同时非法（两条错误按序拼接）",
                      f"{LIST}?pageNum=0&pageSize=abc")
    ds.compare_authed("list · 空 postName 不施加过滤", f"{LIST}?postName=")

    # ---- 2. 排序 ----
    print("\n--- 2. 排序（注入防护 + 多列展开）---")
    ds.compare_authed("list · 单列 desc", f"{LIST}?orderByColumn=postSort&isAsc=desc", strict=True)
    ds.compare_authed("list · 多列共用一个方向", f"{LIST}?orderByColumn=postSort,id&isAsc=desc", strict=True)
    ds.compare_authed("list · 多列各自方向", f"{LIST}?orderByColumn=postSort,id&isAsc=desc,asc", strict=True)
    ds.compare_authed("list · 未知列被静默跳过", f"{LIST}?orderByColumn=nonExistent", strict=True)
    ds.compare_authed("list · 注入载体被字符集校验拦下", f"{LIST}?orderByColumn=id;DROP TABLE sys_post")
    ds.compare_authed("list · isAsc 全为分隔符 → 排序参数有误", f"{LIST}?orderByColumn=postSort&isAsc=,")
    ds.compare_authed("list · 列数与方向数不符", f"{LIST}?orderByColumn=postSort,id&isAsc=asc,desc,asc")

    # ---- 3. optionselect / deptTree ----
    print("\n--- 3. optionselect 与 deptTree ---")
    ds.compare_authed("optionselect · 无参数应返回空数组", "/system/post/optionselect")
    ds.compare_authed("deptTree · 全量部门树（children 键有无是契约）",
                      "/system/post/deptTree", strict=True)
    ds.compare_authed("deptTree · 按 deptName 过滤", "/system/post/deptTree?deptName=%E7%A0%94")

    # ---- 4. 详情 ----
    print("\n--- 4. 详情：存在 / 不存在 / 非法路径参数 ---")
    _, _, go_list = authed_call(ds.GO, ds.GO_TOKEN, "GET", LIST)
    pid = first_post_id(go_list)
    if pid is not None:
        ds.compare_authed("detail · 已存在的岗位", f"/system/post/{pid}")
    else:
        ds.record("detail · 已存在的岗位", False, "种子数据里没有岗位，无法测试")
    ds.compare_authed("detail · 不存在的岗位（无权限访问岗位数据!）", "/system/post/999999999999")
    ds.compare_authed("detail · 路径参数非整数（Pydantic 文案）", "/system/post/abc")

    # ---- 5. 框架级行为 ----
    print("\n--- 5. 框架级：尾斜杠与方法不匹配 ---")
    ds.compare_authed("list · 尾斜杠应 307", f"{LIST}/")
    py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", LIST)
    go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", LIST)
    ds.record("list · 方法不匹配应 405",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    # ---- 6. 写入：校验文案 ----
    print("\n--- 6. 新增校验（Pydantic field_validator 文案与顺序）---")
    compare_mutate("add · 空对象应触发全部非空校验（按声明序拼接）", "POST", "/system/post", body={})
    compare_mutate("add · 缺 postSort", "POST", "/system/post",
                   body={"deptId": 100, "postCode": "c1", "postName": "n1"})
    compare_mutate("add · postCode 为空白串", "POST", "/system/post",
                   body={"deptId": 100, "postCode": "   ", "postName": "n1", "postSort": 1})
    compare_mutate("add · postName 超长（>50 字符，按字符数不是字节数）", "POST", "/system/post",
                   body={"deptId": 100, "postCode": "c1", "postName": "名" * 51, "postSort": 1})
    compare_mutate("add · postCategory 超长（>100 字符）", "POST", "/system/post",
                   body={"deptId": 100, "postCode": "c1", "postName": "n1",
                         "postSort": 1, "postCategory": "x" * 101})
    # ⚠️ 这个用例刻意用**种子数据里已存在的编码 ceo**：
    # 请求会撞唯一性校验而失败，故不会真的落库。
    # 而「失败原因是编码重复」而非「deptId 类型错误」，正好证明字符串 "100"
    # 被 Pydantic/go 两侧都按 lax 模式成功强转成了整数——既测到强制转换，
    # 又不留下两端 id 不同的脏数据（那会污染后续所有列表比对）。
    compare_mutate("add · deptId 传字符串数字应被 lax 模式接受（撞唯一性校验故不落库）",
                   "POST", "/system/post",
                   body={"deptId": "100", "postCode": "ceo", "postName": "n1",
                         "postSort": 1, "status": "1"})
    compare_mutate("update · 缺 id", "PUT", "/system/post",
                   body={"deptId": 100, "postCode": "c1", "postName": "n1", "postSort": 1})

    # ---- 7. 增改删闭环（用唯一编码避免重复运行冲突）----
    print("\n--- 7. 增改删闭环 ---")
    stamp = str(int(time.time()))[-6:]
    code = f"gotest{stamp}"
    name = f"契约测试岗{stamp}"

    compare_mutate("add · 正常新增", "POST", "/system/post",
                   body={"deptId": 100, "postCode": code, "postName": name,
                         "postSort": 999, "status": "0", "remark": "contract-diff"})

    # ⚠️ py 端「写后立即可读」存在毫秒级竞态：全应用无显式 commit()，
    # db_getter 的 session.begin() 在 yield teardown 才提交，而 FastAPI ≥0.106
    # 的 teardown 发生在**响应发送之后** → 新增返回 200 时提交可能尚未落库，
    # 紧跟的查询会读到空（2026-11-03 实测命中一次）。Go 端在写响应前提交，无此窗口。
    # 故此处只对 py 加重试窗口，把断言语义定为「5 秒内可见」；两端实现行为都不改，
    # 也不要在 Go 端镜像这个竞态。见 docs/go/01-续作交接.md §6。
    deadline = time.time() + 5
    while time.time() < deadline:
        _, _, raw = authed_call(ds.PY, ds.PY_TOKEN, "GET", f"{LIST}?postCode={code}")
        if (json.loads(raw).get("data") or {}).get("rows"):
            break
        time.sleep(0.1)

    # 新增后按编码查回来，验证落库字段与列表形态
    compare_authed_norm("list · 新增后可按 postCode 查到（id/createTime 归一化）",
                        f"{LIST}?postCode={code}")

    compare_mutate("add · 同部门同名应报「岗位名称已存在」", "POST", "/system/post",
                   body={"deptId": 100, "postCode": f"{code}b", "postName": name,
                         "postSort": 999, "status": "0"})
    compare_mutate("add · 同编码应报「岗位编码已存在」", "POST", "/system/post",
                   body={"deptId": 100, "postCode": code, "postName": f"{name}二",
                         "postSort": 999, "status": "0"})

    # 取回新建记录的 id（两端各自的库里 id 不同，故各取各的）
    ids = {}
    for side, base, tok in (("py", ds.PY, ds.PY_TOKEN), ("go", ds.GO, ds.GO_TOKEN)):
        _, _, raw = authed_call(base, tok, "GET", f"{LIST}?postCode={code}")
        rows = (json.loads(raw).get("data") or {}).get("rows") or []
        ids[side] = rows[0]["id"] if rows else None
    if ids["py"] is None or ids["go"] is None:
        ds.record("update/delete · 取回新建记录 id", False, f"未能取回: {ids}")
    else:
        ds.record("update/delete · 取回新建记录 id", True)
        # 两端库里的 id 不同，故各自构造请求体、再比对响应
        py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "PUT", "/system/post",
                                    body={"id": int(ids["py"]), "status": "1"})
        go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "PUT", "/system/post",
                                    body={"id": int(ids["go"]), "status": "1"})
        ds.record("update · 停用未分配用户的岗位",
                  (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
                  f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

        py_s, _, py_b = authed_call(ds.PY, ds.PY_TOKEN, "DELETE", f"/system/post/{ids['py']}")
        go_s, _, go_b = authed_call(ds.GO, ds.GO_TOKEN, "DELETE", f"/system/post/{ids['go']}")
        ds.record("delete · 删除未分配用户的岗位",
                  (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
                  f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    compare_mutate("delete · ID 串含非数字（岗位ID格式有误）", "DELETE", "/system/post/1,abc")
    compare_mutate("delete · ID 串为空（岗位ID不能为空）", "DELETE", "/system/post/%20")

    # ---- 8. 导出 ----
    compare_export()

    return ds._summary()


if __name__ == "__main__":
    sys.exit(main())
