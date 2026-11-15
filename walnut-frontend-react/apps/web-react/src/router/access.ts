import type { GenerateMenuAndRoutesOptions } from '@vben-core/typings';

import { getPreferences } from '@vben/preferences';
import { generateAccessible } from '@vben/utils';

import { getAllMenusApi } from '@/api/core/menu';
import { $t } from '@/locales';
import { getMessageApi } from '@/services/message';

import { backMenuToRouteObjects } from './back-menu';

/**
 * 生成当前用户可访问的路由与菜单（对应 Vue 侧 router/access.ts 的 generateAccess）。
 *
 * 与 Vue 版的差异：不再传 `router` 实例。Vue 侧 `generateAccessible` 内部用
 * `router.addRoute/removeRoute` 重建 Root；React Router 7 改用 `router.patchRoutes()`，
 * 需要 router 实例，故那部分逻辑拆到 `./route-tree` 与 `./guard`，本函数只负责
 * 「拉菜单 → 转路由 → 生成菜单树」，是纯数据流。
 */
async function generateAccess(
  options: Omit<GenerateMenuAndRoutesOptions, 'fetchMenuListAsync'>,
) {
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
  });
}

export { generateAccess };
