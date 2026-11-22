import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { WalnutIcon } from '@walnut-core/shadcn-ui';
import { $t } from '@walnut/locales';
import { DictEnum } from '@walnut/constants';
import { getPopupContainer } from '@walnut/utils';
import { Input } from 'antd';

import { getDictOptions } from '@/utils/dict';
import { renderDict } from '@/utils/render';

/**
 * 菜单管理的查询表单 / 表格列 / 抽屉表单 schema
 * （对应 Vue 侧 views/system/menu/data.tsx，400 行）。
 *
 * 与 Vue 版的三处差异：
 * 1. **图标类名 → Iconify 名**。Vue 用 UnoCSS 的 `Iconify(material-symbols--menu)`
 *    类名（`<span class="...">`），React 侧两套 tailwind 配置都没装 iconify class
 *    插件，照抄会渲染成空 span，统一改为 `<WalnutIcon icon="material-symbols:menu" />`。
 * 2. **zod → validator**。`path` / `component` 两个字段的校验规则随 `isFrame`
 *    和其它字段动态变化，Vue 用 `dependencies.rules` 返回 zod schema，
 *    React 侧返回等价的 `FormItemRule[]`。
 * 3. `dependencies.componentProps` 里那段「切换菜单类型时清空所有校验错误」的
 *    hack **不迁移**：Vue 注释自己写了「直接抄的源码 没有清空校验的方法」，
 *    而 React 侧 `FormApi.setFieldValue` 本来就会清掉该字段的 error，无需绕过。
 */

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'menuName',
    label: '菜单名称 ',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_NORMAL_DISABLE),
    },
    fieldName: 'status',
    label: '菜单状态 ',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_SHOW_HIDE),
    },
    fieldName: 'visible',
    label: '显示状态',
  },
];

// 菜单类型（M目录 C菜单 F按钮）
export const menuTypeOptions = [
  { label: '目录', value: 'M' },
  { label: '菜单', value: 'C' },
  { label: '按钮', value: 'F' },
];

export const yesNoOptions = [
  { label: '是', value: '0' },
  { label: '否', value: '1' },
];

// （M目录 C菜单 F按钮）
export const menuTypes: Record<
  string,
  { icon: string; value: string } | undefined
> = {
  C: { icon: 'material-symbols:menu', value: '菜单' },
  F: { icon: 'mdi:button-pointer', value: '按钮' },
  M: { icon: 'flat-color-icons:folder', value: '目录' },
};

