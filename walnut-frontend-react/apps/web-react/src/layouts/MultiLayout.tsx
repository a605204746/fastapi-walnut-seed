import type { CSSProperties, ReactNode } from 'react';

import type { MenuRecordRaw, RouteHandle, RouteMeta } from '@vben-core/typings';

import { useEffect, useMemo, useRef, useState } from 'react';

import { usePreferences } from '@vben/preferences';
import { useAccessStore } from '@vben/stores';
import { findMenuByPath, setObjToUrlParams } from '@vben/utils';
import { isHttpUrl } from '@vben-core/shared/utils';
import { VbenIcon } from '@vben-core/shadcn-ui';
import { Breadcrumb, ConfigProvider, Layout, Menu, Watermark, theme as antdTheme } from 'antd';
import { Outlet, useLocation, useMatches, useNavigate } from 'react-router';

import { VbenMenu } from '@/adapter/menu';
import { VbenTabs } from '@/adapter/tabs';
import { usePreferenceEffects } from '@/hooks/use-preference-effects';
import { $t } from '@/locales';
import { mergeMatchMeta } from '@/router/route-tree';
import { useUiStore } from '@/store/ui';

import { UserDropdown } from './BasicLayout';
import { FullscreenToggle, LanguageToggle, ThemeToggle } from './widgets';
import { PreferencesButton } from './widgets/preferences';
import { RouteFeedback } from './widgets/route-feedback';

const { Content, Header, Sider } = Layout;

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
  const { isDark, layout, preferences, preferencesButtonPosition } = usePreferences();
  const navigate = useNavigate();
  const location = useLocation();
  const matches = useMatches();
  const accessMenus = useAccessStore((state) => state.accessMenus);
  const maximized = useUiStore((state) => state.maximized);
  const refreshCounter = useUiStore((state) => state.refreshCounter);
  usePreferenceEffects();

  const contentRef = useRef<HTMLDivElement>(null);
  const [contentHeight, setContentHeight] = useState(0);
  const [openKeys, setOpenKeys] = useState<string[]>([]);

  useEffect(() => {
    const element = contentRef.current;
    if (!element) return;
    const update = () => setContentHeight(element.clientHeight);
    const observer = new ResizeObserver(update);
    observer.observe(element);
    update();
    return () => observer.disconnect();
  }, []);

  const meta = useMemo(() => mergeMatchMeta(matches), [matches]);
  const activePath = (meta?.activePath as string) || location.pathname;

  // 混合导航：当前激活的顶层菜单
  const activeRoot = useMemo(() => {
    const contains = (menu: MenuRecordRaw): boolean => {
      if (menu.path === activePath) return true;
      return (menu.children ?? []).some(contains);
    };
    return accessMenus.find(contains) ?? accessMenus[0];
  }, [accessMenus, activePath]);

  const isMixed = layout === 'mixed-nav' || layout === 'header-mixed-nav';
  const isDoubleCol = layout === 'sidebar-mixed-nav' || layout === 'header-mixed-nav';
  const sideMenus = isMixed || isDoubleCol ? (activeRoot?.children ?? []) : accessMenus;
  const horizontalMenus = isMixed
    ? accessMenus.map((root) => ({ ...root, children: undefined }))
    : accessMenus;

  useEffect(() => {
    const parents = findMenuByPath(accessMenus, activePath)?.parents ?? [];
    if (parents.length === 0) return;
    setOpenKeys((prev) =>
      parents.every((key) => prev.includes(key)) ? prev : [...new Set([...prev, ...parents])],
    );
  }, [accessMenus, activePath]);

  function handleMenuSelect(menu: MenuRecordRaw) {
    if (isHttpUrl(menu.path)) {
      window.open(menu.path, '_blank');
      return;
    }
    const target = isMixed && menu.children?.length ? firstLeaf(menu) : menu;
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
  const semiDarkTheme = {
    algorithm: antdTheme.darkAlgorithm,
    components: {
      Layout: { siderBg: 'hsl(222.34 10.43% 12.27%)' },
      Menu: {
        darkItemBg: 'transparent',
        darkItemSelectedBg: 'hsl(var(--primary))',
        darkItemSelectedColor: '#fff',
        darkSubMenuItemBg: 'transparent',
        itemBg: 'transparent',
        subMenuItemBg: 'transparent',
      },
    },
  };

  const showHeader = preferences.header.enable && !maximized;
  const showTabbar = preferences.tabbar.enable && !maximized;
  const showSider =
    preferences.sidebar.enable &&
    !maximized &&
    ['mixed-nav', 'sidebar-mixed-nav', 'header-mixed-nav', 'header-sidebar-nav'].includes(layout);

  const logo = (compact = false) => (
    <div className="flex-center h-12 shrink-0 gap-2 overflow-hidden px-3">
      <img alt="logo" className="size-8 shrink-0" src={preferences.logo.source} />
      {compact ? null : (
        <span className="text-foreground truncate text-base font-semibold">{preferences.app.name}</span>
      )}
    </div>
  );

  const rightWidgets = (
    <div className="flex shrink-0 items-center">
      {preferences.app.enablePreferences && preferencesButtonPosition.header ? <PreferencesButton /> : null}
      {preferences.widget.themeToggle ? <ThemeToggle /> : null}
      {preferences.widget.languageToggle ? <LanguageToggle /> : null}
      {preferences.widget.fullscreen ? <FullscreenToggle /> : null}
      <UserDropdown />
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
            {item.title}
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
      onMenuSelect={handleMenuSelect}
      selectedKeys={[isMixed ? (activeRoot?.path ?? '') : activePath]}
      style={{ background: 'transparent', borderBottom: 'none', flex: 1, minWidth: 0 }}
    />
  );

  const doubleColSider = (
    <div className="bg-sidebar border-border flex h-full shrink-0 border-r">
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
          <span className="text-foreground truncate text-base font-semibold">{activeRoot?.name ?? preferences.app.name}</span>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">{inlineMenu(sideMenus)}</div>
      </div>
    </div>
  );

  const singleSider = (
    <Sider
      className="bg-sidebar border-border border-r"
      collapsible
      trigger={null}
      width={preferences.sidebar.width}
    >
      <div className="flex h-full flex-col">
        <div className="min-h-0 flex-1 overflow-y-auto">{inlineMenu(sideMenus)}</div>
      </div>
    </Sider>
  );

  const content = (
    <Content className="relative overflow-hidden" ref={contentRef} style={{ '--vben-content-height': `${contentHeight}px` } as CSSProperties}>
      <Watermark
        content={preferences.app.watermark ? preferences.app.watermarkContent || preferences.app.name : undefined}
        font={{ color: 'rgba(0,0,0,0.15)', fontSize: 16 }}
        gap={[100, 100]}
        rotate={-22}
      >
        <div
          className={`h-full overflow-y-auto ${preferences.transition.enable ? `page-transition page-transition--${preferences.transition.name}` : ''}`}
          key={`${refreshCounter}-${location.pathname}`}
        >
          <Outlet />
        </div>
      </Watermark>
      <RouteFeedback />
    </Content>
  );

  const footer = preferences.footer.enable ? (
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
        className="bg-header border-border flex items-center justify-between gap-2 border-b px-3"
        style={{ height: preferences.header.height }}
      >
        <div className="flex min-w-0 flex-1 items-center gap-2">
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
      <Layout className="h-screen overflow-hidden">
        <ConfigProvider theme={preferences.theme.semiDarkSidebar && semiDark ? semiDarkTheme : undefined}>
          {doubleColSider}
        </ConfigProvider>
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
        <Layout className="min-h-0">
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
