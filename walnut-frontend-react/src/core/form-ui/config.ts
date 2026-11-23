import type { ComponentType } from 'react';

/**
 * 表单组件注册表（对应 Vue 侧 adapter/component 的 componentMap）。
 *
 * 应用层在 bootstrap 时调用 `setComponentMap({ Input, Select, ... })` 注册，
 * schema 里的字符串 `component: 'Input'` 据此解析为实际 React 组件。
 */
const componentMap = new Map<string, ComponentType<any>>();

export function setComponentMap(map: Record<string, ComponentType<any>>) {
  for (const [key, component] of Object.entries(map)) {
    componentMap.set(key, component);
  }
}

export function getComponent(name: string): ComponentType<any> | undefined {
  return componentMap.get(name);
}

export function hasComponent(name: string): boolean {
  return componentMap.has(name);
}

/**
 * 必填校验的文案解析器。
 *
 * Vue 基线在 `adapter/form.ts` 的表单 setup（{ defineRules }）里注册
 * `required` / `selectRequired` 两条规则，文案走 `$t('ui.formRules.required', [label])`
 * （词典里是 `请输入{0}` / `请选择{0}`）。`@core/*` 不能依赖 `@walnut/locales`，
 * 故同样采用注入式解析器（与 popup-ui 的 `setDrawerTextResolver` 一个套路），
 * app 在 bootstrap 时调 `initSetupWalnutForm()` 注入。
 *
 * 解析器在**校验时**调用，语言切换后自动生效。
 */
export type FormRuleMessageKey = 'required' | 'selectRequired';

let ruleMessageResolver = (
  key: FormRuleMessageKey,
  label: string,
): string =>
  key === 'required' ? `${label}不能为空` : `请选择${label}`;

export function setFormRuleMessages(
  resolver: (key: FormRuleMessageKey, label: string) => string,
) {
  ruleMessageResolver = resolver;
}

export function resolveRuleMessage(
  key: FormRuleMessageKey,
  label: string,
): string {
  return ruleMessageResolver(key, label);
}
