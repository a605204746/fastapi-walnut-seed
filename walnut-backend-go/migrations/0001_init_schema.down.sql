-- WalnutSeed Go 后端 · 回滚初始 schema
--
-- 按依赖逆序删除。本项目 17 张表**均无外键约束**，故顺序理论上无关，
-- 仍按 up 的逆序书写以便阅读与将来加入外键时不出错。
--
-- ⚠️ MySQL 的 DDL 非事务性：中途失败无法回滚，需人工检查残留表。

DROP TABLE IF EXISTS `sys_user_role`;
DROP TABLE IF EXISTS `sys_user_post`;
DROP TABLE IF EXISTS `sys_user`;
DROP TABLE IF EXISTS `sys_social`;
DROP TABLE IF EXISTS `sys_role_menu`;
DROP TABLE IF EXISTS `sys_role_dept`;
DROP TABLE IF EXISTS `sys_role`;
DROP TABLE IF EXISTS `sys_post`;
DROP TABLE IF EXISTS `sys_oper_log`;
DROP TABLE IF EXISTS `sys_notice`;
DROP TABLE IF EXISTS `sys_menu`;
DROP TABLE IF EXISTS `sys_logininfor`;
DROP TABLE IF EXISTS `sys_dict_type`;
DROP TABLE IF EXISTS `sys_dict_data`;
DROP TABLE IF EXISTS `sys_dept`;
DROP TABLE IF EXISTS `sys_config`;
DROP TABLE IF EXISTS `sys_client`;
