import { ApiSelect, ApiTreeSelect, ApiCascader } from '@/components/api-component';
import { FileUpload, ImageUpload } from '@/components/upload';
import { IconPicker } from '@/components/icon-picker';
const RichTextarea = lazy(() => import('@/components/tiptap').then(module => ({ default: module.Tiptap })));
import type { ComponentType } from 'react';

import { lazy, Suspense } from 'react';

import { setComponentMap } from '@/core/form-ui';

import { $t } from '@/locales';
import { SecretInput } from '@/components/global/secret-input';

/**
 * 表单组件注册表（对应 Vue 侧 `adapter/component/index.ts` 的 `componentMap`）。
 *
 * Vue 侧共 31 个组件名，本文件注册其中 **24 个**（antd 6 原生可直接映射的全部），
 * 外加 React 侧独有的 `SecretInput`（Vue 是用表单插槽注入的，见下）。
 * 富组件注册项覆盖异步选项、图标、上传和富文本（见
 * `docs/frontend/06-React迁移-总计划与进度.md`）：
 *
 * - `ApiSelect` / `ApiTreeSelect` / `ApiCascader` ← `common-ui/api-component`
 * - `IconPicker` ← `common-ui/icon-picker`
 * - `FileUpload` / `ImageUpload` ← `#/components/upload`
 * - `RichTextarea` ← `#/components/tinymce`
 *
 * React 侧统一 `value` + `onChange` 受控，无需 Vue 的 `modelPropName` 映射。
 *
 * `SecretInput`（客户端密钥输入框）是 React 侧**新增的注册项**：Vue 的
 * `client-drawer.vue` 通过 `<BasicForm><template #clientSecret>` 这种字段级插槽
 * 注入自定义控件，React 版 form-ui 没有插槽机制，改为注册成具名控件，
 * schema 里写 `component: 'SecretInput'` 即可，能力等价。
 */

/**
 * 已注册的组件名（与 Vue 侧 `StrictComponentType` 对齐，减去 A3 待做的 7 个）。
 * 供 schema 的 `component` 字段做类型收窄用。
 */
export type StrictComponentType =
  | 'ApiSelect' | 'ApiTreeSelect' | 'ApiCascader' | 'FileUpload' | 'ImageUpload' | 'IconPicker' | 'RichTextarea'
  | 'AutoComplete'
  | 'Cascader'
  | 'Checkbox'
  | 'CheckboxGroup'
  | 'DatePicker'
  | 'DefaultButton'
  | 'Divider'
  | 'Input'
  | 'InputNumber'
  | 'InputPassword'
  | 'Mentions'
  | 'PrimaryButton'
  | 'Radio'
  | 'RadioGroup'
  | 'RangePicker'
  | 'Rate'
  | 'SecretInput'
  | 'Select'
  | 'Space'
  | 'Switch'
  | 'Textarea'
  | 'TimePicker'
  | 'TimeRangePicker'
  | 'TreeSelect'
  | 'Upload';

/**
 * 懒加载一个 antd 组件（对应 Vue 侧的 `defineAsyncComponent(() => import('antdv-next/dist/xxx'))`）。
 *
 * ⚠️ **不要图省事改成从 `antd` 顶层静态 import。** `setupComponents()` 在 `main.tsx`
 * 的 bootstrap 里同步执行，静态 import 会把全部 24 个控件拖进首屏依赖图：实测
 * `antd-form` chunk 从 425.86 kB 涨到 560.24 kB（gzip 131.5 → 173.8 kB）。
 * Vue 侧正是因为这个才逐个 `defineAsyncComponent`。
 *
 * 走 `antd/es/*` 子路径（antd 未声明 `exports` 字段，子路径可直接解析），
 * 与 `internal/vite-config/src/config/code-split.ts` 的 `matchAntdFormChunk`
 * 前缀清单对齐，分包结果可控。
 */
function lazyAntd(
  loader: () => Promise<{ default: ComponentType<any> }>,
): ComponentType<any> {
  const LazyComponent = lazy(loader);

  function LazyAntd(props: Record<string, any>) {
    // fallback 用 null 而不是 Spin：表单控件尺寸小，闪一个 spinner 反而抖动；
    // 首帧空白与 Vue 的 defineAsyncComponent 默认行为一致。
    return (
      <Suspense fallback={null}>
        <LazyComponent {...props} />
      </Suspense>
    );
  }

  return LazyAntd;
}

