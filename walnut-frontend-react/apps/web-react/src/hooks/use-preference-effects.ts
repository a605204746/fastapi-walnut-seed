import { useEffect } from 'react';

import { usePreferences } from '@vben/preferences';
import { useMatches } from 'react-router';

import { mergeMatchMeta } from '@/router/route-tree';
import { useAuthStore } from '@/store/auth';
import { useUiStore } from '@/store/ui';

/**
 * 文档级偏好消费端（对应 Vue 侧散落在 app.vue / use-preferences / widgets 里的副作用）。
 *
 * 此前这些偏好「写进了 store 但没人读」，开关是死的；这里把消费端补上：
 * - `app.dynamicTitle`：路由变化时更新 document.title
 * - `app.colorGrayMode` / `colorWeakMode`：html 根节点 filter（灰度 / 色弱）
 * - `shortcutKeys.globalLogout`：Alt+Q 退出登录
 * - `shortcutKeys.globalEscape`：Esc 退出内容最大化
 *
 * 未接（依赖尚未迁移的组件）：globalSearch(Ctrl K) / globalLockScreen(Alt L) /
 * timezone / enableCheckUpdates —— 对应 widget 属 B1，接了也是空操作。
 */
export function usePreferenceEffects() {
  const { preferences } = usePreferences();
  const matches = useMatches();
  const logout = useAuthStore((state) => state.logout);
  const maximized = useUiStore((state) => state.maximized);
  const toggleMaximized = useUiStore((state) => state.toggleMaximized);

  const { app, shortcutKeys } = preferences;
  const meta = mergeMatchMeta(matches);

  // 动态标题
  useEffect(() => {
    if (!app.dynamicTitle) return;
    const title = (meta?.title as string) || '';
    document.title = title ? `${title} - ${app.name}` : app.name;
  }, [app.dynamicTitle, app.name, meta?.title]);

  // 灰色 / 色弱模式
  useEffect(() => {
    const filters: string[] = [];
    if (app.colorGrayMode) filters.push('grayscale(100%)');
    if (app.colorWeakMode) filters.push('invert(80%)');
    document.documentElement.style.filter = filters.join(' ');
  }, [app.colorGrayMode, app.colorWeakMode]);

  // 全局快捷键
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (!shortcutKeys.enable) return;
      const alt = e.altKey;
      if (alt && shortcutKeys.globalLogout && e.key.toLowerCase() === 'q') {
        e.preventDefault();
        void logout();
      }
      if (shortcutKeys.globalEscape && e.key === 'Escape' && maximized) {
        toggleMaximized();
      }
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [shortcutKeys, logout, maximized, toggleMaximized]);
}
