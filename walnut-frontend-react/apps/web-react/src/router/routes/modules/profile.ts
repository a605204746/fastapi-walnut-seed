import type { AppRouteObject } from '@vben-core/typings';

import { lazy } from 'react';

import { $t } from '@vben/locales';

/**
 * 个人中心路由（对应 Vue 侧 router/routes/modules/profile.ts）。
 *
 * `hideInMenu` + `requireHomeRedirect` 与 Vue 一致：不进侧边栏，
 * 且作为「登录后回跳」的候选目标。
 *
 * ⚠️ `Component` 必须是 `React.lazy` 包好的组件（见 typings 里
 * ComponentRecordType 的注释：app 层包好 lazy，避免运行时区分组件与 loader）。
 */
const routes: AppRouteObject[] = [
  {
    Component: lazy(() => import('@/views/_core/profile/index')),
    meta: {
      hideInMenu: true,
      order: -1,
      requireHomeRedirect: true,
      title: 'ui.widgets.profile',
    },
    path: '/profile',
  },
];

export default routes;
