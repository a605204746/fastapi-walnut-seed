/**
 * 运行时配置读取。
 *
 * 对应 Vue 侧 packages/effects/hooks/src/use-app-config.ts。
 * 开发读取 import.meta.env；生产优先读取 extra-app-config 插件注入的运行时配置。
 */
const runtime = typeof window === 'undefined' ? {} : (window as unknown as { _WALNUT_ADMIN_PRO_APP_CONF_?: Record<string, string> })._WALNUT_ADMIN_PRO_APP_CONF_ ?? {};
const env = { ...import.meta.env, ...(import.meta.env.PROD ? runtime : {}) };

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