const AutoComplete = lazyAntd(() => import('antd/es/auto-complete'));
const Button = lazyAntd(() => import('antd/es/button'));
const Cascader = lazyAntd(() => import('antd/es/cascader'));
const Checkbox = lazyAntd(() => import('antd/es/checkbox'));
const CheckboxGroup = lazyAntd(() => import('antd/es/checkbox/Group'));
const DatePicker = lazyAntd(() => import('antd/es/date-picker'));
const Divider = lazyAntd(() => import('antd/es/divider'));
const Input = lazyAntd(() => import('antd/es/input'));
const InputNumber = lazyAntd(() => import('antd/es/input-number'));
const InputPassword = lazyAntd(() => import('antd/es/input/Password'));
const Mentions = lazyAntd(() => import('antd/es/mentions'));
const Radio = lazyAntd(() => import('antd/es/radio'));
const RadioGroup = lazyAntd(() => import('antd/es/radio/group'));
const Rate = lazyAntd(() => import('antd/es/rate'));
const Select = lazyAntd(() => import('antd/es/select'));
const Space = lazyAntd(() => import('antd/es/space'));
const Switch = lazyAntd(() => import('antd/es/switch'));
const TextArea = lazyAntd(() => import('antd/es/input/TextArea'));
const TimePicker = lazyAntd(() => import('antd/es/time-picker'));
const TreeSelect = lazyAntd(() => import('antd/es/tree-select'));
const Upload = lazyAntd(() => import('antd/es/upload'));

// 区间选择器没有独立子路径模块，只能从各自的 index 上取 `.RangePicker`
const RangePicker = lazyAntd(() =>
  import('antd/es/date-picker').then((mod) => ({
    default: mod.default.RangePicker as ComponentType<any>,
  })),
);
const TimeRangePicker = lazyAntd(() =>
  import('antd/es/time-picker').then((mod) => ({
    default: mod.default.RangePicker as ComponentType<any>,
  })),
);

/**
 * 注入默认 placeholder（对应 Vue 侧的 `withDefaultPlaceholder`）。
 *
 * Vue 版用 `computed` 让 placeholder 跟随语言切换实时变化，并用注释说明这是为了
 * 规避 keepalive / 语言切换 / tab 切换下 placeholder 不更新的问题。React 侧同理：
 * `$t` 在**渲染期**调用，语言切换触发重渲染即自动生效，不能提到模块顶层求值。
 *
 * 调用方显式传入的 `placeholder` 优先（与 Vue 的 `props?.placeholder || attrs?.placeholder || $t(...)` 一致）。
 */
function withDefaultPlaceholder(
  Component: ComponentType<any>,
  type: 'input' | 'select',
  defaultProps: Record<string, any> = {},
): ComponentType<any> {
  function PlaceholderWrapper(props: Record<string, any>) {
    const { placeholder, ...rest } = props;
    return (
      <Component
        {...defaultProps}
        {...rest}
        placeholder={placeholder ?? $t(`ui.placeholder.${type}`)}
      />
    );
  }
  PlaceholderWrapper.displayName = `WithDefaultPlaceholder(${type})`;
  return PlaceholderWrapper;
}

/** 主按钮（对应 Vue 侧 `PrimaryButton`：`h(Button, {type:'primary'})`） */
function PrimaryButton(props: Record<string, any>) {
  return <Button type="primary" {...props} />;
}

/** 默认按钮（对应 Vue 侧 `DefaultButton`：`h(Button, {type:'default'})`） */
function DefaultButton(props: Record<string, any>) {
  return <Button type="default" {...props} />;
}

export function setupComponents() {
  setComponentMap({
    ApiSelect, ApiTreeSelect, ApiCascader, FileUpload, ImageUpload, IconPicker, RichTextarea: (props: any) => <Suspense fallback={null}><RichTextarea {...props} /></Suspense>,
    AutoComplete: withDefaultPlaceholder(AutoComplete, 'input'),
    Cascader: withDefaultPlaceholder(Cascader, 'select'),
    Checkbox,
    CheckboxGroup,
    DatePicker,
    DefaultButton,
    Divider,
    Input: withDefaultPlaceholder(Input, 'input'),
    // Vue 侧同样强制 width:100%，否则 InputNumber 默认宽度只有 90px
    InputNumber: withDefaultPlaceholder(InputNumber, 'input', {
      style: { width: '100%' },
    }),
    InputPassword: withDefaultPlaceholder(InputPassword, 'input'),
    Mentions: withDefaultPlaceholder(Mentions, 'input'),
    PrimaryButton,
    Radio,
    RadioGroup,
    RangePicker,
    Rate,
    SecretInput,
    Select: withDefaultPlaceholder(Select, 'select'),
    Space,
    Switch,
    Textarea: withDefaultPlaceholder(TextArea, 'input'),
    TimePicker,
    TimeRangePicker,
    TreeSelect: withDefaultPlaceholder(TreeSelect, 'select'),
    Upload,
  });
}

/** 已注册的组件名清单，供单测断言注册表完整性 */
export const REGISTERED_COMPONENT_NAMES: StrictComponentType[] = [
  'ApiSelect', 'ApiTreeSelect', 'ApiCascader', 'FileUpload', 'ImageUpload', 'IconPicker', 'RichTextarea',
  'AutoComplete',
  'Cascader',
  'Checkbox',
  'CheckboxGroup',
  'DatePicker',
  'DefaultButton',
  'Divider',
  'Input',
  'InputNumber',
  'InputPassword',
  'Mentions',
  'PrimaryButton',
  'Radio',
  'RadioGroup',
  'RangePicker',
  'Rate',
  'SecretInput',
  'Select',
  'Space',
  'Switch',
  'Textarea',
  'TimePicker',
  'TimeRangePicker',
  'TreeSelect',
  'Upload',
];
