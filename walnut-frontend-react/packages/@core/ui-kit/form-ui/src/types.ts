import type { ComponentType } from 'react';

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
  /** 为 false 时字段隐藏但保留值 */
  show?: (values: Record<string, any>) => boolean;
  triggerFields: string[];
}

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
  hideLabel?: boolean;
  label?: string;
  labelWidth?: number;
  modelPropName?: string;
  rules?: 'required' | 'selectRequired' | FormItemRule | FormItemRule[];
}

export interface FormCommonConfig {
  componentProps?: Record<string, any>;
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
  schema: FormSchema[];
  showDefaultActions?: boolean;
  wrapperClass?: string;
}

export interface FormValidateResult {
  errors: Record<string, string>;
  valid: boolean;
  values: Record<string, any>;
}
