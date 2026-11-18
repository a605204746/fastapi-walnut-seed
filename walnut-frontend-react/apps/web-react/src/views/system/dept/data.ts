import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@vben/constants';
import { getPopupContainer } from '@vben/utils';

import { getDictOptions } from '@/utils/dict';
import { renderDict } from '@/utils/render';

/**
 * 部门管理的查询表单 / 表格列 / 抽屉表单 schema
 * （对应 Vue 侧 views/system/dept/data.ts）。
 *
 * ⚠️ 与 Vue 版的差异：Vue 用 zod（`z.string().regex(...)`）写手机号/邮箱校验，
 * React 侧 form-ui 用的是轻量 `FormItemRule`（见 `form-ui/src/types.ts`），
 * 改为等价的 `validator` 实现。语义保持一致：**留空放行，非空才校验格式**。
 */

const PHONE_RE = /^1[3,4578]\d{9}$/;
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'deptName',
    label: '部门名称',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_NORMAL_DISABLE),
    },
    fieldName: 'status',
    label: '部门状态',
  },
];

export const columns: GridColumn[] = [
  {
    field: 'deptName',
    title: '部门名称',
    treeNode: true,
  },
  {
    field: 'deptCategory',
    title: '类别编码',
  },
  {
    field: 'orderNum',
    title: '排序',
  },
  {
    field: 'status',
    slots: {
      default: ({ row }) =>
        renderDict(row.status, DictEnum.SYS_NORMAL_DISABLE),
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
  },
  {
    component: 'TreeSelect',
    componentProps: {
      getPopupContainer,
    },
    // 顶级部门（parentId === 0）不显示「上级部门」
    dependencies: {
      show: (model) => model.parentId !== 0,
      triggerFields: ['parentId'],
    },
    fieldName: 'parentId',
    label: '上级部门',
    rules: 'selectRequired',
  },
  {
    component: 'Input',
    fieldName: 'deptName',
    label: '部门名称',
    rules: 'required',
  },
  {
    component: 'InputNumber',
    componentProps: {
      className: 'flex-1',
    },
    defaultValue: 1,
    fieldName: 'orderNum',
    label: '显示排序',
    rules: 'required',
  },
  {
    component: 'Input',
    fieldName: 'deptCategory',
    label: '类别编码',
  },
  {
    component: 'Select',
    componentProps: {
      // 选中了就只能修改，不能重置为无负责人
      allowClear: false,
      getPopupContainer,
    },
    fieldName: 'leader',
    label: '负责人',
  },
  {
    component: 'Input',
    fieldName: 'phone',
    label: '联系电话',
    rules: {
      validator: (value) => {
        if (value === undefined || value === null || value === '') return true;
        return PHONE_RE.test(String(value)) || '请输入正确的手机号';
      },
    },
  },
  {
    component: 'Input',
    fieldName: 'email',
    label: '邮箱',
    rules: {
      validator: (value) => {
        if (value === undefined || value === null || value === '') return true;
        return EMAIL_RE.test(String(value)) || '请输入正确的邮箱';
      },
    },
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
