import { useMemo } from 'react';

import { diff } from '@/core/shared/utils';
import { useStore } from 'zustand';

import { preferencesManager, preferencesStore } from './preferences';
import { isDarkTheme } from './update-css-variables';

/**
 * 偏好设置的派生视图。
 *
 * 与 Vue 版的差异：Vue 返回一堆 `ComputedRef`（JS 里要 `.value`，模板里自动解包），
 * React 这里直接返回**普通值**，并订阅 store —— 任一偏好变化时组件自动重渲染。
 * 键名与 Vue 版逐一对应，便于把调用点机械搬过来（去掉 `.value` 即可）。
 */
function usePreferences() {
  const { custom: customPreferences, preferences } =
    useStore(preferencesStore);

  return useMemo(() => {
    const initialPreferences = preferencesManager.getInitialPreferences();
    const initialCustomPreferences =
      preferencesManager.getInitialCustomPreferences();
    const preferencesExtension = preferencesManager.getPreferencesExtension();

    /** 计算偏好设置的变化 */
    const diffPreference = diff(initialPreferences, preferences);
    const diffCustomPreference = diff(
      initialCustomPreferences,
      customPreferences,
    );

    const appPreferences = preferences.app;
    const shortcutKeysPreferences = preferences.shortcutKeys;

    /**
     * 判断是否为暗黑模式
     */
    const isDark = isDarkTheme(preferences.theme.mode);

    const locale = appPreferences.locale;
    const isMobile = appPreferences.isMobile;
    const theme = isDark ? 'dark' : 'light';

    /**
     * 布局方式
     */
    const layout = isMobile ? 'sidebar-nav' : appPreferences.layout;

    /**
     * 是否显示顶栏
     */
    const isShowHeaderNav = preferences.header.enable;

    /**
     * 是否全屏显示content，不需要侧边、底部、顶部、tab区域
     */
    const isFullContent = appPreferences.layout === 'full-content';

    /**
     * 是否侧边导航模式
     */
    const isSideNav = appPreferences.layout === 'sidebar-nav';

    /**
     * 是否侧边混合模式
     */
    const isSideMixedNav = appPreferences.layout === 'sidebar-mixed-nav';

    /**
     * 是否为头部导航模式
     */
    const isHeaderNav = appPreferences.layout === 'header-nav';

    /**
     * 是否为头部混合导航模式
     */
    const isHeaderMixedNav = appPreferences.layout === 'header-mixed-nav';

    /**
     * 是否为顶部通栏+侧边导航模式
     */
    const isHeaderSidebarNav = appPreferences.layout === 'header-sidebar-nav';

    /**
     * 是否为混合导航模式
     */
    const isMixedNav = appPreferences.layout === 'mixed-nav';

    /**
     * 是否包含侧边导航模式
     */
    const isSideMode =
      isMixedNav ||
      isSideMixedNav ||
      isSideNav ||
      isHeaderMixedNav ||
      isHeaderSidebarNav;

    const sidebarCollapsed = preferences.sidebar.collapsed;

    /**
     * 是否开启keep-alive
     * 在tabs可见以及开启keep-alive的情况下才开启
     */
    const keepAlive = preferences.tabbar.enable && preferences.tabbar.keepAlive;

    /**
     * 登录注册页面布局是否为左侧
     */
    const authPanelLeft = appPreferences.authPageLayout === 'panel-left';

    /**
     * 登录注册页面布局是否为右侧
     */
    const authPanelRight = appPreferences.authPageLayout === 'panel-right';

    /**
     * 登录注册页面布局是否为中间
     */
    const authPanelCenter = appPreferences.authPageLayout === 'panel-center';

    /**
     * 内容是否已经最大化
     * 排除 full-content模式
     */
    const contentIsMaximize =
      preferences.header.hidden &&
      preferences.sidebar.hidden &&
      !isFullContent;

    /**
     * 是否启用全局搜索快捷键
     */
    const { enable: shortcutEnable } = shortcutKeysPreferences;
    const globalSearchShortcutKey =
      shortcutEnable && shortcutKeysPreferences.globalSearch;

    /**
     * 是否启用全局注销快捷键
     */
    const globalLogoutShortcutKey =
      shortcutEnable && shortcutKeysPreferences.globalLogout;

    /**
     * 是否启用全局退出快捷键
     */
    const globalEscapeShortcutKey =
      shortcutEnable && shortcutKeysPreferences.globalEscape;

    const globalLockScreenShortcutKey =
      shortcutEnable && shortcutKeysPreferences.globalLockScreen;

    /**
     * 偏好设置按钮位置
     */
    const resolvePreferencesButtonPosition = () => {
      const { enablePreferences, preferencesButtonPosition } = preferences.app;
      // 如果没有启用偏好设置按钮
      if (!enablePreferences) {
        return {
          fixed: false,
          header: false,
          userDropdown: false,
        };
      }

      const { header, sidebar } = preferences;
      const maximized = header.hidden && sidebar.hidden;

      const isHeaderPosition = preferencesButtonPosition === 'header';
      const isUserDropdownPosition =
        preferencesButtonPosition === 'user-dropdown';

      // 如果设置了固定位置
      if (preferencesButtonPosition !== 'auto') {
        return {
          fixed: preferencesButtonPosition === 'fixed',
          header: isHeaderPosition,
          userDropdown: isUserDropdownPosition,
        };
      }

      // 如果是全屏模式或者没有固定在顶部，
      const fixed =
        maximized || isFullContent || isMobile || !isShowHeaderNav;

      return {
        fixed,
        header: !fixed,
        userDropdown: !fixed && isUserDropdownPosition,
      };
    };

    return {
      app: appPreferences,
      authPanelCenter,
      authPanelLeft,
      authPanelRight,
      contentIsMaximize,
      customPreferences,
      diffCustomPreference,
      diffPreference,
      globalEscapeShortcutKey,
      globalLockScreenShortcutKey,
      globalLogoutShortcutKey,
      globalSearchShortcutKey,
      isDark,
      isFullContent,
      isHeaderMixedNav,
      isHeaderNav,
      isHeaderSidebarNav,
      isMixedNav,
      isMobile,
      isSideMixedNav,
      isSideMode,
      isSideNav,
      keepAlive,
      layout,
      locale,
      preferences,
      preferencesButtonPosition: resolvePreferencesButtonPosition(),
      preferencesExtension,
      sidebarCollapsed,
      theme,
    };
  }, [customPreferences, preferences]);
}

export { usePreferences };
