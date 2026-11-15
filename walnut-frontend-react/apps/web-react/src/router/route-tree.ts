import type { AppRouteObject, RouteHandle } from '@vben-core/typings';
import type {
  IndexRouteObject,
  NonIndexRouteObject,
  RouteObject,
} from 'react-router';

import { redirect } from 'react-router';

/**
 * `AppRouteObject` → React Router 7 `RouteObject` 的边界转换层。
 *
 * RR7 的 `RouteObject` 是联合类型别名（`IndexRouteObject | NonIndexRouteObject`），
 * 无法通过声明合并扩展，且它没有 `name` / `meta` / `redirect` 概念。
 * 因此本仓在内部一律使用 `AppRouteObject`（字段与 Vue 侧 `RouteRecordRaw` 对齐），
 * 只在 `patchRoutes` / `createBrowserRouter` 的边界处调用本模块转换一次：
 *
 * - `meta` / `name`  → 折进 `handle`，`useMatches()` 可原样读回
 * - `name`           → 同时写入 `id`（RR7 用 id 做路由标识与 patch 定位）
 * - `redirect`       → 有子路由时生成一条 `index` 子路由 + loader；无子路由时挂自身 loader
 * - 绝对子路径        → 剥掉父级前缀还原成 RR7 期望的相对路径
 * - `path: ''`       → 转成 `index: true`（RootMenu 形态的子路由）
 */

/** 把绝对子路径还原成相对于父级的路径 */
function relativizePath(parentPath: string, childPath: string): string {
  if (!childPath) {
    return '';
  }
  // 本来就是相对路径，直接用
  if (!childPath.startsWith('/')) {
    return childPath;
  }
  const base = parentPath === '/' ? '' : parentPath.replace(/\/$/, '');
  // 父级为根路径时，绝对子路径本身就已合法（RR7 要求绝对子路径以父级组合路径开头）
  if (!base) {
    return childPath;
  }
  if (childPath === base) {
    return '';
  }
  if (childPath.startsWith(`${base}/`)) {
    return childPath.slice(base.length + 1);
  }
  // 无法相对化，保留绝对路径（RR7 仍会校验其是否以父级路径开头）
  return childPath;
}

function buildHandle(route: AppRouteObject): RouteHandle | undefined {
  if (!route.meta && !route.name) {
    return route.handle;
  }
  return {
    ...route.handle,
    ...(route.meta ? { meta: route.meta } : {}),
    ...(route.name ? { name: route.name } : {}),
  };
}

/** 路由 id：优先用 name（后端菜单名全局唯一），退化到 path */
function resolveRouteId(route: AppRouteObject, fallback: string): string {
  return route.id ?? route.name ?? route.path ?? fallback;
}

/**
 * 递归转换。
 *
 * @param routes 应用路由树（path 为绝对路径）
 * @param parentPath 父级绝对路径，用于把子路径相对化
 */
function toRouteObjects(
  routes: AppRouteObject[],
  parentPath = '',
): RouteObject[] {
  const result: RouteObject[] = [];

  routes.forEach((route, index) => {
    const absolutePath = route.path ?? '';
    const id = resolveRouteId(route, `${parentPath || '/'}.${index}`);
    const handle = buildHandle(route);

    const {
      children,
      index: _indexFlag,
      meta,
      name,
      path,
      redirect: redirectPath,
      ...rest
    } = route;
    // 这几个字段不进 RR7：
    // - meta / name 已折进 handle
    // - path 由 relativizePath 重算（绝对 → 相对）
    // - index 由本转换器根据「path 为空且无子路由」自行判定，不接受调用方指定
    void _indexFlag;
    void meta;
    void name;
    void path;

    // rest 剩下的都是 RR7 BaseRouteObject 的合法字段
    const base = rest as Omit<
      NonIndexRouteObject,
      'children' | 'handle' | 'id' | 'index' | 'path'
    >;

    const childPath = relativizePath(parentPath, absolutePath);
    const hasChildren = !!children && children.length > 0;

    // 空路径 + 无子路由 → 索引路由（RootMenu 的唯一子节点即此形态）
    // ⚠️ 索引路由不能带 path 也不能带 children
    if (childPath === '' && !hasChildren) {
      const indexRoute: IndexRouteObject = {
        ...base,
        handle,
        id,
        index: true,
        ...(redirectPath ? { loader: () => redirect(redirectPath) } : {}),
      };
      result.push(indexRoute);
      return;
    }

    const convertedChildren = hasChildren
      ? toRouteObjects(children as AppRouteObject[], absolutePath)
      : undefined;

    // 目录路由的 redirect 注入一条 index 子路由，
    // 而不是挂在父级 loader 上——父级 loader 会在所有后代导航时都执行。
    let finalChildren = convertedChildren;
    if (redirectPath && finalChildren) {
      const alreadyHasIndex = finalChildren.some(
        (child) => (child as IndexRouteObject).index === true,
      );
      if (!alreadyHasIndex) {
        const redirectRoute: IndexRouteObject = {
          handle: { meta: { title: 'Redirect' } },
          id: `${id}__redirect`,
          index: true,
          loader: () => redirect(redirectPath),
        };
        finalChildren = [redirectRoute, ...finalChildren];
      }
    }

    const layoutRoute: NonIndexRouteObject = {
      ...base,
      ...(childPath === '' ? {} : { path: childPath }),
      handle,
      id,
      ...(finalChildren ? { children: finalChildren } : {}),
      // 叶子路由的 redirect 只能挂在自身 loader 上
      ...(!finalChildren && redirectPath
        ? { loader: () => redirect(redirectPath) }
        : {}),
    };

    result.push(layoutRoute);
  });

  return result;
}

/**
 * 从 RR7 的 match 上取回路由元信息。
 *
 * `useMatches()` 返回的每个 match 都带 `handle`，本函数把「最近一层带 meta 的祖先」
 * 与叶子 meta 合并，等价于 Vue 侧 `route.meta`（vue-router 会把 matched 链上的 meta 合并）。
 */
function mergeMatchMeta(matches: Array<{ handle?: unknown }>): RouteHandle['meta'] {
  const merged: Record<string, any> = {};
  for (const match of matches) {
    const handle = match.handle as RouteHandle | undefined;
    if (handle?.meta) {
      Object.assign(merged, handle.meta);
    }
  }
  return merged as RouteHandle['meta'];
}

/**
 * 摘掉顶层动态路由的 Component（当它有子路由时）。
 *
 * 后端对一级目录下发 `component: 'Layout'`，`back-menu` 把它映射成 `BasicLayout`；
 * 而 Root 本身已经是 BasicLayout，不摘就会套两层后台外壳。对应 Vue 侧
 * `packages/effects/access/src/accessible.ts` 的 `delete route.component`
 * （该处注释同样标明这是为避免多层 BasicLayout 的兼容写法）。
 *
 * 中间层（`ParentView`）的 component 是空串，`generateRoutesByBackend` 不会给它赋
 * `Component`，天然就是透传节点，无需在这里处理——所以只处理顶层，与 Vue 侧一致。
 *
 * 同时过滤掉外链路由：它们不进路由表，由菜单直接开新窗口（同 Vue 侧）。
 */
function stripNestedLayout(routes: AppRouteObject[]): AppRouteObject[] {
  return routes
    .filter((route) => !/^https?:\/\//.test(route.path ?? ''))
    .map((route) => {
      if (!route.children || route.children.length === 0) {
        return route;
      }
      const { Component: _nestedLayout, ...rest } = route;
      void _nestedLayout;
      return rest;
    });
}

export { mergeMatchMeta, relativizePath, stripNestedLayout, toRouteObjects };
