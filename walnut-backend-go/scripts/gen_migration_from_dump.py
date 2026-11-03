"""把 mysqldump 导出的 Python 库 DDL 转换成 golang-migrate 迁移文件。

为什么不手写 SQL：17 张表、300+ 行 DDL，索引名 / 字符集 / collation / 默认值 /
注释任一处偏差就是计划 R13 说的 schema 漂移。**从已迁移好的真实库导出再转换**，
比从 Alembic 的 Python 代码人工翻译可靠得多，而且这本身就是 R13 的验收方法。

输入：%TEMP%/py_schema.sql（由 mysqldump --no-data 导出）
输出：walnut-backend-go/migrations/0001_init_schema.up.sql / .down.sql
"""

import re
import sys
from datetime import date
from pathlib import Path

SRC = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(r"C:\Users\shendu\AppData\Local\Temp\py_schema.sql")
MIG_DIR = Path(r"D:\code_work\github_work\fastapi-walnut-seed\walnut-backend-go\migrations")

UP_HEADER = """\
-- WalnutSeed Go 后端 · 初始 schema
--
-- 版本: 0001
-- 权威来源: 从**已迁移完成的 Python 库** walnut_seed_python 导出 DDL 后转换生成
--           （mysqldump --no-data），而非人工翻译 Alembic 的 Python 代码。
--           对应 Alembic 版本 51e22ab7d9b4_init_schema.py。
-- 转换脚本: scripts/gen_migration_from_dump.py
--
-- 范围: 17 张 sys_* 表。**不含 ai_* 四张表**（module_ai 在 Python 端尚未挂载到
--       router.py，不属于当前契约；见 docs/go/00-Go后端实施计划.md §11.5）。
--       也不含 alembic_version（那是 Alembic 自有的版本表；本端由 golang-migrate
--       的 schema_migrations 表承担同一职责）。
--
-- 约定:
--   - 无外键约束：与 Python / Java 端一致（RuoYi 风格，关联完整性由应用层维护）
--   - 索引命名遵循 Python 端 base_model.py 的 NAMING_CONVENTION：
--       ix_<table>_<column> / uq_<table>_<column> / fk_<table>_<column> / ck_<table>_<column>
--   - 全部表 ENGINE=InnoDB, CHARSET=utf8mb4, COLLATE=utf8mb4_unicode_ci
--   - 主键 id 为 bigint（雪花 ID），**不设 AUTO_INCREMENT**——ID 由应用层生成
--
-- ⚠️ 本文件是不可变的历史记录：schema 变更请新增 0002_xxx.up.sql，不要修改本文件。
"""

DOWN_HEADER = """\
-- WalnutSeed Go 后端 · 回滚初始 schema
--
-- 按依赖逆序删除。本项目 17 张表**均无外键约束**，故顺序理论上无关，
-- 仍按 up 的逆序书写以便阅读与将来加入外键时不出错。
--
-- ⚠️ MySQL 的 DDL 非事务性：中途失败无法回滚，需人工检查残留表。
"""


def main() -> int:
    if not SRC.exists():
        raise SystemExit(f"找不到输入文件: {SRC}")

    raw = SRC.read_text(encoding="utf-8")

    # 剥离 mysqldump 的会话变量噪音行（/*!40101 SET ... */;）
    lines = [
        line
        for line in raw.splitlines()
        if line.strip() and not re.match(r"^/\*!\d+\s+SET", line.strip())
    ]

    body = "\n".join(lines).strip()

    # 提取表名（保持 dump 中的顺序）
    tables = re.findall(r"^CREATE TABLE `([^`]+)`", body, flags=re.MULTILINE)
    if len(tables) != 17:
        raise SystemExit(f"期望 17 张表，实际解析到 {len(tables)} 张: {tables}")

    # 表间用空行 + 分隔注释，便于审阅
    blocks = re.split(r"(?=^CREATE TABLE `)", body, flags=re.MULTILINE)
    blocks = [b.strip() for b in blocks if b.strip()]
    if len(blocks) != len(tables):
        raise SystemExit(f"表块数 {len(blocks)} 与表名数 {len(tables)} 不一致")

    up_parts = [UP_HEADER]
    for name, block in zip(tables, blocks):
        # mysqldump 的 CREATE TABLE 已以 ';' 结尾，不要再补（否则产生 ';;'）
        if not block.endswith(";"):
            block += ";"
        up_parts.append(f"\n-- ---------- {name} ----------\n{block}\n")

    down_parts = [DOWN_HEADER]
    for name in reversed(tables):
        down_parts.append(f"DROP TABLE IF EXISTS `{name}`;")

    MIG_DIR.mkdir(parents=True, exist_ok=True)
    up_path = MIG_DIR / "0001_init_schema.up.sql"
    down_path = MIG_DIR / "0001_init_schema.down.sql"
    up_path.write_text("".join(up_parts), encoding="utf-8")
    down_path.write_text("\n".join(down_parts) + "\n", encoding="utf-8")

    print(f"[OK] 生成 {up_path.name}（{len(tables)} 张表，{up_path.stat().st_size} 字节）")
    print(f"[OK] 生成 {down_path.name}（{down_path.stat().st_size} 字节）")
    print()
    print("表清单（up 顺序 / down 逆序）：")
    for i, name in enumerate(tables, 1):
        print(f"  {i:2d}. {name}")

    # 自检：确认没有 ai_* 表混入，也没有 alembic_version
    for bad in ("ai_document", "ai_conversation", "ai_message", "alembic_version"):
        if f"`{bad}`" in body:
            raise SystemExit(f"意外包含表 {bad}，请检查 mysqldump 的表清单")
    print()
    print("[OK] 自检通过：未包含 ai_* 与 alembic_version")
    print(f"[i]  生成日期 {date.today().isoformat()}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
