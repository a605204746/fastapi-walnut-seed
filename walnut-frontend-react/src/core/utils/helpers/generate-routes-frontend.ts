import type { AppRouteObject } from '@/core/typings';

import { filterTree, mapTree } from '@/core/shared/utils';

/**
 * 动态生成路由 - 前端方式
 *
 * 与 Vue 侧 generate-routes-frontend.ts 逻辑一致。
 * ⚠️ filterTree 会原地改写入参，调用方（generate-accessible）必须先 cloneDeep。
 */
async function generateRoutesByFrontend(
  routes: AppRouteObject[],
  roles: string[],
  forbiddenComponent?: AppRouteObject['Component'],
): Promise<AppRouteObject[]> {
  // 根据角色标识过滤路由表,判断当前用户是否拥有指定权限
  const finalRoutes = filterTree(routes, (route) => {
    return hasAuthority(route, roles);
  });

  if (!forbiddenComponent) {
    return finalRoutes;
  }

  // 如果有禁止访问的页面，将禁止访问的页面替换为 403 页面
  return mapTree(finalRoutes, (route) => {
    if (menuHasVisibleWithForbidden(route)) {
      route.Component = forbiddenComponent;
    }
    return route;
  });
}

/**
 * 判断路由是否有权限访问
 */
function hasAuthority(route: AppRouteObject, access: string[]) {
  const authority = route.meta?.authority;
  if (!authority) {
    return true;
  }
  const canAccess = access.some((value) => authority.includes(value));

  return canAccess || (!canAccess && menuHasVisibleWithForbidden(route));
}

/**
 * 判断路由是否在菜单中显示，但是访问会被重定向到 403
 */
function menuHasVisibleWithForbidden(route: AppRouteObject) {
  return (
    !!route.meta?.authority &&
    Reflect.has(route.meta || {}, 'menuVisibleWithForbidden') &&
    !!route.meta?.menuVisibleWithForbidden
  );
}

export { generateRoutesByFrontend, hasAuthority };
