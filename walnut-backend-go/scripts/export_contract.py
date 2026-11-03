"""导出 golden 契约快照（对应 P0 任务 0.2）。

直接调用 FastAPI 的 `create_app()` 生成 OpenAPI 文档，**不需要启动服务**：
无端口冲突、可重复、且 create_app 内部会跑 `audit_routes_auth`（路由认证审计），
导出成功本身就意味着 Python 端的路由认证约束是通过的。

产出三个文件到 walnut-backend-go/api/openapi/：

  contract.json    完整 OpenAPI 文档（golden 契约事实源）
  routes.txt       排序后的 "METHOD path" 清单，便于 diff 与人工审阅
  modules.json     路由 → 归属模块 的映射（按 endpoint.__module__ 判定）

modules.json 的用途：Go 端首期**不实现 module_ai**，契约 diff 需要一份
「哪些路由属于排除范围」的清单。用 endpoint.__module__ 程序化判定，
比手写路径前缀猜测可靠——路径改了归属不会错，新增 AI 路由也会自动被识别。

用法：
    cd walnut-backend-python
    .venv/Scripts/python.exe ../walnut-backend-go/scripts/export_contract.py
"""

import json
import os
import sys
from collections import defaultdict
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
PY_BACKEND = REPO / "walnut-backend-python"
OUT_DIR = REPO / "walnut-backend-go" / "api" / "openapi"

# 导出前固定环境，避免受 shell 里残留的 ENVIRONMENT 影响
os.environ.setdefault("ENVIRONMENT", "dev")
sys.path.insert(0, str(PY_BACKEND))

# Go 端首期排除的模块（见 docs/go/00-Go后端实施计划.md §0：module_ai 在 Python 端
# 仍是未提交的在制品，不追移动靶）。用模块名前缀匹配 endpoint.__module__。
EXCLUDED_MODULE_PREFIXES = ("app.api.v1.module_ai",)


def collect_routes(app):
    """递归展开 app.routes，返回 (routes, module_of, mounts)。

    ⚠️ 不能直接遍历 app.routes 就完事：新版 FastAPI 的 include_router 是**惰性合并**，
    app.routes 里出现的是 _IncludedRouter，其真实端点藏在 effective_candidates() 中，
    展开后得到的是 _EffectiveRouteContext（既不是 APIRoute 也不是 Mount 子类）。
    若不显式识别这两个类型，**全部业务路由会被静默跳过**——本项目 init_app.py 的
    audit_routes_auth 里专门为此写了注释，本函数复刻其 walk() 逻辑。

    实测教训：只判断 APIRoute / Mount 时，本脚本只收到 3 条路由（全是 register_docs
    直接挂在 app 上的文档路由），100+ 条业务路由全部丢失且不报错。
    """
    from fastapi.routing import APIRoute
    from starlette.routing import Mount

    # 这两个私有类型在旧版本 FastAPI 中不存在，故做容错导入
    try:
        from fastapi.routing import _IncludedRouter
    except ImportError:
        _IncludedRouter = ()
    try:
        from fastapi.routing import _EffectiveRouteContext
    except ImportError:
        _EffectiveRouteContext = ()

    routes: list[tuple[str, str]] = []
    module_of: dict[str, str] = {}
    mounts: list[str] = []

    def record(path, methods, endpoint) -> None:
        mod = getattr(endpoint, "__module__", "<unknown>") if endpoint is not None else "<unknown>"
        for method in sorted(methods or []):
            if method == "HEAD":
                # HEAD 由框架从 GET 派生，不属于显式契约
                continue
            routes.append((method, path))
            module_of[f"{method} {path}"] = mod

    def walk(items) -> None:
        for route in items:
            if _IncludedRouter and isinstance(route, _IncludedRouter):
                walk(route.effective_candidates())
                walk(route.effective_low_priority_routes())
                continue
            if _EffectiveRouteContext and isinstance(route, _EffectiveRouteContext):
                if getattr(route, "endpoint", None) is not None:
                    record(route.path, route.methods, route.endpoint)
                continue
            if isinstance(route, APIRoute):
                record(route.path, route.methods, getattr(route, "endpoint", None))
            elif isinstance(route, Mount):
                # 静态托管挂载（/static、/web）不是 API 端点，不参与契约 diff
                mounts.append(route.path)

    walk(app.routes)
    return routes, module_of, mounts


def main() -> int:
    # create_app 会执行 validate_security_settings 与 audit_routes_auth，
    # 任一失败都会抛异常——导出成功即代表 Python 端安全门禁通过
    from main import create_app

    app = create_app()
    spec = app.openapi()

    routes, module_of, skipped_mounts = collect_routes(app)

    # 同一路径可能因 include_router 的优先级机制出现多次，去重但保留归属
    seen: set[tuple[str, str]] = set()
    unique_routes: list[tuple[str, str]] = []
    for item in routes:
        if item not in seen:
            seen.add(item)
            unique_routes.append(item)
    routes = sorted(unique_routes, key=lambda r: (r[1], r[0]))

    # ---- 模块统计 ----
    by_module: dict[str, list[str]] = defaultdict(list)
    for key, mod in module_of.items():
        by_module[mod].append(key)

    excluded = sorted(
        key
        for key, mod in module_of.items()
        if mod.startswith(EXCLUDED_MODULE_PREFIXES)
    )
    in_scope = sorted(key for key in module_of if key not in set(excluded))

    # ---- 写文件 ----
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    (OUT_DIR / "contract.json").write_text(
        json.dumps(spec, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    (OUT_DIR / "routes.txt").write_text(
        "\n".join(f"{m} {p}" for m, p in routes) + "\n",
        encoding="utf-8",
    )
    (OUT_DIR / "modules.json").write_text(
        json.dumps(
            {
                "generatedFrom": "walnut-backend-python create_app().openapi()",
                "excludedModulePrefixes": list(EXCLUDED_MODULE_PREFIXES),
                "counts": {
                    "total": len(routes),
                    "inScopeForGo": len(in_scope),
                    "excludedModuleAi": len(excluded),
                },
                "inScope": in_scope,
                "excluded": excluded,
                "byModule": {m: sorted(v) for m, v in sorted(by_module.items())},
            },
            ensure_ascii=False,
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )

    # ---- 报告 ----
    print("=" * 74)
    print("golden 契约已导出 ->", OUT_DIR)
    print("=" * 74)
    print(f"  OpenAPI 版本 : {spec.get('openapi')}")
    print(f"  标题 / 版本  : {spec.get('info', {}).get('title')} / {spec.get('info', {}).get('version')}")
    print(f"  路由总数     : {len(routes)}")
    print(f"  Go 首期范围内: {len(in_scope)}")
    print(f"  排除(module_ai): {len(excluded)}")
    print(f"  跳过的静态挂载: {skipped_mounts}")
    print("-" * 74)
    print("  按模块统计：")
    for mod in sorted(by_module):
        flag = "  [排除]" if mod.startswith(EXCLUDED_MODULE_PREFIXES) else ""
        # 缩短模块路径便于阅读
        short = mod.replace("app.api.v1.", "")
        print(f"    {len(by_module[mod]):3d}  {short}{flag}")
    print("=" * 74)
    if excluded:
        print("  被排除的 module_ai 路由（Go 首期不实现，契约 diff 须跳过）：")
        for key in excluded:
            print(f"    {key}")
        print("=" * 74)
    return 0


if __name__ == "__main__":
    sys.exit(main())
