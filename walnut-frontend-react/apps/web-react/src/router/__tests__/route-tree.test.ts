import type { AppRouteObject } from '@vben-core/typings';

import { describe, expect, it } from 'vitest';

import { mergeMatchMeta, relativizePath, toRouteObjects } from '../route-tree';

/** 取出 loader 的重定向目标（react-router 的 redirect() 返回 302 Response） */
async function loaderTarget(loader: unknown): Promise<null | string> {
  if (typeof loader !== 'function') {
    return null;
  }
  const result = await (loader as () => any)();
  if (result instanceof Response) {
    return result.headers.get('Location');
  }
  return null;
}

describe('relativizePath', () => {
  it('剥掉父级前缀', () => {
    expect(relativizePath('/system', '/system/config')).toBe('config');
    expect(relativizePath('/system/log', '/system/log/operlog')).toBe(
      'operlog',
    );
  });

  it('父级为根路径时保留绝对路径（RR7 允许以 / 开头的子路径）', () => {
    expect(relativizePath('/', '/dashboard')).toBe('/dashboard');
  });

  it('子路径等于父级路径时返回空串', () => {
    expect(relativizePath('/system', '/system')).toBe('');
  });

  it('本来就是相对路径或空串时原样返回', () => {
    expect(relativizePath('/system', 'config')).toBe('config');
    expect(relativizePath('/system', '')).toBe('');
  });

  it('带参路径同样按前缀剥离', () => {
    expect(relativizePath('/system', '/system/user-auth/role/:userId')).toBe(
      'user-auth/role/:userId',
    );
  });
});

