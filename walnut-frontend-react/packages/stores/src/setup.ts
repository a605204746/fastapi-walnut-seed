import { useAccessStore } from './modules/access';
import { useUserStore } from './modules/user';
import { storeNamespace } from './namespace';

export interface InitStoreOptions {
  /** 应用名，用作持久化 key 前缀，防止多 app 缓存冲突 */
  namespace: string;
}

/**
 * @zh_CN 初始化 stores（对应 Vue 侧 packages/stores/src/setup.ts 的 initStores）。
 *
 * PoC 阶段使用明文 localStorage 持久化；生产环境的 secure-ls（AES + 压缩）
 * storage adapter 在阶段 ③ 补齐（密钥 VITE_APP_STORE_SECURE_KEY 不变）。
 */
export function initStores(options: InitStoreOptions) {
  storeNamespace.prefix = options.namespace;
  // 仅 access store 持久化（与 Vue 侧一致，user store 无 persist）
  useAccessStore.persist.setOptions({
    name: `${options.namespace}-core-access`,
  });
  void useAccessStore.persist.rehydrate();
}

/** 重置所有 store（登出时调用），对应 Vue 侧 resetAllStores */
export function resetAllStores() {
  useAccessStore.getState().$reset();
  useUserStore.getState().$reset();
}
