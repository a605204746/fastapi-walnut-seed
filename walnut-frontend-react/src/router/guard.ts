import type { DataRouter } from 'react-router';

import { useAccessStore, useUserStore, useTabbarStore } from '@/core/stores';

import { useAuthStore } from '@/store/auth';

import { generateAccess } from './access';
import { stripNestedLayout, toRouteObjects } from './route-tree';
import { accessRoutes } from './routes';

/**
 * Root 路由 id。动态路由作为它的 children 注入，对应 Vue 侧
 * `accessible.ts` 里 `router.getRoutes().find(item => item.name === 'Root')`。
 */
const ROOT_ROUTE_ID = 'Root';

let boundRouter: DataRouter | null = null;
let inflight: null | Promise<void> = null;

/**
 * 由 `router/index.tsx` 在创建 router 后调用。
 *
 * 用绑定而非直接 import，是为了断开 `router/index → layouts/RootLayout →
 * router/guard → router/index` 这条循环依赖。
 */
function bindRouter(router: DataRouter) {
  boundRouter = router;
}

async function loadAccessRoutes() {
  const router = boundRouter;
  if (!router) {
    throw new Error('[router] bindRouter() 尚未调用，无法注入动态路由');
  }

  const accessStore = useAccessStore.getState();
  const { userInfo } = useUserStore.getState();

  // 与 Vue 侧 guard.ts 一致：用户信息与菜单并行拉取。
  // 硬刷新时 user store 不持久化，userInfo 为空需重新取（homePath 依赖它）。
  const [, routeInfo] = await Promise.all([
    userInfo ?? useAuthStore.getState().fetchUserInfo(),
    generateAccess({ routes: accessRoutes }),
  ]);

  const { accessibleMenus, accessibleRoutes } = routeInfo;

  accessStore.setAccessMenus(accessibleMenus);
  accessStore.setAccessRoutes(accessibleRoutes);
  function affixed(routes: typeof accessibleRoutes): typeof accessibleRoutes { return routes.flatMap(route => [...(route.meta?.affixTab ? [route] : []), ...affixed(route.children ?? [])]); }
  useTabbarStore.getState().setAffixTabs(affixed(accessibleRoutes));

  router.patchRoutes(
    ROOT_ROUTE_ID,
    toRouteObjects(stripNestedLayout(accessibleRoutes), '/'),
  );

  // ⚠️ `patchRoutes` 只更新路由表，不会对当前 location 重新匹配——重新匹配只发生在
  // RR7 的 `patchRoutesOnNavigation` 钩子内部（本仓未用该钩子，因为 Root 带静态 `*`
  // 子路由，任何路径都能匹配上，钩子不会触发）。因此这里主动再发起一次同址导航，
  // 否则首屏会停在 `*` 兜底的 404 上，直到用户手动点一次菜单才正常。
  const { pathname, search } = router.state.location;
  await router.navigate(`${pathname}${search}`, { replace: true });

  // ⚠️ 必须等重导航完成后再置位。`isAccessChecked` 是 RootLayout 渲染外壳的开关，
  // 提前置位会让外壳在「当前匹配仍是 `*` 兜底路由」的那一刻挂载，此时 meta.hideInTab
  // 为真，多页签的 addTab 会被跳过，而地址未变导致它再也不会重试。
  accessStore.setIsAccessChecked(true);
}

/**
 * 确保动态路由已注入 Root。并发调用共享同一次加载；失败后清空 inflight 以便重试。
 *
 * 调用方（`RootLayout`）负责在 await 期间显示加载态、在 reject 时显示错误态。
 */
async function ensureAccessRoutes() {
  if (useAccessStore.getState().isAccessChecked) {
    return;
  }
  inflight ??= loadAccessRoutes().finally(() => {
    inflight = null;
  });
  await inflight;
}

export { bindRouter, ensureAccessRoutes, ROOT_ROUTE_ID };
