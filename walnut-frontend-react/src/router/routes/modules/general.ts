import { lazy } from 'react';
import type { AppRouteObject } from '@/core/typings';
const routes: AppRouteObject[] = [
  {
    path: '/about',
    Component: lazy(() => import('@/views/_core/about/index')),
    meta: { hideInMenu: true, title: 'demos.walnut.about' },
  },
  {
    path: '/500',
    Component: lazy(() => import('@/views/_core/fallback/internal-error')),
    meta: { hideInMenu: true, title: 'ui.fallback.internalError' },
  },
  {
    path: '/offline',
    Component: lazy(() => import('@/views/_core/fallback/offline')),
    meta: { hideInMenu: true, title: 'ui.fallback.offlineError' },
  },
  {
    path: '/coming-soon',
    Component: lazy(() => import('@/views/_core/fallback/coming-soon')),
    meta: { hideInMenu: true, title: 'ui.fallback.comingSoon' },
  },
];
export default routes;
