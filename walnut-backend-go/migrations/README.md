# 迁移脚本目录

版本化 SQL 由 golang-migrate 管理，P1 阶段接入：

```
migrations/
  0001_init_schema.up.sql      # 17 张表，须与 Alembic 51e22ab7d9b4 逐列/索引/约束一致
  0001_init_schema.down.sql
  0002_xxx.up.sql
  0002_xxx.down.sql
```

**生产环境严禁使用 GORM 的 `AutoMigrate`**——Schema 的唯一事实源必须是这里的版本化 SQL，
否则迁移历史与实际结构会脱节，造成三端 schema 漂移。
