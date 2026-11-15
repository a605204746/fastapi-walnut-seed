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
