import type {
  ComponentRecordType,
  RouteMeta,
  RouteRecordStringComponent,
} from '@walnut-core/typings';

import type { Menu } from '@/api/core/menu';

import { cloneDeep } from '@walnut-core/shared/utils';
import { lazy } from 'react';

/**
 * 后端返回的 meta 有时候不包括需要的信息（比如 activePath 等），在这里定义映射。
 * 对应 Vue 侧 router/access.ts 的 routeMetaMapping。
 */
const routeMetaMapping: Record<string, Omit<RouteMeta, 'title'>> = {
  '/system/role-auth/user/:roleId': {
    activePath: '/system/role',
    requireHomeRedirect: true,
  },
};

/**
 * 后台菜单转应用路由对象（对应 Vue 基线的后台菜单转路由函数）。
 *
 * 五种特殊 component 字符串（`Layout` / `ParentView` / `Link` / `InnerLink` / `RootMenu`）
 * 的判断分支**逐行照搬** Vue 版，只把输出类型换成本仓的 `RouteRecordStringComponent`。
 *
 * 与 Vue 版的两处刻意差异：
 * 1. 顶层入参先 cloneDeep。Vue 版会原地改写 `menu.component` / `menu.path` / `menu.meta`，
 *    对「同一份菜单数据调用两次」的场景（单测、切换租户后重载菜单）不安全。
 * 2. 额外把后端的 `meta.activeMenu` 映射到 `meta.activePath`。后端对三个带参隐藏路由
 *    （`user-auth/role/:userId`、`role-auth/user/:roleId`、`dict-data/index/:dictId`）
 *    都下发了 `activeMenu`，但 Vue 版只为其中一条写了 routeMetaMapping，另两条的菜单
 *    高亮实际是失效的。此处统一映射，routeMetaMapping 仅保留后端未下发的
 *    `requireHomeRedirect`。
 *
 * @param menuList 后台菜单
 * @param parentPath 上级目录（递归时传入，顶层为空串）
 */
