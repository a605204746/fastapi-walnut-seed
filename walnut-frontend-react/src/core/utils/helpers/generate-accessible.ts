import type {
  AccessModeType,
  AppRouteObject,
  GenerateMenuAndRoutesOptions,
  MenuRecordRaw,
} from '@/core/typings';

import { cloneDeep, mapTree } from '@/core/shared/utils';

import { generateMenus } from './generate-menus';
import { generateRoutesByBackend } from './generate-routes-backend';
import { generateRoutesByFrontend } from './generate-routes-frontend';
import { setObjToUrlParams } from './request';

interface AccessibleResult {
  accessibleMenus: MenuRecordRaw[];
  accessibleRoutes: AppRouteObject[];
}

/**
 * 生成可访问的路由与菜单（对应 Vue 侧 packages/effects/access/src/accessible.ts）。
 *
 * 与 Vue 版的差异：
 * - Vue 版在这里直接操作 router（`addRoute`/`removeRoute` 重建 Root）；
 *   React Router 7 用 `router.patchRoutes()`，需要 router 实例，因此**拆到 app 层**
 *   （`src/router/route-tree.ts`）。本函数保持纯函数，便于单测。
 * - Vue 版会给 `keepAlive` 的路由包一层同名 `defineComponent` 以配合 `<KeepAlive>`
 *   的按名缓存；React 侧用 `<Activity>` + 路由 key 缓存，不需要这层包装，故删除。
 */
async function generateAccessible(
  mode: AccessModeType,
  options: GenerateMenuAndRoutesOptions,
): Promise<AccessibleResult> {
  // ⚠️ filterTree / mapTree 都会原地改写入参，必须先深拷贝
  const clonedOptions = { ...options, routes: cloneDeep(options.routes) };

  const accessibleRoutes = await generateRoutes(mode, clonedOptions);
  const accessibleMenus = generateMenus(accessibleRoutes);

  return { accessibleMenus, accessibleRoutes };
}

/**
 * Generate routes
 */
async function generateRoutes(
  mode: AccessModeType,
  options: GenerateMenuAndRoutesOptions,
): Promise<AppRouteObject[]> {
  const { forbiddenComponent, roles, routes } = options;

  let resultRoutes: AppRouteObject[] = routes;
  switch (mode) {
    case 'backend': {
      resultRoutes = await generateRoutesByBackend(options);
      break;
    }
    case 'frontend': {
      resultRoutes = await generateRoutesByFrontend(
        routes,
        roles || [],
        forbiddenComponent,
      );
      break;
    }
    case 'mixed': {
      const [frontendRoutes, backendRoutes] = await Promise.all([
        generateRoutesByFrontend(routes, roles || [], forbiddenComponent),
        generateRoutesByBackend(options),
      ]);
      resultRoutes = mergeRoutesByName(backendRoutes, frontendRoutes);
      break;
    }
  }

  /**
   * 调整路由树：对未添加 redirect 的目录路由补上「重定向到第一个子路由」。
   * （Vue 侧此处还有 keepAlive 的组件改名逻辑，React 不需要，见上方说明）
   */
  resultRoutes = mapTree(resultRoutes, (route) => {
    // 如果有 redirect 或者没有子路由，则直接返回
    if (route.redirect || !route.children || route.children.length === 0) {
      return route;
    }
    const firstChild = route.children[0];

    // 如果子路由不是以 / 开头，则直接返回；这种情况需要计算全部父级的 path
    // 才能得出正确的 path，这里不做处理（RootMenu 的子路由 path 为 ''，即走此分支）
    if (!firstChild?.path || !firstChild.path.startsWith('/')) {
      return route;
    }

    // 第一个路由如果有 query 参数 需要加上参数
    const firstChildQuery = route.children[0]?.meta?.query;
    // 根目录菜单固定只有一个 children 且 path 为 / 不需要添加 redirect
    route.redirect =
      firstChildQuery && route.children.length !== 1 && route.path !== '/'
        ? setObjToUrlParams(firstChild.path, firstChildQuery)
        : firstChild.path;

    return route;
  });

  return resultRoutes;
}

/**
 * 根据 name 合并前后端路由（mixed 模式）
 * @param baseRoutes 后端路由
 * @param extraRoutes 前端路由
 */
function mergeRoutesByName(
  baseRoutes: AppRouteObject[],
  extraRoutes: AppRouteObject[],
): AppRouteObject[] {
  const result: AppRouteObject[] = [];
  const routeMap = new Map<string, AppRouteObject>();

  for (const route of baseRoutes) {
    const clone = { ...route } as AppRouteObject;
    result.push(clone);
    if (clone.name) {
      routeMap.set(clone.name, clone);
    }
  }

  for (const route of extraRoutes) {
    if (route.name && routeMap.has(route.name)) {
      const existing = routeMap.get(route.name);
      if (!existing) {
        continue;
      }
      const existingChildren = existing.children ?? [];
      const routeChildren = route.children ?? [];

      const merged: AppRouteObject = {
        ...route,
        ...existing, // keep backend as base
        meta: {
          ...route.meta,
          ...existing.meta, // backend meta wins on conflicts
        } as AppRouteObject['meta'],
      };

      if (existingChildren.length > 0 || routeChildren.length > 0) {
        merged.children = mergeRoutesByName(existingChildren, routeChildren);
      }

      Object.assign(existing, merged);
    } else {
      const clone = { ...route } as AppRouteObject;
      result.push(clone);
      if (clone.name) {
        routeMap.set(clone.name, clone);
      }
    }
  }

  return result;
}

export { generateAccessible, mergeRoutesByName };
