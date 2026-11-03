# 种子数据目录

P1 阶段接入 `mysql_data.sql`，基线须与 Python 端 `app/seed/sql/mysql_data.sql`
及 Java 端 Flyway 的 `V1__init_walnut_seed_boot.sql` 一致：

- `admin / admin123`（**生产使用前必须改密**）
- pc 客户端（`clientid` 与登录校验联动）
- 完整菜单 / 角色 / 字典 / 部门 / 岗位

种子写入必须**幂等**：容器重启不能产生重复数据。
