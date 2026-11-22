import { cp, mkdir } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig } from 'tsdown';

const rootDir = dirname(fileURLToPath(import.meta.url));
const loadingAssets = ['default-loading-antd.html', 'default-loading.html'];

export default defineConfig({
  clean: true,
  deps: {
    neverBundle: ['@walnut/node-utils'],
    skipNodeModulesBundle: true,
  },
  // 不生成 d.ts：postcss 的类型在 rolldown dts 解析下会报 MISSING_EXPORT。
  // package.json 的 exports 把 types 指向 src/index.ts、default 指向 dist，
  // 类型与运行时分别解析，与 Vue 侧各包的做法一致。
  dts: false,
  entry: ['src/index.ts'],
  format: ['esm'],
  hooks: {
    'build:done': async (context) => {
      const outDir = context.options.outDir;
      if (!outDir) {
        return;
      }

      await mkdir(outDir, { recursive: true });

      for (const file of loadingAssets) {
        await cp(
          join(rootDir, 'src/plugins/inject-app-loading', file),
          join(outDir, file),
        );
      }
    },
  },
});
