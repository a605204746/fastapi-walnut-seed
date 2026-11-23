import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@/core/constants';
import { getPopupContainer } from '@/core/utils';

import { getDictOptions } from '@/utils/dict';

/**
 * 用户管理的查询表单 / 表格列 / 抽屉表单 schema
 * （对应 Vue 侧 views/system/user/data.tsx，214 行）。
 *
 * 与 Vue 版的差异：手机号 / 邮箱的 zod 校验改为等价 `validator`
 * （留空放行，非空才校验格式，与 `.optional().or(z.literal(''))` 语义一致）；
 * `roleIds` 的动态 rules 返回 `null` 表示「新增时不必填」。
 */

const PHONE_RE = /^1[3-9]\d{9}$/;
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'userName',
    label: '用户账号',
  },
  {
    component: 'Input',
    fieldName: 'nickName',
    label: '用户昵称',
  },
  {
    component: 'Input',
    fieldName: 'phonenumber',
    label: '手机号码',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_NORMAL_DISABLE),
    },
    fieldName: 'status',
    label: '用户状态',
  },
  {
    component: 'RangePicker',
    fieldName: 'createTime',
    label: '创建时间',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'userName',
    minWidth: 80,
    title: '名称',
  },
  {
    field: 'nickName',
    minWidth: 130,
    title: '昵称',
  },
  {
    field: 'avatar',
    minWidth: 80,
    slots: { default: 'avatar' },
    title: '头像',
  },
  {
    field: 'deptName',
    minWidth: 120,
    title: '部门',
  },
  {
    field: 'phonenumber',
    formatter: ({ cellValue }) => cellValue || '暂无',
    minWidth: 120,
    title: '手机号',
  },
  {
    field: 'status',
    minWidth: 100,
    slots: { default: 'status' },
    title: '状态',
  },
  {
    field: 'createTime',
    minWidth: 150,
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
    component: 'Input',
    fieldName: 'userName',
    label: '用户账号',
    rules: 'required',
  },
  {
    component: 'InputPassword',
    fieldName: 'password',
    label: '用户密码',
    rules: 'required',
  },
  {
    component: 'Input',
    fieldName: 'nickName',
    label: '用户昵称',
    rules: 'required',
  },
  {
    component: 'TreeSelect',
    // 在 drawer 里更新，这里不需要默认的 componentProps
    defaultValue: undefined,
    fieldName: 'deptId',
    label: '所属部门',
    rules: 'selectRequired',
  },
  {
    component: 'Input',
    defaultValue: undefined,
    fieldName: 'phonenumber',
    label: '手机号码',
    rules: {
      validator: (value) => {
        if (value === undefined || value === null || value === '') return true;
        return PHONE_RE.test(String(value)) || '请输入正确的手机号码';
      },
    },
  },
  {
    component: 'Input',
    defaultValue: undefined,
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
      options: getDictOptions(DictEnum.SYS_USER_SEX),
    },
    defaultValue: '0',
    fieldName: 'sex',
    formItemClass: 'col-span-2 lg:col-span-1',
    label: '性别',
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
    formItemClass: 'col-span-2 lg:col-span-1',
    label: '状态',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      mode: 'multiple',
      optionFilterProp: 'label',
      optionLabelProp: 'label',
      placeholder: '请先选择部门',
    },
    fieldName: 'postIds',
    help: '选择部门后, 将自动加载该部门下所有的岗位',
    label: '岗位',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      mode: 'multiple',
      optionFilterProp: 'title',
      optionLabelProp: 'title',
    },
    dependencies: {
      // 后端逻辑为新增可以为空，编辑时不能为空
      rules: (model) => (model.id ? 'selectRequired' : null),
      triggerFields: ['id'],
    },
    fieldName: 'roleIds',
    label: '角色',
  },
  {
    component: 'Textarea',
    fieldName: 'remark',
    formItemClass: 'items-start',
    label: '备注',
  },
];
