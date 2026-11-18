/**
 * 角色管理页的 schema 与共享常量（对应 Vue 侧 views/system/role/data.tsx）。
 *
 * ⚠️ 当前只落地了 `authScopeOptions`，因为 **user 页的抽屉也要用它**
 * （Vue 侧 `user-drawer.vue` 就是 `import { authScopeOptions } from '#/views/system/role/data'`，
 * 原文件注释也写了「authScopeOptions user也会用到」）。
 * 角色页的 querySchema / columns / drawerSchema 随 A3 的 `system/role` 页一起补。
 */

/** 数据权限范围选项（值与后端 `sys_role.data_scope` 一致） */
export const authScopeOptions = [
  { color: 'green', label: '全部数据权限', value: '1' },
  { color: 'default', label: '自定数据权限', value: '2' },
  { color: 'orange', label: '本部门数据权限', value: '3' },
  { color: 'cyan', label: '本部门及以下数据权限', value: '4' },
  { color: 'error', label: '仅本人数据权限', value: '5' },
  { color: 'default', label: '部门及以下或本人数据权限', value: '6' },
];
