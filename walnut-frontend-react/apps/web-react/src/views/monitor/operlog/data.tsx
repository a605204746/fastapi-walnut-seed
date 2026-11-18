import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@vben/constants';

import { getDictOptions } from '@/utils/dict';
import { renderDict } from '@/utils/render';

/** 操作日志的查询表单 / 表格列（对应 Vue 侧 monitor/operlog/data.tsx） */

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'title',
    label: '系统模块',
  },
  {
    component: 'Input',
    fieldName: 'operName',
    label: '操作人员',
  },
  {
    component: 'Select',
    componentProps: {
      options: getDictOptions(DictEnum.SYS_OPER_TYPE),
    },
    fieldName: 'businessType',
    label: '操作类型',
  },
  {
    component: 'Input',
    fieldName: 'operIp',
    label: '操作IP',
  },
  {
    component: 'Select',
    componentProps: {
      options: getDictOptions(DictEnum.SYS_COMMON_STATUS),
    },
    fieldName: 'status',
    label: '状态',
  },
  {
    component: 'RangePicker',
    componentProps: {
      valueFormat: 'YYYY-MM-DD HH:mm:ss',
    },
    fieldName: 'createTime',
    label: '操作时间',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  { field: 'title', title: '系统模块' },
  {
    field: 'businessType',
    slots: {
      default: ({ row }) => renderDict(row.businessType, DictEnum.SYS_OPER_TYPE),
    },
    title: '操作类型',
  },
  { field: 'operName', title: '操作人员' },
  { field: 'operIp', title: 'IP地址' },
  { field: 'operLocation', title: 'IP信息', width: 200 },
  {
    field: 'status',
    slots: {
      default: ({ row }) => renderDict(row.status, DictEnum.SYS_COMMON_STATUS),
    },
    title: '操作状态',
  },
  { field: 'operTime', sortable: true, title: '操作日期' },
  {
    field: 'costTime',
    formatter: ({ cellValue }) => `${cellValue} ms`,
    sortable: true,
    title: '操作耗时',
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
