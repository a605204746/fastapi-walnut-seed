import { lazy, Suspense } from 'react';

import { getPreferences } from '@vben/preferences';
import { useUserStore } from '@vben/stores';
import { createBrowserRouter, Navigate } from 'react-router';

import { AuthLayout } from '@/layouts/AuthLayout';
import { RootLayout } from '@/layouts/RootLayout';
import { LoginPage } from '@/views/_core/authentication/login';

import { bindRouter, ROOT_ROUTE_ID } from './guard';

const NotFound = lazy(() => import('@/views/_core/fallback/not-found'));

/**
 * 首页重定向（对应 Vue 侧 Root 路由的 `redirect: preferences.app.defaultHomePath`）。
 * RR7 的路由对象没有 `redirect` 字段，用一条 index 路由 + `<Navigate>` 等价实现。
 *
 * 只在 Root 挂载后渲染，此时 `RootLayout` 已完成 bootstrap，userInfo 一定可用。
 */
function HomeRedirect() {
  const userInfo = useUserStore((state) => state.userInfo);
  const homePath = userInfo?.homePath || getPreferences().app.defaultHomePath;
  return <Navigate replace to={homePath as string} />;
}

/**
 * 应用路由表（对应 Vue 侧 `router/routes/core.ts` 的 coreRoutes）。
 *
 * 静态部分只有认证页与 Root 骨架；业务路由由后端菜单在运行时 patch 进 Root
 * （见 `./guard.ts`）。Root 的 `*` 子路由兜住未知路径，同时保证任何导航都会
 * 先经过 `RootLayout` 的鉴权与 bootstrap。
 */
export const router = createBrowserRouter([
  {
    children: [{ element: <LoginPage />, path: 'login' }],
    element: <AuthLayout />,
    id: 'Auth',
    path: '/auth',
  },
  {
    children: [
      { element: <HomeRedirect />, id: 'HomeRedirect', index: true },
      {
        element: (
          <Suspense fallback={null}>
            <NotFound />
          </Suspense>
        ),
        id: 'FallbackNotFound',
        path: '*',
      },
    ],
    element: <RootLayout />,
    id: ROOT_ROUTE_ID,
    path: '/',
  },
]);

// 动态路由注入需要 router 实例。绑定放在创建之后，
// 以断开 router/index → layouts/RootLayout → router/guard → router/index 的循环依赖。
bindRouter(router);
