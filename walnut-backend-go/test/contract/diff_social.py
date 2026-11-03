"""社会化关系模块（/system/social，1 条路由）的双端契约比对。

仅 /list：需登录、无权限码；返回当前用户的绑定列表（种子数据通常为空数组）。
另覆盖未登录 401 与框架级尾斜杠/方法不匹配。

用法：
    python test/contract/diff_social.py [--py ...] [--go ...]
"""

import argparse
import sys

import diff_system as ds

LIST = "/system/social/list"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--py", default="http://127.0.0.1:8012")
    ap.add_argument("--go", default="http://127.0.0.1:8011")
    args = ap.parse_args()
    ds.PY, ds.GO = args.py, args.go
    print("=" * 82)
    print(f"社会化关系契约比对：Python({ds.PY})  vs  Go({ds.GO})")
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

    print("\n--- 1. 列表 ---")
    ds.compare_authed("list · 当前用户绑定列表（种子态逐字节）", LIST, strict=True)
    ds.compare_authed("list · 尾斜杠应 307", f"{LIST}/")

    print("\n--- 2. 认证 ---")
    py_s, _, py_b = ds.call(ds.PY, "GET", LIST)
    go_s, _, go_b = ds.call(ds.GO, "GET", LIST)
    ds.record("list · 未登录应 401「认证失败，请登录后再试」",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b))
              and json_code(py_b) == 401,
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")
    py_s, _, py_b = ds.call(ds.PY, "POST", LIST, body={})
    go_s, _, go_b = ds.call(ds.GO, "POST", LIST, body={})
    ds.record("list · 方法不匹配应 405",
              (py_s, ds.canon(py_b)) == (go_s, ds.canon(go_b)),
              f"py={py_s} {ds.canon(py_b)}\n           go={go_s} {ds.canon(go_b)}")

    return ds._summary()


def json_code(raw):
    import json
    try:
        return json.loads(raw).get("code")
    except Exception:  # noqa: BLE001
        return None


if __name__ == "__main__":
    sys.exit(main())
