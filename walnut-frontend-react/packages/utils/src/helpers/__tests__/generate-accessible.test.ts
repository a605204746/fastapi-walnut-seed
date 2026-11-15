import type {
  AppRouteObject,
  ComponentRecordType,
  RouteRecordStringComponent,
} from '@vben-core/typings';

import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  generateAccessible,
  mergeRoutesByName,
} from '../generate-accessible';
import { generateMenus } from '../generate-menus';
import {
  generateRoutesByBackend,
  normalizeViewPath,
} from '../generate-routes-backend';
import {
  generateRoutesByFrontend,
  hasAuthority,
} from '../generate-routes-frontend';

function Page() {
  return null;
}
function Forbidden() {
  return null;
}
function NotFound() {
  return null;
}

/** 模拟 import.meta.glob('../views/**\/*.tsx') 的键形态 */
const pageMap: ComponentRecordType = {
  '../views/system/config/index.tsx': Page,
  '../views/system/user/index.tsx': Page,
};
const layoutMap: ComponentRecordType = {
  BasicLayout: Page,
  NotFoundComponent: NotFound,
};

/**
 * 后端菜单转换后的中间形态（component 仍是字符串）。
 *
 * ⚠️ 必须用工厂函数：`convertRoutes` 会原地改写节点（删 component、加 Component），
 * 这与 Vue 侧一致——生产环境里数据来自 `backMenuToRouteObjects`（已 cloneDeep）
 * 或网络响应，天然是一次性的；单测里复用同一份常量会串味。
 */
function makeBackendRoutes(): RouteRecordStringComponent[] {
  return [
    {
      children: [
        {
          component: '/system/config/index',
          meta: { title: '参数设置' },
          name: 'Config106',
          path: '/system/config',
        },
        {
          component: '/system/missing/index',
          meta: { title: '缺失页' },
          name: 'Missing999',
          path: '/system/missing',
        },
      ],
      component: 'BasicLayout',
      meta: { title: '系统管理' },
      name: 'System1',
      path: '/system',
    },
  ];
}

