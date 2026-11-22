import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { expect, test, type Page } from '@playwright/test';

import { installMockApi } from './mock-api';

test.use({ viewport: { width: 1440, height: 900 } });

const BASE = process.env.REACT_BASE ?? 'http://localhost:8020';
// These checks run against Vite, just like capture.spec.ts. Resolve from this file
// so Windows and POSIX checkouts use the same preferences module as the app.
const preferencesUrl = '/@fs/' + path.resolve(path.dirname(fileURLToPath(import.meta.url)),
  '../../../packages/preferences/src/index.ts').replaceAll('\\', '/');

async function preferences(page: Page, value: Record<string, unknown>) {
  await page.evaluate(async ({ url, value }) => {
    const module = await import(/* @vite-ignore */ url);
    module.updatePreferences(value);
  }, { url: preferencesUrl, value });
}

async function expectContentFits(page: Page) {
  const main = page.locator('main');
  const card = page.locator('.rounded-md.bg-card').first();
  await expect(card).toBeVisible();
  await expect.poll(async () => {
    const bounds = await card.boundingBox();
    return bounds ? bounds.height : 0;
  }).toBeGreaterThan(400);
  const bounds = (await card.boundingBox())!;
  const mainBounds = (await main.boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(mainBounds.x);
  expect(bounds.y).toBeGreaterThanOrEqual(mainBounds.y);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(901);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(1441);
}

test.beforeEach(async ({ page }) => {
  await installMockApi(page);
  await page.goto(`${BASE}/auth/login?redirect=%2Fsystem%2Fconfig`);
  await page.locator('input[type="text"]').first().fill('admin');
  await page.locator('input[type="password"]').first().fill('admin123');
  await page.getByRole('button', { name: 'login', exact: true }).click();
  await page.waitForURL('**/system/config');
  await expectContentFits(page);
  await preferences(page, { transition: { enable: false } });
});

test('all seven layouts keep content visible after switching and reloading', async ({ page }, testInfo) => {
  test.setTimeout(180_000);
  for (const layout of ['header-nav', 'mixed-nav', 'header-mixed-nav',
    'sidebar-mixed-nav', 'header-sidebar-nav', 'full-content', 'sidebar-nav']) {
    await preferences(page, { app: { layout } });
    await expectContentFits(page);
    if (layout.includes('mixed-nav')) {
      expect((await page.locator('main').boundingBox())!.x).toBeGreaterThan(200);
    }
    if (layout === 'full-content') {
      await expect(page.locator('header')).toHaveCount(0);
      await expect(page.locator('.walnut-tabs')).toHaveCount(0);
    }
    await page.screenshot({ path: testInfo.outputPath(`${layout}.png`) });
    // Wait for the preference manager's debounced persistence before reloading.
    await expect.poll(() => page.evaluate((layout) => Object.entries(localStorage)
      .some(([key, value]) => key.endsWith('-preferences') && value.includes(`"layout":"${layout}"`)), layout)).toBe(true);
    await page.reload({ waitUntil: 'domcontentloaded' });
    await expectContentFits(page);
  }
});

test('header spacing, menu radius and tab height follow the design tokens', async ({ page }) => {
  await expect(page.locator('header')).toHaveCSS('padding-left', '12px');
  await expect(page.locator('.walnut-tabs')).toHaveCSS('height', '38px');
  await expect(page.locator('.ant-menu-item-selected')).toHaveCSS('border-radius', '6px');
  await expect(page.locator('.ant-tabs-tab-active')).toHaveCSS('background-color', 'rgba(20, 118, 255, 0.15)');
  await preferences(page, { theme: { radius: '0.75' }, breadcrumb: { styleType: 'background' } });
  await expect(page.locator('.ant-menu-item-selected')).toHaveCSS('border-radius', '12px');
  await expect(page.locator('.walnut-breadcrumb-background')).toHaveCSS('border-radius', '12px');
});

test('light, dark and semi-dark themes retain contrast in every shell', async ({ page }, testInfo) => {
  for (const layout of ['sidebar-nav', 'header-nav', 'mixed-nav', 'header-mixed-nav',
    'sidebar-mixed-nav', 'header-sidebar-nav']) {
    await preferences(page, { app: { layout }, theme: { mode: 'light', semiDarkHeader: true, semiDarkSidebar: true } });
    await expectContentFits(page);
    await expect(page.locator('header')).toHaveCSS('background-color', 'rgb(28, 30, 35)');
    // The icon controls use Tailwind; the Menu uses antd. Both must receive the local dark theme.
    await expect(page.locator('header')).toHaveCSS('color', 'rgb(242, 242, 242)');
    await expect(page.locator('header .theme-toggle')).toHaveCSS('--foreground', '0 0% 95%');
    await page.screenshot({ path: testInfo.outputPath(`${layout}-semi-dark.png`) });
    await preferences(page, { theme: { mode: 'dark' } });
    await expect(page.locator('header')).toHaveCSS('background-color', 'rgb(28, 30, 35)');
    await expectContentFits(page);
    await preferences(page, { theme: { mode: 'light', semiDarkHeader: false, semiDarkSidebar: false } });
    await expect(page.locator('header')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  }
});

test('maximized content has a working restore control in both shell implementations', async ({ page }) => {
  for (const layout of ['sidebar-nav', 'sidebar-mixed-nav']) {
    await preferences(page, { app: { layout, watermark: true } });
    await page.getByRole('button', { name: 'maximize-content', exact: true }).click();
    await expect(page.locator('header')).toHaveCount(0);
    await expectContentFits(page);
    await page.getByRole('button', { name: 'restore-layout', exact: true }).click();
    await expect(page.locator('header')).toBeVisible();
    await expectContentFits(page);
  }
});

test('mixed navigation selects a leaf and hidden shell regions stay hidden', async ({ page }) => {
  await preferences(page, { app: { layout: 'mixed-nav' } });
  await page.locator('header').getByRole('menuitem', { name: '系统管理', exact: true }).click();
  await expect(page).toHaveURL(/\/system\/config$/);
  await expectContentFits(page);
  for (const layout of ['header-sidebar-nav', 'sidebar-mixed-nav', 'sidebar-nav']) {
    await preferences(page, { app: { layout }, header: { hidden: true }, sidebar: { hidden: true } });
    await expect(page.locator('header')).toHaveCount(0);
    await expect(page.locator('main')).toHaveCSS('width', '1440px');
    await expectContentFits(page);
  }
});
