import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test, type Page } from '@playwright/test';
import { installFullApi } from './full-api';
test.describe.configure({ mode: 'parallel' });
const base = process.env.REACT_BASE ?? 'http://localhost:8020';
const preferencesUrl =
  '/@fs/' +
  path
    .resolve(
      path.dirname(fileURLToPath(import.meta.url)),
      '../../../src/core/preferences/index.ts',
    )
    .replaceAll('\\', '/');
async function prefs(page: Page, value: Record<string, unknown>) {
  await page.evaluate(
    async ({ url, value }) => {
      const module = await import(/* @vite-ignore */ url);
      module.updatePreferences(value);
    },
    { url: preferencesUrl, value },
  );
}
async function login(page: Page) {
  await installFullApi(page);
  await page.goto(`${base}/auth/login?redirect=%2Fsystem%2Fconfig`);
  await page.locator('input[type=text]').first().fill('admin');
  await page.locator('input[type=password]').first().fill('admin123');
  await page.getByRole('button', { name: 'login', exact: true }).click();
  await expect(page.locator('main')).toContainText('参数列表');
}
test('expired credentials open a login modal while keeping the current route', async ({ page }) => {
  await login(page);
  await prefs(page, { app: { loginExpiredMode: 'modal' } });
  let expired = true;
  await page.route(
    (url) => url.pathname.endsWith('/system/config/list'),
    (route) =>
      expired
        ? route.fulfill({
            contentType: 'application/json',
            body: JSON.stringify({ code: 401, msg: 'expired' }),
          })
        : route.fallback(),
  );
  await page.getByRole('button', { name: 'refresh', exact: true }).click();
  const modal = page.getByRole('dialog', { name: '登录已过期' });
  await expect(modal).toBeVisible();
  await expect(page).toHaveURL(/system\/config$/);
  await modal.locator('input[type=password]').fill('admin123');
  expired = false;
  await modal.getByRole('button', { name: 'login', exact: true }).click();
  await expect(modal).not.toBeVisible();
  await expect(page.locator('main')).toContainText('参数列表');
});
test('lock screen rejects the wrong password and restores the same page', async ({ page }) => {
  await login(page);
  await prefs(page, { widget: { lockScreen: true } });
  await page.getByRole('button', { name: 'lock-screen', exact: true }).click();
  const modal = page.getByRole('dialog', { name: '锁定屏幕' });
  await modal.getByLabel('锁屏密码', { exact: true }).fill('screen-test');
  await modal.getByRole('button', { name: /确\s*定/ }).click();
  const screen = page.getByRole('dialog', { name: '屏幕已锁定' });
  await expect(screen).toBeVisible();
  await screen.getByLabel('解锁密码', { exact: true }).fill('wrong');
  await screen.getByRole('button', { name: /解\s*锁/ }).click();
  await expect(screen).toContainText('密码错误');
  await screen.getByLabel('解锁密码', { exact: true }).fill('screen-test');
  await screen.getByRole('button', { name: /解\s*锁/ }).click();
  await expect(screen).not.toBeVisible();
  await expect(page.locator('main')).toContainText('参数列表');
});
test('mobile navigation opens as a drawer and leaves a usable content width', async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  await expect.poll(async () => (await page.locator('main').boundingBox())?.width).toBe(390);
  await page.getByRole('button', { name: 'toggle-sidebar', exact: true }).click();
  const drawer = page.getByRole('dialog');
  await expect(drawer).toBeVisible();
  await drawer.getByRole('menuitem', { name: '通知公告', exact: false }).click();
  await expect(drawer).not.toBeVisible();
  await expect(page.locator('main')).toContainText('通知公告列表');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.screenshot({ path: info.outputPath('mobile-notice.png') });
});
