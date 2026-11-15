import type {
  FormItemRule,
  FormProps,
  FormSchema,
  FormValidateResult,
} from './types';

import { Store } from '@vben-core/shared/store';
import { cloneDeep } from '@vben-core/shared/utils';
import dayjs from 'dayjs';

interface FormState {
  errors: Record<string, string>;
  values: Record<string, any>;
}

function isEmptyValue(value: any): boolean {
  if (value === undefined || value === null || value === '') return true;
  if (Array.isArray(value)) return value.length === 0;
  return false;
}

function normalizeRules(
  rules: FormSchema['rules'],
): FormItemRule[] {
  if (!rules) return [];
  if (rules === 'required' || rules === 'selectRequired') {
    return [{ required: true }];
  }
  return Array.isArray(rules) ? rules : [rules];
}

function buildDefaults(schema: FormSchema[]): Record<string, any> {
  const values: Record<string, any> = {};
  for (const field of schema) {
    values[field.fieldName] =
      field.defaultValue === undefined ? undefined : cloneDeep(field.defaultValue);
  }
  return values;
}

/**
 * 表单命令式 API（对应 Vue 侧 form-api.ts 的 FormApi）。
 *
 * 值存放在 TanStack Store 中（与 Vue 侧一致），Form 渲染组件通过 useStore 订阅。
 * 保留 `getValues / setValues / setFieldValue / validate / resetForm` 的方法签名，
 * 使搬运过来的调用点无需改动。
 */
export class FormApi {
  /** 最新的表单配置（非响应式，渲染时读取；由 useVbenForm 每次渲染同步） */
  latestOptions: FormProps;

  private readonly store: Store<FormState>;

  constructor(options: FormProps) {
    this.latestOptions = options;
    this.store = new Store<FormState>({
      errors: {},
      values: buildDefaults(options.schema ?? []),
    });
  }

  /** 供 Form 渲染组件订阅 */
  getStore() {
    return this.store;
  }

  getState(): Record<string, any> {
    return this.store.state.values;
  }

  /** 同步动态配置（schema 变化时不重置已填值） */
  syncOptions(options: FormProps) {
    const prevSchema = this.latestOptions.schema;
    this.latestOptions = options;
    // schema 新增字段时补默认值
    if (options.schema !== prevSchema) {
      const values = this.store.state.values;
      let changed = false;
      const next = { ...values };
      for (const field of options.schema) {
        if (!(field.fieldName in next)) {
          next[field.fieldName] =
            field.defaultValue === undefined
              ? undefined
              : cloneDeep(field.defaultValue);
          changed = true;
        }
      }
      if (changed) {
        this.store.setState((prev) => ({ ...prev, values: next }));
      }
    }
  }

  async getValues(): Promise<Record<string, any>> {
    const { fieldMappingTime } = this.latestOptions;
    let values = cloneDeep(this.store.state.values);

    if (fieldMappingTime && fieldMappingTime.length > 0) {
      for (const [field, [startKey, endKey], [startFmt, endFmt]] of
        fieldMappingTime) {
        const range = values[field];
        if (Array.isArray(range) && range[0] && range[1]) {
          values[startKey] = dayjs(range[0]).format(startFmt);
          values[endKey] = dayjs(range[1]).format(endFmt);
        }
        delete values[field];
      }
    }

    // 移除值为 undefined 的键，避免污染查询参数
    for (const key of Object.keys(values)) {
      if (values[key] === undefined) {
        delete values[key];
      }
    }
    return values;
  }

  async setValues(values: Record<string, any>) {
    this.store.setState((prev) => ({
      ...prev,
      values: { ...prev.values, ...cloneDeep(values) },
    }));
  }

  async setFieldValue(field: string, value: any) {
    this.store.setState((prev) => ({
      ...prev,
      errors: { ...prev.errors, [field]: '' },
      values: { ...prev.values, [field]: value },
    }));
    this.latestOptions.handleValuesChange?.(this.store.state.values, {
      [field]: value,
    });
  }

  async resetForm() {
    this.store.setState((prev) => ({
      ...prev,
      errors: {},
      values: buildDefaults(this.latestOptions.schema ?? []),
    }));
  }

  setErrors(errors: Record<string, string>) {
    this.store.setState((prev) => ({ ...prev, errors }));
  }

  /** 供联动字段变化时触发重算（React 侧值变化即自动重渲染，这里保留空实现兼容调用点） */
  async validate(): Promise<FormValidateResult> {
    const values = this.store.state.values;
    const errors: Record<string, string> = {};

    for (const field of this.latestOptions.schema ?? []) {
      const deps = field.dependencies;
      if (deps?.if && !deps.if(values)) continue;

      const value = values[field.fieldName];
      const rules = normalizeRules(field.rules);
      const dynamicRequired = deps?.required ? deps.required(values) : undefined;

      for (const rule of rules) {
        const isRequired = rule.required || dynamicRequired === true;
        if (isRequired && isEmptyValue(value)) {
          errors[field.fieldName] =
            rule.message ?? `${field.label ?? field.fieldName}不能为空`;
          break;
        }
        if (
          rule.min !== undefined &&
          typeof value === 'string' &&
          value.length < rule.min
        ) {
          errors[field.fieldName] =
            rule.message ?? `${field.label ?? field.fieldName}长度不足`;
          break;
        }
        if (rule.validator) {
          const result = await rule.validator(value, values);
          if (result === false) {
            errors[field.fieldName] =
              rule.message ?? `${field.label ?? field.fieldName}校验失败`;
            break;
          }
          if (typeof result === 'string') {
            errors[field.fieldName] = result;
            break;
          }
        }
      }
    }

    this.setErrors(errors);
    return { errors, valid: Object.keys(errors).length === 0, values };
  }
}

export type { FormProps, FormSchema };
