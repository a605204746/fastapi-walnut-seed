import type { ComponentRecordType } from '@vben-core/typings';

import type { Menu } from '@/api/core/menu';

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { generateRoutesByBackend, normalizeViewPath } from '@vben/utils';
import { describe, expect, it } from 'vitest';

import menuTree from '../../__fixtures__/backend-menu-tree.json';
import { backMenuToRouteObjects, createLazyPageMap } from '../back-menu';

/**
 * 门禁：后端菜单下发的 `component` 必须能解析到真实页面文件。
 *
 * 补的是 `back-menu.test.ts` 的盲区 —— 那个文件只验证「后端菜单能被正确转成
 * RR7 路由树」（component 字符串对不对），**不验证字符串能不能查到组件**。
 * 查不到时 `generateRoutesByBackend` 会静默退化成 `NotFoundComponent`
 * （只在控制台 error），页面上表现为 404，类型检查也发现不了。
 *
 * 进度口径见 `docs/frontend/06-React迁移-总计划与进度.md` §1.3：
 * A1 之前 14 个契约 component 只命中 1 个（`system/config/index`）。
 */

/** 与 router/access.ts 完全一致的 pageMap 构造方式（glob 路径按本文件位置调整） */
const viewModules = import.meta.glob<{ default: any }>('../../views/**/*.tsx');
const pageMap: ComponentRecordType = createLazyPageMap(viewModules);

/**
 * `createLazyPageMap` 保留 glob 的**原始相对 key**（如 `../../views/x/y.tsx`），
 * 归一化发生在 `generateRoutesByBackend` 内部。这里用同一个 `normalizeViewPath`
 * 复现它的 key 形态（`/system/config/index.tsx`），才能与后端下发的
 * `sys_menu.component` 直接对齐比较。
 */
const pageKeys = new Set(
  Object.keys(pageMap).map((key) => normalizeViewPath(key)),
);

function isImplemented(component: string): boolean {
  return pageKeys.has(`/${component}.tsx`);
}

/** 三后端 seed 完全一致的 14 条 sys_menu.component */
const CONTRACT_COMPONENTS = [
  'system/client/index',
  'system/config/index',
  'system/dept/index',
  'system/dict/data',
  'system/dict/index',
  'system/menu/index',
  'system/notice/index',
  'system/post/index',
  'system/role/authUser',
  'system/role/index',
  'system/user/authRole',
  'system/user/index',
  'monitor/logininfor/index',
  'monitor/operlog/index',
];

/**
 * 尚未落地的 component —— **这是一个进度计数器，不是永久的豁免清单**。
 *
 * 每迁完一个页面就必须从这里删掉对应条目；删漏了下面第 2 条用例会失败
 * （它断言「未命中集合」与本清单**精确相等**）。全部迁完时本数组应为空，
 * 届时第 2 条用例就等价于「14 个契约 component 全覆盖」。
 *
 * 按计划：A2 阶段清掉 8 个纯 CRUD 页，A3 阶段清掉 user/role/notice 3 个。
 */
const PENDING_COMPONENTS: string[] = [];

/** 哨兵组件：路由解析失败时 generateRoutesByBackend 会落到它身上 */
function NotFoundComponent() {
  return null;
}

function BasicLayout() {
  return null;
}

function flatten(routes: any[]): any[] {
  return routes.flatMap((route) => [
    route,
    ...(route.children ? flatten(route.children) : []),
  ]);
}

/** 走真实解析链路，返回退化成 NotFoundComponent 的路由 path */
async function resolveDegradedRoutes(): Promise<string[]> {
  const routes = await generateRoutesByBackend({
    fetchMenuListAsync: async () =>
      backMenuToRouteObjects(menuTree as unknown as Menu[]),
    forbiddenComponent: NotFoundComponent,
    layoutMap: { BasicLayout, NotFoundComponent },
    pageMap,
    // 后端模式不消费 routes（它是前端模式 generateRoutesByFrontend 的入参），
    // 但类型上是必填项，传空数组即可
    routes: [],
  });
  return flatten(routes)
    .filter((route) => route.Component === NotFoundComponent)
    .map((route) => String(route.path));
}

describe('pageMap 覆盖后端菜单契约', () => {
  it('已落地的页面能在 pageMap 中命中', () => {
    const implemented = CONTRACT_COMPONENTS.filter(isImplemented);
    // A1 交付：config（阶段⑤）+ 三个被后端菜单直接引用的占位/转发页
    // A2 进度：post（样板页）、logininfor、operlog、dept（树形）、dict（三层嵌套）、client、menu
    // A3 进度：user（含 4 个子弹窗 + ApiSwitch + DeptTree）
    expect(implemented).toEqual(CONTRACT_COMPONENTS);
  });

  it('走真实解析链路，退化成 404 的路由数 == 未落地的 component 数', async () => {
    /**
     * fixture 共 16 个节点 = 1 个 Layout + 1 个 ParentView + 14 个页面组件，
     * 前两者走 layoutMap，因此退化数应当精确等于未落地的页面数。
     * 多出来说明「明明建了文件却没解析上」（例如漏了 export default、
     * 或路径与 sys_menu.component 对不上），这是最容易漏的一类回归。
     */
    const degraded = await resolveDegradedRoutes();
    expect(degraded).toHaveLength(PENDING_COMPONENTS.length);
  });

  it('未落地的 component 与计划清单精确一致（迁完一个就删一行）', () => {
    const missing = CONTRACT_COMPONENTS.filter(
      (component) => !isImplemented(component),
    );
    expect(missing.sort()).toEqual([...PENDING_COMPONENTS].sort());
  });

  it('契约页面文件都有 export default（createLazyPageMap 的硬约束）', () => {
    /**
     * `router/access.ts` 的 glob 是 `../views/**\/*.tsx`，会把 `data.tsx`、
     * `*-modal.tsx`、`*-drawer.tsx` 这类**非路由目标**也收进 pageMap。它们永远不会
     * 被查到，所以不要求 default export。
     *
     * 真正的风险面是「后端 `sys_menu.component` 指向的文件」：
     * `React.lazy` 不校验未使用条目，缺 default export 只在**渲染时**才炸，
     * typecheck 也发现不了（清单 §16.4③）。所以这里只校验契约清单里已落地的文件。
     *
     * 顺带说明为什么 Vue 侧没有这个问题：Vue 的 glob 只匹配 `.vue` 后缀，
     * `data.tsx` 这类文件根本不在其中；React 全用 `.tsx`，glob 必然会过度收集。
     */
    const offenders: string[] = [];
    for (const component of CONTRACT_COMPONENTS) {
      const relativePath = `../../views/${component}.tsx`;
      // 未落地的文件由「PENDING 清单精确一致」那条用例负责
      if (!viewModules[relativePath]) continue;
      const absolute = fileURLToPath(new URL(relativePath, import.meta.url));
      const source = readFileSync(absolute, 'utf8');
      if (!/export\s+default\b/.test(source)) {
        offenders.push(relativePath);
      }
    }
    expect(
      offenders,
      `以下契约页面缺少 export default：${offenders.join(', ')}`,
    ).toEqual([]);
  });
});
