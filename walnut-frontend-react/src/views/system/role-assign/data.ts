import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

/**
 * 分配用户页的查询表单 / 表格列（对应 Vue 侧 views/system/role-assign/data.tsx）。
 *
 * Vue 侧文件后缀是 `.tsx`，但内容里并没有 JSX（action 列用的是具名字符串 slot），
 * React 侧改成 `.ts` 更准确。
 */

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'userName',
    label: '用户账号',
  },
  {
    component: 'Input',
    fieldName: 'phonenumber',
    label: '手机号码',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'userName',
    title: '用户账号',
  },
  {
    field: 'nickName',
    title: '用户昵称',
  },
  {
    field: 'email',
    title: '邮箱',
  },
  {
    field: 'phonenumber',
    title: '手机号',
  },
  {
    field: 'action',
    fixed: 'right',
    resizable: false,
    slots: { default: 'action' },
    title: '操作',
    width: 'auto',
  },
];
