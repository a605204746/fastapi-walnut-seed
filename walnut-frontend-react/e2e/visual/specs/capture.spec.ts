import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { test, type Page } from '@playwright/test';

import { installMockApi } from './mock-api';

const HERE = path.dirname(fileURLToPath(import.meta.url));

/** 当前截取的一端：vue(golden) 或 react */
const SIDE = (process.env.VISUAL_CAPTURE ?? 'react') as 'react' | 'vue';

const VUE_BASE = process.env.VUE_BASE ?? 'http://localhost:8010';
const REACT_BASE = process.env.REACT_BASE ?? 'http://localhost:8020';
const BASE = SIDE === 'vue' ? VUE_BASE : REACT_BASE;

const OUT_DIR = path.resolve(HERE, '..', 'artifacts', SIDE);

/** 登录凭据（与两端 seed 一致） */
const CREDENTIALS = { password: 'admin123', username: 'admin' };

interface Target {
  /** 截图文件名 */
  name: string;
  /** 路由路径 */
  path: string;
  /** 是否需要先登录 */
  auth?: boolean;
  /** 暗色主题 */
  dark?: boolean;
  /** 元素级截图选择器（双端共有的表格卡片容器）；缺省为整页截图 */
  selector?: string;
  /** 渲染完成判定选择器（等待其可见再截图，避免截到首屏 loading 闪屏） */
  waitSelector: string;
}

const TARGETS: Target[] = [
  { name: 'login-light', path: '/auth/login', waitSelector: 'button[aria-label="login"]' },
  {
    dark: true,
    name: 'login-dark',
    path: '/auth/login',
    waitSelector: 'button[aria-label="login"]',
  },
  {
    auth: true,
    name: 'config',
    path: '/system/config',
    selector: '.rounded-md.bg-card',
    waitSelector: '.rounded-md.bg-card',
  },
  {
    // 整页外壳（侧边栏 + 顶栏 + 内容区）。不用元素级截图：两端侧边栏没有共有的
    // class（Vue 是自绘，React 是 antd Sider），只有整页能双端对齐。
    auth: true,
    name: 'shell',
    path: '/system/config',
    waitSelector: '.rounded-md.bg-card',
  },
  {
    auth: true,
    dark: true,
    name: 'shell-dark',
    path: '/system/config',
    waitSelector: '.rounded-md.bg-card',
  },
];

async function settle(page: Page) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(() => document.fonts?.ready);
  await page.waitForTimeout(300);
}

async function switchToDark(page: Page) {
  // 两端主题切换按钮均带 .theme-toggle 类
  await page.locator('.theme-toggle').first().click();
  await page.waitForTimeout(300);
}

async function login(page: Page) {
  await page.goto(`${BASE}/auth/login?redirect=%2Fsystem%2Fconfig`);
  await page.waitForSelector('button[aria-label="login"]', { timeout: 30_000 });
  await page.locator('input[type="text"]').first().fill(CREDENTIALS.username);
  await page.locator('input[type="password"]').first().fill(CREDENTIALS.password);
  await page.locator('button[aria-label="login"]').first().click();
  await page.waitForURL('**/system/config');
}

/** 冻结动画/过渡的 CSS，保证截图逐帧可复现（在每次导航前注入） */
const FREEZE_CSS =
  '*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}.ant-message,.ant-notification,#nprogress,.walnut-progress-bar{display:none!important}';

test.describe(`capture:${SIDE}`, () => {
  test.beforeEach(async ({ page }) => {
    // 诊断：把页面错误/控制台错误打到测试输出，便于定位渲染失败
    page.on('pageerror', (error) => {
      // eslint-disable-next-line no-console
      console.log(`[pageerror] ${error.message}`);
    });
    page.on('console', (message) => {
      if (message.type() === 'error') {
        // eslint-disable-next-line no-console
        console.log(`[console.error] ${message.text()}`);
      }
    });
    // 接口 mock：门禁不依赖真实后端，双端渲染相同 fixture（Spec §6 确定性处理）
    await installMockApi(page);
    await page.addInitScript((css: string) => {
      const inject = () => {
        const el = document.createElement('style');
        el.textContent = css;
        (document.head || document.documentElement).appendChild(el);
      };
      if (document.head || document.documentElement) inject();
      else document.addEventListener('DOMContentLoaded', inject);
    }, FREEZE_CSS);
  });

  test.beforeAll(() => {
    fs.mkdirSync(OUT_DIR, { recursive: true });
  });

  for (const target of TARGETS) {
    test(`${target.name}`, async ({ page }) => {
      if (target.auth) {
        await login(page);
        await page.goto(`${BASE}${target.path}`);
      } else {
        await page.goto(`${BASE}${target.path}`);
      }
      await settle(page);
      // 等待真实渲染完成（而非首屏 loading 闪屏）
      await page.waitForSelector(target.waitSelector, {
        state: 'visible',
        timeout: 30_000,
      });

      if (target.dark) {
        await switchToDark(page);
        await settle(page);
      }

      const file = path.join(OUT_DIR, `${target.name}.png`);
      const selector = target.selector;
      if (selector) {
        const el = page.locator(selector).first();
        await el.waitFor({ state: 'visible', timeout: 15_000 });
        await el.screenshot({ path: file });
      } else {
        await page.screenshot({ fullPage: false, path: file });
      }
      // eslint-disable-next-line no-console
      console.log(`[capture:${SIDE}] ${target.name} -> ${file}`);
    });
  }
});
