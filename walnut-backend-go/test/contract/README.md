# 契约 diff 跑分器

对同一路径分别请求 **Python** 与 **Go** 后端，归一化易变字段后**逐字节比对** JSON 响应。
这是 [`docs/go/00-Go后端实施计划.md`](../../../docs/go/00-Go后端实施计划.md) §4.2 描述的核心工具。

> **为什么它是准入门槛而不是验收工具**：三份实现（Python / Java / Go）最大的风险是契约漂移，
> 而 `module_ai` 只落在 Python 端已经证明漂移会真实发生。用它**驱动开发**
> （写一个端点 → 立刻 diff → 修差异）的返工成本，远低于写完再验收。

## 用法

```bash
# 1. 启动 Go 后端（默认 8011）
cd walnut-backend-go && make dev

# 2. 另开终端，启动 Python 后端作为参照（换端口避免冲突）
cd walnut-backend-python
SERVER_PORT=8012 DATABASE_AUTO_MIGRATE=False uv run main.py run --env dev

# 3. 跑分
make contract-diff
# 或直接：
python test/contract/diff_contract.py --py http://127.0.0.1:8012 --go http://127.0.0.1:8011
```

退出码：全部一致返回 0，有任何差异返回 1（可直接进 CI）。

## 比对维度

| 维度 | 说明 |
|---|---|
| HTTP 状态码 | 业务错误应为 200；404/405/503 应为真实状态码 |
| 响应体 | 归一化后逐字节比对（键排序 + 紧凑分隔符） |
| Content-Type | 前端拦截器会看 |
| `Access-Control-Expose-Headers` | 必须含 `encrypt-key`，否则前端跨域读不到加密协商头 |

## 归一化规则（§4.3）

`VOLATILE_KEYS` 里的字段值**必然不同**，只验证「存在性与是否为 null」：

| 字段 | 处理 |
|---|---|
| `timestamp` | → `<TS>` |
| `uptime_seconds` | → `<UPTIME>` |
| `latency_ms` | 保留 null 与非 null 的区别（`null` 表示探测失败，与 `0` 语义不同） |
| `disk_usage` | → `<DISK>`（Windows 开发机恒为 -1，Linux 容器内是真实值） |

后续接入业务端点时需扩充：雪花 ID（映射为占位符但保持**同值同占位**，以验证引用完整性）、
`createTime`/`updateTime`、trace/correlation id、上传文件 url 里的 uuid。

## 用例清单

`CASES` 是按阶段逐步扩充的清单。当前（P0）：

```
GET  /common/health/check     200  信封 + snake_case 载荷
GET  /common/health/live      200  存活探针
GET  /common/health/ready     503  依赖未就绪时的门禁语义
GET  /                        200  JSON 字符串（非信封）
GET  /nonexistent/path        404  真实状态码 + 信封体
POST /common/health/check     405  方法不匹配（需 HandleMethodNotAllowed=true）
GET  /common/health/check/    200  尾斜杠重定向（见下）
```

## 已经抓到过的真实偏差

**尾斜杠（R16）**：`GET /common/health/check/`

- Python：Starlette 的 `redirect_slashes` 返回 **307**，客户端跟随后得到 200
- Go（初版）：为规避 gin 对 GET 返回 **301**（会被浏览器永久缓存）而关闭了
  `RedirectTrailingSlash`，但没有补上重定向 → 直接 **404**

修法见 `internal/bootstrap/router.go` 的 `slashAwareNoRoute`：关闭 gin 内置重定向，
改用自定义 `NoRoute` 精确复刻 Starlette 语义——**对所有方法统一用 307**，
既对齐契约又避开 301 的永久缓存陷阱。

> 这个偏差是跑分器发现的，不是代码审查发现的。

## 后续

P3 起业务端点大量接入后，建议：

1. 把 `CASES` 拆成 YAML/JSON 用例文件，支持带请求体、请求头、登录态的场景
2. 增加「路由清单 diff」：从 Python 的 `/openapi.json` 提取 `(method, path)` 全集，
   与 Go 的 `Builder.Routes()` 对比，缺失或多余都算失败（防止私自增删接口造成漂移）
3. 接入 `scripts/smoke_all.py` 已有的全端点冒烟能力，避免重复造轮子
