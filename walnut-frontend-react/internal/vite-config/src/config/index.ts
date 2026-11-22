import type { DefineConfig, WalnutViteConfig } from '../typing';

import { defineApplicationConfig } from './application';

export * from './application';

/**
 * 定义 Vite 配置。
 *
 * React 版 monorepo 里所有包都是「源码直出」（exports 指向 src/*.ts，不跑 tsdown），
 * 因此不存在 library 构建形态，这里只保留 application 一种。
 */
function defineConfig(userConfigPromise?: DefineConfig): WalnutViteConfig {
  return defineApplicationConfig(userConfigPromise);
}

export { defineConfig };
