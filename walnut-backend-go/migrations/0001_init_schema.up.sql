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

-- ---------- sys_client ----------
CREATE TABLE `sys_client` (
  `client_id` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '客户端id',
  `client_key` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '客户端key',
  `client_secret` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '客户端秘钥',
  `grant_type` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '授权类型（逗号分隔）',
  `device_type` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '设备类型',
  `active_timeout` int DEFAULT NULL COMMENT 'token活跃超时时间（秒）',
  `timeout` int DEFAULT NULL COMMENT 'token固定超时时间（秒）',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '状态（0正常 1停用）',
  `del_flag` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '删除标志（0代表存在 1代表删除）',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_client_del_flag` (`del_flag`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统授权表';

-- ---------- sys_config ----------
CREATE TABLE `sys_config` (
  `config_name` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '参数名称',
  `config_key` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '参数键名',
  `config_value` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '参数键值',
  `config_type` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '系统内置（Y是 N否）',
  `remark` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_config_config_key` (`config_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='参数配置表';

-- ---------- sys_dept ----------
CREATE TABLE `sys_dept` (
  `parent_id` bigint DEFAULT NULL COMMENT '父部门ID',
  `ancestors` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '祖级列表',
  `dept_name` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '部门名称',
  `dept_category` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '部门类别编码',
  `order_num` int DEFAULT NULL COMMENT '显示顺序',
  `leader` bigint DEFAULT NULL COMMENT '负责人（用户ID）',
  `phone` varchar(11) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '联系电话',
  `email` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '邮箱',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '部门状态（0正常 1停用）',
  `del_flag` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '删除标志（0代表存在 1代表删除）',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_dept_del_flag` (`del_flag`),
  KEY `ix_sys_dept_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='部门表';

-- ---------- sys_dict_data ----------
CREATE TABLE `sys_dict_data` (
  `dict_sort` int DEFAULT NULL COMMENT '字典排序',
  `dict_label` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '字典标签',
  `dict_value` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '字典键值',
  `dict_type` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '字典类型',
  `css_class` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '样式属性（其他样式扩展）',
  `list_class` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '表格回显样式',
  `is_default` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '是否默认（Y是 N否）',
  `remark` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_dict_data_dict_type` (`dict_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='字典数据表';

-- ---------- sys_dict_type ----------
CREATE TABLE `sys_dict_type` (
  `dict_name` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '字典名称',
  `dict_type` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '字典类型',
  `remark` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_sys_dict_type_dict_type` (`dict_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='字典类型表';

-- ---------- sys_logininfor ----------
CREATE TABLE `sys_logininfor` (
  `id` bigint NOT NULL COMMENT '访问ID（雪花）',
  `user_name` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '用户账号',
  `client_key` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '客户端',
  `device_type` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '设备类型',
  `ipaddr` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '登录IP地址',
  `login_location` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '登录地点',
  `browser` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '浏览器类型',
  `os` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '操作系统',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '登录状态（0成功 1失败）',
  `msg` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '提示消息',
  `login_time` datetime DEFAULT NULL COMMENT '访问时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_logininfor_login_time` (`login_time`),
  KEY `ix_sys_logininfor_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统访问记录';

-- ---------- sys_menu ----------
CREATE TABLE `sys_menu` (
  `menu_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '菜单名称',
  `parent_id` bigint DEFAULT NULL COMMENT '父菜单ID',
  `order_num` int DEFAULT NULL COMMENT '显示顺序',
  `path` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '路由地址',
  `component` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '组件路径',
  `query_param` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '路由参数',
  `is_frame` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '是否为外链（0是 1否）',
  `is_cache` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '是否缓存（0缓存 1不缓存）',
  `menu_type` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '菜单类型（M目录 C菜单 F按钮）',
  `visible` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '显示状态（0显示 1隐藏）',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '菜单状态（0正常 1停用）',
  `perms` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '权限标识',
  `icon` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '菜单图标',
  `remark` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_menu_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='菜单权限表';

-- ---------- sys_notice ----------
CREATE TABLE `sys_notice` (
  `notice_title` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '公告标题',
  `notice_type` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '公告类型（1通知 2公告）',
  `notice_content` blob COMMENT '公告内容（UTF-8 字节）',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '公告状态（0正常 1关闭）',
  `remark` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知公告表';

-- ---------- sys_oper_log ----------
CREATE TABLE `sys_oper_log` (
  `id` bigint NOT NULL COMMENT '日志主键（雪花）',
  `title` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '模块标题',
  `business_type` int DEFAULT NULL COMMENT '业务类型（0其它 1新增 2修改 3删除）',
  `method` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '方法名称',
  `request_method` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求方式',
  `operator_type` int DEFAULT NULL COMMENT '操作类别（0其它 1后台用户 2手机端用户）',
  `oper_name` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '操作人员',
  `dept_name` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '部门名称',
  `oper_url` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求URL',
  `oper_ip` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '主机地址',
  `oper_location` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '操作地点',
  `oper_param` varchar(4000) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求参数',
  `json_result` varchar(4000) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '返回参数',
  `status` int DEFAULT NULL COMMENT '操作状态（0正常 1异常）',
  `error_msg` varchar(4000) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '错误消息',
  `oper_time` datetime DEFAULT NULL COMMENT '操作时间',
  `cost_time` bigint DEFAULT NULL COMMENT '消耗时间（毫秒）',
  PRIMARY KEY (`id`),
  KEY `ix_sys_oper_log_business_type` (`business_type`),
  KEY `ix_sys_oper_log_oper_time` (`oper_time`),
  KEY `ix_sys_oper_log_status` (`status`),
  KEY `ix_sys_oper_log_title` (`title`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='操作日志记录';

-- ---------- sys_post ----------
CREATE TABLE `sys_post` (
  `dept_id` bigint NOT NULL COMMENT '部门ID',
  `post_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '岗位编码',
  `post_category` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '岗位类别编码',
  `post_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '岗位名称',
  `post_sort` int NOT NULL COMMENT '显示顺序',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态（0正常 1停用）',
  `remark` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='岗位信息表';

-- ---------- sys_role ----------
CREATE TABLE `sys_role` (
  `role_name` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '角色名称',
  `role_key` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '角色权限字符串',
  `role_sort` int NOT NULL COMMENT '显示顺序',
  `data_scope` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '数据范围（1全部 2自定义 3本部门 4本部门及以下 5仅本人 6本部门及以下或本人）',
  `menu_check_strictly` int DEFAULT NULL COMMENT '菜单树选择项是否关联显示',
  `dept_check_strictly` int DEFAULT NULL COMMENT '部门树选择项是否关联显示',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '角色状态（0正常 1停用）',
  `remark` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `del_flag` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '删除标志（0代表存在 1代表删除）',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_role_del_flag` (`del_flag`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色信息表';

-- ---------- sys_role_dept ----------
CREATE TABLE `sys_role_dept` (
  `role_id` bigint NOT NULL COMMENT '角色ID',
  `dept_id` bigint NOT NULL COMMENT '部门ID',
  PRIMARY KEY (`role_id`,`dept_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色和部门关联表';

-- ---------- sys_role_menu ----------
CREATE TABLE `sys_role_menu` (
  `role_id` bigint NOT NULL COMMENT '角色ID',
  `menu_id` bigint NOT NULL COMMENT '菜单ID',
  PRIMARY KEY (`role_id`,`menu_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色和菜单关联表';

-- ---------- sys_social ----------
CREATE TABLE `sys_social` (
  `user_id` bigint NOT NULL COMMENT '用户ID',
  `auth_id` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '平台+平台唯一id',
  `source` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '用户来源',
  `open_id` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '平台openid',
  `user_name` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '登录账号',
  `nick_name` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '用户昵称',
  `email` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '用户邮箱',
  `avatar` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '用户头像',
  `access_token` varchar(2000) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '用户的授权令牌',
  `expire_in` int DEFAULT NULL COMMENT 'access_token的过期时间',
  `refresh_token` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '刷新令牌',
  `access_code` varchar(2000) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '授权code',
  `union_id` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '平台union_id',
  `scope` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '授予的权限',
  `token_type` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '令牌类型',
  `id_token` varchar(2000) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'id_token(部分平台可用)',
  `mac_algorithm` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'MAC算法',
  `mac_key` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'MAC密钥',
  `code` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求码',
  `oauth_token` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'OAuth token',
  `oauth_token_secret` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'OAuth token secret',
  `del_flag` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '删除标志（0代表存在 1代表删除）',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_social_del_flag` (`del_flag`),
  KEY `ix_sys_social_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='社交授权关系表';

-- ---------- sys_user ----------
CREATE TABLE `sys_user` (
  `dept_id` bigint DEFAULT NULL COMMENT '部门ID',
  `user_name` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '用户账号',
  `nick_name` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '用户昵称',
  `user_type` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '用户类型（sys_user系统用户）',
  `email` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '用户邮箱',
  `phonenumber` varchar(11) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '手机号码',
  `sex` varchar(1) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '用户性别',
  `avatar` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '用户头像',
  `password` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '密码',
  `status` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '帐号状态（0正常 1停用）',
  `login_ip` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '最后登录IP',
  `login_date` datetime DEFAULT NULL COMMENT '最后登录时间',
  `remark` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  `del_flag` varchar(1) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '删除标志（0代表存在 1代表删除）',
  `id` bigint NOT NULL COMMENT '主键ID（雪花）',
  `create_dept` bigint DEFAULT NULL COMMENT '创建部门',
  `create_by` bigint DEFAULT NULL COMMENT '创建者',
  `create_time` datetime DEFAULT NULL COMMENT '创建时间',
  `update_by` bigint DEFAULT NULL COMMENT '更新者',
  `update_time` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `ix_sys_user_del_flag` (`del_flag`),
  KEY `ix_sys_user_dept_id` (`dept_id`),
  KEY `ix_sys_user_user_name` (`user_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户信息表';

-- ---------- sys_user_post ----------
CREATE TABLE `sys_user_post` (
  `user_id` bigint NOT NULL COMMENT '用户ID',
  `post_id` bigint NOT NULL COMMENT '岗位ID',
  PRIMARY KEY (`user_id`,`post_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户与岗位关联表';

-- ---------- sys_user_role ----------
CREATE TABLE `sys_user_role` (
  `user_id` bigint NOT NULL COMMENT '用户ID',
  `role_id` bigint NOT NULL COMMENT '角色ID',
  PRIMARY KEY (`user_id`,`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户和角色关联表';
