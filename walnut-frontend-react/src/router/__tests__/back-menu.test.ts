import type { Menu } from '@/api/core/menu';

import { describe, expect, it } from 'vitest';

import menuTree from '../../__fixtures__/backend-menu-tree.json';
import { backMenuToRouteObjects } from '../back-menu';

/** 真实后端 GET /system/menu/getRouters 的返回（阶段①抓取，16 节点） */
const BACKEND_MENU = menuTree as unknown as Menu[];

/** 三后端 seed 完全一致的 14 条 sys_menu.component（docs/frontend/05 §1.1） */
const CONTRACT_COMPONENTS = [
  'system/user/index',
  'system/user/authRole',
  'system/role/index',
  'system/role/authUser',
  'system/menu/index',
  'system/dept/index',
  'system/post/index',
  'system/dict/index',
  'system/dict/data',
  'system/config/index',
  'system/notice/index',
  'system/client/index',
  'monitor/logininfor/index',
  'monitor/operlog/index',
];

/** 深度遍历转换结果，收集所有节点 */
function flatten(routes: ReturnType<typeof backMenuToRouteObjects>): any[] {
  return routes.flatMap((route) => [
    route,
    ...(route.children ? flatten(route.children) : []),
  ]);
}

function makeMenu(overrides: Partial<Menu> & { path: string }): Menu {
  return {
    alwaysShow: false,
    children: [],
    component: 'Layout',
    hidden: false,
    meta: { icon: '', link: '', noCache: false, title: 't' },
    name: 'N',
    redirect: '',
    ...overrides,
  } as Menu;
}

describe('backMenuToRouteObjects - 真实后端菜单树', () => {
  const routes = backMenuToRouteObjects(BACKEND_MENU);
  const all = flatten(routes);

  it('14 条契约 component 全部转换为 /<component> 形式', () => {
    const components = all
      .map((route) => route.component as string)
      .filter((component) => component.startsWith('/'));

    for (const expected of CONTRACT_COMPONENTS) {
      expect(components, `缺少契约组件 ${expected}`).toContain(`/${expected}`);
    }
  });

  it('一级目录 Layout → BasicLayout，且 path 保持绝对', () => {
    expect(routes).toHaveLength(1);
    expect(routes[0]).toMatchObject({
      component: 'BasicLayout',
      name: 'System1',
      path: '/system',
    });
  });

  it('多级目录 ParentView → 空 component（避免套两层 BasicLayout）', () => {
    const log = all.find((route) => route.name === 'Log108');
    expect(log).toBeDefined();
    expect(log.component).toBe('');
    expect(log.path).toBe('/system/log');
    expect(log.children.map((child: any) => child.path)).toEqual([
      '/system/log/operlog',
      '/system/log/logininfor',
    ]);
  });

  it('子路由 path 逐级拼接为绝对路径', () => {
    expect(all.map((route) => route.path)).toContain('/system/user');
    expect(all.map((route) => route.path)).toContain('/system/config');
    expect(all.map((route) => route.path)).toContain('/system/client');
  });

  it('name 保留后端原值（带菜单 ID 后缀、含 / 与 :）', () => {
    const names = all.map((route) => route.name);
    expect(names).toContain('System1');
    expect(names).toContain('User100');
    expect(names).toContain('User-auth/role/:userId131');
    expect(names).toContain('Dict-data/index/:dictId132');
  });

  it('带参隐藏路由：hidden → hideInMenu，activeMenu → activePath', () => {
    const authRole = all.find(
      (route) => route.name === 'User-auth/role/:userId131',
    );
    expect(authRole.path).toBe('/system/user-auth/role/:userId');
    expect(authRole.meta).toMatchObject({
      activePath: '/system/user',
      hideInMenu: true,
      keepAlive: false,
      title: '分配角色',
    });
  });

  it('routeMetaMapping 覆盖：role-auth 额外带 requireHomeRedirect', () => {
    const authUser = all.find(
      (route) => route.name === 'Role-auth/user/:roleId130',
    );
    expect(authUser.path).toBe('/system/role-auth/user/:roleId');
    expect(authUser.meta).toMatchObject({
      activePath: '/system/role',
      requireHomeRedirect: true,
    });
  });

  it('noCache=false → keepAlive=true，icon 原样保留', () => {
    const config = all.find((route) => route.name === 'Config106');
    expect(config.meta).toMatchObject({
      hideInMenu: false,
      icon: 'ant-design:setting-outlined',
      keepAlive: true,
      title: '参数设置',
    });
  });

  it('后端下发的 redirect: noRedirect 不会出现在转换结果里（由 generateAccessible 重算）', () => {
    expect(routes[0]?.redirect).toBeUndefined();
  });

  it('不改写入参（可重复调用，切换租户后重载菜单安全）', () => {
    const snapshot = JSON.stringify(BACKEND_MENU);
    backMenuToRouteObjects(BACKEND_MENU);
    expect(JSON.stringify(BACKEND_MENU)).toBe(snapshot);
  });
});

