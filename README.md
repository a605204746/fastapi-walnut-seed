# Walnut-Seed

[![Python](https://img.shields.io/badge/Python-3.12%2B-3776AB?logo=python&logoColor=white)](https://www.python.org/)
[![FastAPI](https://img.shields.io/badge/FastAPI-0.138-009688?logo=fastapi&logoColor=white)](https://fastapi.tiangolo.com/)
[![Java](https://img.shields.io/badge/Java-25-ED8B00?logo=openjdk&logoColor=white)](https://openjdk.org/)
[![Spring Boot](https://img.shields.io/badge/Spring%20Boot-3.5-6DB33F?logo=springboot&logoColor=white)](https://spring.io/projects/spring-boot)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Gin](https://img.shields.io/badge/Gin-1.12-00ADD8?logo=go&logoColor=white)](https://gin-gonic.com/)
[![Vue](https://img.shields.io/badge/Vue-3.5-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![Node.js](https://img.shields.io/badge/Node.js-22%2B-5FA04E?logo=nodedotjs&logoColor=white)](https://nodejs.org/)
[![uv](https://img.shields.io/badge/uv-managed-DE5FE4?logo=uv&logoColor=white)](https://docs.astral.sh/uv/)
[![pnpm](https://img.shields.io/badge/pnpm-11-F69220?logo=pnpm&logoColor=white)](https://pnpm.io/)

[English](./README.en.md) | **简体中文**

> **FastAPI / Spring Boot / Gin 全栈脚手架** —— 开箱即用的**后台管理系统**模板（admin scaffold / boilerplate），支持 **RBAC 权限管理、JWT 认证、接口加解密、数据库迁移、Docker 一键部署**，前端基于 **Vue3 + Vben + Ant Design Vue Next**。

现代化全栈应用**脚手架（scaffold / boilerplate / starter template）**：**三后端**（FastAPI / Python 3.12、Spring Boot 3 / Java 25、Gin / Go 1.26）+ **Vue3**（Vben 5 / antdv-next）前端，开箱即用地构建**中后台管理系统**。三个后端遵循**同一套接口契约**（路由、响应信封、认证流程、接口加密协议完全一致），前端无感互换。

适合场景：快速搭建**企业级后台管理**、**数据看板**、**运营平台**、**SaaS 多租户系统**；支持 Python 团队与 Java 团队共用同一前端，按技术栈任选后端。

```
GitHub: https://github.com/a605204746/walnut-seed
```

![WalnutSeed 登录页](./docs/img/img.png)

## 特性

内置基础设施，直接支撑真实的管理类业务：

- **三后端同契约** —— Python（FastAPI）、Java（Spring Boot 3）与 Go（Gin）三份后端实现，同一前端、同一接口契约，按团队技术栈任选，Docker 编排一键切换
- **统一响应信封** —— 全部接口统一返回 `{"code", "msg", "data"}`
- **认证与授权** —— JWT 登录、RBAC 菜单/按钮权限、行级数据权限（fail-closed）
- **默认加固** —— 限流、防重提交（幂等）、接口加解密（RSA + AES）、XSS 过滤
- **缓存与异步原语** —— Redis 缓存/CRUD 辅助、SSE 与 WebSocket 支持
- **文件存储（OSS）** —— 任意 S3 兼容对象存储（默认 SeaweedFS），可切阿里云 OSS
- **i18n** —— 按请求头 `Accept-Language` 自动生效（zh_CN/en_US，业务消息逐步迁移中）
- **数据库迁移** —— Alembic 作为 Schema 唯一事实来源（不再使用启动时 `create_all`）
- **种子数据** —— 完整后台基线：user / role / menu / dept / post / dict / config / notice / client / social / log
- **运维友好** —— 健康/存活/就绪探针、Loguru 结构化日志、Typer CLI、全端点冒烟脚本

## 技术栈

| 端 | 技术 |
| --- | --- |
| 后端（Python） | Python 3.12、FastAPI 0.138、SQLAlchemy 2.0（async）、Alembic、Redis（redis.asyncio）、PyJWT、Loguru、Typer/Uvicorn；依赖管理 uv；接口加解密 cryptography（RSA + AES），可选国密 gmssl |
| 后端（Java） | Java 25、Spring Boot 3.5、MyBatis-Plus、Sa-Token（JWT）、Redisson、Flyway、springdoc；构建 Maven（仓库镜像已内置国内源） |
| 后端（Go） | Go 1.26、Gin 1.12、GORM v2、golang-migrate、go-redis v9、koanf v2、slog + lumberjack；存储 minio-go v7；Excel excelize |
| 前端 | Vue 3.5、Vben 5.7.0、antdv-next 1.3.0（替代已停止维护的 ant-design-vue）；pnpm 11.2.2 + turbo monorepo；Node.js `^22.18.0 \|\| ^24.0.0` |
| 存储 | MySQL 8（数据库仅支持 MySQL）、Redis 7、SeaweedFS（S3 兼容对象存储，可切阿里云 OSS） |

## 文档

详细教程与设计说明位于 [`docs/`](./docs/README.md)：

- **[文档总入口](./docs/README.md)** —— 按公共约定、Python、Java、Go、前端和部署分别阅读
- **[从零新增一个业务模块](./docs/python/08-新增CRUD模块.md)** —— Python 端端到端开发教程（五件套 → 迁移 → 菜单权限 → 前端页面）
- 开发指南：菜单与权限、Alembic 迁移、接口加解密、i18n
- 架构说明与部署运维手册

在本脚手架上做二次开发，建议从这里开始。

## 目录结构

```
walnut-seed/
  walnut-backend-python/    # Python 后端（FastAPI + SQLAlchemy 2.0 异步 + Redis）
  walnut-backend-java/      # Java 后端（Spring Boot 3 + MyBatis-Plus + Sa-Token），与 Python 端同契约
  walnut-backend-go/        # Go 后端（Gin + GORM + go-redis），与 Python/Java 端同契约，路由覆盖率 122/122
  walnut-frontend/          # Vue3 前端 monorepo（pnpm + turbo），主应用位于 apps/web-antd/
  docker/                   # compose 编排与容器配置（Python / Java / Go 三套全栈编排 + 中间件编排）
  data/                     # 运行时产物（不入 git，自动创建）：logs/、upload/
```

### 后端结构（Python）

```
walnut-backend-python/
  main.py                  # typer CLI（run/revision/upgrade/downgrade/stamp/current/history）+ create_app
  pyproject.toml           # uv 依赖与工具配置
  alembic.ini              # 数据库迁移配置
  docker-entrypoint.sh     # 容器启动入口：先 alembic upgrade 再启动应用
  banner.txt               # 启动 Banner
  env/                     # .env.dev / .env.example 环境配置
  app/
    init_app.py            # lifespan + register_*
    config/                # setting.py / path_conf.py
    common/                # constant / enums / response / request / dataclasses
    core/                  # 基础设施：database / migrate / redis_crud / security / dependencies /
                           # permission / middlewares / exceptions / logger / base_model /
                           # base_schema / base_crud / router_class / validator / idempotent /
                           # rate_limiter / sse / websocket / file_storage / encrypt
    utils/                 # 工具：common / string / date / ip / xss / sql / import /
                           # excel / snowflake / i18n / banner
    api/v1/                # 业务模块（module_*）
                           #   module_system：user/role/menu/dept/post/dict/config/notice/client/social/log
                           #   module_web：auth/captcha/register
                           #   module_common：health / file
                           #   module_resource：SSE / WebSocket
    seed/                  # 种子数据（initialize.py + sql/）
    i18n/                  # messages_zh_CN / messages_en_US
    alembic/               # env.py / script.py.mako / versions/（迁移脚本）
  static/                  # swagger-ui / redoc / image
  scripts/                 # 开发脚本（smoke_all.py 全端点冒烟、密钥生成器）
  tests/                   # pytest
```

`app/api/v1/module_*` 下的业务子模块统一按五件套组织：

| 文件 | 职责 |
|---|---|
| `controller.py` | 路由定义与入参校验入口 |
| `service.py` | 业务逻辑编排 |
| `crud.py` | 数据访问层（基于 `app/core/base_crud.py`） |
| `model.py` | SQLAlchemy ORM 模型 |
| `schema.py` | Pydantic 请求/响应模型 |

本地直接启动时日志写入仓库根目录 `data/logs/`；Docker 启动时后台日志挂载到 `docker/volumes/logs/`。两种方式都通过文件名区分后端：Python 为 `walnut-seed-python.log`，Java 为 `walnut-seed-java-console.log`、`walnut-seed-java-info.log`、`walnut-seed-java-error.log`（按配置轮转）。

### 后端结构（Java）

```
walnut-backend-java/
  pom.xml                          # Maven 构建（Java 25 / Spring Boot 3.5，内置国内镜像仓库）
  Dockerfile                       # 两阶段构建：镜像内 Maven 打包 + Liberica JDK 25 运行
  src/main/java/com/walnut/seed/
    WalnutSeedApplication.java     # 启动类
    common/                        # 基础设施：响应信封 / Sa-Token 安全 / Redis(Redisson) /
                                   # 接口加解密 / MyBatis-Plus / OSS / SSE / XSS / 限流防重
    module/web/                    # 认证（登录/登出/注册/验证码）+ 健康检查 + 文件
    module/system/                 # 系统管理（user/role/menu/dept/post/dict/config/notice/client/social）
                                   # 与监控（操作日志/登录日志）
  src/main/resources/
    application*.yml               # 配置（dev 默认；生产经环境变量覆盖）
    db/migration/                  # Flyway 迁移脚本（建表 + 种子数据，与 Python 端基线一致）
```

路由面与 Python 端一致：`/auth/*`、`/system/*`、`/monitor/*`、`/common/health/*`、`/common/file/*`、`/resource/sse`、`/upload/*`（根前缀，前端代理剥离 `/api`）。

### 后端结构（Go）

```
walnut-backend-go/
  main.go                    # 单二进制入口 → cobra CLI
  cmd/                       # 子命令：root / run / migrate / seed
  internal/
    bootstrap/               # 组装根：app.go / router.go / audit.go
    config/                  # 配置唯一事实源（koanf）+ 启动期安全校验
    pkg/                     # 跨模块共享内核
      envelope / errs        #   响应信封 / 业务异常
      idtype / timex         #   JSInt 条件序列化 / JSONTime
      cryptox                #   AES-ECB / RSA 分段 / bcrypt / JWT（已与 Python 互通验证）
      idgen                  #   雪花 ID（位布局已跨语言验证）
      i18nx / validatex      #   go:embed 消息表 / Pydantic 文案映射
      logx / middleware      #   slog + 轮转 + 脱敏 / CORS·Locale·ApiDecrypt·ReqLog·Correlation
      databasex / cachex     #   GORM + 建库引导 + golang-migrate / Redis + 键前缀
      permguard / permx      #   功能权限守卫 / 行级数据权限（fail-closed）
      storagex / ssex        #   对象存储（S3/OSS） / SSE 连接管理
      crudx / excelx         #   分页排序 / Excel 导入导出（excelize）
    module/
      common/health/         #   check / live / ready
      web/auth/              #   登录 / 登出 / 注册 / 验证码 / 认证中间件
      system/                #   user / role / menu / dept / post / dict / config / notice / client / social / log
      resource/              #   SSE 连接 / 关闭
  migrations/                # 0001_init_schema.{up,down}.sql（17 张系统表）
  seed/                      # mysql_data.sql（幂等播种，与 Python 端 SHA256 一致）
  api/openapi/               # golden 契约（routes.txt 125 条，由 Python 端导出）
  test/contract/             # 双端契约 diff（11 个 diff_*.py 脚本 + routes_inventory_test.go）
  test/cryptointerop/        # Python↔Go 加解密互通测试
  env/                       # .env.dev / .env.example
  Makefile  Dockerfile  .golangci.yml  .go-version
```

每个业务模块按**五件套**组织（handler / service / repo / dto / model），依赖方向单向向内。

## 快速开始

```bash
git clone https://github.com/deepin/walnut-seed.git
cd walnut-seed
```

前置：Docker Desktop（含 compose）；本机原生开发另需 `uv`（Python 3.12）、Node.js `^22.18.0 || ^24.0.0` 与 pnpm 11；本机跑 Java 后端还需 JDK 25 + Maven；本机跑 Go 后端需 Go 1.26+（走 Docker 则无需本机工具链）。

### 方式一：仅中间件 + 本机应用（日常开发推荐）

中间件（MySQL/Redis/SeaweedFS）跑在 Docker，后端/前端在本机原生运行（热重载最流畅、好调试）。

先启动中间件（MySQL localhost:3307，root/walnut123，Python 库 walnut_seed_python / Java 库 walnut_seed_java / Go 库 walnut_seed_go 自动创建；Redis localhost:6380；SeaweedFS S3 API localhost:8333，filer UI `http://localhost:8888`）：

```bash
docker compose -f docker/docker-compose.middleware.yml up -d
```

然后启动应用（后端三选一，三者监听同一端口 8011，前端无需任何改动）：

```bash
# Python 后端
cd walnut-backend-python
uv sync
uv run main.py run --env dev   # http://localhost:8011

# 或 Java 后端（需 JDK 25 + Maven）
cd walnut-backend-java
mvn spring-boot:run            # http://localhost:8011

# 或 Go 后端（需 Go 1.26+）
cd walnut-backend-go
go run . run --env dev         # http://localhost:8011

# 前端（新终端）
cd walnut-frontend
pnpm install
pnpm dev:antd                  # http://localhost:8010
```

连接信息：Python 在 `walnut-backend-python/env/.env.dev`、Java 在 `walnut-backend-java/src/main/resources/application-dev.yml`、Go 在 `walnut-backend-go/env/.env.dev`，均已指向 Docker 中间件。前端 dev server 默认运行于 `http://localhost:8010`，将 `/api` 代理到 `http://localhost:8011`。

### 方式二：生产/全栈

```bash
cp docker/.env.example docker/.env    # 然后设置 JWT_SECRET_KEY（必填，见下）

# Python 后端全栈
docker compose -f docker/docker-compose.yml up -d --build

# 或 Java 后端全栈（前端完全相同，仅后端实现互换）
docker compose -f docker/docker-compose.java.yml up -d --build

# 或 Go 后端全栈（前端完全相同，仅后端实现互换）
docker compose -f docker/docker-compose.go.yml up -d --build
```

启动后访问 `http://localhost:8010`，初始账号 `admin / admin123`（**生产使用前必须改密**）。

包含 MySQL 8 + Redis 7 + 后端 + 前端(nginx)；启动时后端容器先执行迁移再写入种子数据（Python: Alembic；Java: Flyway；Go: golang-migrate）。全部服务带 restart 策略与健康检查；`JWT_SECRET_KEY` 必须在 `docker/.env` 提供（三套编排以 `:?` 强制，缺失或为空直接启动失败）。

> 三套全栈编排经 `include` 共用中间件定义且端口互斥，**不能同时运行**。停止均为对应的 `docker compose -f <文件> down`；中间件数据保留在 `docker/volumes`（MySQL/Redis 绑定挂载）与具名卷 `seaweedfs-data`（SeaweedFS 的 filer 元数据在 Windows 绑定挂载下会静默丢失，故用具名卷）；后台日志统一绑定到 `docker/volumes/logs`，不会因 `down` 删除。中间件编排使用非默认端口（MySQL 3307 / Redis 6380），一般不与宿主机自装的 MySQL/Redis（3306/6379）冲突。

### 常用开发命令

后端（在 `walnut-backend-python/` 下）：

```bash
uv sync                                     # 安装依赖（含 dev）
cp env/.env.example env/.env.dev            # 首次：拷贝环境配置（默认已提供 MySQL + 本地 Redis）
uv run main.py run --env dev                # 启动（裸跑 uv run main.py 等价于此命令，默认 dev；DATABASE_AUTO_MIGRATE=True 时启动自动迁移）
uv run pytest                               # 测试
uv run python -m scripts.smoke_all          # 全端点冒烟（需本地 Redis 与已初始化的数据库）
uv run ruff check .                         # 代码检查
uv run ruff format .                        # 代码格式化
```

前端（在 `walnut-frontend/` 下，使用 pnpm）：

```bash
pnpm install      # 安装依赖
pnpm dev:antd     # 开发服务
pnpm build:antd   # 构建生产包
```

> 启用接口加密时，RSA 公私钥须与后端配置配对，且是**两对**密钥：前端请求加密密钥对应后端解密密钥，后端响应加密密钥对应前端解密密钥。见 `apps/web-antd/.env.development`。

## 三后端（Python ↔ Java ↔ Go）

三个后端是**同一接口契约的三份实现**，前端与部署层对其无感：

| 契约面 | 一致性 |
|---|---|
| 路由面 | `/auth/*`、`/system/*`、`/monitor/*`、`/common/health/*`、`/common/file/*`、`/resource/sse`、`GET /upload/*`（根前缀） |
| 响应信封 | `{"code": 200, "msg", "data"}`，分页 `{"rows", "total"}`；业务错误 HTTP 200 + body code |
| 登录认证 | 同一 `clientId`、验证码流程、`access_token` 响应；`Authorization: Bearer` + `clientid` 头 |
| 接口加密 | `encrypt-key` 头的 RSA + AES 混合协议；开发密钥对三方共用（前端 `.env.development`、Python `env/.env.dev`、Java `application.yml`） |
| 种子数据 | 同一基线：`admin / admin123`、pc 客户端、完整菜单/角色/字典 |

差异与注意事项：

- **数据库相互独立**：Python 用 `walnut_seed_python`（Alembic），Java 用 `walnut_seed_java`（Flyway），Go 用 `walnut_seed_go`（golang-migrate）。切换后端不会互串数据，各自维护各自的迁移脚本；**新增表结构变更时三端需各自落地迁移**
- **JWT 密钥独立**：三后端各用各的签名密钥（compose 统一经 `JWT_SECRET_KEY` 注入），切换后端后前端需重新登录（token 不跨后端通用）
- 加密行为均为**透明模式**：请求带 `encrypt-key` 头即自动解密，明文请求直接放行（生产前端默认不加密）
- 选择建议：团队 Python 技术栈选 `walnut-backend-python`，Java 技术栈选 `walnut-backend-java`，Go 技术栈选 `walnut-backend-go`；Docker 层切换只需换一份编排文件

## 后端接口契约

- 统一响应信封：`{"code": int, "msg": str, "data": T | null}`
  - `code` 200=成功、500=失败、601=警告；业务异常几乎都返回 HTTP 200，前端据 body.code 判断
- 分页载荷：`{"rows": [...], "total": N}`
- 错误码：认证模块使用 10000 段错误码（`AuthErrorCode`，如 10005 用户名或密码错误）；其余业务异常统一 HTTP 200 + `code=500`，参数校验错误 `code=400`，401/403/404/405 返回真实 HTTP 状态码（仍带信封体）
- 认证：`Authorization: Bearer <jwt>` + `clientid` 请求头
- JSON 规则：日期时间 `yyyy-MM-dd HH:mm:ss`、超出 JS 安全整数范围的大整数转字符串

## 安全机制

- **密钥治理**：JWT 签名密钥生产环境强制校验（占位符 / 过短 / 未设置直接拒绝启动）；接口加解密 RSA 密钥不内置任何默认值——密钥缺失或无效时加密层自动停用并告警（绝不静默使用默认密钥），已知公开泄露的密钥直接拒绝。生成工具：`uv run python scripts/gen_rsa_keys.py`（RSA 密钥对）、`scripts/gen_secret_key.py`（SECRET_KEY）
- **路由认证审计**：启动时扫描全部路由，白名单（`WHITE_API_LIST_PATH`）之外的路由缺少认证依赖即启动失败（fail-fast），杜绝漏挂依赖导致的接口裸奔
- **数据权限 fail-closed**：数据权限组件异常时拒绝访问而非放行；无角色用户仅可见本人数据
- **登录防护**：登录接口限流（每 IP 10 次/分钟）；失败锁定按「用户名 + IP」计数（防恶意锁定他人账号）；用户不存在与密码错误统一文案（防账号枚举）
- **可信代理**：`TRUSTED_PROXY_IPS` 列表内的来源才解析 `X-Forwarded-For` 等转发头，反向代理部署须按拓扑配置，否则限流与审计日志按直连地址计
- **文件上传**：扩展名白名单（`ALLOWED_EXTENSIONS`）+ 大小限制（`MAX_FILE_SIZE`）；上传文件访问统一附带 `Content-Disposition: attachment` 与 `nosniff`，防存储型 XSS
- **日志脱敏**：写日志前自动剔除密码类字段（`EXCLUDE_PROPERTIES`：`password` / `oldPassword` / `newPassword` / `confirmPassword`）

## 文件存储（OSS）

默认 **SeaweedFS**（任意 S3 兼容对象存储，基于 minio SDK，配置层为通用 `OSS_S3_*` 命名）；可切 `OSS_TYPE=aliyun` 用阿里云 OSS（需自行安装 `oss2`）。
Docker 编排已内置 seaweedfs 服务（单容器 `server -s3`：S3 API 8333；filer UI `http://localhost:8888`）。
注意：当前 SeaweedFS 版本静态 `-s3.config` 身份不生效（上游 #4728/#8331），S3 网关默认信任访问者——编排仅把 8333/8888 暴露给内网/宿主机调试，生产外网部署请置于防火墙/代理之后；后端仍按配置发送 AK/SK（网关兼容接受）。

- 上传对象 key 规则：`{yyyy/MM/dd}/{uuid}.{ext}`，桶由后端启动时自动创建
- 上传返回 url 形如 `/upload/{key}`（中性路径，跨环境可渲染），经 `GET /upload/{key}` 由后端从对象存储流式返回：
  - 本机开发：vite 代理 `/upload` → 后端 8011
  - Docker 编排：nginx `location /upload/` 转发 → 后端
- 相关配置见 `walnut-backend-python/env/.env.example` 的 "OSS 文件存储" 段

## 数据库迁移（Alembic）

Schema 的唯一事实来源是 `walnut-backend-python/app/alembic/versions/` 下的迁移脚本，**不再使用启动时 `create_all`**。

### 执行时机

| 环境 | 时机 | 说明 |
| --- | --- | --- |
| dev（本机） | 应用启动时自动执行 | `.env.dev` 中 `DATABASE_AUTO_MIGRATE=True`；失败仅告警不阻断启动 |
| prod（Docker） | 容器启动前显式执行 | `docker-entrypoint.sh` 先 `upgrade head` 再启动应用，失败即快速失败 |

### 日常变更流程

1. 修改 `app/api/v1/**/model.py` 中的 ORM 模型；
2. `uv run main.py revision --env dev -m "变更描述"` —— autogenerate 对比模型与数据库生成迁移脚本；
3. **人工审查生成的脚本**（autogenerate 不能覆盖所有场景，如数据迁移、索引调整）；
4. `uv run main.py upgrade --env dev` 应用到本地库验证；
5. 迁移脚本随代码提交，生产由 entrypoint 自动应用。

### 其他命令

```bash
uv run main.py downgrade --env dev -r -1     # 回退一步（-r base 全部回退）
uv run main.py stamp --env dev               # 仅写版本标记不执行 DDL（存量库接入用）
uv run main.py current --env dev             # 查看当前版本
uv run main.py history --env dev             # 查看版本链
```

### 存量库接入（一次性）

旧版通过 `create_all` 建库、结构已等同模型的数据库，执行 `uv run main.py stamp --env dev`
写入 `alembic_version` 即可接入（不执行任何 DDL）。**切勿对未 stamp 的存量库直接 upgrade**
（会因表已存在而失败）。Docker 实例建议直接删除 `docker/volumes` 重建。

### 注意事项

- MySQL 的 DDL 不受事务保护（`transaction_per_migration` 对 DDL 无效）：迁移中途失败可能留下
  半应用的 schema，需人工核对数据库状态后重新 `upgrade` 或 `stamp`。
- 回退初始迁移会删除全部表，请在沙箱库操作。
- 约束/索引命名遵循 `app/core/base_model.py` 中的 `NAMING_CONVENTION`（`ix_*`/`uq_*` 等），
  旧库 stamp 后首次 autogenerate 可能出现索引改名的表面 diff，保留（收敛命名）或手工剪掉均可。

## 配置摆放

原则：**部署级配置进 `docker/`，应用级配置留在应用侧**；容器环境变量以 compose 为唯一入口，不另设 env 文件。

### 我想改 X，去哪改

| 想改什么 | 去哪改 | 说明 |
|---|---|---|
| 本地开发的 Python 后端行为（连库、密钥等） | `walnut-backend-python/env/.env.dev` | 只放差异项，其余走代码默认值 |
| 本地开发的 Java 后端行为 | `walnut-backend-java/src/main/resources/application-dev.yml` | dev profile 默认值 |
| 本地开发的 Go 后端行为 | `walnut-backend-go/env/.env.dev` | koanf 三级优先级 |
| 任何环境的 Python 配置项默认值 | `walnut-backend-python/app/config/setting.py` | 唯一事实源；改动影响所有环境 |
| 任何环境的 Go 配置项默认值 | `walnut-backend-go/internal/config/config.go` | 唯一事实源；改动影响所有环境 |
| Docker 部署的 Python 后端环境变量 | `docker/docker-compose.yml` 的 `backend.environment` | 环境变量优先于 env 文件 |
| Docker 部署的 Java 后端环境变量 | `docker/docker-compose.java.yml` 的 `backend.environment` | Spring relaxed-binding 环境变量（`SPRING_*`）覆盖 yml |
| Docker 部署的 Go 后端环境变量 | `docker/docker-compose.go.yml` 的 `backend.environment` | 环境变量优先于 env 文件 |
| 密码 / JWT 密钥 / 后端端口等共用值 | `docker/.env`（模板 `.env.example`） | compose 变量插值，一处改多处生效（三套编排共用） |
| 中间件端口映射 / 健康检查 / 启动参数 | `docker/docker-compose.middleware.yml` | 全栈与中间件编排共用这份定义 |
| 前端 nginx 路由 / 代理 / HTTPS | `docker/config/nginx.conf` | 构建期经 `additional_contexts` 注入前端镜像，改后需 `--build frontend` |
| 前端构建期变量（API 地址、RSA 公钥等）/ dev 代理 | `walnut-frontend/apps/web-antd/.env.*`、`vite.config.ts` | 构建时注入 / vite dev server 代理 |

### 配置加载优先级（后端）

```
真实环境变量（compose / shell 注入）
  > env/.env.{ENVIRONMENT} 文件（本地开发）
  > setting.py 代码默认值（兜底）
```

Docker 镜像不打包 `env/` 目录（见 `.dockerignore`），容器内配置全部来自 compose 环境变量；`docker/.env` 是 compose 的**变量插值文件**，与后端的 `env/` 目录是两套互不相干的机制。

## 首次部署清单

1. `cp docker/.env.example docker/.env`，**必须**设置 `JWT_SECRET_KEY`（编排以 `:?` 强制校验，缺失或为空直接启动失败）：
   `python -c "import secrets; print(secrets.token_hex(32))"`
2. 按需修改 `DB_PASSWORD`（注意：已有数据卷的 MySQL 密码不会因改此值而变，需同步 `ALTER USER`）
3. `docker compose -f docker/docker-compose.yml up -d --build`（Java 后端则用 `docker-compose.java.yml`）
