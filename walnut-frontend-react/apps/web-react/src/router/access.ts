import type { ComponentType } from 'react';

import type {
  ComponentRecordType,
  GenerateMenuAndRoutesOptions,
} from '@walnut-core/typings';

import { lazy } from 'react';

import { getPreferences } from '@walnut/preferences';
import { generateAccessible } from '@walnut/utils';

import { getAllMenusApi } from '@/api/core/menu';
import { BasicLayout } from '@/layouts/BasicLayout';
import { $t } from '@/locales';
import { getMessageApi } from '@/services/message';

import { backMenuToRouteObjects, createLazyPageMap } from './back-menu';

/**
 * 页面组件表（对应 Vue 侧 `import.meta.glob('../views/**\/*.vue')`）。
 *
 * key 形如 `/system/config/index.tsx`，与后端 `sys_menu.component` 拼上 `/` 和
 * `.tsx` 后的结果对齐（见 `generate-routes-backend.ts` 的 `normalizeViewPath`）。
 * ⚠️ 因此**每个页面文件必须有 default export**，`createLazyPageMap` 靠 `React.lazy` 装载。
 */
const pageMap: ComponentRecordType = createLazyPageMap(
  import.meta.glob<{ default: ComponentType<any> }>('../views/**/*.tsx'),
);

/**
 * 布局组件表。后端对一级目录下发 `component: 'Layout'`，`back-menu` 映射成
 * `BasicLayout`；查不到组件时 `generateRoutesByBackend` 会退化到 `NotFoundComponent`。
 */
const layoutMap: ComponentRecordType = {
  BasicLayout,
  NotFoundComponent: lazy(() => import('@/views/_core/fallback/not-found')),
};

/** `meta.menuVisibleWithForbidden` 为 true 时替换成的 403 组件 */
const forbiddenComponent = lazy(
  () => import('@/views/_core/fallback/forbidden'),
);

type AccessOptions = Omit<
  GenerateMenuAndRoutesOptions,
  'fetchMenuListAsync' | 'forbiddenComponent' | 'layoutMap' | 'pageMap'
>;

/**
 * 生成当前用户可访问的路由与菜单（对应 Vue 侧 router/access.ts 的 generateAccess）。
 *
 * 与 Vue 版的差异：不再传 `router` 实例。Vue 侧 `generateAccessible` 内部用
 * `router.addRoute/removeRoute` 重建 Root；React Router 7 改用 `router.patchRoutes()`，
 * 需要 router 实例，故那部分逻辑拆到 `./route-tree` 与 `./guard`，本函数只负责
 * 「拉菜单 → 转路由 → 生成菜单树」，是纯数据流。
 */
async function generateAccess(options: AccessOptions) {
  return await generateAccessible(getPreferences().app.accessMode, {
    ...options,
    fetchMenuListAsync: async () => {
      // 清除以前的 message（对应 Vue 侧 window.message.destroy()）
      const messageApi = getMessageApi()?.message;
      messageApi?.destroy?.();
      messageApi?.loading?.({
        content: `${$t('common.loadingMenu')}...`,
        duration: 1,
      });
      // 后台返回路由/菜单
      const backMenuList = await getAllMenusApi();
      // 转换为应用路由对象
      return backMenuToRouteObjects(backMenuList ?? []);
    },
    forbiddenComponent,
    layoutMap,
    pageMap,
  });
}

export { generateAccess };
