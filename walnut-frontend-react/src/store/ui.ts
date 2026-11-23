import { create } from 'zustand';

interface UiState {
  /** 内容区最大化（隐藏侧边栏/顶栏/页签），对应页签栏「最大化」按钮 */
  maximized: boolean;
  /** 内容区重挂载计数：顶栏刷新 / 页签右键刷新共用（Vue 侧靠 Activity 缓存剔除实现） */
  refreshCounter: number;
  revisionByPath: Record<string, number>;
  refresh: () => void;
  toggleMaximized: () => void;
}

/**
 * 外壳级 UI 状态。
 *
 * Vue 侧「刷新当前页签」靠 `<Activity>` 缓存把页面剔掉再重挂；React 侧 `<Activity>`
 * 属 B4 未落地，这里用「重挂载计数」等价实现：BasicLayout 把它作为内容区的 `key`。
 * 「最大化」在 Vue 侧是 tabbar store 的 `maximize` 标记，这里独立成 ui store，
 * 由 BasicLayout 消费（隐藏 sider/header/tabbar）。
 */
export const useUiStore = create<UiState>((set) => ({
  maximized: false,
  revisionByPath: {},
  refresh: () => set((state) => {
    const path = window.location.pathname + window.location.search;
    return { refreshCounter: state.refreshCounter + 1, revisionByPath: { ...state.revisionByPath, [path]: (state.revisionByPath[path] ?? 0) + 1 } };
  }),
  refreshCounter: 0,
  toggleMaximized: () => set((state) => ({ maximized: !state.maximized })),
}));
