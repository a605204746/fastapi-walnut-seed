# golang-migrate 迁移实战

> 更新日期：2026-11-04 · 适用版本：WalnutSeed v1.0

本项目 Schema 的唯一事实来源是 `walnut-backend-go/migrations/` 下的版本化 SQL 文件。Go 端**不使用** GORM AutoMigrate（生产禁用），而是用 golang-migrate。

## 1. 迁移是怎么跑起来的

### 1.1 入口

`internal/pkg/migratex/`：

```go
m, err := migratex.Open(ctx, params, cfg.MigrationsDir)
m.Up()
```

迁移文件命名：`0001_init_schema.up.sql` / `0001_init_schema.down.sql`。

### 1.2 两条触发路径

| 环境 | 触发点 | 失败行为 |
|---|---|---|
| dev | 应用启动时（`DATABASE_AUTO_MIGRATE=true`） | 仅告警，不阻断启动 |
| prod | `docker-entrypoint.sh` 显式执行 | `set -e` 快速失败 |

### 1.3 建库引导

Go 的 mysql 驱动没有 JDBC 的 `createDatabaseIfNotExist`。`databasex.EnsureDatabase` 在迁移前连接无库名的 MySQL，执行 `CREATE DATABASE IF NOT EXISTS`。

## 2. 工作流

### 2.1 查看当前版本

```bash
go run . migrate-status --env dev
```

### 2.2 执行迁移

```bash
go run . migrate --env dev        # up to latest
go run . migrate --env dev --to 3 # up/down to version 3
```

### 2.3 新增迁移文件

手动在 `migrations/` 下创建：

```
migrations/
├── 0001_init_schema.up.sql
├── 0001_init_schema.down.sql
├── 0002_add_blog_tables.up.sql    ← 新增
└── 0002_add_blog_tables.down.sql  ← 新增
```

up.sql 写 CREATE/ALTER，down.sql 写反向 DROP。

## 3. 与 Python 端 Schema 一致性

`scripts/verify_schema_parity.py` 比对两端数据库 schema（表/列/索引/约束），确保逐条一致：

```bash
python scripts/verify_schema_parity.py
```

基线：17 张系统表 / 208 列 / 40 索引 / 18 约束逐项一致。

## 4. 种子数据

`seed/mysql_data.sql` 由 `seedx.Run` 幂等执行：

```bash
go run . seed --env dev
```

种子数据包含 admin/admin123 测试账号、完整菜单/角色/字典。

## 5. 注意事项

- **DIRTY 状态**：golang-migrate 会在 `schema_migrations` 表标记 dirty，下次启动前必须手动清理
- **multiStatements=true**：仅用于迁移专用连接（`databasex.OpenForMigration`），不要加到应用主 DSN
- **禁用 AutoMigrate**：`gorm.Config` 未开启 AutoMigrate，GORM 不会自动建表/改列
