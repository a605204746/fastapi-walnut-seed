import type {
  AppRouteObject,
  MaybeReadonlyRef,
  TabDefinition,
  TabMeta,
} from '@vben-core/typings';

import { preferences } from '@vben-core/preferences';
import {
  createStack,
  openWindow,
  Stack,
  startProgress,
  stopProgress,
} from '@vben-core/shared/utils';
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';

import { storeNamespace } from '../namespace';

/**
 * @zh_CN 多页签状态（对应 Vue 侧 packages/stores/src/modules/tabbar.ts）。
 *
 * 与 Vue 版的两处刻意差异：
 *
 * **① store 内不做路由跳转。** Vue 版的签名是 `closeTab(tab, router)` /
 * `_goToTab(tab, router)`，把 router 实例传进 store。React 侧 store 拿不到 router
 * 实例（`useNavigate` 只能在组件/hook 里取），也不该拿。所以凡是「关闭后需要跳转」
 * 的动作一律**返回目标 path（`null` = 不需要跳转）**，由调用方自己 `navigate()`，
 * store 因此是纯状态机、可直接单测。
 * 代价：Vue 版从 `router.currentRoute` 自己读当前激活页签，React 版必须由调用方
 * 通过 `currentTabKey` 显式传入（`closeTab` / `closeTabByKey` 的第二个参数）；
 * 不传即视为「关的不是当前页签」，只摘页签、不跳转。
 *
 * **② 不移植 KeepAlive 的 VNode 缓存。** Vue 版的 `cachedRoutes` /
 * `addCachedRoute` / `removeCachedRoute` / `getCachedRoutes` 存的是 Vue `VNode`，
 * React 没有对应物，留给阶段 ⑥-b 的 `<Activity>` + LRU 方案。
 * `cachedTabs` / `excludeCachedTabs` / `updateCacheTabs()` 是纯字符串集合
 * （由 `meta.keepAlive` 推导），照常移植，将来 `<Activity>` 直接消费。
 *
 * **持久化**：与 Vue 侧一致地**无条件**把 `tabs` / `visitHistory` 写进 sessionStorage。
 * `preferences.tabbar.persist` 在 Vue 侧同样不控制 pinia 插件（插件是无条件挂载的），
 * 真正生效的位置是 `layouts/src/basic/tabbar/tabbar.vue`：挂载时若为 `false` 就调
 * `closeOtherTabs(route)`，把刚水合回来的页签丢掉。React 侧保持同一语义——本 store
 * 不引入任何 persist 开关，由阶段 ⑥-a 的 tabbar 外壳组件在挂载时读
 * `preferences.tabbar.persist` 并调用 `closeOtherTabs(currentTab)`。
 */

interface TabbarState {
  /**
   * 当前打开的标签页列表缓存（由 `meta.keepAlive` 推导，供 `<Activity>` 消费）
   */
  cachedTabs: Set<string>;
  /**
   * 拖拽结束的索引，tabs-ui 靠它的变化触发滚动定位
   */
  dragEndIndex: number;
  /**
   * 需要排除缓存的标签页（refresh 期间临时写入）
   */
  excludeCachedTabs: Set<string>;
  /**
   * 标签右键菜单列表
   */
  menuList: string[];
  /**
   * 是否渲染路由视图，refresh 时短暂置 false
   */
  renderRouteView: boolean;
  /**
   * 当前打开的标签页列表
   */
  tabs: TabDefinition[];
  /**
   * 更新时间。深度监听 tabs 损耗性能，需要强制刷新时改这个
   */
  updateTime: number;
  /**
   * 访问历史（栈顶 = 最近访问），关闭当前页签时据此决定跳到哪。
   *
   * ⚠️ `Stack` 只有原地修改的 API（与 Vue 版一致），而 zustand 只在 `set()` 时
   * 通知订阅者、写持久化，所以每处修改后都要用同一个引用重新 `set({ visitHistory })`
   * 提交一次，否则 sessionStorage 里存的永远是上一次的快照。UI 不应订阅本字段
   * （引用不变，不会触发重渲染）。
   */
  visitHistory: Stack<string>;
}

