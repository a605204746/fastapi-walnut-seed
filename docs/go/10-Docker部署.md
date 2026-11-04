# Docker 全栈部署

> 更新日期：2026-11-04 · 适用版本：WalnutSeed v1.0

## 1. 编排文件

`docker/` 下三份编排，通过 `include` 复用中间件定义：

| 编排文件 | 场景 | 包含服务 |
|---|---|---|
| `docker-compose.go.yml` | 生产/全栈（Go 后端） | mysql + redis + seaweedfs + **backend(go) + frontend** |
| `docker-compose.yml` | 生产/全栈（Python 后端） | mysql + redis + seaweedfs + backend(python) + frontend |
| `docker-compose.java.yml` | 生产/全栈（Java 后端） | mysql + redis + seaweedfs + backend(java) + frontend |
| `docker-compose.middleware.yml` | 仅中间件（开发用） | mysql + redis + seaweedfs |

**互斥运行**：全栈编排项目名相同（`walnut-seed`），容器名和端口映射一致，不能同时起。但正因容器名和数据卷一致，**互切不丢数据**（各后端业务库独立：`walnut_seed_go` / `walnut_seed_python` / `walnut_seed_java`）。

## 2. Go 后端镜像

`walnut-backend-go/Dockerfile`：

- 构建阶段：`golang:1.26-alpine`
- 运行阶段：`alpine:3.20`（最终镜像约 76MB）
- `CGO_ENABLED=0` 静态二进制
- 非 root uid 1000

### 2.1 Docker Hub 镜像拉取

Docker Hub 在部分网络不可达，需经镜像源拉取后重打标签：

```bash
docker pull docker.m.daocloud.io/library/golang:1.26-alpine
docker pull docker.m.daocloud.io/library/alpine:3.20
docker pull docker.m.daocloud.io/docker/dockerfile:1

docker tag docker.m.daocloud.io/library/golang:1.26-alpine golang:1.26-alpine
docker tag docker.m.daocloud.io/library/alpine:3.20 alpine:3.20
docker tag docker.m.daocloud.io/docker/dockerfile:1 docker/dockerfile:1
```

> 第三条最容易漏：`# syntax=docker/dockerfile:1` 指令本身也要从 Docker Hub 拉 BuildKit frontend。

## 3. 启动全栈

```bash
cp docker/.env.example docker/.env    # 配置 JWT_SECRET_KEY（必填）
docker compose -f docker/docker-compose.go.yml up -d --build
```

访问 `http://localhost:8010`，初始账号 `admin / admin123`。

## 4. 服务端口

| 服务 | 容器端口 | 宿主机端口 |
|---|---|---|
| mysql | 3306 | 3307 |
| redis | 6379 | 6380 |
| seaweedfs S3 | 8333 | 8333 |
| seaweedfs filer | 8888 | 8888 |
| frontend (nginx) | 80 | 8010 |
| backend | 8011 | 不暴露（仅容器网络） |

## 5. 启动链路

`docker-entrypoint.sh`：

```bash
set -e
go run . migrate --env prod    # 迁移失败即退出
go run . seed --env prod       # 幂等播种
go run . run --env prod        # 启动 HTTP 服务
```

`depends_on` 配合中间件 healthy 状态门禁，`start_interval: 2s`。

## 6. 优雅停机

- `stop_grace_period: 20s`（大于应用收尾的 15s）
- `http.Server.Shutdown(ctx)` → 关 GORM/Redis 池 → 排空后台 worker
- 信号处理与 HTTP 退出可能各触发一次收尾（`sync.Once` 保护）
