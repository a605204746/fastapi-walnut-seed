import { createJSONStorage } from 'zustand/middleware';
import { initStoreStorage, getStoreStorage } from './storage';
import { useTimezoneStore } from './modules/timezone';
import { useAccessStore } from './modules/access';
import { useTabbarStore } from './modules/tabbar';
import { useUserStore } from './modules/user';
import { storeNamespace } from './namespace';

export interface InitStoreOptions {
  /** 应用名，用作持久化 key 前缀，防止多 app 缓存冲突 */
  namespace: string;
}

/**
 * @zh_CN 初始化 stores（对应 Vue 侧 packages/stores/src/setup.ts 的 initStores）。
 *
 * 开发环境使用 localStorage；生产环境与 Vue 一样使用 secure-ls AES + 压缩。
 * namespace 在任何持久化数据水合前设置，避免跨应用读取。
 */
export function initStores(options: InitStoreOptions) {
  storeNamespace.prefix = options.namespace;
  initStoreStorage(options.namespace);
  // access → localStorage、tabbar → sessionStorage，均与 Vue 侧一致（user 无 persist）
  useAccessStore.persist.setOptions({
    name: `${options.namespace}-core-access`, storage: createJSONStorage(getStoreStorage),
  });
  void useAccessStore.persist.rehydrate();
  useTabbarStore.persist.setOptions({
    name: `${options.namespace}-core-tabbar`, storage: createJSONStorage(() => sessionStorage),
  });
  void useTabbarStore.persist.rehydrate();
  useTimezoneStore.persist.setOptions({ name: `${options.namespace}-core-timezone`, storage: createJSONStorage(getStoreStorage) });
  void useTimezoneStore.persist.rehydrate();
  void useTimezoneStore.getState().initTimezone().catch(console.error);
}

/** 重置所有 store（登出时调用），对应 Vue 侧 resetAllStores */
export function resetAllStores() {
  useAccessStore.getState().$reset();
  useTabbarStore.getState().$reset();
  useUserStore.getState().$reset();
}
