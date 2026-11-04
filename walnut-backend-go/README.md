# WalnutSeed Go 后端

WalnutSeed 全栈脚手架的 **Go 后端实现**，与 Python(FastAPI) / Java(Spring Boot) 共享**同一套接口契约**——路由面、响应信封、认证流程、接口加密协议完全一致，前端无感互换。

> 状态：**P0–P4 功能完成**，路由覆盖率 **122/122 = 100%**（golden 契约 125 条，豁免 3 条 docs UI）。
> 配置 / 日志 / 信封 / 错误体系 / 健康探针 / 路由认证审计 / 优雅停机 / 认证链路 / 系统管理 CRUD /
> 文件上传下载 / SSE 推送 / Excel 导入导出均已完成并通过双端契约比对。
> 完整设计与落地路线：[`docs/go/00-Go后端实施计划.md`](../docs/go/00-Go后端实施计划.md)

## 技术栈

| 关注点 | 选型 |
|---|---|
| 语言 | Go 1.26+（`.go-version`；依赖链要求，见下方说明） |
| Web 框架 | Gin v1.12 |
| ORM | GORM v2 + `go-sql-driver/mysql` |
| 迁移 | golang-migrate（P1 接入，**生产禁用 `AutoMigrate`**） |
| 缓存 | `redis/go-redis/v9` |
| 配置 | koanf v2（三级优先级：环境变量 > `.env.{ENV}` > 代码默认值） |
| 日志 | 标准库 `log/slog` + lumberjack 轮转 + 敏感字段脱敏 |
| CLI | cobra（单二进制 + 子命令，对齐 Python 端 typer） |
| 存储 | 任意 S3 兼容对象存储（默认 SeaweedFS），可切阿里云 OSS |

> **为什么是 Go 1.26**：`gin v1.12.0` 要求 `go >= 1.25.0`，`go-redis v9.23.0` 要求 `go >= 1.26.0`。
> 本机 Go 版本低于此也没关系——`GOTOOLCHAIN=auto` 会经 GOPROXY 自动拉取所需工具链（已验证可用）。

## 快速开始

### 0. 配置 Go 环境（国内网络必读）

`goproxy.cn` / `goproxy.io` / `sum.golang.google.cn` 在部分网络下会被连接重置。实测可用配方：

```bat
go env -w GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
go env -w GOSUMDB=sum.golang.org
go env -w GOTOOLCHAIN=auto
```

两个反直觉的点：

1. **`GOSUMDB` 保持默认值**。显式改成 `sum.golang.google.cn` 会让 Go **绕过代理直连**该主机（被重置）；保持默认时 Go 会把 sumdb 查询**经 GOPROXY 转发**，阿里云镜像支持这个转发。
2. **不要用 `GOSUMDB=off`**。工具链模块（`golang.org/toolchain`）强制要求校验，关掉后连已缓存的工具链都拒绝使用。

海外环境或 CI 直接用默认 `proxy.golang.org` 即可，Makefile 的 `GOPROXY` 可被环境变量覆盖。

### 1. 起中间件

```bash
docker compose -f docker/docker-compose.middleware.yml up -d
```

MySQL `localhost:3307`（root/walnut123）、Redis `localhost:6380`、SeaweedFS S3 `localhost:8333`。

### 2. 启动 Go 后端

```bash
cd walnut-backend-go
cp env/.env.example env/.env.dev   # 或直接使用仓库内已提供的 .env.dev
make dev                           # 等价 go run . run --env dev
```

服务监听 `http://localhost:8011`——与 Python / Java 端相同端口，前端无需任何改动。

数据库 `walnut_seed_go` **不存在时会自动创建**：Go 的 mysql 驱动没有 JDBC 的 `createDatabaseIfNotExist`，中间件编排的 `MYSQL_DATABASE` 又只能建一个库，故由 `databasex.EnsureDatabase` 显式引导。

### 3. 起前端

```bash
cd walnut-frontend
pnpm install && pnpm dev:antd      # http://localhost:8010，/api 代理到 8011
```

## 常用命令

```bash
make dev              # 本地启动（--env dev）
make build            # 编译到 bin/（CGO_ENABLED=0 静态二进制）
make test             # 运行测试
make test-race        # 竞态检测（CI 硬门槛）
make lint             # golangci-lint
make fmt              # gofumpt 格式化
make check            # 提交前完整检查：fmt-check + vet + lint + test-race
make migrate          # 应用迁移（P1 起可用）
make contract-export  # 从运行中的 Python 后端导出 golden 契约
make contract-diff    # 双后端响应 diff 跑分
make docker-build     # 构建镜像 walnut-seed-backend-go
```

## 目录结构

