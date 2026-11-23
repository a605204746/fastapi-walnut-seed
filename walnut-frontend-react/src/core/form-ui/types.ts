import type { ComponentType, ReactNode } from 'react';

/**
 * 表单字段组件类型：可以是注册表里的字符串名（'Input'/'Select'/...），
 * 也可以是直接的 React 组件（对应 Vue 侧 `markRaw(Component)`）。
 */
export type FormComponentType = ComponentType<any> | string;

/** 单条校验规则（PoC 用轻量结构替代 zod，语义与 'required'/'selectRequired' 对齐） */
export interface FormItemRule {
  message?: string;
  /** 数组/字符串最小长度 */
  min?: number;
  required?: boolean;
  validator?: (
    value: any,
    values: Record<string, any>,
  ) => boolean | string | Promise<boolean | string>;
}

/** 字段联动，对应 Vue 侧 form-render/dependencies.ts 的子集 */
export interface FormFieldDependencies {
  disabled?: (values: Record<string, any>) => boolean;
  /** 为 false 时字段不渲染、其值也不参与提交 */
  if?: (values: Record<string, any>) => boolean;
  required?: (values: Record<string, any>) => boolean;
  /**
   * 动态校验规则（对应 Vue 侧 `dependencies.rules`）。
   *
   * Vue 版返回 zod schema（`z.string().min(1, '请输入路由地址')`），React 版返回
   * `FormItemRule`；提供时**覆盖** `FormSchema.rules`。用于「校验规则随其它字段
   * 变化」的场景，例如菜单管理的路由地址：外链时必须 `http(s)://` 开头，
   * 非外链时必须不带前导 `/`。
   */
  rules?: (
    values: Record<string, any>,
  ) =>
    | 'required'
    | 'selectRequired'
    | FormItemRule
    | FormItemRule[]
    | null
    | undefined;
  /** 为 false 时字段隐藏但保留值 */
  show?: (values: Record<string, any>) => boolean;
  triggerFields: string[];
}

/**
 * `renderComponentContent` 的返回值：键是插槽名，值是节点或返回节点的函数。
 *
 * - `default`：**替换整个控件**（对应 Vue 的默认插槽，如角色页把 TreeSelect
 *   换成自绘的 `TreeSelectPanel`）
 * - 其它键（`addonBefore` / `addonAfter` / `prefix` / `suffix` …）：作为 prop
 *   合并给控件（对应 Vue 的具名插槽 → antd 的同名 prop）
 */
export type FormComponentContent = Record<
  string,
  ((attrs: Record<string, any>) => ReactNode) | ReactNode
>;

export interface FormSchema {
  /** 组件名（查注册表）或 React 组件 */
  component: FormComponentType;
  /** 组件 props，可为函数以支持动态 props */
  componentProps?:
    | ((values: Record<string, any>) => Record<string, any>)
    | Record<string, any>;
  /** 受控值绑定的 prop 名，默认 'value'（React 统一 value+onChange） */
  defaultValue?: any;
  dependencies?: FormFieldDependencies;
  fieldName: string;
  formItemClass?: string;
  /** label 旁的帮助提示（对应 Vue 侧 FormSchema.help，渲染为 Tooltip） */
  help?: string;
  hideLabel?: boolean;
  label?: string;
  labelWidth?: number;
  modelPropName?: string;
  /**
   * 渲染控件的附加内容 / 替换控件本体（对应 Vue 侧同名属性）。
   * 参数是当前表单值，返回插槽名 → 节点的映射，详见 `FormComponentContent`。
   */
  renderComponentContent?: (
    values: Record<string, any>,
  ) => FormComponentContent;
  rules?: 'required' | 'selectRequired' | FormItemRule | FormItemRule[];
}

export interface FormCommonConfig {
  componentProps?: Record<string, any>;
  /** 所有表单项共用的容器 class（字段级 `formItemClass` 优先级更高） */
  formItemClass?: string;
  hideLabel?: boolean;
  hideRequiredMark?: boolean;
  labelWidth?: number;
}

/** 日期区间字段映射：[字段名, [开始key, 结束key], [开始格式, 结束格式]] */
export type FieldMappingTime = [string, [string, string], [string, string]];

export interface FormProps {
  commonConfig?: FormCommonConfig;
  /** 紧凑模式（表单项 pb-2），查询表单用 */
  compact?: boolean;
  fieldMappingTime?: FieldMappingTime[];
  handleValuesChange?: (
    values: Record<string, any>,
    changed: Record<string, any>,
  ) => void;
  /**
   * 自定义「重置」行为（对应 Vue 侧 FormProps.handleReset）。
   *
   * 提供后**完全接管**重置：表格适配层不再执行默认的
   * `resetForm() + 重新查询`。用于「重置时还要清掉表单之外的联动状态」的场景，
   * 例如 `system/post`、`system/user` 页需要在重置时一并清空左侧部门树的选中。
   */
  handleReset?: () => Promise<void> | void;
  /**
   * 布局方向（对应 Vue 侧 FormProps.layout）。
   * `vertical` = label 在控件上方；默认 `horizontal` = label 在左、宽度取 labelWidth。
   */
  layout?: 'horizontal' | 'vertical';
  schema: FormSchema[];
  showDefaultActions?: boolean;
  wrapperClass?: string;
}

export interface FormValidateResult {
  errors: Record<string, string>;
  valid: boolean;
  values: Record<string, any>;
}
