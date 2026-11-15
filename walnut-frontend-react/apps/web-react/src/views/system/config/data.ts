import type { FormSchema } from '@vben-core/form-ui';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@vben/constants';
import { getPopupContainer } from '@vben/utils';

import { getDictOptions } from '@/utils/dict';
import { renderDict } from '@/utils/render';

export type FormSchemaGetter = () => FormSchema[];

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'configName',
    label: '参数名称',
  },
  {
    component: 'Input',
    fieldName: 'configKey',
    label: '参数键名',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_YES_NO),
    },
    fieldName: 'configType',
    label: '系统内置',
  },
  {
    component: 'RangePicker',
    defaultValue: [null, null],
    fieldName: 'createTime',
    label: '创建时间',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'configName',
    title: '参数名称',
  },
  {
    field: 'configKey',
    title: '参数KEY',
  },
  {
    field: 'configValue',
    title: '参数Value',
  },
  {
    field: 'configType',
    slots: {
      default: ({ row }) => renderDict(row.configType, DictEnum.SYS_YES_NO),
    },
    title: '系统内置',
    width: 120,
  },
  {
    field: 'remark',
    title: '备注',
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

export const modalSchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    dependencies: {
      show: () => false,
      triggerFields: [''],
    },
    fieldName: 'id',
    label: '参数主键',
  },
  {
    component: 'Input',
    fieldName: 'configName',
    label: '参数名称',
    rules: 'required',
  },
  {
    component: 'Input',
    fieldName: 'configKey',
    label: '参数键名',
    rules: 'required',
  },
  {
    component: 'Textarea',
    componentProps: {
      autoSize: true,
    },
    fieldName: 'configValue',
    formItemClass: 'items-start',
    label: '参数键值',
    rules: 'required',
  },
  {
    component: 'RadioGroup',
    componentProps: {
      buttonStyle: 'solid',
      optionType: 'button',
      options: getDictOptions(DictEnum.SYS_YES_NO),
    },
    defaultValue: 'N',
    fieldName: 'configType',
    label: '是否内置',
    rules: 'required',
  },
  {
    component: 'Textarea',
    fieldName: 'remark',
    formItemClass: 'items-start',
    label: '备注',
  },
];
