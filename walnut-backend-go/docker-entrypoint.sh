#!/bin/sh
# WalnutSeed Go 后端容器入口脚本
#
# 生产策略：**应用启动前**显式执行 schema 迁移，失败即快速失败（set -e），
# 绝不带着过期 schema 对外服务。
#
# 为什么不在应用内自动迁移：多副本滚动发布时，各副本会并发执行迁移并相互竞争
# （MySQL DDL 非事务，竞争失败的副本可能留下 dirty 状态）。统一在发布流程前置执行
# 才能避免。这也是 Python 端 docker-entrypoint.sh 的同一策略。
#
# 与 Python 端的对应关系：
#   python - <<PYEOF (CREATE DATABASE IF NOT EXISTS ...)  ↔  已内置于 migrate up
#                                                            （databasex.EnsureDatabase）
#   python main.py upgrade --env prod                     ↔  walnut-seed-go migrate up
#   exec "$@"                                             ↔  exec "$@"（保留信号传递）
#
# ⚠️ 本文件必须是 **LF 行尾**。CRLF 会让容器内 /bin/sh 报
#    "not found" 或 "bad interpreter"——这是 Docker-on-Windows 的经典坑。
#    仓库 .gitattributes 已强制 *.sh 使用 LF；若在 Windows 上手工编辑过，
#    构建前请确认行尾。
set -e

BIN=/walnut-backend/walnut-seed-go
MIGRATIONS_DIR=${MIGRATIONS_DIR:-/walnut-backend/migrations}
ENV_NAME=${ENVIRONMENT:-prod}

echo "[entrypoint] environment=${ENV_NAME}"

# ---- 建库 + 应用迁移 ----
# migrate up 内部会先执行 databasex.EnsureDatabase（CREATE DATABASE IF NOT EXISTS），
# 因为 Go 的 mysql 驱动没有 JDBC 的 createDatabaseIfNotExist，
# 而中间件编排的 MYSQL_DATABASE 只能建一个库（默认是 Python 端的库）。
#
# -y 跳过交互确认：容器内没有 TTY，等待输入会一直挂到 healthcheck 超时。
echo "[entrypoint] ensuring database exists and applying migrations..."
"${BIN}" migrate up --env "${ENV_NAME}" --dir "${MIGRATIONS_DIR}" -y

# ---- 启动应用 ----
# 用 exec 替换当前 shell 进程，使 Go 进程成为 PID 1 的直接继承者，
# 这样 docker stop 发出的 SIGTERM 能直达应用、触发优雅停机。
# 若写成不带 exec 的形式，信号会被 /bin/sh 吞掉，容器只能等
# stop_grace_period 到期后被 SIGKILL 强杀（在途请求被中断）。
echo "[entrypoint] starting application: ${BIN} $*"
exec "${BIN}" "$@"