/** Vue 版 getters 的对应物。Zustand 没有 getter，这里做成按需派生的方法。 */
interface TabbarSelectors {
  /** 固定页签，按 `meta.affixTabOrder` 升序 */
  affixTabs(): TabDefinition[];
  getCachedTabs(): string[];
  getExcludeCachedTabs(): string[];
  getMenuList(): string[];
  getTabByKey(key: string): TabDefinition | undefined;
  /** 固定页签（已排序）在最前，普通页签按打开顺序在后 */
  getTabs(): TabDefinition[];
}

interface TabbarActions {
  _bulkCloseByKeys(keys: string[]): void;
  _close(tab: TabDefinition): void;
  /** 第一个页签的跳转目标；没有页签时返回 null */
  _goToDefaultTab(): null | string;
  addTab(routeTab: TabDefinition): TabDefinition;
  /** 返回关闭后应跳转的 path */
  closeAllTabs(): null | string;
  closeLeftTabs(tab: TabDefinition): void;
  closeOtherTabs(tab: TabDefinition): void;
  closeRightTabs(tab: TabDefinition): void;
  /** 返回关闭后应跳转的 path；关的不是当前页签时返回 null */
  closeTab(tab: TabDefinition, currentTabKey?: null | string): null | string;
  closeTabByKey(key: string, currentTabKey?: null | string): null | string;
  openTabInNewWindow(tab: TabDefinition): void;
  pinTab(tab: TabDefinition): void;
  refresh(name: string): Promise<void>;
  refreshByName(name: string): Promise<void>;
  resetTabTitle(tab: TabDefinition): void;
  setAffixTabs(routes: AppRouteObject[]): void;
  setMenuList(list: string[]): void;
  setTabTitle(tab: TabDefinition, title: MaybeReadonlyRef<string>): void;
  setUpdateTime(): void;
  sortTabs(oldIndex: number, newIndex: number): void;
  toggleTabPin(tab: TabDefinition): void;
  unpinTab(tab: TabDefinition): void;
  updateCacheTabs(): void;
  $reset(): void;
}

export type TabbarStore = TabbarActions & TabbarSelectors & TabbarState;

/**
 * @zh_CN 访问历史记录最大数量
 */
const MAX_VISIT_HISTORY = 50;

function createInitialState(): TabbarState {
  return {
    cachedTabs: new Set<string>(),
    dragEndIndex: 0,
    excludeCachedTabs: new Set<string>(),
    menuList: [
      'close',
      'affix',
      'maximize',
      'reload',
      'open-in-new-window',
      'close-left',
      'close-right',
      'close-other',
      'close-all',
    ],
    renderRouteView: true,
    tabs: [],
    updateTime: Date.now(),
    visitHistory: createStack<string>(true, MAX_VISIT_HISTORY),
  };
}

/** Vue 版 getter `affixTabs` */
export function selectAffixTabs(state: TabbarState): TabDefinition[] {
  return state.tabs
    .filter((tab) => isAffixTab(tab))
    .toSorted(
      (a, b) => (a.meta?.affixTabOrder ?? 0) - (b.meta?.affixTabOrder ?? 0),
    );
}

/** Vue 版 getter `getTabs` */
export function selectTabs(state: TabbarState): TabDefinition[] {
  return [
    ...selectAffixTabs(state),
    ...state.tabs.filter((tab) => !isAffixTab(tab)),
  ];
}