export const columns: GridColumn[] = [
  {
    align: 'left',
    field: 'menuName',
    slots: {
      // 需要 i18n 支持，否则返回原始值
      default: ({ row }) => $t(row.menuName),
    },
    title: '菜单名称',
    treeNode: true,
    width: 200,
  },
  {
    field: 'icon',
    slots: {
      default: ({ row }) => {
        if (row?.icon === '#') {
          return '';
        }
        return (
          <span className="flex justify-center">
            <WalnutIcon icon={row.icon} />
          </span>
        );
      },
    },
    title: '图标',
    width: 80,
  },
  {
    field: 'orderNum',
    title: '排序',
    width: 120,
  },
  {
    field: 'menuType',
    slots: {
      default: ({ row }) => {
        const current = menuTypes[row.menuType as string];
        if (!current) {
          return '未知';
        }
        return (
          <span className="flex items-center justify-center gap-1">
            <WalnutIcon icon={current.icon} />
            <span>{current.value}</span>
          </span>
        );
      },
    },
    title: '组件类型',
    width: 150,
  },
  {
    field: 'perms',
    title: '权限标识',
  },
  {
    field: 'component',
    title: '组件路径',
  },
  {
    field: 'status',
    slots: {
      default: ({ row }) =>
        renderDict(row.status, DictEnum.SYS_NORMAL_DISABLE),
    },
    title: '状态',
    width: 100,
  },
  {
    field: 'visible',
    slots: {
      default: ({ row }) => renderDict(row.visible, DictEnum.SYS_SHOW_HIDE),
    },
    title: '显示',
    width: 100,
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

const URL_RE = /^https?:\/\//;

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
    defaultValue: 0,
    fieldName: 'parentId',
    label: '上级菜单',
    rules: 'selectRequired',
  },
  {
    component: 'RadioGroup',
    componentProps: {
      buttonStyle: 'solid',
      optionType: 'button',
      options: menuTypeOptions,
    },
    defaultValue: 'M',
    fieldName: 'menuType',
    label: '菜单类型',
  },
  { component: 'IconPicker', fieldName: 'icon', label: '菜单图标', dependencies: { show: values => values.menuType !== 'F', triggerFields: ['menuType'] } },
  {
    component: 'Input',
    fieldName: 'menuName',
    help: '支持i18n写法, 如: menu.system.user',
    label: '菜单名称',
    rules: 'required',
  },
  {
    component: 'InputNumber',
    defaultValue: 1,
    fieldName: 'orderNum',
    help: '排序, 数字越小越靠前',
    label: '显示排序',
    rules: 'required',
  },
  {
    component: 'Input',
    componentProps: (model) => ({
      placeholder:
        model.isFrame === '0'
          ? '填写链接地址http(s)://  使用新页面打开'
          : '填写`路由地址`或者`链接地址`  链接默认使用内部iframe内嵌打开',
    }),
    dependencies: {
      rules: (model) => {
        // 外链（isFrame === '0'）：必须是 http(s):// 开头
        if (model.isFrame === '0') {
          return [
            { message: '请输入链接地址', required: true },
            {
              message: '请输入正确的链接地址',
              validator: (value: any) =>
                !value || URL_RE.test(String(value)) || '请输入正确的链接地址',
            },
          ];
        }
        // 路由地址：必填且不带前导 /
        return [
          { message: '请输入路由地址', required: true },
          {
            message: '路由地址不需要带/',
            validator: (value: any) =>
              !value ||
              !String(value).startsWith('/') ||
              '路由地址不需要带/',
          },
        ];
      },
      // 类型不为按钮时显示
      show: (values) => values?.menuType !== 'F',
      triggerFields: ['isFrame', 'menuType'],
    },
    fieldName: 'path',
    help: `路由地址不带/, 如: menu, user\n 链接为http(s)://开头\n 链接默认使用内部iframe打开, 可通过{是否外链}控制打开方式`,
    label: '路由地址',
  },
  {
    component: 'Input',
    componentProps: (model) => ({
      // 为链接时组件 disabled
      disabled: model.isFrame === '0',
    }),
    defaultValue: '',
    dependencies: {
      rules: (model) => {
        // 非链接时为必填项
        if (model.path && !URL_RE.test(String(model.path))) {
          return [
            { message: '非链接时必填组件路径', required: true },
            {
              message: '组件路径开头/末尾不需要带/',
              validator: (value: any) => {
                if (!value) return true;
                const text = String(value);
                return (
                  (!text.startsWith('/') && !text.endsWith('/')) ||
                  '组件路径开头/末尾不需要带/'
                );
              },
            },
          ];
        }
        // 为链接时非必填
        return [];
      },
      // 类型为菜单时显示
      show: (values) => values.menuType === 'C',
      triggerFields: ['menuType', 'path'],
    },
    fieldName: 'component',
    help: '填写./src/views下的组件路径, 如system/menu/index',
    label: '组件路径',
  },
  {
    component: 'RadioGroup',
    componentProps: {
      buttonStyle: 'solid',
      optionType: 'button',
      options: yesNoOptions,
    },
    defaultValue: '1',
    dependencies: {
      // 类型不为按钮时显示
      show: (values) => values.menuType !== 'F',
      triggerFields: ['menuType'],
    },
    fieldName: 'isFrame',
    help: '外链为http(s)://开头\n 选择否时, 使用iframe从内部打开页面, 否则新窗口打开',
    label: '是否外链',
  },
  {
    component: 'RadioGroup',
    componentProps: {
      buttonStyle: 'solid',
      optionType: 'button',
      options: getDictOptions(DictEnum.SYS_SHOW_HIDE),
    },
    defaultValue: '0',
    dependencies: {
      // 类型不为按钮时显示
      show: (values) => values.menuType !== 'F',
      triggerFields: ['menuType'],
    },
    fieldName: 'visible',
    help: '隐藏后不会出现在菜单栏, 但仍然可以访问',
    label: '是否显示',
  },
  {
    component: 'RadioGroup',
    componentProps: {
      buttonStyle: 'solid',
      optionType: 'button',
      options: getDictOptions(DictEnum.SYS_NORMAL_DISABLE),
    },
    defaultValue: '0',
    dependencies: {
      // 类型不为按钮时显示
      show: (values) => values.menuType !== 'F',
      triggerFields: ['menuType'],
    },
    fieldName: 'status',
    help: '停用后不会出现在菜单栏, 也无法访问',
    label: '菜单状态',
  },
  {
    component: 'Input',
    dependencies: {
      // 类型为菜单/按钮时显示
      show: (values) => values.menuType !== 'M',
      triggerFields: ['menuType'],
    },
    fieldName: 'perms',
    help: `控制器中定义的权限字符\n 如: @SaCheckPermission("system:user:import")`,
    label: '权限标识',
  },
  {
    component: 'Input',
    componentProps: (model) => ({
      // 为链接时组件 disabled
      disabled: model.isFrame === '0',
      placeholder: '必须为json字符串格式',
    }),
    dependencies: {
      // 类型为菜单时显示
      show: (values) => values.menuType === 'C',
      triggerFields: ['menuType'],
    },
    fieldName: 'queryParam',
    help: 'vue-router中的query属性\n 如{"name": "xxx", "age": 16}',
    label: '路由参数',
  },
  {
    component: 'RadioGroup',
    componentProps: {
      buttonStyle: 'solid',
      optionType: 'button',
      options: yesNoOptions,
    },
    defaultValue: '0',
    dependencies: {
      // 类型为菜单时显示
      show: (values) => values.menuType === 'C',
      triggerFields: ['menuType'],
    },
    fieldName: 'isCache',
    help: '路由的keepAlive属性',
    label: '是否缓存',
  },
  {
    component: 'Input',
    fieldName: 'remark',
    formItemClass: 'items-baseline',
    label: '拓展字段',
    // Vue 侧这段说明文字写在 menu-drawer.vue 的 `<template #remark>` 插槽里，
    // React 侧 form-ui 没有字段级插槽的模板写法，改为 schema 内的
    // renderComponentContent.default（替换整个控件）
    renderComponentContent: () => ({
      default: (attrs) => (
        <div className="flex flex-col gap-2">
          <Input {...attrs} />
          <span className="text-[14px] leading-[1.5] text-black/45">
            在ele作为activePath使用 但是非json格式 v5无法使用
            建议自行在apps/web-react/src/router/access.ts更改
          </span>
        </div>
      ),
    }),
  },
];