beforeEach(() => {
  // convertRoutes 对缺失组件会 console.error，属预期行为，静音以免污染测试输出
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

describe('normalizeViewPath', () => {
  it('剥掉 glob 相对前缀与 /views 目录，补上前导斜杠', () => {
    expect(normalizeViewPath('../views/system/config/index.tsx')).toBe(
      '/system/config/index.tsx',
    );
    expect(normalizeViewPath('./views/a/b.tsx')).toBe('/a/b.tsx');
    expect(normalizeViewPath('/system/config/index')).toBe(
      '/system/config/index',
    );
    expect(normalizeViewPath('system/config/index')).toBe(
      '/system/config/index',
    );
  });
});

describe('generateRoutesByBackend', () => {
  it('layoutMap 命中时用布局组件，pageMap 命中时用页面组件', async () => {
    const routes = await generateRoutesByBackend({
      fetchMenuListAsync: async () => makeBackendRoutes(),
      layoutMap,
      pageMap,
      routes: [],
    });

    expect(routes[0]?.Component).toBe(Page);
    expect(routes[0]?.children?.[0]?.Component).toBe(Page);
  });

  it('pageMap 未命中的组件回退到 NotFoundComponent 并报错', async () => {
    const routes = await generateRoutesByBackend({
      fetchMenuListAsync: async () => makeBackendRoutes(),
      layoutMap,
      pageMap,
      routes: [],
    });

    expect(routes[0]?.children?.[1]?.Component).toBe(NotFound);
    expect(console.error).toHaveBeenCalledWith(
      '未找到对应组件: /views/system/missing/index.tsx',
    );
  });

  it('字符串 component 字段在转换后被摘掉', async () => {
    const routes = await generateRoutesByBackend({
      fetchMenuListAsync: async () => makeBackendRoutes(),
      layoutMap,
      pageMap,
      routes: [],
    });

    expect((routes[0] as Record<string, any>).component).toBeUndefined();
    expect(
      (routes[0]?.children?.[0] as Record<string, any>).component,
    ).toBeUndefined();
  });

  it('menuVisibleWithForbidden 的节点替换为 forbiddenComponent', async () => {
    const routes = await generateRoutesByBackend({
      fetchMenuListAsync: async () => [
        {
          component: '/system/config/index',
          meta: { menuVisibleWithForbidden: true, title: 'x' },
          name: 'F1',
          path: '/f',
        },
      ],
      forbiddenComponent: Forbidden,
      layoutMap,
      pageMap,
      routes: [],
    });

    expect(routes[0]?.Component).toBe(Forbidden);
  });

  it('fetchMenuListAsync 缺省时返回空数组', async () => {
    await expect(generateRoutesByBackend({ routes: [] })).resolves.toEqual([]);
  });
});

describe('generateRoutesByFrontend / hasAuthority', () => {
  function makeRoutes(): AppRouteObject[] {
    return [
      {
        children: [
          { meta: { title: 'a' }, name: 'A', path: '/a' },
          { meta: { authority: ['admin'], title: 'b' }, name: 'B', path: '/b' },
          {
            meta: {
              authority: ['admin'],
              menuVisibleWithForbidden: true,
              title: 'c',
            },
            name: 'C',
            path: '/c',
          },
        ],
        meta: { title: 'root' },
        name: 'Root',
        path: '/',
      },
    ];
  }

  it('无 authority 的路由对所有角色开放', () => {
    expect(hasAuthority({ meta: { title: 'x' } }, [])).toBe(true);
  });

  it('按角色过滤，menuVisibleWithForbidden 的节点保留以便展示 403', async () => {
    const result = await generateRoutesByFrontend(
      makeRoutes(),
      ['guest'],
      Forbidden,
    );

    const names = (result[0]?.children ?? []).map((child) => child.name);
    // A 无 authority 保留；B 无权限且未声明 menuVisibleWithForbidden 被剪掉；
    // C 无权限但声明了 menuVisibleWithForbidden，保留并替换为 403 组件
    expect(names).toEqual(['A', 'C']);
    expect(result[0]?.children?.[1]?.Component).toBe(Forbidden);
  });

  it('拥有权限时全部保留', async () => {
    const result = await generateRoutesByFrontend(makeRoutes(), ['admin']);
    expect((result[0]?.children ?? []).map((child) => child.name)).toEqual([
      'A',
      'B',
      'C',
    ]);
  });
});

describe('mergeRoutesByName', () => {
  it('同名路由以后端为基准，meta 冲突时后端胜出', () => {
    const backend: AppRouteObject[] = [
      { meta: { order: 1, title: '后端标题' }, name: 'S', path: '/s' },
    ];
    const frontend: AppRouteObject[] = [
      { meta: { order: 9, title: '前端标题' }, name: 'S', path: '/s' },
      { meta: { title: 'F' }, name: 'F', path: '/f' },
    ];

    const merged = mergeRoutesByName(backend, frontend);

    expect(merged).toHaveLength(2);
    expect(merged[0]?.meta).toEqual({ order: 1, title: '后端标题' });
    expect(merged[1]?.name).toBe('F');
  });

  it('同名路由的 children 递归合并', () => {
    const backend: AppRouteObject[] = [
      {
        children: [{ meta: { title: 'b1' }, name: 'C1', path: '/s/c1' }],
        name: 'S',
        path: '/s',
      },
    ];
    const frontend: AppRouteObject[] = [
      {
        children: [{ meta: { title: 'f2' }, name: 'C2', path: '/s/c2' }],
        name: 'S',
        path: '/s',
      },
    ];

    const merged = mergeRoutesByName(backend, frontend);
    expect((merged[0]?.children ?? []).map((child) => child.name)).toEqual([
      'C1',
      'C2',
    ]);
  });
});

describe('generateAccessible', () => {
  it('backend 模式：给无 redirect 的目录补上「重定向到第一个子路由」', async () => {
    const { accessibleRoutes } = await generateAccessible('backend', {
      fetchMenuListAsync: async () => makeBackendRoutes(),
      layoutMap,
      pageMap,
      routes: [],
    });

    expect(accessibleRoutes[0]?.redirect).toBe('/system/config');
  });

  it('已有 redirect 的路由不被覆盖', async () => {
    const { accessibleRoutes } = await generateAccessible('backend', {
      fetchMenuListAsync: async () => {
        const [root] = makeBackendRoutes();
        return [{ ...root!, redirect: '/system/keep' }];
      },
      layoutMap,
      pageMap,
      routes: [],
    });

    expect(accessibleRoutes[0]?.redirect).toBe('/system/keep');
  });

  it('RootMenu 形态（子路由 path 为空）不补 redirect', async () => {
    const { accessibleRoutes } = await generateAccessible('backend', {
      fetchMenuListAsync: async () => [
        {
          children: [
            {
              component: '/system/config/index',
              meta: { hideChildrenInMenu: true, title: 'x' },
              name: 'Root1Child',
              path: '',
            },
          ],
          component: 'BasicLayout',
          meta: { hideChildrenInMenu: true, title: 'x' },
          name: 'Root1',
          path: '/root',
        },
      ],
      layoutMap,
      pageMap,
      routes: [],
    });

    expect(accessibleRoutes[0]?.redirect).toBeUndefined();
  });

  it('mixed 模式：前端静态路由与后端路由按 name 合并', async () => {
    const { accessibleRoutes } = await generateAccessible('mixed', {
      fetchMenuListAsync: async () => makeBackendRoutes(),
      layoutMap,
      pageMap,
      routes: [
        {
          children: [
            {
              meta: { affixTab: true, order: -1, title: '分析页' },
              name: 'Analytics',
              path: '/analytics',
            },
          ],
          meta: { order: -1, title: 'Dashboard' },
          name: 'Dashboard',
          path: '/dashboard',
          redirect: '/analytics',
        },
      ],
    });

    expect(accessibleRoutes.map((route) => route.name)).toEqual([
      'System1',
      'Dashboard',
    ]);
    // 前端路由自带的 redirect 保留
    expect(accessibleRoutes[1]?.redirect).toBe('/analytics');
  });

  it('不改动传入的 routes（内部先 cloneDeep，规避 filterTree 原地写入的坑）', async () => {
    const routes: AppRouteObject[] = [
      {
        children: [
          {
            meta: { authority: ['admin'], title: 'b' },
            name: 'B',
            path: '/b',
          },
        ],
        meta: { title: 'root' },
        name: 'Root',
        path: '/',
      },
    ];
    const snapshot = JSON.stringify(routes);

    await generateAccessible('frontend', { roles: ['guest'], routes });

    expect(JSON.stringify(routes)).toBe(snapshot);
  });

  it('同时产出菜单树，hideInMenu 的节点被过滤', async () => {
    const { accessibleMenus } = await generateAccessible('backend', {
      fetchMenuListAsync: async () => [
        {
          children: [
            {
              component: '/system/config/index',
              meta: { title: '参数设置' },
              name: 'Config106',
              path: '/system/config',
            },
            {
              component: '/system/user/index',
              meta: { hideInMenu: true, title: '分配角色' },
              name: 'AuthRole131',
              path: '/system/user-auth/role/:userId',
            },
          ],
          component: 'BasicLayout',
          meta: { icon: 'setting', title: '系统管理' },
          name: 'System1',
          path: '/system',
        },
      ],
      layoutMap,
      pageMap,
      routes: [],
    });

    expect(accessibleMenus).toHaveLength(1);
    expect(accessibleMenus[0]).toMatchObject({
      icon: 'setting',
      name: '系统管理',
      path: '/system',
    });
    expect(accessibleMenus[0]?.children).toHaveLength(1);
    expect(accessibleMenus[0]?.children?.[0]).toMatchObject({
      name: '参数设置',
      path: '/system/config',
    });
  });
});

describe('generateMenus', () => {
  it('按 order 排序，order 缺省视为 999', () => {
    const menus = generateMenus([
      { meta: { order: 5, title: 'e' }, name: 'E', path: '/e' },
      { meta: { order: -1, title: 'a' }, name: 'A', path: '/a' },
      { meta: { title: 'z' }, name: 'Z', path: '/z' },
    ]);

    expect(menus.map((menu) => menu.name)).toEqual(['a', 'e', 'z']);
  });

  it('hideChildrenInMenu 时丢弃子级，path 取 redirect', () => {
    const menus = generateMenus([
      {
        children: [{ meta: { title: 'child' }, name: 'C', path: '/r/c' }],
        meta: { hideChildrenInMenu: true, title: 'root' },
        name: 'R',
        path: '/r',
        redirect: '/r/c',
      },
    ]);

    expect(menus[0]?.children).toEqual([]);
    expect(menus[0]?.path).toBe('/r/c');
  });

  it('meta.link 优先作为菜单路径（外链）', () => {
    const menus = generateMenus([
      {
        meta: { link: 'https://github.com', title: 'ext' },
        name: 'X',
        path: '/x',
      },
    ]);

    expect(menus[0]?.path).toBe('https://github.com');
  });

  it('子菜单带上 parent / parents 链', () => {
    const menus = generateMenus([
      {
        children: [
          {
            children: [
              { meta: { title: 'leaf' }, name: 'L', path: '/a/b/c' },
            ],
            meta: { title: 'mid' },
            name: 'B',
            path: '/a/b',
          },
        ],
        meta: { title: 'top' },
        name: 'A',
        path: '/a',
      },
    ]);

    const mid = menus[0]?.children?.[0];
    const leaf = mid?.children?.[0];
    expect(mid?.parent).toBe('/a');
    expect(mid?.parents).toEqual(['/a']);
    expect(leaf?.parent).toBe('/a/b');
    expect(leaf?.parents).toEqual(['/a', '/a/b']);
  });

  it('name 取 meta.title，缺失时退化到路由 name', () => {
    const menus = generateMenus([
      { meta: { title: '' }, name: 'RouteName', path: '/n' },
    ]);

    expect(menus[0]?.name).toBe('RouteName');
  });
});
