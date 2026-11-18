import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@vben/constants';
import { getPopupContainer } from '@vben/utils';

import { getDictOptions } from '@/utils/dict';
import { renderDict, renderDictTags } from '@/utils/render';

/**
 * 客户端管理的查询表单 / 表格列 / 抽屉表单 schema
 * （对应 Vue 侧 views/system/client/data.tsx）。
 *
 * 与 Vue 版的差异：
 * - `clientSecret` 字段 Vue 用 `component: 'Input'` + 表单插槽注入 SecretInput；
 *   React 侧直接把 `SecretInput` 注册进控件表，schema 写 `component: 'SecretInput'`。
 * - 列上的 `showOverflow: true` 未移植（React 适配层没有这个 vxe 专有配置，
 *   溢出省略由 antd Table 的默认样式 + `scroll.x` 承担），列入 B 阶段像素补齐。
 */

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'clientKey',
    label: '客户端key',
  },
  {
    component: 'Input',
    fieldName: 'clientSecret',
    label: '客户端密钥',
  },
  {
    component: 'Select',
    componentProps: {
      options: getDictOptions(DictEnum.SYS_NORMAL_DISABLE),
    },
    fieldName: 'status',
    label: '状态',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'clientId',
    title: '客户端ID',
  },
  {
    field: 'clientKey',
    title: '客户端key',
  },
  {
    field: 'clientSecret',
    title: '客户端密钥',
  },
  {
    field: 'grantTypeList',
    slots: {
      default: ({ row }) => {
        if (!row.grantTypeList) {
          return '无';
        }
        return renderDictTags(
          row.grantTypeList,
          getDictOptions(DictEnum.SYS_GRANT_TYPE),
          true,
          4,
        );
      },
    },
    title: '授权类型',
  },
  {
    field: 'deviceType',
    slots: {
      default: ({ row }) => renderDict(row.deviceType, DictEnum.SYS_DEVICE_TYPE),
    },
    title: '设备类型',
  },
  {
    field: 'activeTimeout',
    formatter: ({ row }) => `${row.activeTimeout}秒`,
    title: 'token活跃时间',
  },
  {
    field: 'timeout',
    formatter: ({ row }) => `${row.timeout}秒`,
    title: 'token超时时间',
  },
  {
    field: 'status',
    slots: { default: 'status' },
    title: '状态',
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
    component: 'Input',
    componentProps: {
      disabled: true,
    },
    dependencies: {
      show: () => false,
      triggerFields: [''],
    },
    fieldName: 'clientId',
    label: '客户端ID',
  },
  {
    component: 'Input',
    fieldName: 'clientKey',
    label: '客户端key',
    rules: 'required',
  },
  {
    component: 'SecretInput',
    fieldName: 'clientSecret',
    label: '客户端密钥',
    rules: 'required',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      mode: 'multiple',
      optionFilterProp: 'label',
      options: getDictOptions(DictEnum.SYS_GRANT_TYPE),
    },
    fieldName: 'grantTypeList',
    label: '授权类型',
    rules: 'selectRequired',
  },
  {
    component: 'Select',
    componentProps: {
      allowClear: false,
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_DEVICE_TYPE),
    },
    fieldName: 'deviceType',
    label: '设备类型',
    rules: 'selectRequired',
  },
  {
    component: 'InputNumber',
    componentProps: {
      addonAfter: '秒',
      placeholder: '请输入',
      style: {
        '--ant-border-radius': 'var(--radius) 0 0 var(--radius)',
      },
    },
    defaultValue: 1800,
    fieldName: 'activeTimeout',
    formItemClass: 'col-span-2 lg:col-span-1',
    help: '指定时间无操作则过期(单位：秒), 默认30分钟(1800秒)',
    label: 'Token活跃超时时间',
    rules: 'required',
  },
  {
    component: 'InputNumber',
    componentProps: {
      addonAfter: '秒',
      style: {
        '--ant-border-radius': 'var(--radius) 0 0 var(--radius)',
      },
    },
    defaultValue: 604_800,
    fieldName: 'timeout',
    formItemClass: 'col-span-2 lg:col-span-1',
    help: '指定时间必定过期(单位：秒)，默认七天(604800秒)',
    label: 'Token固定超时时间',
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
    label: '状态',
  },
];
