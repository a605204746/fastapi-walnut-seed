import type { MaybeReadonlyRef } from './helper';
import type { RouteMeta } from './react-router';

/**
 * 标签页匹配链上的一节（对应 Vue 侧 `RouteRecordNormalized` 的最小子集）。
 *
 * tabbar store 只用到 `meta` / `name` / `path`，故摊平成结构化形状，
 * 让 store 层不必依赖 react-router 的类型。
 */
interface TabMatchedRoute {
  meta?: RouteMeta;
  name?: string;
  path: string;
}

/**
 * 标签页 meta。
 *
 * Vue 侧的 `RouteMeta` 继承了 vue-router 自带的
 * `extends Record<string | number | symbol, unknown>` 索引签名，所以
 * `newTabTitle` 这种「只在运行时写入、从未被声明过」的字段也能通过类型检查；
 * React 侧的 `RouteMeta` 是封闭接口，必须在这里显式补齐。
 *
 * `title` 放宽为可选：页签可能由后端菜单派生，标题在 i18n 解析前不一定存在。
 */
interface TabMeta extends Omit<RouteMeta, 'title'> {
  /**
   * 运行时覆盖的标题，优先级高于 `title`
   * （tabs-ui 取 `newTabTitle || title || name`）
   */
  newTabTitle?: MaybeReadonlyRef<string>;
  title?: string;
}

/**
 * 标签页定义（对应 Vue 侧 `@core/base/typings/src/tabs.ts` 的 `TabDefinition`）。
 *
 * Vue 版是 `extends RouteLocationNormalized`，React 版摊平为自包含结构：
 * store 层不依赖路由库，由外壳负责把 `useLocation()` + `useMatches()` 的结果
 * 组装成 `TabDefinition`。
 *
 * ⚠️ `fullPath` 必须是 `pathname + search`：关闭页签时 store 回传它作为
 * `navigate()` 的目标，缺了 query 会让带参数的页签跳回无参地址。
 */
interface TabDefinition {
  fullPath?: string;
  hash?: string;
  /**
   * 标签页唯一 key，未提供时由 `getTabKey` 从 `query.pageKey` / `fullPath` / `path` 推导
   */
  key?: string;
  /**
   * 匹配到的路由链（由浅到深），`updateCacheTabs` 靠它收集父级 name
   */
  matched?: TabMatchedRoute[];
  meta?: TabMeta;
  name?: string;
  params?: Record<string, string | string[]>;
  path: string;
  query?: Record<string, string | string[]>;
  redirectedFrom?: string;
}

export type { TabDefinition, TabMatchedRoute, TabMeta };
