# Golden 契约快照

本目录是 **Go 后端必须对齐的接口契约事实源**，由 Python 端（当前最完整的实现）导出。

> 生成方式：`make contract-export`（或 `python scripts/export_contract.py`）
> 该脚本直接调用 FastAPI 的 `create_app()` 生成 OpenAPI 文档，**不需要启动服务**——
> 无端口冲突、结果可重复，且 `create_app()` 内部会跑 `audit_routes_auth`，
> 导出成功本身就意味着 Python 端的路由认证约束是通过的。

## 文件

| 文件 | 内容 | 用途 |
|---|---|---|
| `contract.json` | 完整 OpenAPI 3.1 文档 | 契约事实源：路径、方法、参数、响应 schema |
| `routes.txt` | 排序后的 `METHOD path` 清单（125 行） | 路由清单 diff，比 JSON 易审阅 |
| `modules.json` | 路由 → 归属模块映射 + 范围统计 | 判定哪些路由在 Go 首期范围内 |

## 当前快照统计（2026-11-02）

```
OpenAPI 版本   : 3.1.0
标题 / 版本    : WalnutSeed 接口文档 / 1.0.0
路由总数       : 125
Go 首期范围内  : 125
排除(module_ai): 0
跳过的静态挂载 : /static
```

按模块分布：

| 路由数 | 模块 |
|---|---|
| 21 | `module_system.user.controller` |
| 15 | `module_system.dict.controller` |
| 15 | `module_system.role.controller` |
| 9 | `module_system.config.controller` |
| 9 | `module_system.log.controller` |
| 9 | `module_system.menu.controller` |
| 8 | `module_system.dept.controller` |
| 8 | `module_system.post.controller` |
| 7 | `module_system.client.controller` |
| 5 | `module_system.notice.controller` |
| 5 | `module_web.auth.controller` |
| 4 | `module_common.file.controller` |
| 3 | `module_common.health.controller` |
| 3 | `app.init_app`（`/docs`、`/docs/oauth2-redirect`、`/redoc`） |
| 2 | `module_resource.controller` |
| 1 | `module_system.social.controller` |
| 1 | `router`（`GET /` 欢迎文案） |

## 两个需要知道的事实

### ① `module_ai` 尚未挂载，因此不在契约内

`app/api/v1/module_ai/`（chat / conversation / kb）代码已写完，但 `app/api/v1/router.py`
**没有 include 它**——全部 `module_ai` 引用都在模块内部。所以它既不出现在 `routes.txt`，
也不出现在 `contract.json`。

`scripts/export_contract.py` 里的 `EXCLUDED_MODULE_PREFIXES` 机制已就位：
一旦 AI 模块被挂载，其路由会按 `endpoint.__module__` **自动**归入排除清单，
无需手工维护路径前缀（路径改了归属也不会错）。

### ② 3 条文档路由是 Go 侧的已知缺口

`/docs`、`/docs/oauth2-redirect`、`/redoc` 由 Python 的 `register_docs()` 提供
（Swagger UI / ReDoc）。Go 端 P0 未实现，属于**有意缺口**，计划在 P5 用 swaggo
或直接托管同一套静态 Swagger UI 资源补齐。

做路由清单 diff 时应把这 3 条（以及 `/openapi.json`、`/static*`）列入已知豁免，
否则会一直报「Go 缺失 4 条路由」的假失败。

## 导出脚本的一个坑（已修复，勿重犯）

新版 FastAPI 的 `include_router` 是**惰性合并**：`app.routes` 里出现的是 `_IncludedRouter`，
真实端点藏在 `effective_candidates()` 中，展开后得到 `_EffectiveRouteContext`
（既不是 `APIRoute` 也不是 `Mount` 子类）。

只判断 `APIRoute` / `Mount` 会**静默跳过全部业务路由**——首版脚本因此只收到 3 条路由
（全是直接挂在 app 上的文档路由），125 条里丢了 122 条，且不报任何错。

`collect_routes()` 已复刻 `init_app.audit_routes_auth` 的 `walk()` 逻辑处理这两个私有类型。
若日后升级 FastAPI 导致路由数骤降，先查这里。