describe('toRouteObjects', () => {
  it('meta / name 折进 handle，name 同时作为 id', () => {
    const [route] = toRouteObjects([
      {
        meta: { keepAlive: true, title: '参数设置' },
        name: 'Config106',
        path: '/system/config',
      },
    ]) as any[];

    expect(route.id).toBe('Config106');
    expect(route.handle).toEqual({
      meta: { keepAlive: true, title: '参数设置' },
      name: 'Config106',
    });
    // meta / name 不再作为顶层字段泄漏给 RR7
    expect(route.meta).toBeUndefined();
    expect(route.name).toBeUndefined();
  });

  it('绝对子路径转相对，父级路径保留', () => {
    const [parent] = toRouteObjects([
      {
        children: [
          { name: 'A', path: '/system/user' },
          {
            children: [{ name: 'C', path: '/system/log/operlog' }],
            name: 'B',
            path: '/system/log',
          },
        ],
        name: 'System1',
        path: '/system',
      },
    ]) as any[];

    expect(parent.path).toBe('/system');
    expect(parent.children.map((child: any) => child.path)).toEqual([
      'user',
      'log',
    ]);
    expect(parent.children[1].children[0].path).toBe('operlog');
  });

  it('目录路由的 redirect 转成 index 子路由 + loader', async () => {
    const [parent] = toRouteObjects([
      {
        children: [{ name: 'A', path: '/system/user' }],
        name: 'System1',
        path: '/system',
        redirect: '/system/user',
      },
    ]) as any[];

    expect(parent.children).toHaveLength(2);
    const [indexRoute] = parent.children;
    expect(indexRoute.index).toBe(true);
    expect(indexRoute.id).toBe('System1__redirect');
    expect(await loaderTarget(indexRoute.loader)).toBe('/system/user');
    // 父级自身不挂 loader，避免在所有后代导航上都执行
    expect(parent.loader).toBeUndefined();
  });

  it('已存在 index 子路由时不重复注入 redirect', () => {
    const [parent] = toRouteObjects([
      {
        children: [
          { index: true, name: 'Idx', path: '' },
          { name: 'A', path: '/system/user' },
        ],
        name: 'System1',
        path: '/system',
        redirect: '/system/user',
      },
    ]) as any[];

    expect(parent.children).toHaveLength(2);
    expect(parent.children.filter((c: any) => c.index)).toHaveLength(1);
  });

  it('叶子路由的 redirect 挂在自身 loader 上', async () => {
    const [route] = toRouteObjects([
      { name: 'Leaf', path: '/leaf', redirect: '/target' },
    ]) as any[];

    expect(route.children).toBeUndefined();
    expect(await loaderTarget(route.loader)).toBe('/target');
  });

  it("path 为 '' 的叶子转成 index 路由（RootMenu 形态）", () => {
    const [parent] = toRouteObjects([
      {
        children: [{ name: 'Dashboard1', path: '' }],
        name: 'Root1',
        path: '/dashboard',
      },
    ]) as any[];

    expect(parent.children).toHaveLength(1);
    expect(parent.children[0].index).toBe(true);
    expect(parent.children[0].path).toBeUndefined();
    expect(parent.children[0].id).toBe('Dashboard1');
  });

  it('无 name 时 id 退化到绝对 path，无 path 时退化到位置索引', () => {
    const routes = toRouteObjects([{ path: '/a' }, {}]) as any[];

    expect(routes[0].id).toBe('/a');
    expect(routes[1].id).toBe('/.1');
  });

  it('Component / element 等 RR7 原生字段原样透传', () => {
    function Demo() {
      return null;
    }
    const [route] = toRouteObjects([
      { Component: Demo, caseSensitive: true, name: 'D', path: '/d' },
    ]) as any[];

    expect(route.Component).toBe(Demo);
    expect(route.caseSensitive).toBe(true);
  });

  it('转换结果满足 RR7 的结构约束（index 不带 path/children，id 全局唯一）', () => {
    // 按真实菜单树形状构造：一级目录 + 二级目录 + 叶子 + 带参隐藏路由 + redirect
    const tree: AppRouteObject[] = [
      {
        children: [
          {
            meta: { hideInMenu: true, title: '分配角色' },
            name: 'User-auth/role/:userId131',
            path: '/system/user-auth/role/:userId',
          },
          {
            children: [
              { name: 'Operlog500', path: '/system/log/operlog' },
              { name: 'Logininfor501', path: '/system/log/logininfor' },
            ],
            name: 'Log108',
            path: '/system/log',
            redirect: '/system/log/operlog',
          },
          { name: 'Config106', path: '/system/config' },
        ],
        meta: { title: '系统管理' },
        name: 'System1',
        path: '/system',
        redirect: '/system/user-auth/role/:userId131',
      },
    ];

    const ids: string[] = [];
    const violations: string[] = [];

    const walk = (routes: any[]) => {
      for (const route of routes) {
        ids.push(route.id);
        if (route.index === true) {
          if (route.path !== undefined) {
            violations.push(`index 路由带了 path: ${route.id}`);
          }
          if (route.children) {
            violations.push(`index 路由带了 children: ${route.id}`);
          }
        }
        if (route.children) {
          walk(route.children);
        }
      }
    };
    walk(toRouteObjects(tree) as any[]);

    expect(violations).toEqual([]);
    expect(new Set(ids).size, `id 重复: ${ids.join(', ')}`).toBe(ids.length);
  });
});

describe('mergeMatchMeta', () => {
  it('按 matched 链自顶向下合并 meta，叶子覆盖祖先', () => {
    const meta = mergeMatchMeta([
      { handle: { meta: { keepAlive: true, title: 'Root' } } },
      { handle: { meta: { title: '系统管理' } } },
      { handle: { name: 'Config106' } },
      { handle: { meta: { hideInMenu: true, title: '参数设置' } } },
    ]);

    expect(meta).toEqual({
      hideInMenu: true,
      keepAlive: true,
      title: '参数设置',
    });
  });

  it('无 handle 的 match 被安全跳过', () => {
    expect(mergeMatchMeta([{}, { handle: undefined }])).toEqual({});
  });
});
