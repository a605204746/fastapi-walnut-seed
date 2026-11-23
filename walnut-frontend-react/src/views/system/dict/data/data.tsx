import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@/core/constants';
import { getPopupContainer } from '@/core/utils';
import { Tag } from 'antd';

import { tagSelectOptions, tagTypes } from '@/components/dict';
import { getDictOptions } from '@/utils/dict';
import { renderDict } from '@/utils/render';

/**
 * 字典数据的查询表单 / 表格列 / 弹窗表单 schema
 * （对应 Vue 侧 views/system/dict/data/data.tsx，147 行）。
 */

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'dictLabel',
    label: '字典标签',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'dictLabel',
    title: '字典标签',
  },
  {
    field: 'dictValue',
    title: '字典键值',
  },
  {
    field: 'dictSort',
    title: '字典排序',
    width: 90,
  },
  {
    field: 'isDefault',
    slots: {
      default: ({ row }) => renderDict(row.isDefault, DictEnum.SYS_YES_NO),
    },
    title: '是否默认',
    width: 100,
  },
  {
    field: 'listClass',
    slots: {
      default: ({ row }) => {
        const listClass = (row.listClass || '') as string;
        if (!listClass) {
          return <span>-</span>;
        }
        const tagType = tagTypes[listClass];
        // 预设样式显示预设 label，自定义颜色直接作为 tag 颜色
        return (
          <Tag color={tagType?.color ?? listClass}>
            {tagType?.label ?? listClass}
          </Tag>
        );
      },
    },
    title: '回显样式',
    width: 150,
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
    label: 'id',
  },
  {
    component: 'Input',
    fieldName: 'dictLabel',
    label: '字典标签',
    rules: 'required',
  },
  {
    component: 'Input',
    fieldName: 'dictValue',
    label: '字典键值',
    rules: 'required',
  },
  {
    component: 'InputNumber',
    defaultValue: 0,
    fieldName: 'dictSort',
    label: '字典排序',
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
    fieldName: 'isDefault',
    label: '是否默认',
    rules: 'required',
  },
  {
    component: 'Select',
    componentProps: {
      allowClear: true,
      getPopupContainer,
      options: tagSelectOptions(),
    },
    fieldName: 'listClass',
    help: 'DictTag组件表格回显样式',
    label: '回显样式',
  },
  {
    component: 'Input',
    fieldName: 'cssClass',
    help: '自定义css类名',
    label: '样式属性',
  },
  {
    component: 'Textarea',
    fieldName: 'remark',
    label: '备注',
  },
];
