import type {
  AppRouteObject,
  ExAppRouteObject,
  MenuRecordRaw,
  RouteMeta,
} from '@walnut-core/typings';

import { filterTree, mapTree, sortTree } from '@walnut-core/shared/utils';

/**
 * 根据 routes 生成菜单列表。
 *
 * 与 Vue 侧 generate-menus.ts 的差异：Vue 版需要 `router` 实例，用
 * `router.getRoutes()` 把相对路径解析成最终绝对路径；React 侧的路由树在
 * `backMenuToRouteObjects` 阶段就已经逐级拼成绝对路径，故直接取 `route.path`，
 * 不需要 router 依赖（也让本函数可脱离浏览器环境单测）。
 *
 * @param routes - 路由配置列表（path 均为绝对路径）
 * @returns 生成的菜单列表
 */
function generateMenus(routes: AppRouteObject[]): MenuRecordRaw[] {
  let menus = mapTree<ExAppRouteObject, MenuRecordRaw>(
    routes as ExAppRouteObject[],
    (route) => {
      // 获取最终的路由路径
      const path = route.path ?? '';

      const {
        children = [],
        meta = {} as RouteMeta,
        name: routeName,
        redirect,
      } = route;
      const {
        activeIcon,
        badge,
        badgeType,
        badgeVariants,
        hideChildrenInMenu = false,
        icon,
        link,
        order,
        query,
        title = '',
      } = meta;

      // 确保菜单名称不为空
      const name = (title || routeName || '') as string;

      // 处理子菜单
      const resultChildren = hideChildrenInMenu
        ? []
        : ((children as unknown as MenuRecordRaw[]) ?? []);

      // 设置子菜单的父子关系
      // ⚠️ 依赖 mapTree「先父后子」的遍历顺序：这里写入的是原始子节点对象，
      //    子节点随后被 mapper 读取时才能拿到 parent/parents。
      if (resultChildren.length > 0) {
        resultChildren.forEach((child) => {
          const childRoute = child as unknown as ExAppRouteObject;
          childRoute.parents = [...(route.parents ?? []), path];
          childRoute.parent = path;
        });
      }

      // 确定最终路径
      const resultPath = hideChildrenInMenu ? redirect || path : link || path;

      return {
        activeIcon,
        badge,
        badgeType,
        badgeVariants,
        children: resultChildren,
        icon,
        name,
        order,
        parent: route.parent,
        parents: route.parents,
        path: resultPath,
        query,
        show: !meta.hideInMenu,
      };
    },
  );

  // 对菜单进行排序，避免 order=0 时被替换成 999 的问题
  menus = sortTree(menus, (a, b) => (a?.order ?? 999) - (b?.order ?? 999));

  // 过滤掉隐藏的菜单项
  // ⚠️ filterTree 会原地改写入参，且不保留「匹配节点的祖先」——此处 menus 是
  //    mapTree 产出的全新树，改写无副作用；过滤条件是自顶向下的 show，祖先必然可见。
  return filterTree(menus, (menu) => !!menu.show);
}

export { generateMenus };