export const useTabbarStore = create<TabbarStore>()(
  persist(
    (set, get) => ({
      ...createInitialState(),

      $reset: () => set(createInitialState()),

      _bulkCloseByKeys: (keys) => {
        const keySet = new Set(keys);
        set({
          tabs: get().tabs.filter(
            (item) => !keySet.has(getTabKeyFromTab(item)),
          ),
        });
        if (isVisitHistory()) {
          const { visitHistory } = get();
          visitHistory.remove(...keys);
          set({ visitHistory });
        }
        get().updateCacheTabs();
      },

      _close: (tab) => {
        if (isAffixTab(tab)) {
          return;
        }
        const index = get().tabs.findIndex((item) => equalTab(item, tab));
        if (index === -1) {
          return;
        }
        const tabs = [...get().tabs];
        tabs.splice(index, 1);
        set({ tabs });
      },

      _goToDefaultTab: () => {
        const firstTab = selectTabs(get())[0];
        return firstTab ? getTabNavigatePath(firstTab) : null;
      },

      addTab: (routeTab) => {
        let tab = cloneTab(routeTab);
        if (!tab.key) {
          tab.key = getTabKey(routeTab);
        }
        if (!isTabShown(tab)) {
          return tab;
        }

        const { tabs } = get();
        const tabIndex = tabs.findIndex((item) => equalTab(item, tab));

        if (tabIndex === -1) {
          const nextTabs = [...tabs];
          const maxCount = preferences.tabbar.maxCount;
          // 获取动态路由打开数，超过 0 即代表需要限制打开数
          const maxNumOfOpenTab = routeTab?.meta?.maxNumOfOpenTab ?? -1;
          if (
            maxNumOfOpenTab > 0 &&
            nextTabs.filter((item) => item.name === routeTab.name).length >=
              maxNumOfOpenTab
          ) {
            // 关闭第一个同名页签
            const index = nextTabs.findIndex(
              (item) => item.name === routeTab.name,
            );
            if (index !== -1) {
              nextTabs.splice(index, 1);
            }
          } else if (maxCount > 0 && nextTabs.length >= maxCount) {
            // 关闭第一个非固定页签
            const index = nextTabs.findIndex((item) => !isAffixTab(item));
            if (index !== -1) {
              nextTabs.splice(index, 1);
            }
          }
          nextTabs.push(tab);
          set({ tabs: nextTabs });
        } else {
          // 页面已经存在，不重复添加选项卡，只更新选项卡参数
          const currentTab = tabs[tabIndex];
          if (!currentTab) {
            return tab;
          }
          const curMeta = currentTab.meta;
          const mergedMeta: TabMeta = { ...curMeta, ...tab.meta };
          if (curMeta) {
            if (Reflect.has(curMeta, 'affixTab')) {
              mergedMeta.affixTab = curMeta.affixTab;
            }
            if (Reflect.has(curMeta, 'newTabTitle')) {
              mergedMeta.newTabTitle = curMeta.newTabTitle;
            }
          }
          const mergedTab: TabDefinition = {
            ...currentTab,
            ...tab,
            meta: mergedMeta,
          };
          tab = mergedTab;
          const nextTabs = [...tabs];
          nextTabs.splice(tabIndex, 1, mergedTab);
          set({ tabs: nextTabs });
        }

        get().updateCacheTabs();
        if (isVisitHistory() && tab.key) {
          const { visitHistory } = get();
          visitHistory.push(tab.key);
          set({ visitHistory });
        }
        return tab;
      },

      closeAllTabs: () => {
        const { tabs } = get();
        const affixTabs = tabs.filter((tab) => isAffixTab(tab));
        const newTabs =
          affixTabs.length > 0 ? affixTabs : [...tabs].splice(0, 1);
        set({ tabs: newTabs });
        if (isVisitHistory()) {
          const { visitHistory } = get();
          visitHistory.retain(newTabs.map((item) => getTabKeyFromTab(item)));
          set({ visitHistory });
        }
        const target = get()._goToDefaultTab();
        get().updateCacheTabs();
        return target;
      },

      closeLeftTabs: (tab) => {
        const { tabs } = get();
        const index = tabs.findIndex((item) => equalTab(item, tab));
        if (index < 1) {
          return;
        }
        get()._bulkCloseByKeys(
          tabs
            .slice(0, index)
            .filter((item) => !isAffixTab(item))
            .map((item) => getTabKeyFromTab(item)),
        );
      },

      closeOtherTabs: (tab) => {
        const keepKey = getTabKeyFromTab(tab);
        get()._bulkCloseByKeys(
          get()
            .tabs.filter(
              (item) => !isAffixTab(item) && getTabKeyFromTab(item) !== keepKey,
            )
            .map((item) => getTabKeyFromTab(item)),
        );
      },

      closeRightTabs: (tab) => {
        const { tabs } = get();
        const index = tabs.findIndex((item) => equalTab(item, tab));
        if (index === -1 || index >= tabs.length - 1) {
          return;
        }
        get()._bulkCloseByKeys(
          tabs
            .slice(index + 1)
            .filter((item) => !isAffixTab(item))
            .map((item) => getTabKeyFromTab(item)),
        );
      },

      closeTab: (tab, currentTabKey) => {
        const tabKey = getTabKeyFromTab(tab);

        // 关闭的不是当前激活页签（或调用方没告知当前页签）：只摘页签，不跳转
        if (currentTabKey !== tabKey) {
          get()._close(tab);
          get().updateCacheTabs();
          if (isVisitHistory()) {
            const { visitHistory } = get();
            visitHistory.remove(tabKey);
            set({ visitHistory });
          }
          return null;
        }

        if (selectTabs(get()).length <= 1) {
          console.error('Failed to close the tab; only one tab remains open.');
          return null;
        }

        if (isVisitHistory()) {
          const { visitHistory } = get();
          visitHistory.remove(currentTabKey);
          get()._close(tab);

          let previousTab: TabDefinition | undefined;
          for (;;) {
            const previousTabKey = visitHistory.pop();
            if (!previousTabKey) {
              break;
            }
            previousTab = get().getTabByKey(previousTabKey);
            if (previousTab) {
              break;
            }
          }
          set({ visitHistory });
          return previousTab
            ? getTabNavigatePath(previousTab)
            : get()._goToDefaultTab();
        }

        // 未开启访问历史记录，直接跳转下一个或上一个tab
        const tabs = selectTabs(get());
        const index = tabs.findIndex(
          (item) => getTabKeyFromTab(item) === currentTabKey,
        );
        const target = tabs[index + 1] ?? tabs[index - 1];
        if (!target) {
          return null;
        }
        get()._close(tab);
        return getTabNavigatePath(target);
      },

      closeTabByKey: (key, currentTabKey) => {
        const originKey = decodeURIComponent(key);
        const index = get().tabs.findIndex(
          (item) => getTabKeyFromTab(item) === originKey,
        );
        if (index === -1) {
          return null;
        }
        const tab = get().tabs[index];
        return tab ? get().closeTab(tab, currentTabKey) : null;
      },

      affixTabs: () => selectAffixTabs(get()),

      getCachedTabs: () => [...get().cachedTabs],

      getExcludeCachedTabs: () => [...get().excludeCachedTabs],

      getMenuList: () => get().menuList,

      getTabByKey: (key) =>
        selectTabs(get()).find((item) => getTabKeyFromTab(item) === key),

      getTabs: () => selectTabs(get()),

      openTabInNewWindow: (tab) => {
        // Vue 版走 router.resolve() 拿 href（会带上 router base）。React 侧 store
        // 不持有 router，直接按当前 origin 解析；应用若配了 basename 需在外壳层处理。
        openWindow(new URL(tab.fullPath || tab.path, location.href).href, {
          target: '_blank',
        });
      },

      pinTab: (tab) => {
        const { tabs } = get();
        const index = tabs.findIndex((item) => equalTab(item, tab));
        if (index === -1) {
          return;
        }
        const oldTab = tabs[index];
        const nextTabs = [...tabs];
        nextTabs.splice(index, 1, {
          ...tab,
          meta: { ...tab.meta, affixTab: true, title: oldTab?.meta?.title },
        });
        set({ tabs: nextTabs });
        // 移到固定区末尾。Vue 版是在**未排序**的 `tabs.filter(isAffixTab)` 里找下标，
        // 不能用 affixTabs()（那个按 affixTabOrder 排过序，下标会对不上）。
        const newIndex = get()
          .tabs.filter((item) => isAffixTab(item))
          .findIndex((item) => equalTab(item, tab));
        get().sortTabs(index, newIndex);
      },

      refresh: async (name) => {
        const excluded = new Set(get().excludeCachedTabs);
        excluded.add(name);
        set({ excludeCachedTabs: excluded, renderRouteView: false });
        startProgress();

        await new Promise((resolve) => setTimeout(resolve, 200));

        const restored = new Set(get().excludeCachedTabs);
        restored.delete(name);
        set({ excludeCachedTabs: restored, renderRouteView: true });
        stopProgress();
      },

      refreshByName: async (name) => {
        const excluded = new Set(get().excludeCachedTabs);
        excluded.add(name);
        set({ excludeCachedTabs: excluded });

        await new Promise((resolve) => setTimeout(resolve, 200));

        const restored = new Set(get().excludeCachedTabs);
        restored.delete(name);
        set({ excludeCachedTabs: restored });
      },

      resetTabTitle: (tab) => {
        // Vue 版原样保留：入参已有 newTabTitle 就直接 return，也就是说这个方法
        // 只在「本来就没有自定义标题」时才走到下面的清除分支——看起来是写反了，
        // 但 tabs-ui 依赖现有行为，先不擅自修正。
        if (tab?.meta?.newTabTitle) {
          return;
        }
        const { tabs } = get();
        const index = tabs.findIndex((item) => equalTab(item, tab));
        if (index === -1) {
          return;
        }
        const findTab = tabs[index];
        if (!findTab) {
          return;
        }
        const nextTabs = [...tabs];
        nextTabs[index] = {
          ...findTab,
          meta: { ...findTab.meta, newTabTitle: undefined },
        };
        set({ tabs: nextTabs });
        get().updateCacheTabs();
      },

      setAffixTabs: (routes) => {
        for (const route of routes) {
          get().addTab({
            ...routeToTab(route),
            meta: { ...route.meta, affixTab: true },
          });
        }
      },

      setMenuList: (list) => set({ menuList: list }),

      setTabTitle: (tab, title) => {
        const { tabs } = get();
        const index = tabs.findIndex((item) => equalTab(item, tab));
        if (index === -1) {
          return;
        }
        const findTab = tabs[index];
        if (!findTab) {
          return;
        }
        const nextTabs = [...tabs];
        nextTabs[index] = {
          ...findTab,
          meta: { ...findTab.meta, newTabTitle: title },
        };
        set({ tabs: nextTabs });
        get().updateCacheTabs();
      },

      setUpdateTime: () => set({ updateTime: Date.now() }),

      sortTabs: (oldIndex, newIndex) => {
        const currentTab = get().tabs[oldIndex];
        if (!currentTab) {
          return;
        }
        const tabs = [...get().tabs];
        tabs.splice(oldIndex, 1);
        tabs.splice(newIndex, 0, currentTab);
        set({ dragEndIndex: get().dragEndIndex + 1, tabs });
      },

      toggleTabPin: (tab) => {
        if (tab?.meta?.affixTab ?? false) {
          get().unpinTab(tab);
        } else {
          get().pinTab(tab);
        }
      },

      unpinTab: (tab) => {
        const { tabs } = get();
        const index = tabs.findIndex((item) => equalTab(item, tab));
        if (index === -1) {
          return;
        }
        const oldTab = tabs[index];
        const nextTabs = [...tabs];
        nextTabs.splice(index, 1, {
          ...tab,
          meta: { ...tab.meta, affixTab: false, title: oldTab?.meta?.title },
        });
        set({ tabs: nextTabs });
        // 移到固定区之后，也就是活动区的第一个位置
        get().sortTabs(
          index,
          get().tabs.filter((item) => isAffixTab(item)).length,
        );
      },

      updateCacheTabs: () => {
        const cacheMap = new Set<string>();
        for (const tab of get().tabs) {
          if (!tab.meta?.keepAlive) {
            continue;
          }
          (tab.matched ?? []).forEach((item, i) => {
            if (i > 0 && item.name) {
              cacheMap.add(item.name);
            }
          });
          if (tab.name) {
            cacheMap.add(tab.name);
          }
        }
        set({ cachedTabs: cacheMap });
      },
    }),
    {
      name: `${storeNamespace.prefix}-core-tabbar`,
      storage: createJSONStorage(() => sessionStorage),
      // 与 Vue 侧 persist.pick 一致：tabs 不进 localStorage，只进 sessionStorage
      partialize: (state) => ({
        tabs: state.tabs,
        visitHistory: state.visitHistory,
      }),
      version: 1,
      merge: (persisted, current) => ({
        ...current,
        ...(persisted as object),
        visitHistory: reviveVisitHistory(
          (persisted as { visitHistory?: unknown } | null)?.visitHistory,
        ),
      }),
    },
  ),
);

