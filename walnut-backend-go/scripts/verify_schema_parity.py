"""对比 walnut_seed_python 与 walnut_seed_go 两库的表结构（计划 R13 的验收方法）。

**比 information_schema 元数据，而不是 SHOW CREATE TABLE 的文本。**

为什么不用 SHOW CREATE TABLE / mysqldump 文本对比（首版就是这么做的，结论是错的）：
它的输出含**语义冗余的显示差异**。实测两库所有列的 charset/collation 在
information_schema 里完全一致，但 Go 库的 SHOW CREATE 每个 varchar 列都多出
`CHARACTER SET utf8mb4` 前缀，而 Python 库省略——因为 MySQL 只在「无法从表默认值
推断」时才打印该子句，而这个判断受建表路径影响。该子句重述的就是表默认值，
是 no-op，**不影响任何行为**，却会造成 216 行假差异。

information_schema 是权威来源，比对以下五个维度：
  1. TABLES   : engine / table_collation / table_comment
  2. COLUMNS  : 列序、类型、长度、精度、可空性、默认值、charset、collation、extra、注释
  3. STATISTICS: 索引名、列序、唯一性、索引类型
  4. TABLE_CONSTRAINTS + KEY_COLUMN_USAGE: 主键与唯一约束
  5. 表清单本身（多表/少表都要报）

用法：python scripts/verify_schema_parity.py
"""

import subprocess
import sys
from collections import defaultdict

CONTAINER = "walnut-mysql"
MYSQL_PWD = "walnut123"
PY_DB = "walnut_seed_python"
GO_DB = "walnut_seed_go"

# 17 张 sys_* 表。不含各迁移工具自有的版本表
# （schema_migrations vs alembic_version，本就该不同）。
TABLES = [
    "sys_client", "sys_config", "sys_dept", "sys_dict_data", "sys_dict_type",
    "sys_logininfor", "sys_menu", "sys_notice", "sys_oper_log", "sys_post",
    "sys_role", "sys_role_dept", "sys_role_menu", "sys_social", "sys_user",
    "sys_user_post", "sys_user_role",
]

TABLE_LIST = "'" + "','".join(TABLES) + "'"


def query(sql: str) -> list[tuple[str, ...]]:
    """在容器内执行 SQL，返回行列表（TSV 解析）。"""
    cmd = [
        "docker", "exec", CONTAINER,
        "mysql", f"-uroot", f"-p{MYSQL_PWD}", "-N", "-B", "-e", sql,
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True, encoding="utf-8")
    if proc.returncode != 0:
        raise SystemExit(f"查询失败: {proc.stderr.strip()}")
    rows = []
    for line in proc.stdout.splitlines():
        if not line.strip() or line.startswith("mysql:"):
            continue
        # NULL 在 -B 模式下输出为字面量 NULL，保留原样以便区分「NULL」与「空串」
        rows.append(tuple(line.split("\t")))
    return rows


def fetch(db: str, sql_tpl: str) -> list[tuple]:
    return query(sql_tpl.replace("{db}", db).replace("{tables}", TABLE_LIST))


# ---- 五个维度的查询 ----

SQL_TABLES = """
SELECT table_name, engine, table_collation, IFNULL(table_comment,'')
FROM information_schema.tables
WHERE table_schema='{db}' AND table_name IN ({tables})
ORDER BY table_name;
"""

SQL_COLUMNS = """
SELECT table_name, ordinal_position, column_name, column_type,
       is_nullable, IFNULL(column_default,'<NULL>'), IFNULL(character_set_name,''),
       IFNULL(collation_name,''), extra, IFNULL(column_comment,'')
FROM information_schema.columns
WHERE table_schema='{db}' AND table_name IN ({tables})
ORDER BY table_name, ordinal_position;
"""

SQL_INDEXES = """
SELECT table_name, index_name, seq_in_index, column_name, non_unique, index_type
FROM information_schema.statistics
WHERE table_schema='{db}' AND table_name IN ({tables})
ORDER BY table_name, index_name, seq_in_index;
"""

