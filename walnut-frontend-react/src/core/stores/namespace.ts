/**
 * 持久化 key 的命名空间前缀。
 *
 * 与 Vue 侧 `initStores(app, { namespace })` 的 `${namespace}-${storeId}` 方案一致。
 * 默认值取应用的 VITE_APP_NAMESPACE（walnut-seed-admin），
 * 调用 `initStores({ namespace })` 时会覆盖并触发 rehydrate。
 *
 * 单独成模块以打破 setup ↔ modules 的循环依赖。
 */
export const storeNamespace = {
  prefix: 'walnut-seed-admin',
};
