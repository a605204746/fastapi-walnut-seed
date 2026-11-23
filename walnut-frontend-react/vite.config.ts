import { fileURLToPath } from 'node:url';

import { defineConfig } from './build/vite';

export default defineConfig(async () => {
  // 后端服务地址，与 Vue 基线 walnut-frontend/apps/web-antd/vite.config.ts 保持一致
  const backend = 'http://localhost:8011';

  return {
    application: {},
    vite: {
      resolve: {
        alias: {
          '@': fileURLToPath(new URL('./src', import.meta.url)),
        },
      },
      server: {
        proxy: {
          '/api': {
            changeOrigin: true,
            rewrite: (path) => path.replace(/^\/api/, ''),
            target: backend,
            ws: true,
          },
          // 上传文件访问：后端返回中性路径 /upload/{key}
          '/upload': {
            changeOrigin: true,
            target: backend,
          },
        },
      },
    },
  };
});