describe('backMenuToRouteObjects - 特殊形态', () => {
  it('RootMenu：根目录只有一个子菜单时上提路径并隐藏子级', () => {
    const routes = backMenuToRouteObjects([
      makeMenu({
        children: [
          makeMenu({
            component: 'dashboard/index',
            meta: { icon: 'dash', link: '', noCache: false, title: '仪表盘' },
            name: 'Dashboard1',
            path: 'dashboard',
          }),
        ],
        name: 'Root1',
        path: '/',
      }),
    ]);

    expect(routes[0]).toMatchObject({
      component: 'BasicLayout',
      path: '/dashboard',
    });
    expect(routes[0]?.meta).toMatchObject({
      hideChildrenInMenu: true,
      title: '仪表盘',
    });
    // 子路径被置空，转 RR7 时会成为 index 路由
    expect(routes[0]?.children?.[0]).toMatchObject({
      component: '/dashboard/index',
      path: '',
    });
  });

  it('RootMenu 且子级为 http 外链时转 InnerLink 并清洗 path', () => {
    const routes = backMenuToRouteObjects([
      makeMenu({
        children: [
          makeMenu({
            component: 'whatever',
            name: 'Ext1',
            path: 'https://example.com/#/doc',
          }),
        ],
        name: 'Root2',
        path: '/',
      }),
    ]);

    // 子级 path 清洗后上提为父级路径。
    // ⚠️ 这里锁定的是 Vue 侧的**原有行为**（非笔误）：`replaceAll('/#/', '')`
    // 把 hash 分隔符整段删掉，`example.com/#/doc` 会拼成 `example.comdoc`。
    // 该清洗规则是为 hash 路由型外链设计的，1:1 复刻要求保持一致。
    expect(routes[0]?.path).toBe('/example.comdoc');
    expect(routes[0]?.component).toBe('BasicLayout');
    expect(routes[0]?.meta).toMatchObject({ hideChildrenInMenu: true });
    // 子级 component 已被改成 InnerLink，但没有 meta.link 故不会转 IFrameView，
    // 落到 default 分支；path 置空，转 RR7 时成为 index 路由。
    expect(routes[0]?.children?.[0]).toMatchObject({
      component: '/InnerLink',
      path: '',
    });
  });

  it('http 开头的一级目录 → Link，meta.link 保留', () => {
    const routes = backMenuToRouteObjects([
      makeMenu({
        component: 'Layout',
        meta: {
          icon: '',
          link: 'https://github.com',
          noCache: false,
          title: '外链',
        },
        name: 'Link1',
        path: 'https://github.com',
      }),
    ]);

    expect(routes[0]).toMatchObject({ component: 'BasicLayout' });
    expect(routes[0]?.meta).toMatchObject({ link: 'https://github.com' });
  });

  it('InnerLink + meta.link → IFrameView，iframeSrc 写入且 path 已拼接为绝对', () => {
    const routes = backMenuToRouteObjects([
      makeMenu({
        children: [
          makeMenu({
            component: 'InnerLink',
            meta: {
              icon: '',
              link: 'https://doc.example.com/#/guide?v=2',
              noCache: false,
              title: '内嵌文档',
            },
            name: 'Iframe1',
            path: 'inner',
          }),
        ],
        name: 'Parent1',
        path: '/parent',
      }),
    ]);

    const iframe = routes[0]?.children?.[0];
    expect(iframe?.component).toBe('IFrameView');
    expect(iframe?.meta).toMatchObject({
      iframeSrc: 'https://doc.example.com/#/guide?v=2',
    });
    // path 本身不是 URL（URL 在 meta.link 里），故清洗规则不生效，只做绝对路径拼接
    expect(iframe?.path).toBe('/parent/inner');
  });

  it('menu.query 为合法 JSON 时写入 meta.query，非法时不抛错', () => {
    const valid = backMenuToRouteObjects([
      makeMenu({
        component: 'system/user/index',
        name: 'Q1',
        path: '/q',
        query: '{"id":1}',
      }),
    ]);
    expect(valid[0]?.meta?.query).toEqual({ id: 1 });

    const invalid = backMenuToRouteObjects([
      makeMenu({
        component: 'system/user/index',
        name: 'Q2',
        path: '/q2',
        query: 'not-json',
      }),
    ]);
    expect(invalid[0]?.meta?.query).toBeUndefined();
  });
});
