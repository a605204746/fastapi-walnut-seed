import type { Page } from '@playwright/test';

/**
 * 像素门禁的接口 mock（对应 Spec §6「确定性处理」）。
 *
 * 在浏览器层用 page.route 拦截 `/api/**` 并返回固定 fixture，使门禁**不依赖真实后端**，
 * 且 Vue / React 双端渲染完全相同的数据（验证码关闭、租户关闭、固定列表/字典），
 * 避免动态内容造成假差异。
 */

const ok = (data: unknown, msg = 'ok') => ({ code: 200, data, msg });

const DICT_YES_NO = [
  {
    createBy: 'admin',
    createTime: '2026-01-01 00:00:00',
    cssClass: '',
    default: false,
    dictLabel: '是',
    dictSort: 1,
    dictType: 'sys_yes_no',
    dictValue: 'Y',
    id: 1,
    isDefault: 'N',
    listClass: 'success',
    remark: '',
    status: '0',
  },
  {
    createBy: 'admin',
    createTime: '2026-01-01 00:00:00',
    cssClass: '',
    default: false,
    dictLabel: '否',
    dictSort: 2,
    dictType: 'sys_yes_no',
    dictValue: 'N',
    id: 2,
    isDefault: 'Y',
    listClass: 'default',
    remark: '',
    status: '0',
  },
];

const CONFIG_ROWS = [
  {
    configKey: 'sys.account.captchaEnabled',
    configName: '账号自助-验证码开关',
    configType: 'Y',
    configValue: 'true',
    createTime: '2026-01-01 00:00:00',
    id: 1,
    remark: '验证码开关',
  },
  {
    configKey: 'sys.account.registerUser',
    configName: '账号自助-是否开启用户注册',
    configType: 'N',
    configValue: 'false',
    createTime: '2026-01-02 00:00:00',
    id: 2,
    remark: '注册开关',
  },
  {
    configKey: 'sys.index.sideTheme',
    configName: '侧边栏主题',
    configType: 'Y',
    configValue: 'theme-dark',
    createTime: '2026-01-03 00:00:00',
    id: 3,
    remark: '侧边栏',
  },
];

/** 供 Vue 动态菜单使用的最小路由树（含 system/config） */
const MENU_TREE = [
  {
    alwaysShow: true,
    children: [
      {
        component: 'system/config/index',
        hidden: false,
        meta: { icon: '', link: null, noCache: false, title: '参数配置' },
        name: 'Config10',
        path: 'config',
      },
    ],
    component: 'Layout',
    hidden: false,
    meta: { icon: 'setting', link: null, noCache: false, title: '系统管理' },
    name: 'System1',
    path: '/system',
    redirect: 'noRedirect',
  },
];

const USER_INFO = {
  permissions: ['*:*:*'],
  roles: ['superadmin'],
  user: {
    avatar: '',
    createTime: '2026-01-01 00:00:00',
    deptId: 1,
    deptName: '研发部',
    email: 'admin@example.com',
    id: 1,
    loginDate: '2026-01-01 00:00:00',
    loginIp: '127.0.0.1',
    nickName: '管理员',
    phonenumber: '13800000000',
    remark: '',
    roles: [],
    sex: '0',
    status: '0',
    tenantId: '000000',
    userName: 'admin',
    userType: 'sys_user',
  },
};

function json(route: any, data: unknown) {
  return route.fulfill({
    body: JSON.stringify(data),
    contentType: 'application/json',
    status: 200,
  });
}

/** 安装接口 mock；必须在 page.goto 之前调用 */
export async function installMockApi(page: Page) {
  // ⚠️ 不能用 glob '**/api/**'：Vite dev 的源码模块路径 /src/api/*.ts 也含 '/api/'，
  // 会被误拦截返回 JSON 导致模块加载失败。只匹配 pathname 以 /api/ 开头的真实接口请求。
  await page.route((url) => url.pathname.startsWith('/api/'), async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api/, '');
    const method = request.method().toUpperCase();

    if (method === 'POST' && path === '/auth/login') {
      return json(
        route,
        ok({ access_token: 'mock-access-token', client_id: 'mock', expire_in: 7200 }),
      );
    }
    if (method === 'GET' && path === '/auth/code') {
      return json(route, ok({ captchaEnabled: false, img: '', uuid: '' }));
    }
    if (method === 'GET' && path === '/auth/tenant/list') {
      return json(route, ok({ tenantEnabled: false, voList: [] }));
    }
    if (method === 'GET' && path === '/system/user/getInfo') {
      return json(route, ok(USER_INFO));
    }
    if (method === 'GET' && path === '/system/menu/getRouters') {
      return json(route, ok(MENU_TREE));
    }
    if (method === 'GET' && path === '/system/config/list') {
      return json(route, ok({ rows: CONFIG_ROWS, total: CONFIG_ROWS.length }));
    }
    if (method === 'GET' && path.startsWith('/system/dict/data/type/')) {
      return json(route, ok(DICT_YES_NO));
    }
    if (path.startsWith('/system/config')) {
      return json(route, ok(null, '操作成功'));
    }
    // 兜底：其余接口返回成功空数据，避免未拦截请求打断渲染
    return json(route, ok(null));
  });
}
