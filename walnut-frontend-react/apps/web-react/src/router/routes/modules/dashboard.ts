import { lazy } from 'react';
import type { AppRouteObject } from '@vben-core/typings';
import { $t } from '@/locales';
const routes: AppRouteObject[] = [
  {
    name: 'Dashboard',
    path: '/',
    meta: { order: -1, title: 'page.dashboard.title', icon: 'lucide:layout-dashboard' },
    children: [
      {
        name: 'Analytics',
        path: '/analytics',
        Component: lazy(() => import('@/views/dashboard/analytics/index')),
        meta: { affixTab: true, keepAlive: true, title: 'page.dashboard.analytics' },
      },
      {
        name: 'Workspace',
        path: '/workspace',
        Component: lazy(() => import('@/views/dashboard/workspace/index')),
        meta: { keepAlive: true, title: 'page.dashboard.workspace' },
      },
    ],
  },
  {
    path: '/dashboard',
    redirect: '/analytics',
    meta: { hideInMenu: true, hideInTab: true, title: 'page.dashboard.title' },
  },
];
export default routes;