```
walnut-backend-go/
├── main.go                  # 单二进制入口 → cmd.Execute()
├── cmd/                     # cobra 命令面（run / migrate / seed / genkey）
├── internal/
│   ├── bootstrap/           # 组装根：app.go / router.go / audit.go
│   ├── config/              # 配置唯一事实源 + 启动期安全校验
│   ├── pkg/                 # 跨模块共享内核
│   │   ├── envelope/        #   响应信封 {code,msg,data} / 分页 {rows,total}
│   │   ├── errs/            #   业务异常 + 错误码 + 全局 recover
│   │   ├── idtype/          #   JSInt 条件序列化（雪花 ID）
│   │   ├── timex/           #   JSONTime 等时间类型
│   │   ├── logx/            #   slog + 轮转 + 脱敏
│   │   ├── middleware/      #   CORS / RequestLog / CorrelationId
│   │   ├── databasex/       #   GORM + 连接池 + 建库引导
│   │   └── cachex/          #   Redis + 键前缀
│   └── module/              # 业务模块，每个模块五件套：
│       └── common/health/   #   handler.go / service.go / repo.go / model.go / dto.go
├── migrations/              # 版本化 SQL（P1）
├── seed/                    # 种子数据（P1）
├── api/openapi/             # ★ golden 契约快照（由 scripts/export_contract.py 从 Python 端导出）
│   ├── contract.json        #   完整 OpenAPI 3.1 文档
│   ├── routes.txt           #   125 条 "METHOD path" 清单
│   ├── modules.json         #   路由 → 归属模块映射
│   └── README.md
├── scripts/
│   └── export_contract.py   # 导出 golden 契约（调 create_app()，无需启动服务）
├── test/
│   ├── contract/
│   │   ├── diff_contract.py         # ★ 双后端响应逐字节 diff（需两端同时运行）
│   │   ├── routes_inventory_test.go # ★ 路由清单 diff（纯静态，可进每次 CI）
│   │   └── README.md
│   └── cryptointerop/               # Python↔Go 加解密互通测试（AES 密文逐字节比对）
├── env/                     # .env.example / .env.dev（镜像不打包）
├── Makefile  Dockerfile  .golangci.yml
```

## 契约防漂移

三份实现（Python / Java / Go）最大的风险是契约漂移。本目录有两道自动门禁：

| 门禁 | 命令 | 需要 | 强度 |
|---|---|---|---|
| **路由清单 diff** | `go test ./test/contract/` | 无（纯静态，读 `api/openapi/routes.txt`） | Go 出现 Python 没有的路由 → **硬失败** |
| **响应逐字节 diff** | `make contract-diff` | 两端同时运行（Go:8011 + Python:8012） | 归一化后响应体不一致 → 失败 |

```bash
make contract-export   # 从 Python 端重新导出 golden 契约（Python 端路由变更后必须执行）
make contract-diff     # 双后端响应比对
```

> **每加一个端点就扩一次 `CASES` 跑一遍 diff**，不要留到最后验收。
> P0 期间这个跑分器抓到了一个代码审查没发现的尾斜杠偏差（详见 `test/contract/README.md`）。

## 三端并存的隔离约定

| 维度 | Python | Java | **Go** |
|---|---|---|---|
| 数据库 | `walnut_seed_python` | `walnut_seed_java` | `walnut_seed_go` |
| Redis 键前缀 | `walnut-seed-python` | `walnut-seed-java` | `walnut-seed-go` |
| 日志文件名 | `walnut-seed-python.log` | `walnut-seed-java-*.log` | `walnut-seed-go.log` |
| 镜像名 | `walnut-seed-backend` | `walnut-seed-backend-java` | `walnut-seed-backend-go` |
| 迁移工具 | Alembic | Flyway | golang-migrate |

三端共用同一个 Redis 实例，**键前缀不区分会导致登录态、验证码、限流计数互相串键**。

## 契约要点（与 Python / Java 端一致）

- 响应信封 `{"code": int, "msg": str, "data": T | null}`；分页 `{"rows": [...], "total": N}`
- `code`：200 成功、500 失败、601 警告、400 参数校验、10000 段为认证模块错误码
- 业务异常几乎都返回 **HTTP 200**，真实结果由 body.code 表达；401/403 同样走 HTTP 200
- 框架级 404/405 与健康探针 503 返回**真实 HTTP 状态码**，但仍带信封体
- 认证：`Authorization: Bearer <jwt>` + `clientid` 请求头
- 日期时间 `yyyy-MM-dd HH:mm:ss`，零值输出 `null`
- **大整数条件转字符串**：`|v| >= 9007199254740991` 才转（雪花 ID 恒超范围 → 恒为字符串；
  `status`/`total` 等小整数**必须保持数字**）。见 `internal/pkg/idtype/jsint.go`
- 健康探针字段是 **snake_case**（`latency_ms`/`uptime_seconds`/`disk_usage`），与全站 camelCase 不同

## 已固化的两条强约束

1. **路由认证审计（fail-fast）**：启动时遍历全部路由，白名单（`WHITE_API_LIST_PATH`）之外未挂认证的一律拒绝启动，杜绝接口裸奔。实现见 `internal/bootstrap/audit.go`。
2. **密钥校验与降级**：`SECRET_KEY` 弱值在 prod 拒绝启动；RSA 密钥缺失/无效/命中公开密钥黑名单时自动停用接口加解密并告警——**绝不带假密钥运行**。实现见 `internal/config/validate.go`。