SQL_CONSTRAINTS = """
SELECT c.table_name, c.constraint_name, c.constraint_type,
       GROUP_CONCAT(k.column_name ORDER BY k.ordinal_position)
FROM information_schema.table_constraints c
JOIN information_schema.key_column_usage k
  ON k.constraint_name=c.constraint_name AND k.table_schema=c.table_schema
 AND k.table_name=c.table_name
WHERE c.table_schema='{db}' AND c.table_name IN ({tables})
GROUP BY c.table_name, c.constraint_name, c.constraint_type
ORDER BY c.table_name, c.constraint_name;
"""

DIMENSIONS = [
    ("表属性 (engine/collation/comment)", SQL_TABLES),
    ("列定义 (序/类型/可空/默认值/charset/collation/extra/注释)", SQL_COLUMNS),
    ("索引 (名/列序/唯一性/类型)", SQL_INDEXES),
    ("约束 (主键/唯一键)", SQL_CONSTRAINTS),
]


def main() -> int:
    print("=" * 78)
    print(f"表结构对比：{PY_DB} (Alembic)  vs  {GO_DB} (golang-migrate)")
    print("=" * 78)

    # ---- 0. 表清单 ----
    py_tables = {r[0] for r in fetch(PY_DB, SQL_TABLES)}
    go_tables = {r[0] for r in fetch(GO_DB, SQL_TABLES)}
    print(f"  Python 库表数: {len(py_tables)}    Go 库表数: {len(go_tables)}    期望: {len(TABLES)}")

    total_fail = 0
    if py_tables != set(TABLES):
        print(f"  ❌ Python 库缺少表: {sorted(set(TABLES) - py_tables)}")
        total_fail += 1
    if go_tables != set(TABLES):
        missing = sorted(set(TABLES) - go_tables)
        extra = sorted(go_tables - set(TABLES))
        if missing:
            print(f"  ❌ Go 库缺少表: {missing}")
        if extra:
            print(f"  ❌ Go 库多出表: {extra}")
        total_fail += 1
    else:
        print("  ✅ 表清单一致（17 张，无缺失、无多余）")

    # ---- 1~4. 逐维度比对 ----
    for label, sql in DIMENSIONS:
        py_rows = fetch(PY_DB, sql)
        go_rows = fetch(GO_DB, sql)

        # 用集合差而非逐行 diff：行序在同一 ORDER BY 下应一致，
        # 但集合差能给出「哪条属性不同」而不是「第几行不同」，更易定位
        py_set, go_set = set(py_rows), set(go_rows)
        only_py = py_set - go_set
        only_go = go_set - py_set

        if not only_py and not only_go and len(py_rows) == len(go_rows):
            print(f"  ✅ {label}：{len(py_rows)} 条记录完全一致")
            continue

        total_fail += 1
        print(f"  ❌ {label}：{len(only_py)} 条仅 Python 有 / {len(only_go)} 条仅 Go 有")
        for row in sorted(only_py)[:8]:
            print(f"       PY-only: {row}")
        for row in sorted(only_go)[:8]:
            print(f"       GO-only: {row}")
        if len(only_py) + len(only_go) > 16:
            print(f"       ...（其余 {len(only_py) + len(only_go) - 16} 条省略）")

    print("-" * 78)
    if total_fail == 0:
        print("✅ 两库表结构完全一致 —— 计划 R13（schema 漂移）已排除")
        print()
        print("   含义：migrations/0001_init_schema.up.sql 与 Alembic 51e22ab7d9b4 产出的")
        print("         结构在**权威元数据层面**逐项等价（列序、类型、长度、可空性、默认值、")
        print("         charset、collation、extra、注释、索引、约束全部一致）。")
        print()
        print("   注：SHOW CREATE TABLE 的文本可能仍有 `CHARACTER SET utf8mb4` 冗余前缀差异，")
        print("       那是 MySQL 的显示行为、非结构差异，详见本文件顶部说明。")
        return 0

    print(f"❌ 发现 {total_fail} 个维度存在差异，请核对 migrations/0001_init_schema.up.sql")
    return 1


if __name__ == "__main__":
    sys.exit(main())
