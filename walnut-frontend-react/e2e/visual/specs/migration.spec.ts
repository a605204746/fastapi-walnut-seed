import { expect, test, type Page } from '@playwright/test';
import { installFullApi } from './full-api';
test.describe.configure({ mode: 'parallel' });
const base = process.env.REACT_BASE ?? 'http://localhost:8020';
async function login(page: Page, path = '/system/role') {
  await page.goto(`${base}/auth/login?redirect=${encodeURIComponent(path)}`);
  await page.locator('input[type=text]').first().fill('admin');
  await page.locator('input[type=password]').first().fill('admin123');
  await page.getByRole('button', { name: 'login', exact: true }).click();
  await page.waitForURL(`**${path}`);
  await expect(page.locator('header')).toBeVisible();
}
test('all backend menu pages and dashboard render with the full menu fixture', async ({
  page,
}, info) => {
  test.setTimeout(180_000);
  await installFullApi(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await login(page);
  for (const [path, text] of [
    ['/system/role', '角色列表'],
    ['/system/notice', '通知公告列表'],
    ['/system/user', '用户列表'],
    ['/system/post', '岗位列表'],
    ['/system/dept', '部门列表'],
    ['/system/client', '客户端列表'],
    ['/system/menu', '菜单列表'],
    ['/system/config', '参数列表'],
    ['/system/dict', '字典类型'],
    ['/system/dict-data/index/1', '通知类型'],
    ['/system/log/logininfor', '登录日志列表'],
    ['/system/log/operlog', '操作日志列表'],
    ['/system/user-auth/role/2', '测试用户'],
    ['/system/role-auth/user/2', '用户'],
    ['/analytics', '总用户量'],
    ['/workspace', '项目'],
    ['/profile', '测试用户'],
  ]) {
    await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('main')).toBeVisible();
    await expect(page.locator('main')).toContainText(text, { timeout: 30000 });
    await expect(page.locator('main')).not.toContainText('页面不存在');
    await page.screenshot({ path: info.outputPath(path.replaceAll('/', '_') + '.png') });
  }
  expect(errors).toEqual([]);
});
test('role creation includes associated ancestors and data-scope updates clear hidden departments', async ({
  page,
}) => {
  const api = await installFullApi(page);
  await login(page);
  await page.getByRole('button', { name: /新\s*增/ }).click();
  const drawer = page.getByRole('dialog');
  await drawer.getByLabel('角色名称', { exact: true }).fill('新增验收角色');
  await drawer.getByLabel('权限标识', { exact: true }).fill('acceptance');
  await drawer.getByRole('checkbox', { name: '新增用户', exact: true }).check();
  await drawer.getByRole('button', { name: /确\s*定/ }).click();
  await expect(drawer).not.toBeVisible();
  const saved = api.mutations.find(
    (item) => item.method === 'POST' && item.path === '/system/role',
  )!;
  expect(saved.body.menuIds.toSorted()).toEqual([1, 2, 3]);
  await expect(page.locator('main')).toContainText('新增验收角色');
  await page
    .getByRole('button', { name: /权\s*限/ })
    .first()
    .click();
  const modal = page.getByRole('dialog');
  await modal.getByLabel('权限范围').click();
  await page.getByText('全部数据权限', { exact: true }).last().click();
  await modal.getByRole('button', { name: /确\s*定/ }).click();
  await expect
    .poll(() => api.mutations.filter((item) => item.path === '/system/role/dataScope').length)
    .toBe(1);
  expect(api.mutations.at(-1)!.body.deptIds).toEqual([]);
});
test('notice rich text saves and reloads compatible HTML', async ({ page }) => {
  const api = await installFullApi(page);
  await login(page, '/system/notice');
  await page
    .getByRole('button', { name: /编\s*辑/ })
    .first()
    .click();
  const modal = page.getByRole('dialog');
  const editor = modal.getByRole('textbox', { name: '公告内容编辑器' });
  await expect(editor).toContainText('兼容内容');
  await editor.click();
  await page.keyboard.press('Control+End');
  await page.keyboard.type('迁移后编辑');
  await modal.getByRole('button', { name: /确\s*定/ }).click();
  await expect(modal).not.toBeVisible();
  expect(
    api.mutations.find((item) => item.path === '/system/notice')!.body.noticeContent,
  ).toContain('<strong>');
  await page
    .getByRole('button', { name: /编\s*辑/ })
    .first()
    .click();
  await expect(page.getByRole('textbox', { name: '公告内容编辑器' })).toContainText('迁移后编辑');
});
test('cached tabs preserve search input and refresh resets only the active page', async ({
  page,
}) => {
  await installFullApi(page);
  await login(page, '/system/config');
  await page.getByLabel('参数名称', { exact: true }).fill('保留的查询');
  await page.getByRole('menuitem', { name: '通知公告', exact: false }).click();
  await expect(page.locator('main')).toContainText('通知公告列表');
  await page.getByRole('tab', { name: /参数设置/ }).click();
  await expect(page.getByLabel('参数名称', { exact: true })).toHaveValue('保留的查询');
  await page.getByRole('button', { name: 'refresh', exact: true }).click();
  await expect(page.getByLabel('参数名称', { exact: true })).toHaveValue('');
});
test('global search keyboard shortcut opens and navigates', async ({ page }) => {
  await installFullApi(page);
  await login(page, '/system/config');
  await page.keyboard.press('Control+k');
  await page.getByRole('textbox', { name: '搜索菜单' }).fill('通知公告');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/system\/notice$/);
});
