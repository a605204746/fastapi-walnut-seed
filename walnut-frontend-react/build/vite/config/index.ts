import type { DefineConfig, WalnutViteConfig } from '../typing';

import { defineApplicationConfig } from './application';

export * from './application';

/**
 * 定义 Vite 配置。
 *
 * 单包形态：源码直出（无 library 构建），只保留 application 一种。
 */
function defineConfig(userConfigPromise?: DefineConfig): WalnutViteConfig {
  return defineApplicationConfig(userConfigPromise);
}

export { defineConfig };
