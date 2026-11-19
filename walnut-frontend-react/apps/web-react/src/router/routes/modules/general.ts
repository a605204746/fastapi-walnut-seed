import { lazy } from 'react';
import type { AppRouteObject } from '@vben-core/typings';
const routes: AppRouteObject[] = [
  {
    path: '/about',
    Component: lazy(() => import('@/views/_core/about/index')),
    meta: { hideInMenu: true, title: '关于项目' },
  },
  {
    path: '/500',
    Component: lazy(() => import('@/views/_core/fallback/internal-error')),
    meta: { hideInMenu: true, title: '服务器错误' },
  },
  {
    path: '/offline',
    Component: lazy(() => import('@/views/_core/fallback/offline')),
    meta: { hideInMenu: true, title: '网络已断开' },
  },
  {
    path: '/coming-soon',
    Component: lazy(() => import('@/views/_core/fallback/coming-soon')),
    meta: { hideInMenu: true, title: '即将上线' },
  },
];
export default routes;
