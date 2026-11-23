import type {
  FormItemRule,
  FormProps,
  FormSchema,
  FormValidateResult,
} from './types';

import { Store } from '@/core/shared/store';
import { cloneDeep, mergeWithArrayOverride } from '@/core/shared/utils';
import dayjs from 'dayjs';

import { resolveRuleMessage } from './config';

interface FormState {
  errors: Record<string, string>;
  /**
   * schema 版本号。`FormRenderer` 只订阅 store，而 schema 存放在
   * `latestOptions` 上（非响应式），因此 `updateSchema` 必须 bump 这个计数器
   * 才能触发重渲染。
   */
  revision: number;
  values: Record<string, any>;
}

function isEmptyValue(value: any): boolean {
  if (value === undefined || value === null || value === '') return true;
  if (Array.isArray(value)) return value.length === 0;
  return false;
}

export function normalizeRules(
  // 允许 null：`dependencies.rules` 可以返回 null 表示「本状态下不校验」
  // （如用户页的角色字段：新增可为空、编辑必填）
  rules: FormSchema['rules'] | null,
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
  /** 最新的表单配置（非响应式，渲染时读取；由 useWalnutForm 每次渲染同步） */
  latestOptions: FormProps;

  /** 调用方传入的原始 schema（未叠加 updateSchema 的覆盖层） */
  private baseSchema: FormSchema[];

  /** `updateSchema` 累积的覆盖层，按 fieldName 索引；每次渲染都会重新叠加到 baseSchema 上 */
  private readonly schemaOverrides = new Map<string, Partial<FormSchema>>();

  private readonly store: Store<FormState>;

  constructor(options: FormProps) {
    this.latestOptions = options;
    this.baseSchema = options.schema ?? [];
    this.store = new Store<FormState>({
      errors: {},
      revision: 0,
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

  /** 把 updateSchema 的覆盖层叠加到基础 schema 上 */
  private composeSchema(base: FormSchema[]): FormSchema[] {
    if (this.schemaOverrides.size === 0) return base;
    return base.map((field) => {
      const override = this.schemaOverrides.get(field.fieldName);
      // 与 Vue 侧一致：mergeWithArrayOverride(覆盖值, 原值)，前者胜出、数组整体替换
      return override
        ? (mergeWithArrayOverride(override, field) as FormSchema)
        : field;
    });
  }

  /** 同步动态配置（schema 变化时不重置已填值） */
  syncOptions(options: FormProps) {
    const prevBase = this.baseSchema;
    const nextBase = options.schema ?? [];
    this.baseSchema = nextBase;
    this.latestOptions = { ...options, schema: this.composeSchema(nextBase) };
    // schema 新增字段时补默认值
    if (nextBase !== prevBase) {
      const values = this.store.state.values;
      let changed = false;
      const next = { ...values };
      for (const field of this.composeSchema(nextBase)) {
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

  /**
   * 更新 schema（对应 Vue 侧 form-api.ts 的 `updateSchema`）。
   *
   * 业务页大量用它注入异步选项（部门树、字典、岗位下拉等）。语义与 Vue 侧逐条对齐：
   *
   * - 每一项都必须带非空 `fieldName`，否则**整批不生效**并报错（Vue 侧同样如此）
   * - 只更新已存在的字段，**不会新增字段**
   * - 深合并、数组整体覆盖（`mergeWithArrayOverride`）
   * - 多次调用**累积**生效
   *
   * React 侧差异：schema 不在 store 里，故通过 bump `revision` 触发 FormRenderer 重渲染。
   */
  async updateSchema(schema: Partial<FormSchema>[]) {
    const hasField = schema.every(
      (item) => 'fieldName' in item && !!item.fieldName,
    );
    if (!hasField) {
      console.error(
        'All items in the schema array must have a valid `fieldName` property to be updated',
      );
      return;
    }

    for (const item of schema) {
      const fieldName = item.fieldName as string;
      const prev = this.schemaOverrides.get(fieldName);
      this.schemaOverrides.set(
        fieldName,
        prev ? (mergeWithArrayOverride(item, prev) as Partial<FormSchema>) : item,
      );
    }

    this.latestOptions = {
      ...this.latestOptions,
      schema: this.composeSchema(this.baseSchema),
    };
    this.store.setState((prev) => ({ ...prev, revision: prev.revision + 1 }));
  }

  async getValues(): Promise<Record<string, any>> {
    const { fieldMappingTime } = this.latestOptions;
    const values = cloneDeep(this.store.state.values);

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
      // 对齐 Vue form-field.vue 的 fieldRules：show 隐藏的字段不参与校验。
      // 否则如菜单类型为目录时，隐藏的「组件路径」仍按必填校验，永远无法提交。
      if (deps?.show && !deps.show(values)) continue;

      const value = values[field.fieldName];
      // dependencies.rules 是动态规则（随其它字段变化），提供时覆盖 field.rules
      const effectiveRules = deps?.rules ? deps.rules(values) : field.rules;
      const rules = normalizeRules(effectiveRules);
      const dynamicRequired = deps?.required ? deps.required(values) : undefined;
      const label = field.label ?? field.fieldName;
      // 'required' → 请输入{label}；'selectRequired' → 请选择{label}
      // （对应 Vue 侧 adapter/form.ts 里 defineRules 的两条国际化规则）
      const presetMessage = resolveRuleMessage(
        effectiveRules === 'selectRequired' ? 'selectRequired' : 'required',
        label,
      );
      // selectRequired 把 false 也视为空（Vue 侧 `[false, null, undefined].includes(value)`）
      const isEmpty =
        isEmptyValue(value) ||
        (effectiveRules === 'selectRequired' && value === false);

      for (const rule of rules) {
        const isRequired = rule.required || dynamicRequired === true;
        if (isRequired && isEmpty) {
          errors[field.fieldName] = rule.message ?? presetMessage;
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