function backMenuToRouteObjects(
  menuList: Menu[],
  parentPath = '',
): RouteRecordStringComponent[] {
  const resultList: RouteRecordStringComponent[] = [];
  // 见上方差异说明 1：只在顶层克隆一次，不改写调用方的数据
  const menus = parentPath === '' ? cloneDeep(menuList) : menuList;

  menus.forEach((menu) => {
    // 根目录为菜单形式
    // 固定有一个 children，children 为当前菜单
    if (menu.path === '/' && menu.children && menu.children.length === 1) {
      const [first] = menu.children;
      if (!first) {
        return;
      }

      // 需要处理根目录为内嵌的情况 不会带 InnerLink
      if (/^https?:\/\//.test(first.path)) {
        first.component = 'InnerLink';
        first.path = first.path
          .replaceAll(/^https?:\/\//g, '')
          .replaceAll('/#/', '')
          .replaceAll('#', '')
          .replaceAll(/[?&]/g, '');
      }

      // 取子路径作为父级路径
      const path = first.path;
      // 取子菜单的 meta 作为当前菜单的 meta
      menu.meta = first.meta;
      // 由于在一级路由 父级路径需要加上 /
      menu.path = `/${path}`;
      menu.component = 'RootMenu';
      // 将子路径设置为 ''（转 RR7 时会变成 index 路由）
      first.path = '';
    }

    // 外链: http 开头 & 组件为 Layout || ParentView
    if (
      /^https?:\/\//.test(menu.path) &&
      (menu.component === 'Layout' || menu.component === 'ParentView')
    ) {
      menu.component = 'Link';
    }

    // 内嵌 iframe 组件为 InnerLink
    if (menu.meta?.link && menu.component === 'InnerLink') {
      menu.component = 'IFrameView';
    }

    /**
     * 拼接 path
     * menu.path 为 ''（根目录路由）则不拼接
     */
    if (parentPath && menu.path) {
      menu.path = `${parentPath}/${menu.path}`;
    }

    // 创建路由对象
    const appRoute: RouteRecordStringComponent = {
      component: menu.component,
      meta: {
        // 当前路由不在菜单显示 但是可以通过链接访问
        // 不可访问的路由由后端控制隐藏（不返回对应路由）
        hideInMenu: menu.hidden,
        icon: menu.meta?.icon,
        keepAlive: !menu.meta?.noCache,
        title: menu.meta?.title ?? '',
        // 见上方差异说明 2
        ...(menu.meta?.activeMenu ? { activePath: menu.meta.activeMenu } : {}),
      },
      name: menu.name,
      path: menu.path,
    };

    // 处理 meta 映射
    if (Object.keys(routeMetaMapping).includes(appRoute.path)) {
      const routeMeta = routeMetaMapping[appRoute.path];
      if (routeMeta) {
        appRoute.meta = {
          ...appRoute.meta,
          ...(routeMeta as RouteMeta),
        };
      }
    }

    // 添加路由参数信息
    if (menu.query) {
      try {
        const query = JSON.parse(menu.query);
        appRoute.meta && (appRoute.meta.query = query);
      } catch {
        console.error('错误的路由参数类型, 必须为[json]格式');
      }
    }

    /**
     * 处理不同组件
     */
    switch (menu.component) {
      /**
       * iframe 内嵌
       */
      case 'IFrameView': {
        appRoute.component = 'IFrameView';
        if (appRoute.meta) {
          appRoute.meta.iframeSrc = menu.meta.link;
        }
        /**
         * 需要判断特殊情况 比如 vue 的 hash 是带 # 的
         * 比如链接 aaa.com/#/bbb  path 会转换为 aaa/com/#/bbb
         * 比如链接 aaa.com/?bbb=xxx
         * 需要去除 # 否则无法被添加到路由
         */
        appRoute.path = appRoute.path
          // 替换 https:// 或者 http://
          .replaceAll(/^https?:\/\//g, '')
          .replaceAll('/#/', '')
          .replaceAll('#', '')
          .replaceAll(/[?&]/g, '');
        break;
      }
      case 'Layout': {
        appRoute.component = 'BasicLayout';
        break;
      }
      /**
       * 外链 新窗口打开
       */
      case 'Link': {
        if (appRoute.meta) {
          appRoute.meta.link = menu.meta.link;
        }
        appRoute.component = 'BasicLayout';
        break;
      }
      /**
       * 三级以上菜单 父级 component 为 ParentView
       * 不能为 layout 会套两层 BasicLayout
       */
      case 'ParentView': {
        appRoute.component = '';
        break;
      }
      /**
       * 根目录菜单
       */
      case 'RootMenu': {
        if (appRoute.meta) {
          appRoute.meta.hideChildrenInMenu = true;
        }
        appRoute.component = 'BasicLayout';
        break;
      }
      /**
       * 其他自定义组件 如 system/user/index 拼接 /
       */
      default: {
        appRoute.component = `/${menu.component}`;
        break;
      }
    }

    // children 处理
    if (menu.children && menu.children.length > 0) {
      appRoute.children = backMenuToRouteObjects(menu.children, menu.path);
    }
    // 添加
    resultList.push(appRoute);
  });
  return resultList;
}

/**
 * 把 `import.meta.glob` 的异步页面模块表转成 React 组件表。
 *
 * 统一在入口处 `lazy()` 包好，`ComponentRecordType` 的值就只会是 React 组件，
 * 避免运行时再去区分「组件」和「返回 Promise 的 loader」这两种同为 function 的形态。
 */
function createLazyPageMap(
  modules: Record<string, () => Promise<{ default: any }>>,
): ComponentRecordType {
  return Object.fromEntries(
    Object.entries(modules).map(([path, loader]) => [path, lazy(loader)]),
  );
}

export { backMenuToRouteObjects, createLazyPageMap, routeMetaMapping };
