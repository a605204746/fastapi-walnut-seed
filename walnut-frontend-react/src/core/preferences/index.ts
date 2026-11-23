import type {
  CustomPreferencesRecord,
  Preferences,
  PreferencesExtension,
} from './types';
import type { DeepPartial } from '../typings';

import { preferencesManager, preferencesStore } from './preferences';

export const {
  getPreferences,
  getCustomPreferences,
  getInitialCustomPreferences,
  getPreferencesExtension,
  updatePreferences,
  updateCustomPreferences,
  resetPreferences,
  clearCache,
  initPreferences,
} = preferencesManager;

/**
 * 实时偏好视图。
 *
 * Vue 版这里导出的是 `reactive` 代理，组件读取即自动订阅。React 没有这种隐式订阅，
 * 所以这个代理只保证「**读到的一定是最新值**」（供请求拦截器等命令式代码使用），
 * **不会触发重渲染** —— 组件里请用 `usePreferences()`。
 */
export const preferences: Preferences = new Proxy({} as Preferences, {
  get(_target, prop) {
    return Reflect.get(preferencesStore.getState().preferences, prop);
  },
  getOwnPropertyDescriptor(_target, prop) {
    return Reflect.getOwnPropertyDescriptor(
      preferencesStore.getState().preferences,
      prop,
    );
  },
  has(_target, prop) {
    return Reflect.has(preferencesStore.getState().preferences, prop);
  },
  ownKeys() {
    return Reflect.ownKeys(preferencesStore.getState().preferences);
  },
});

export { preferencesManager, preferencesStore };

export * from './constants';
export type * from './types';
export * from './use-preferences';

/**
 * 应用级默认偏好覆盖入口（原 @walnut/preferences 薄包装并入）：
 * 所有 app 共用的默认偏好在这里定义，而不是改核心默认值。
 */
function defineOverridesPreferences(preferences: DeepPartial<Preferences>) {
  return preferences;
}

function definePreferencesExtension<
  TCustomPreferences extends object = CustomPreferencesRecord,
>(extension: PreferencesExtension<TCustomPreferences>) {
  return extension;
}

export { defineOverridesPreferences, definePreferencesExtension };
