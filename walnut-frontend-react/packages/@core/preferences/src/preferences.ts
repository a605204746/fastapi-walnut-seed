import type { DeepPartial } from '@vben-core/typings';

import type {
  CustomPreferencesField,
  CustomPreferencesRecord,
  InitialOptions,
  Preferences,
  PreferencesExtension,
} from './types';

import { MemoryStorageDriver, StorageManager } from '@vben-core/shared/cache';
import { isMacOs, merge } from '@vben-core/shared/utils';
import { createStore } from 'zustand/vanilla';

import { defaultPreferences } from './config';
import { updateCSSVariables } from './update-css-variables';

const STORAGE_KEYS = {
  CUSTOM: 'preferences-custom',
  MAIN: 'preferences',
  LOCALE: 'preferences-locale',
  THEME: 'preferences-theme',
} as const;

/** Tailwind 的 md 断点（48rem）。等价于 VueUse 的 breakpoints.smaller('md') */
const MOBILE_QUERY = '(max-width: 768px)';

interface PreferencesState {
  custom: CustomPreferencesRecord;
  preferences: Preferences;
}

/**
 * 用 zustand 的 vanilla store 承载偏好状态。
 *
 * 对应 Vue 版的 `reactive<Preferences>()`：vanilla store 与框架无关，
 * 非组件代码用 `preferencesStore.getState()` 读、`usePreferences()` 在组件里订阅。
 */
const preferencesStore = createStore<PreferencesState>()(() => ({
  custom: {},
  preferences: { ...defaultPreferences },
}));

function debounce<T extends (...args: any[]) => void>(fn: T, wait: number) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return (...args: Parameters<T>) => {
    if (timer) {
      clearTimeout(timer);
    }
    timer = setTimeout(() => fn(...args), wait);
  };
}

class PreferenceManager {
  private cache: StorageManager;
  private customPreferencesExtension: null | PreferencesExtension<any> = null;
  private debouncedSave: () => void;
  private initialCustomPreferences: CustomPreferencesRecord = {};
  private initialPreferences: Preferences = defaultPreferences;
  private isInitialized = false;

  constructor() {
    // 使用内存驱动占位，真正带 prefix 的 LocalStorage 实例在 initPreferences 中创建
    this.cache = new StorageManager({ driver: new MemoryStorageDriver() });
    this.debouncedSave = debounce(() => this.saveToCache(), 150);
    // store 是模块级单例（React 侧 usePreferences 要能拿到），
    // 但每个实例都要从干净的默认值开始，否则会跨实例串状态。
    // 深拷贝以免后续更新写穿到 defaultPreferences 上。
    preferencesStore.setState({
      custom: {},
      preferences: structuredClone(defaultPreferences),
    });
  }

  /**
   * 清除所有缓存的偏好设置
   */
  clearCache = async () => {
    await Promise.all(
      Object.values(STORAGE_KEYS).map((key) => this.cache.removeItem(key)),
    );
  };

  /**
   * 获取扩展偏好设置
   */
  getCustomPreferences = <
    TCustomPreferences extends object = CustomPreferencesRecord,
  >(): Readonly<TCustomPreferences> => {
    return preferencesStore.getState().custom as Readonly<TCustomPreferences>;
  };

  /**
   * 获取初始化扩展偏好设置
   */
  getInitialCustomPreferences = <
    TCustomPreferences extends object = CustomPreferencesRecord,
  >() => {
    return this.cloneValue(
      this.initialCustomPreferences,
    ) as Readonly<TCustomPreferences>;
  };

  /**
   * 获取初始化偏好设置
   */
  getInitialPreferences = () => {
    return this.initialPreferences;
  };

  /**
   * 获取当前偏好设置快照。
   *
   * ⚠️ 与 Vue 版不同：这里返回的是**普通对象快照**，不是响应式代理。
   * 组件内需要跟随变化重新渲染时请用 `usePreferences()`；
   * 命令式代码（请求拦截器等）每次调用本方法即可拿到最新值。
   */
  getPreferences = (): Readonly<Preferences> => {
    return preferencesStore.getState().preferences;
  };

  /**
   * 获取扩展偏好设置配置
   */
  getPreferencesExtension = <
    TCustomPreferences extends object = CustomPreferencesRecord,
  >() => {
    return this.customPreferencesExtension
      ? (this.cloneValue(this.customPreferencesExtension) as Readonly<
          PreferencesExtension<TCustomPreferences>
        >)
      : null;
  };

