import type { ReactNode } from 'react';

import type { MenuRecordRaw, RouteHandle, RouteMeta } from '@vben-core/typings';

import { useEffect, useMemo, useState } from 'react';

import { usePreferences } from '@vben/preferences';
import { useAccessStore } from '@vben/stores';
import { findMenuByPath, setObjToUrlParams } from '@vben/utils';
import { isHttpUrl } from '@vben-core/shared/utils';
import { VbenIcon } from '@vben-core/shadcn-ui';
import { Breadcrumb, ConfigProvider, Layout, Menu } from 'antd';
import { useLocation, useMatches, useNavigate } from 'react-router';

import { VbenMenu } from '@/adapter/menu';
import { VbenTabs } from '@/adapter/tabs';
import { $t } from '@/locales';
import { mergeMatchMeta } from '@/router/route-tree';
import { useUiStore } from '@/store/ui';

import { UserDropdown } from './BasicLayout';
import { ContentArea } from './ContentArea';
import { TenantToggle } from './widgets/tenant-toggle';
import { HeaderTools, SearchButton } from './widgets/header-tools';
import { useShellTheme } from './use-shell-theme';
import { useHeaderScroll } from './use-header-scroll';
import { FullscreenToggle, LanguageToggle, ThemeToggle } from './widgets';
import { PreferencesButton } from './widgets/preferences';

const { Header, Sider } = Layout;

/** 第一个叶子菜单（混合导航点顶层时跳转到其首个子页） */
function firstLeaf(menu: MenuRecordRaw): MenuRecordRaw {
  if (!menu.children?.length) return menu;
  return firstLeaf(menu.children[0]!);
}

/**
 * 非默认布局的功能版外壳（B5 功能等价，自绘 1:1 仍属 B 阶段）。
 * 支持 header-nav / header-sidebar-nav / mixed-nav / header-mixed-nav /
 * sidebar-mixed-nav / full-content 六种；默认 sidebar-nav 由 BasicLayout 原渲染承担。
 */
