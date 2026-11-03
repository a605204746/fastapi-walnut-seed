# syntax=docker/dockerfile:1
#
# WalnutSeed Go 后端镜像（两阶段构建）
#
# 与 Java 端 Dockerfile 的约定保持一致：
#   - 构建期依赖走可配置镜像源（ARG GOPROXY / ARG BUILD_IMAGE / ARG RUNTIME_IMAGE），
#     因为国内直连 Docker Hub 与 proxy.golang.org 均不稳定
#     （Java 端曾因拉取失败把基础镜像换成 eclipse-temurin，见 commit a5bff7b）
#   - go mod download 单独成层，源码变更不会打穿依赖缓存
#     （对齐 commit 72534a5 的前端构建缓存优化思路）
#   - 非 root 运行（uid/gid 1000，与 Java 端 appuser 一致），
#     日志目录 docker/volumes/logs 需对 uid 1000 可写

# ==================== 可覆盖的基础镜像 ====================
# ⚠️ 这两个 ARG 必须声明在**第一个 FROM 之前**（全局作用域）。
#    Docker 的 FROM 只能引用全局 ARG；写在某个 stage 内部的 ARG 属于该 stage 作用域，
#    FROM 引用它会报 "base name (${RUNTIME_IMAGE}) should not be blank"。
#
# ⚠️ BUILD_IMAGE 版本必须 >= go.mod 的 go 指令：当前依赖链（gin v1.12 / go-redis v9.23）
#    要求 Go >= 1.26。版本偏低会在 go build 时报 "requires go >= 1.26.0" 直接失败。
#
# 国内网络若拉不到 Docker Hub（registry-1.docker.io 直连超时），先经镜像源拉取再重打标签，
# **无需修改本文件、也无需改 Docker 全局配置**（实测可用源：docker.m.daocloud.io，
# 备选 docker.aityp.com / registry.cn-hangzhou.aliyuncs.com）：
#
#    docker pull docker.m.daocloud.io/library/golang:1.26-alpine
#    docker pull docker.m.daocloud.io/library/alpine:3.20
#    docker pull docker.m.daocloud.io/docker/dockerfile:1
#    docker tag  docker.m.daocloud.io/library/golang:1.26-alpine golang:1.26-alpine
#    docker tag  docker.m.daocloud.io/library/alpine:3.20        alpine:3.20
#    docker tag  docker.m.daocloud.io/docker/dockerfile:1        docker/dockerfile:1
#
# ⚠️ 第三条最容易漏：文件首行的 `# syntax=docker/dockerfile:1` 指令本身也要从 Docker Hub
#    拉 BuildKit frontend 镜像，不重打标签的话构建会在**执行任何一条指令之前**就失败。
#    注意非 library 镜像不带 `library/` 前缀。
# 也可用 --build-arg BUILD_IMAGE=/RUNTIME_IMAGE= 直接指向镜像源地址。
ARG BUILD_IMAGE=golang:1.26-alpine
ARG RUNTIME_IMAGE=alpine:3.20

# ==================== 构建阶段 ====================
FROM ${BUILD_IMAGE} AS builder

# 国内网络可用配方，详见 docs/go/00-Go后端实施计划.md §9.4
# ⚠️ GOSUMDB 保持默认值：显式改成 sum.golang.google.cn 会让 Go 绕过代理直连（被重置），
#    默认值反而会让 sumdb 查询经 GOPROXY 转发。也不要设 off——工具链模块强制要求校验。
ARG GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB} \
    GOTOOLCHAIN=auto \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /build

# 先只拷依赖清单，充分利用层缓存
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 再拷源码构建
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/walnut-seed-go .

# ==================== 运行阶段 ====================
# 用 alpine 而非 scratch：compose 的 healthcheck 依赖 busybox wget 探测
# /common/health/ready（与 Java 端编排同一套门禁写法）
# RUNTIME_IMAGE 已在文件顶部作为全局 ARG 声明，此处不可重复声明（会退回 stage 作用域）
FROM ${RUNTIME_IMAGE}

# ca-certificates: 调用外部 HTTPS（对象存储、第三方登录）
# tzdata:          时区支持，配合 TZ=Asia/Shanghai 让时间字段与另两端一致
# curl 不装：healthcheck 用 busybox 自带的 wget，少一层攻击面
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -g 1000 appgroup \
    && adduser -D -u 1000 -G appgroup appuser

WORKDIR /walnut-backend

# LOG_DIR 留空 = 日志只写 stdout 交给容器采集；
# 需要落盘时由 compose 显式设置 LOG_DIR 并挂载卷（docker-compose.go.yml 已这么做）
ENV TZ=Asia/Shanghai \
    ENVIRONMENT=prod \
    SERVER_HOST=0.0.0.0 \
    SERVER_PORT=8011 \
    LOG_DIR=

COPY --from=builder /out/walnut-seed-go /walnut-backend/walnut-seed-go
# 迁移脚本与种子数据随镜像分发，由 entrypoint 在启动前应用
COPY migrations/ /walnut-backend/migrations/
COPY seed/ /walnut-backend/seed/
# 入口脚本：先 migrate up 再 exec 应用（对齐 Python 端 docker-entrypoint.sh 的策略）
COPY docker-entrypoint.sh /walnut-backend/docker-entrypoint.sh

# entrypoint 需可执行；chmod 与 chown、建日志目录合并成一层，避免多余镜像层
RUN chmod +x /walnut-backend/docker-entrypoint.sh \
    && mkdir -p /walnut-backend/server/logs \
    && chown -R appuser:appgroup /walnut-backend

USER appuser

EXPOSE 8011

# exec 形式的 ENTRYPOINT + CMD：entrypoint 内部用 `exec "$@"` 启动应用，
# 确保 SIGTERM 能直达 Go 进程触发优雅停机（shell 形式会让信号被 /bin/sh 吞掉，
# 容器只能等 stop_grace_period 到期被 SIGKILL，在途请求被中断）。
#
# ⚠️ docker-entrypoint.sh 必须是 LF 行尾，CRLF 会让 /bin/sh 报
#    "not found"——仓库 .gitattributes 已用 `*.sh text eol=lf` 强制。
#
# 需要跳过迁移直接启动时（迁移已在发布流程中统一执行过），可覆盖 entrypoint：
#   docker run --entrypoint /walnut-backend/walnut-seed-go walnut-seed-backend-go run
ENTRYPOINT ["/walnut-backend/docker-entrypoint.sh"]
CMD ["run"]
