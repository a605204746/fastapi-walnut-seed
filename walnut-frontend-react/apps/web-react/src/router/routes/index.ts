import type { AppRouteObject } from '@vben-core/typings';

import { mergeRouteModules } from '@vben/utils';

/**
 * 前端静态路由模块聚合（对应 Vue 侧 `router/routes/index.ts`）。
 *
 * `accessMode: 'mixed'` 下这些模块与后端菜单按 `name` 合并（`mergeRoutesByName`）。
 * 新增模块：在 `./modules/` 下放一个 `.ts` 文件并 `export default` 一个
 * `AppRouteObject[]` 即可，无需注册。
 *
 * 目前 `modules/` 为空：Vue 侧的 `dashboard.ts` / `profile.ts` 要等对应 views
 * 在阶段 ⑦ 移植过来才能挂，否则路由会指向不存在的页面文件。
 */
const dynamicRouteFiles = import.meta.glob('./modules/**/*.ts', { eager: true });

/** 有权限校验的路由列表（对应 Vue 侧 accessRoutes） */
const accessRoutes: AppRouteObject[] = mergeRouteModules(dynamicRouteFiles);

export { accessRoutes };
