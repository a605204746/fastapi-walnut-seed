import type { Plugin } from 'vite';

const REFERENCE_LINE = '@reference "@vben/tailwind-config/theme";\n';

/**
 * 给用到 @apply 的样式文件自动注入 @reference。
 *
 * Tailwind CSS v4 里，只有把 @theme 定义引入作用域后才能 @apply 自定义主题工具类
 * （如 bg-primary、text-foreground）。Vue 侧这个插件只处理 SFC 的 <style> 块；
 * React 侧没有 SFC，改为处理普通 .css / .scss 文件，作用等价：
 * 组件样式不必各自手写 @reference。
 *
 * 已经 `@import "tailwindcss"` 的入口文件本身就有 @theme 作用域，跳过以免重复定义。
 */
export function viteTailwindReferencePlugin(): Plugin {
  return {
    enforce: 'pre',
    name: 'vite:tailwind-reference',
    transform(code, id) {
      const normalizedId = id.replaceAll('\\', '/');
      if (!/\.(?:css|scss)$/.test(normalizedId)) {
        return null;
      }
      // 跳过 node_modules 里的第三方样式
      if (normalizedId.includes('/node_modules/')) {
        return null;
      }
      if (code.includes('@reference')) {
        return null;
      }
      if (/@import\s+['"]tailwindcss['"]/.test(code)) {
        return null;
      }
      // 只在实际用到 @apply 时注入
      if (!code.includes('@apply')) {
        return null;
      }
      return {
        code: REFERENCE_LINE + code,
        map: null,
      };
    },
  };
}
