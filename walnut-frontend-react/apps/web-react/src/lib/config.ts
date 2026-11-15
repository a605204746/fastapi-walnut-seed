/**
 * 运行时配置读取。
 *
 * 对应 Vue 侧 packages/effects/hooks/src/use-app-config.ts。
 * 阶段 ① 只读 import.meta.env；生产环境的 window._VBEN_ADMIN_PRO_APP_CONF_
 * 注入由 internal/vite-config 的 extra-app-config 插件负责，留待阶段 ②。
 */
const env = import.meta.env;

function toBoolean(value: string | undefined): boolean {
  return value === 'true';
}

export const appConfig = {
  apiURL: env.VITE_GLOB_API_URL as string,
  clientId: env.VITE_GLOB_APP_CLIENT_ID as string,
  enableEncrypt: toBoolean(env.VITE_GLOB_ENABLE_ENCRYPT),
  namespace: env.VITE_APP_NAMESPACE as string,
  rsaPrivateKey: env.VITE_GLOB_RSA_PRIVATE_KEY as string,
  rsaPublicKey: env.VITE_GLOB_RSA_PUBLIC_KEY as string,
  sseEnable: toBoolean(env.VITE_GLOB_SSE_ENABLE),
  title: env.VITE_APP_TITLE as string,
  websocketEnable: toBoolean(env.VITE_GLOB_WEBSOCKET_ENABLE),
};

export type AppConfig = typeof appConfig;
