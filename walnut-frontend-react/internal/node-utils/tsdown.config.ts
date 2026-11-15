import { defineConfig } from 'tsdown';

export default defineConfig({
  clean: true,
  deps: {
    skipNodeModulesBundle: true,
  },
  // 不生成 d.ts：tsdown 对本包的复杂再导出（execa / pkg-types 等）会产出
  // 语法有误的 index.d.ts。package.json 的 exports 把 types 指向 src/index.ts。
  dts: false,
  entry: ['src/index.ts'],
  format: ['esm'],
});