  /**
   * 初始化偏好设置
   * @param options - 初始化配置项
   * @param options.namespace - 命名空间，用于隔离不同应用的配置
   * @param options.overrides - 要覆盖的偏好设置
   */
  initPreferences = async <
    TCustomPreferences extends object = CustomPreferencesRecord,
  >({
    namespace,
    overrides,
    extension,
  }: InitialOptions<TCustomPreferences>) => {
    // 防止重复初始化
    if (this.isInitialized) {
      return;
    }

    // 使用命名空间初始化存储管理器
    this.cache = new StorageManager({ prefix: namespace });

    // 合并初始偏好设置：前面的对象优先，后面的对象仅补齐缺失字段
    this.initialPreferences = merge({}, overrides, defaultPreferences);
    this.customPreferencesExtension = extension ?? null;
    this.initialCustomPreferences = this.resolveCustomPreferencesDefaults(
      this.customPreferencesExtension,
    );

    // 加载缓存的偏好设置，并仅用缓存补齐初始化配置中未显式设置的字段
    const cachedPreferences = (await this.loadFromCache()) || {};
    const mergedPreference = merge(
      {},
      cachedPreferences, // 用户缓存的设置优先
      this.initialPreferences, // 初始设置仅补齐缺失字段
    );

    // 更新偏好设置
    this.updatePreferences(mergedPreference);

    const cachedCustom = (await this.loadCustomFromCache()) || {};
    this.replaceCustomPreferences(
      merge(
        {},
        this.sanitizeCustomPreferences(cachedCustom),
        this.initialCustomPreferences,
      ),
    );
    await this.saveToCache();

    // 设置监听器
    this.setupWatcher();

    // 初始化平台标识
    this.initPlatform();

    this.isInitialized = true;
  };

  /**
   * 重置偏好设置到初始状态
   */
  resetPreferences = async () => {
    // 将状态重置为初始偏好设置
    this.replacePreferences(this.initialPreferences);
    this.replaceCustomPreferences(this.initialCustomPreferences);

    // 保存偏好设置至缓存
    await this.saveToCache();

    // 直接触发 UI 更新
    this.handleUpdates(this.getState().preferences);
  };

  /**
   * 更新扩展偏好设置
   * @param updates - 要更新的扩展偏好设置
   */
  updateCustomPreferences = (updates: DeepPartial<object>) => {
    if (!this.customPreferencesExtension) {
      return;
    }

    const sanitizedUpdates = this.sanitizeCustomPreferences(
      updates as DeepPartial<CustomPreferencesRecord>,
    );

    if (Object.keys(sanitizedUpdates).length === 0) {
      return;
    }

    this.replaceCustomPreferences(
      merge({}, sanitizedUpdates, this.getState().custom),
    );
    this.debouncedSave();
  };

  /**
   * 更新偏好设置
   * @param updates - 要更新的偏好设置
   */
  updatePreferences = (updates: DeepPartial<Preferences>) => {
    // 深度合并更新内容和当前状态（updates 优先）
    const mergedState = merge({}, updates, this.getState().preferences);
    preferencesStore.setState({ preferences: mergedState });

    // 根据更新的值执行更新
    this.handleUpdates(updates);

    // 保存到缓存（fire-and-forget，通过 debounce 控制频率）
    this.debouncedSave();
  };

  private cloneValue<T>(value: T): T {
    if (Array.isArray(value)) {
      return value.map((item) => this.cloneValue(item)) as T;
    }

    if (value && typeof value === 'object') {
      return Object.fromEntries(
        Object.entries(value as Record<string, unknown>).map(
          ([key, nestedValue]) => [key, this.cloneValue(nestedValue)],
        ),
      ) as T;
    }

    return value;
  }

  private getState(): PreferencesState {
    return preferencesStore.getState();
  }

  /**
   * 处理更新
   * @param updates - 更新的偏好设置
   */
  private handleUpdates(updates: DeepPartial<Preferences>) {
    const { theme, app } = updates;

    if (
      theme &&
      (Object.keys(theme).length > 0 || Reflect.has(theme, 'fontSize'))
    ) {
      updateCSSVariables(this.getState().preferences);
    }

    if (
      app &&
      (Reflect.has(app, 'colorGrayMode') || Reflect.has(app, 'colorWeakMode'))
    ) {
      this.updateColorMode(this.getState().preferences);
    }
  }

  /**
   * 初始化平台标识
   */
  private initPlatform() {
    document.documentElement.dataset.platform = isMacOs() ? 'macOs' : 'window';
  }

  private isAlmostInteger(value: number, epsilon = Number.EPSILON * 10) {
    return Math.abs(value - Math.round(value)) < epsilon;
  }