export function MultiLayout() {
  const headerOffset = useHeaderScroll();
  const { isDark, layout, preferences, preferencesButtonPosition } = usePreferences();
  const navigate = useNavigate();
  const location = useLocation();
  const matches = useMatches();
  const accessMenus = useAccessStore((state) => state.accessMenus);
  const maximized = useUiStore((state) => state.maximized);

  const [openKeys, setOpenKeys] = useState<string[]>([]);

  const meta = useMemo(() => mergeMatchMeta(matches), [matches]);
  const activePath = (meta?.activePath as string) || location.pathname;

  // 混合导航：当前激活的顶层菜单
  const activeRoot = useMemo(() => {
    const contains = (menu: MenuRecordRaw): boolean => {
      if (menu.path === activePath) return true;
      return (menu.children ?? []).some(contains);
    };
    return accessMenus.find(contains) ?? accessMenus[0];
  }, [accessMenus, activePath, preferences.navigation.accordion]);

  const isMixed = layout === 'mixed-nav' || layout === 'header-mixed-nav';
  const isDoubleCol = layout === 'sidebar-mixed-nav' || layout === 'header-mixed-nav';
  const sideMenus = (isMixed && preferences.navigation.split) || isDoubleCol ? (activeRoot?.children ?? []) : accessMenus;
  const horizontalMenus = isMixed
    ? accessMenus.map((root) => ({ ...root, children: undefined }))
    : accessMenus;

  useEffect(() => {
    const parents = findMenuByPath(accessMenus, activePath)?.parents ?? [];
    if (parents.length === 0) return;
    if (preferences.navigation.accordion) { setOpenKeys(parents); return; }
    setOpenKeys((prev) =>
      parents.every((key) => prev.includes(key)) ? prev : [...new Set([...prev, ...parents])],
    );
  }, [accessMenus, activePath, preferences.navigation.accordion]);

  function handleMenuSelect(menu: MenuRecordRaw) {
    if (isHttpUrl(menu.path)) {
      window.open(menu.path, '_blank');
      return;
    }
    const target = (isMixed || isDoubleCol) && menu.children?.length ? firstLeaf(menu) : menu;
    const { query } = target;
    navigate(
      query && Object.keys(query).length > 0 ? setObjToUrlParams(target.path, query) : target.path,
    );
  }

  function handleOpenChange(keys: string[]) {
    if (!preferences.navigation.accordion) {
      setOpenKeys(keys);
      return;
    }
    const added = keys.filter((key) => !openKeys.includes(key));
    setOpenKeys(added.length > 0 ? [added[added.length - 1]!] : keys);
  }

  const semiDark = !isDark;
  const semiDarkTheme = useShellTheme();

  const showHeader = preferences.header.enable && !preferences.header.hidden && !maximized;
  const showTabbar = preferences.tabbar.enable && !maximized && layout !== 'full-content';
  const showSider =
    preferences.sidebar.enable &&
    !preferences.sidebar.hidden &&
    !maximized &&
    ['mixed-nav', 'sidebar-mixed-nav', 'header-mixed-nav', 'header-sidebar-nav'].includes(layout);

  const logo = (compact = false) => (
    <div className="flex h-[50px] shrink-0 items-center gap-2 overflow-hidden px-3">
      <img alt="logo" className="size-8 shrink-0" src={preferences.logo.source} />
      {compact ? null : (
        <span className="text-foreground truncate text-lg font-semibold">{preferences.app.name}</span>
      )}
    </div>
  );

  const rightWidgets = (
    <div className="flex shrink-0 items-center">
      <TenantToggle />{preferences.widget.globalSearch ? <SearchButton /> : null}
              {preferences.app.enablePreferences && preferencesButtonPosition.header ? <PreferencesButton /> : null}
      {preferences.widget.themeToggle ? <ThemeToggle /> : null}
      {preferences.widget.languageToggle ? <LanguageToggle /> : null}
      {preferences.widget.fullscreen ? <FullscreenToggle /> : null}
      <HeaderTools /><UserDropdown />
    </div>
  );

  const breadcrumbItems = useMemo(() => {
    const { enable, hideOnlyOne, showHome, showIcon } = preferences.breadcrumb;
    if (!enable) return [];
    const metas = matches
      .map((m) => (m.handle as RouteHandle | undefined)?.meta)
      .filter((i): i is RouteMeta => !!i?.title && !i.hideInBreadcrumb);
    if (hideOnlyOne && metas.length <= 1) return [];
    const items: { title: ReactNode }[] = [];
    if (showHome) {
      items.push({
        title: (
          <span className="text-primary flex cursor-pointer items-center" onClick={() => navigate('/')}>
            <VbenIcon className="size-3.5" icon="mdi:home-outline" />
          </span>
        ),
      });
    }
    metas.forEach((item, index) => {
      const isLast = index === metas.length - 1;
      items.push({
        title: (
          <span className={isLast ? 'flex items-center gap-1' : 'text-primary flex cursor-pointer items-center gap-1'}>
            {showIcon && item.icon ? <VbenIcon className="size-3.5" icon={item.icon as string} /> : null}
            {$t(item.title)}
          </span>
        ),
      });
    });
    return items;
  }, [matches, preferences.breadcrumb, navigate]);

  const inlineMenu = (menus: MenuRecordRaw[]) => (
    <VbenMenu
      mode="inline"
      menus={menus}
      onMenuSelect={handleMenuSelect}
      onOpenChange={handleOpenChange}
      openKeys={openKeys}
      selectedKeys={[activePath]}
      style={{ background: 'transparent', borderInlineEnd: 'none' }}
    />
  );

  const horizontalMenu = (
    <VbenMenu
      mode="horizontal"
      menus={horizontalMenus}
      onClick={({ key }) => {
        const root = accessMenus.find((menu) => menu.path === key);
        if (isMixed && root) handleMenuSelect(root);
      }}
      onMenuSelect={isMixed ? undefined : handleMenuSelect}
      selectedKeys={[isMixed ? (activeRoot?.path ?? '') : activePath]}
      style={{ background: 'transparent', borderBottom: 'none', flex: 'auto', minWidth: 0, justifyContent: preferences.header.menuAlign === 'center' ? 'center' : preferences.header.menuAlign === 'end' ? 'flex-end' : 'flex-start' }}
    />
  );

  const doubleColSider = (
    <div className={`${preferences.theme.semiDarkSidebar && semiDark ? 'dark ' : ''}bg-sidebar border-border flex h-full shrink-0 border-r`}>
      <div className="border-border flex flex-col items-center border-r" style={{ width: preferences.sidebar.mixedWidth }}>
        {preferences.logo.enable && layout === 'sidebar-mixed-nav' ? logo(true) : null}
        <Menu
          inlineCollapsed
          items={accessMenus.map((root) => ({
            icon: root.icon ? <VbenIcon icon={root.icon} /> : undefined,
            key: root.path,
            label: root.name,
          }))}
          mode="inline"
          onSelect={({ key }) => {
            const root = accessMenus.find((m) => m.path === key);
            if (root) handleMenuSelect(root);
          }}
          selectedKeys={[activeRoot?.path ?? '']}
          style={{ background: 'transparent', borderInlineEnd: 'none', width: '100%' }}
        />
      </div>
      <div className="flex min-h-0 flex-1 flex-col" style={{ width: preferences.sidebar.width }}>
        <div className="flex-center h-12 shrink-0 px-3">
          <span className="text-foreground truncate text-lg font-semibold">{activeRoot?.name ?? preferences.app.name}</span>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">{inlineMenu(sideMenus)}</div>
      </div>
    </div>
  );

  const singleSider = (
    <Sider
      className={`${preferences.theme.semiDarkSidebar && semiDark ? 'dark ' : ''}bg-sidebar border-border border-r`}
      collapsible
      trigger={null}
      width={preferences.sidebar.width}
    >
      <div className="flex h-full flex-col">
        <div className="min-h-0 flex-1 overflow-y-auto">{inlineMenu(sideMenus)}</div>
      </div>
    </Sider>
  );

  const content = <ContentArea />;

  const footer = preferences.footer.enable && layout !== 'full-content' ? (
    <footer
      className={`bg-header border-border text-muted-foreground flex-center shrink-0 border-t text-xs ${preferences.footer.fixed ? 'absolute bottom-0 left-0 right-0 z-10' : ''}`}
      style={{ height: preferences.footer.height }}
    >
      {preferences.copyright.enable ? (
        <span>
          {preferences.copyright.date} {preferences.copyright.companyName}
        </span>
      ) : null}
    </footer>
  ) : null;

  const headerBar = (withMenu: boolean, withLogo: boolean) => (
    <ConfigProvider theme={preferences.theme.semiDarkHeader && semiDark ? semiDarkTheme : undefined}>
      <Header
        className={`${preferences.theme.semiDarkHeader && semiDark ? 'dark ' : ''}bg-header border-border flex items-center justify-between gap-2 border-b px-3`}
        style={{
              display: 'flex',
              height: preferences.header.height,
              marginTop: headerOffset,
              transition: preferences.header.mode === 'static' ? 'none' : 'margin-top 180ms ease',
              // antd 6 scopes Layout tokens to .ant-layout, not a standalone Header.
              background: 'hsl(var(--header))',
              color: 'hsl(var(--foreground))',
            }}
      >
        <div className="flex min-w-0 flex-1 items-center gap-2" style={{ flex: '1 1 0%', minWidth: 0, display: 'flex' }}>
          {withLogo && preferences.logo.enable ? logo() : null}
          {withMenu ? horizontalMenu : null}
          {!withMenu && breadcrumbItems.length > 0 ? <Breadcrumb items={breadcrumbItems} /> : null}
        </div>
        {rightWidgets}
      </Header>
    </ConfigProvider>
  );

  const bodyColumn = (
    <Layout className="bg-background-deep min-h-0">
      {showTabbar ? <VbenTabs /> : null}
      {content}
      {footer}
    </Layout>
  );

  let shell: ReactNode;
  if (layout === 'full-content') {
    shell = (
      <Layout className="h-screen overflow-hidden">
        {bodyColumn}
      </Layout>
    );
  } else if (layout === 'sidebar-mixed-nav') {
    shell = (
      <Layout className="h-screen overflow-hidden" hasSider={showSider}>
        {showSider ? <ConfigProvider theme={preferences.theme.semiDarkSidebar && semiDark ? semiDarkTheme : undefined}>
          {doubleColSider}
        </ConfigProvider> : null}
        <Layout className="bg-background-deep min-h-0">
          {showHeader ? headerBar(false, false) : null}
          {bodyColumn}
        </Layout>
      </Layout>
    );
  } else {
    // header-nav / header-sidebar-nav / mixed-nav / header-mixed-nav
    const withMenu = layout !== 'header-sidebar-nav';
    shell = (
      <Layout className="h-screen overflow-hidden">
        {showHeader ? headerBar(withMenu, true) : null}
        <Layout className="min-h-0" hasSider={showSider}>
          {showSider ? (
            <ConfigProvider theme={preferences.theme.semiDarkSidebar && semiDark ? semiDarkTheme : undefined}>
              {isDoubleCol ? doubleColSider : singleSider}
            </ConfigProvider>
          ) : null}
          {bodyColumn}
        </Layout>
      </Layout>
    );
  }

  return shell;
}
