import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@/core/constants';
import { getPopupContainer } from '@/core/utils';

import { getDictOptions } from '@/utils/dict';
import { renderDict } from '@/utils/render';

/**
 * 岗位管理的查询表单 / 表格列 / 抽屉表单 schema
 * （对应 Vue 侧 views/system/post/data.ts）。
 *
 * 与 Vue 版的唯一差异：`componentProps.class` → `className`（React 侧组件直接收
 * antd props，没有 Vue 的 class 属性透传）。
 */

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'postCode',
    label: '岗位编码',
  },
  {
    component: 'Input',
    fieldName: 'postName',
    label: '岗位名称',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_NORMAL_DISABLE),
    },
    fieldName: 'status',
    label: '状态',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'postCode',
    title: '岗位编码',
  },
  {
    field: 'postCategory',
    title: '类别编码',
  },
  {
    field: 'postName',
    title: '岗位名称',
  },
  {
    field: 'postSort',
    title: '排序',
  },
  {
    field: 'status',
    slots: {
      default: ({ row }) => renderDict(row.status, DictEnum.SYS_NORMAL_DISABLE),
    },
    title: '状态',
  },
  {
    field: 'createTime',
    title: '创建时间',
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

export const drawerSchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    dependencies: {
      show: () => false,
      triggerFields: [''],
    },
    fieldName: 'id',
    label: 'id',
  },
  {
    component: 'TreeSelect',
    componentProps: {
      getPopupContainer,
    },
    fieldName: 'deptId',
    label: '所属部门',
    rules: 'selectRequired',
  },
  {
    component: 'Input',
    fieldName: 'postName',
    label: '岗位名称',
    rules: 'required',
  },
  {
    component: 'Input',
    fieldName: 'postCode',
    label: '岗位编码',
    rules: 'required',
  },
  {
    component: 'Input',
    fieldName: 'postCategory',
    label: '类别编码',
  },
  {
    component: 'InputNumber',
    componentProps: {
      className: 'flex-1',
    },
    defaultValue: 1,
    fieldName: 'postSort',
    label: '岗位排序',
    rules: 'required',
  },
  {
    component: 'RadioGroup',
    componentProps: {
      buttonStyle: 'solid',
      optionType: 'button',
      options: getDictOptions(DictEnum.SYS_NORMAL_DISABLE),
    },
    defaultValue: '0',
    fieldName: 'status',
    label: '岗位状态',
    rules: 'required',
  },
  {
    component: 'Textarea',
    fieldName: 'remark',
    formItemClass: 'items-start',
    label: '备注',
  },
];
