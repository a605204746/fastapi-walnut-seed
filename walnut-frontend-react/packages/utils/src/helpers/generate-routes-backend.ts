import type {
  AppRouteObject,
  ComponentRecordType,
  GenerateMenuAndRoutesOptions,
  RouteRecordStringComponent,
} from '@vben-core/typings';

import { mapTree } from '@vben-core/shared/utils';

/**
 * 判断路由是否在菜单中显示但访问时展示 403（让用户知悉功能并申请权限）
 */
function menuHasVisibleWithForbidden(route: AppRouteObject): boolean {
  return !!route.meta?.menuVisibleWithForbidden;
}

/**
 * 动态生成路由 - 后端方式
 * 对 meta.menuVisibleWithForbidden 为 true 的项直接替换为 403 组件，让用户知悉功能并申请权限。
 *
 * 与 Vue 侧 generate-routes-backend.ts 逻辑一致，仅把 `component`（字符串）换成
 * RR7 的 `Component`（React 组件），页面文件后缀从 `.vue` 换成 `.tsx`。
 */
async function generateRoutesByBackend(
  options: GenerateMenuAndRoutesOptions,
): Promise<AppRouteObject[]> {
  const {
    fetchMenuListAsync,
    forbiddenComponent,
    layoutMap = {},
    pageMap = {},
  } = options;

  try {
    const menuRoutes = await fetchMenuListAsync?.();
    if (!menuRoutes) {
      return [];
    }

    const normalizePageMap: ComponentRecordType = {};

    for (const [key, value] of Object.entries(pageMap)) {
      normalizePageMap[normalizeViewPath(key)] = value;
    }

    let routes = convertRoutes(menuRoutes, layoutMap, normalizePageMap);

    if (forbiddenComponent) {
      routes = mapTree(routes, (route) => {
        if (menuHasVisibleWithForbidden(route)) {
          route.Component = forbiddenComponent;
        }
        return route;
      });
    }

    return routes;
  } catch (error) {
    console.error(error);
    throw error;
  }
}

/** 转换后的节点：字符串 component 已被替换为 React 组件，原字段需要摘掉 */
type ConvertedRoute = AppRouteObject & { component?: string };

function convertRoutes(
  routes: RouteRecordStringComponent[],
  layoutMap: ComponentRecordType,
  pageMap: ComponentRecordType,
): AppRouteObject[] {
  return mapTree(routes as ConvertedRoute[], (node) => {
    const route = node as ConvertedRoute;
    const { component, name } = node;

    if (!name) {
      console.error('route name is required', route);
    }

    // layout 转换
    if (component && layoutMap[component]) {
      route.Component = layoutMap[component];
      // 页面组件转换
    } else if (component) {
      const normalizePath = normalizeViewPath(component);
      const pageKey = normalizePath.endsWith('.tsx')
        ? normalizePath
        : `${normalizePath}.tsx`;
      if (pageMap[pageKey]) {
        route.Component = pageMap[pageKey];
      } else {
        console.error(`未找到对应组件: /views${component}.tsx`);
        // 默认为 404 页面
        route.Component = layoutMap.NotFoundComponent;
      }
    }

    // 字符串形态的 component 已完成使命，摘掉以免被 RR7 当成未知字段
    delete route.component;

    return route as AppRouteObject;
  });
}

function normalizeViewPath(path: string): string {
  // 去除相对路径前缀
  const normalizedPath = path.replace(/^(\.\/|\.\.\/)+/, '');

  // 确保路径以 '/' 开头
  const viewPath = normalizedPath.startsWith('/')
    ? normalizedPath
    : `/${normalizedPath}`;

  // 这里耦合了 vben-admin 的目录结构
  return viewPath.replace(/^\/views/, '');
}

export { generateRoutesByBackend, normalizeViewPath };