  private isValidCustomPreferenceValue(
    field: CustomPreferencesField,
    value: unknown,
  ) {
    switch (field.component) {
      case 'number': {
        if (typeof value !== 'number' || !Number.isFinite(value)) {
          return false;
        }

        const max = this.resolveNumericConstraint(field.componentProps?.max);
        const min = this.resolveNumericConstraint(field.componentProps?.min);
        const step = this.resolveNumericConstraint(field.componentProps?.step);

        if (min !== undefined && value < min) {
          return false;
        }

        if (max !== undefined && value > max) {
          return false;
        }

        if (step !== undefined) {
          if (step <= 0) {
            return false;
          }

          const stepBase = min ?? 0;
          const stepCount = (value - stepBase) / step;

          if (!this.isAlmostInteger(stepCount)) {
            return false;
          }
        }

        return true;
      }
      case 'select': {
        return (
          typeof value === 'string' &&
          field.options.some((option) => option.value === value)
        );
      }
      case 'switch': {
        return typeof value === 'boolean';
      }
      default: {
        return typeof value === 'string';
      }
    }
  }

  /**
   * 从缓存加载扩展偏好设置
   * @returns 缓存的扩展偏好设置，如果不存在则返回 null
   */
  private async loadCustomFromCache(): Promise<CustomPreferencesRecord | null> {
    return this.cache.getItem<CustomPreferencesRecord>(STORAGE_KEYS.CUSTOM);
  }

  /**
   * 从缓存加载偏好设置
   * @returns 缓存的偏好设置，如果不存在则返回 null
   */
  private async loadFromCache(): Promise<null | Preferences> {
    return this.cache.getItem<Preferences>(STORAGE_KEYS.MAIN);
  }

  private replaceCustomPreferences(preferences: CustomPreferencesRecord) {
    preferencesStore.setState({ custom: { ...preferences } });
  }

  private replacePreferences(preferences: Preferences) {
    preferencesStore.setState({ preferences: this.cloneValue(preferences) });
  }

  private resolveCustomPreferencesDefaults(
    extension: null | PreferencesExtension<any>,
  ) {
    if (!extension) {
      return {};
    }

    const result: CustomPreferencesRecord = {};

    for (const field of extension.fields) {
      result[field.key] = field.defaultValue;
    }

    return result;
  }

  private resolveNumericConstraint(value: unknown) {
    return typeof value === 'number' && Number.isFinite(value)
      ? value
      : undefined;
  }

  private sanitizeCustomPreferences(
    updates: DeepPartial<CustomPreferencesRecord>,
  ) {
    if (!this.customPreferencesExtension) {
      return {};
    }

    const result: CustomPreferencesRecord = {};

    for (const field of this.customPreferencesExtension.fields) {
      const value = updates[field.key];

      if (
        value !== undefined &&
        this.isValidCustomPreferenceValue(field, value)
      ) {
        result[field.key] = value;
      }
    }

    return result;
  }

  /**
   * 保存偏好设置到缓存
   */
  private async saveToCache() {
    try {
      const { custom, preferences } = this.getState();
      await this.cache.setItem(STORAGE_KEYS.MAIN, preferences);
      await this.cache.setItem(STORAGE_KEYS.LOCALE, preferences.app.locale);
      await this.cache.setItem(STORAGE_KEYS.THEME, preferences.theme.mode);

      if (this.customPreferencesExtension) {
        await this.cache.setItem(STORAGE_KEYS.CUSTOM, { ...custom });
        return;
      }

      await this.cache.removeItem(STORAGE_KEYS.CUSTOM);
    } catch (error) {
      console.error('Failed to save preferences to cache:', error);
    }
  }

  /**
   * 监听状态和系统偏好设置的变化
   */
  private setupWatcher() {
    if (this.isInitialized) {
      return;
    }

    // 监听断点，判断是否移动端
    const mobileQuery = window.matchMedia(MOBILE_QUERY);
    const syncIsMobile = (matches: boolean) => {
      if (this.getState().preferences.app.isMobile !== matches) {
        this.updatePreferences({ app: { isMobile: matches } });
      }
    };
    syncIsMobile(mobileQuery.matches);
    mobileQuery.addEventListener('change', ({ matches }) =>
      syncIsMobile(matches),
    );

    // 监听系统主题偏好设置变化
    window
      .matchMedia('(prefers-color-scheme: dark)')
      .addEventListener('change', ({ matches: isDark }) => {
        // 仅在自动模式下跟随系统主题
        if (this.getState().preferences.theme.mode === 'auto') {
          // 先应用实际的主题
          this.updatePreferences({
            theme: { mode: isDark ? 'dark' : 'light' },
          });
          // 再恢复为 auto 模式，保持跟随系统的状态
          this.updatePreferences({
            theme: { mode: 'auto' },
          });
        }
      });
  }

  /**
   * 更新页面颜色模式（灰色、色弱）
   * @param preference - 偏好设置
   */
  private updateColorMode(preference: Preferences) {
    const { colorGrayMode, colorWeakMode } = preference.app;
    const dom = document.documentElement;

    dom.classList.toggle('invert-mode', colorWeakMode);
    dom.classList.toggle('grayscale-mode', colorGrayMode);
  }
}

const preferencesManager = new PreferenceManager();

export { PreferenceManager, preferencesManager, preferencesStore };