/**
 * `Stack` 实例经 JSON 序列化后只剩 `{dedup, items, maxSize}` 三个普通属性，
 * 方法和 getter 全丢，rehydrate 时必须重建
 * （Vue 版在 `persist.serializer.deserialize` 里做的是同一件事）。
 */
function reviveVisitHistory(raw: unknown): Stack<string> {
  if (raw instanceof Stack) {
    return raw;
  }
  const stack = createStack<string>(true, MAX_VISIT_HISTORY);
  const items = (raw as { items?: unknown } | null | undefined)?.items;
  if (Array.isArray(items)) {
    stack.push(...(items as string[]));
  }
  return stack;
}

/**
 * @zh_CN 克隆路由,防止路由被修改
 */
function cloneTab(route: TabDefinition): TabDefinition {
  if (!route) {
    return route;
  }
  const { matched, meta, ...opt } = route;
  return {
    ...opt,
    matched: matched?.map((item) => ({
      meta: item.meta,
      name: item.name,
      path: item.path,
    })),
    meta: meta ? { ...meta } : undefined,
  };
}

/**
 * @zh_CN 是否是固定标签页
 */
function isAffixTab(tab: TabDefinition) {
  return tab?.meta?.affixTab ?? false;
}

/**
 * @zh_CN 是否显示标签
 */
