import { defineConfig, devices } from '@playwright/test';

/**
 * Vue ↔ React 双端像素回归门禁配置。
 *
 * 确定性保障：
 * - 固定视口 1440×900 + deviceScaleFactor 1（消除跨分辨率/缩放差异）
 * - reducedMotion: reduce + addInitScript 冻结所有 animation/transition
 *   （登录页 WalnutSlogan / animate-float / 主题切换过渡等均为动画，不冻结会假差异）
 * - 同机同 Chromium 截图，消除跨平台字体渲染差
 */
export default defineConfig({
  outputDir: './artifacts/test-results',
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: 'report' }],
  ],
  testDir: './specs',
  timeout: 60_000,
  use: {
    deviceScaleFactor: 1,
    reducedMotion: 'reduce',
    viewport: { height: 900, width: 1440 },
  },
});
