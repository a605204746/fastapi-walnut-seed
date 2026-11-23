import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

/**
 * 字典类型的查询表单 / 表格列 / 弹窗表单 schema
 * （对应 Vue 侧 views/system/dict/type/data.ts）。
 *
 * ⚠️ 与 Vue 版的差异：`dictType` 的格式校验 Vue 用 zod
 * （`z.string().regex(/^[a-z_]+$/i, {...})`），React 侧 form-ui 用轻量
 * `FormItemRule.validator`，语义等价。
 *
 * 另外 Vue 侧的 `dict/mitt.ts` 事件总线**不迁移**：全仓只有
 * `type/index-refactor.vue` 引用它，而那个文件是清单 §12 确认的死代码（零引用）。
 */

const DICT_TYPE_RE = /^[a-z_]+$/i;

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'dictName',
    label: '字典名称',
  },
  {
    component: 'Input',
    fieldName: 'dictType',
    label: '字典类型',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'dictName',
    title: '字典名称',
  },
  {
    field: 'dictType',
    title: '字典类型',
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
    fieldName: 'dictName',
    label: '字典名称',
    rules: 'required',
  },
  {
    component: 'Input',
    fieldName: 'dictType',
    help: '使用英文/下划线命名, 如:sys_normal_disable',
    label: '字典类型',
    rules: {
      validator: (value) => {
        if (value === undefined || value === null || value === '') return true;
        return DICT_TYPE_RE.test(String(value)) || '字典类型只能使用英文/下划线命名';
      },
    },
  },
  {
    component: 'Textarea',
    fieldName: 'remark',
    label: '备注',
  },
];