function isTabShown(tab: TabDefinition) {
  const matched = tab?.matched ?? [];
  return (
    !tab?.meta?.hideInTab && matched.every((item) => !item.meta?.hideInTab)
  );
}

/**
 * `getTabKey` 的入参：`TabDefinition` / `AppRouteObject` / 任何带这几个字段的对象
 */
interface TabKeySource {
  fullPath?: string;
  meta?: { fullPathKey?: boolean };
  path?: string;
  query?: Record<string, string | string[]>;
}

/**
 * 从route获取tab页的key
 */
function getTabKey(tab: TabKeySource): string {
  const {
    fullPath,
    path = '',
    meta: { fullPathKey } = {},
    query = {},
  } = tab;
  // pageKey可能是数组（查询参数重复时可能出现）
  const pageKey = Array.isArray(query.pageKey)
    ? query.pageKey[0]
    : query.pageKey;
  let rawKey: string;
  if (pageKey) {
    rawKey = pageKey;
  } else {
    rawKey = fullPathKey === false ? path : (fullPath ?? path);
  }
  try {
    return decodeURIComponent(rawKey);
  } catch {
    return rawKey;
  }
}

/**
 * @zh_CN 是否开启访问历史记录
 */
function isVisitHistory() {
  return preferences.tabbar.visitHistory;
}

/**
 * 从tab获取tab页的key
 * 如果tab没有key,那么就从route获取key
 */
function getTabKeyFromTab(tab: TabDefinition): string {
  return tab.key ?? getTabKey(tab);
}

/**
 * 关闭/跳转动作回传给调用方的目标地址，可直接喂给 `navigate()`。
 * 与 Vue 版 `openTabInNewWindow` 的 `tab.fullPath || tab.path` 取法一致。
 */
function getTabNavigatePath(tab: TabDefinition): string {
  return tab.fullPath || tab.path;
}

/**
 * 比较两个tab是否相等
 */
function equalTab(a: TabDefinition, b: TabDefinition) {
  return getTabKeyFromTab(a) === getTabKeyFromTab(b);
}

function routeToTab(route: AppRouteObject): TabDefinition {
  return {
    key: getTabKey(route),
    meta: route.meta,
    name: route.name,
    path: route.path ?? '',
  };
}

export { getTabKey };
