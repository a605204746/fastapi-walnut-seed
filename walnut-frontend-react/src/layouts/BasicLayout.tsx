import type { ReactNode } from 'react';

import type {
  MenuRecordRaw,
  RouteHandle,
  RouteMeta,
} from '@/core/typings';

import { useEffect, useMemo, useRef, useState } from 'react';

import { updatePreferences, usePreferences } from '@/core/preferences';
import { useAccessStore, useUserStore } from '@/core/stores';
import { findMenuByPath, setObjToUrlParams } from '@/core/utils';
import { isHttpUrl } from '@/core/shared/utils';
import { WalnutIcon, WalnutIconButton } from '@/core/shadcn-ui';
import { Avatar, Badge, Breadcrumb, ConfigProvider, Dropdown, Drawer, Layout } from 'antd';
import {
  ChevronsLeft,
  ChevronsRight,
  LogOut,
  RotateCw,
  Pin,
  PinOff,
  Settings,
  UserRound,
} from 'lucide-react';
import { useLocation, useMatches, useNavigate } from 'react-router';

import { WalnutMenu } from '@/adapter/menu';
import { WalnutTabs } from '@/adapter/tabs';
import { useIsMobile } from '@/hooks/use-is-mobile';
import { usePreferenceEffects } from '@/hooks/use-preference-effects';
import { $t } from '@/locales';
import { mergeMatchMeta } from '@/router/route-tree';
import { useAuthStore } from '@/store/auth';
import { useUiStore } from '@/store/ui';
import { useTenantStore } from '@/store/tenant';

import { ContentArea } from './ContentArea';
import { TenantToggle } from './widgets/tenant-toggle';
import { HeaderTools, SearchButton } from './widgets/header-tools';
import { MultiLayout } from './MultiLayout';
import { useShellTheme } from './use-shell-theme';
import { useHeaderScroll } from './use-header-scroll';
import { FullscreenToggle, LanguageToggle, ThemeToggle } from './widgets';
import { openPreferences, PreferencesButton } from './widgets/preferences';
import { EpExpand, EpFold } from './widgets/sidebar-toggle-icons';

const { Header, Sider } = Layout;

/**
 * 用户下拉（对应 Vue 侧 widgets/user-dropdown.vue，304 行）。
 *
 * ⑥-a 只有退出登录；本轮补上「个人中心」（profile 页已迁移）。
 * 锁屏 / 文档 / 消息通知等入口仍随对应 widget 在 ⑥-b 补齐。
 */
export function UserDropdown() {
  const userInfo = useUserStore((state) => state.userInfo);
  const logout = useAuthStore((state) => state.logout);
  const navigate = useNavigate();
  const { preferences, preferencesButtonPosition } = usePreferences();

  const tenantSelected = useTenantStore(s => s.selected);
  const items: any[] = [
    {
      icon: <UserRound className="size-4" />,
      key: 'profile',
      label: $t('ui.widgets.profile'),
    },
  ];
  if (tenantSelected) items.splice(0,1);
  items.push({ key: 'about', label: $t('demos.walnut.about') });
  if (preferences.app.enablePreferences && preferencesButtonPosition.userDropdown) {
    items.push({
      icon: <Settings className="size-4" />,
      key: 'preferences',
      label: $t('preferences.title'),
    });
  }
  items.push(
    { type: 'divider' },
    {
      danger: true,
      icon: <LogOut className="size-4" />,
      key: 'logout',
      label: $t('common.logout'),
    },
  );

  return (
    <Dropdown
      menu={{
        items,
        onClick: ({ key }) => {
          if (key === 'about') { navigate('/about'); } else if (key === 'logout') {
            void logout();
          } else if (key === 'profile') {
            navigate('/profile');
          } else if (key === 'preferences') {
            openPreferences();
          }
        },
      }}
      trigger={['click']}
    >
      <button type="button" aria-label="user-menu" className="mx-1 rounded-full p-1 hover:bg-accent">
        <Badge dot color="#52c41a" offset={[-2,28]}><Avatar size={32} src={userInfo?.avatar || preferences.app.defaultAvatar}>{(userInfo?.realName ?? 'U').charAt(0)}</Avatar></Badge>
      </button>
    </Dropdown>
  );
}

/**
 * 后台外壳（对应 Vue 侧 packages/effects/layouts/src/basic/layout.vue）。
 *
 * 默认侧边导航；其它六种布局委托 MultiLayout，内容测量统一由 ContentArea 承担。
 * 全局偏好副作用只在这一层注册一次，避免多布局重复监听快捷键。
 */
export function BasicLayout() {
  const mobile = useIsMobile();
  const [mobileOpen, setMobileOpen] = useState(false);
  const headerOffset = useHeaderScroll();
  const { layout, preferences, preferencesButtonPosition } = usePreferences();
  const navigate = useNavigate();
  const location = useLocation();
  const matches = useMatches();
  const accessMenus = useAccessStore((state) => state.accessMenus);

  const [openKeys, setOpenKeys] = useState<string[]>([]);
  const refresh = useUiStore((state) => state.refresh);
  const maximized = useUiStore((state) => state.maximized);

  // 文档级偏好消费端：动态标题 / 灰度色弱 / 全局快捷键
  usePreferenceEffects();

  const meta = useMemo(() => mergeMatchMeta(matches), [matches]);

  // 带参详情页（如 /system/user-auth/role/:userId）用 meta.activePath 把高亮回指列表页
  const activePath = (meta?.activePath as string) || location.pathname;

  // 展开当前菜单的祖先链；用户手动展开的其它项保留不覆盖
  useEffect(() => {
    const parents = findMenuByPath(accessMenus, activePath)?.parents ?? [];
    if (parents.length === 0) {
      return;
    }
    if (preferences.navigation.accordion) { setOpenKeys(parents); return; }
    setOpenKeys((prev) =>
      parents.every((key) => prev.includes(key))
        ? prev
        : [...new Set([...prev, ...parents])],
    );
  }, [accessMenus, activePath, preferences.navigation.accordion]);

  const collapsed = preferences.sidebar.collapsed;
  // 悬停展开（sidebar.expandOnHover）：折叠时鼠标移入临时展开，不写回偏好
  const [hoverExpand, setHoverExpand] = useState(false);
  const effectiveCollapsed =
    collapsed && !(preferences.sidebar.expandOnHover && hoverExpand);
  // 拖拽调宽（sidebar.draggable）：对应 Vue layout-ui 的侧边栏右缘拖拽条
  const dragRef = useRef<null | { startX: number; startWidth: number }>(null);
  function handleSidebarDragStart(e: { clientX: number }) {
    dragRef.current = { startX: e.clientX, startWidth: preferences.sidebar.width };
    const move = (ev: MouseEvent) => {
      if (!dragRef.current) return;
      const width = Math.min(
        320,
        Math.max(160, dragRef.current.startWidth + (ev.clientX - dragRef.current.startX)),
      );
      updatePreferences({ sidebar: { width } });
    };
    const up = () => {
      dragRef.current = null;
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  }
  // 最大化时隐藏侧边栏 / 顶栏 / 页签（对应 Vue 侧 tabbar maximize）
  const showSidebar =
    preferences.sidebar.enable && !preferences.sidebar.hidden && !maximized && !mobile;
  const showHeader =
    preferences.header.enable && !preferences.header.hidden && !maximized && layout !== 'full-content';
  const showTabbar = preferences.tabbar.enable && !maximized && layout !== 'full-content';

  const { isDark } = usePreferences();
  // 半暗侧边栏 / 顶栏：浅色主题下局部套暗色算法（Vue 侧 semiDarkSidebar / semiDarkHeader）
  const semiDarkSidebar = preferences.theme.semiDarkSidebar && !isDark;
  const semiDarkHeader = preferences.theme.semiDarkHeader && !isDark;
  const semiDarkTheme = useShellTheme();

  function toggleCollapsed() {
    updatePreferences({ sidebar: { collapsed: !collapsed } });
  }

  /** 手风琴：只收起同级兄弟子树，保留新展开项的祖先链（对应 Vue menu-ui openMenu） */
  function handleOpenChange(keys: string[]) {
    if (!preferences.navigation.accordion) {
      setOpenKeys(keys);
      return;
    }
    const added = keys.filter((key) => !openKeys.includes(key));
    if (added.length === 0) {
      setOpenKeys(keys);
      return;
    }
    const key = added[added.length - 1]!;
    const parents = findMenuByPath(accessMenus, key)?.parents ?? [];
    setOpenKeys([...parents, key]);
  }

  function handleMenuSelect(menu: MenuRecordRaw) {
    // 外链菜单的 path 就是 URL 本身（generateMenus 里取的 `link || path`）
    if (isHttpUrl(menu.path)) {
      window.open(menu.path, '_blank');
      return;
    }
    const { query } = menu;
    navigate(
      query && Object.keys(query).length > 0
        ? setObjToUrlParams(menu.path, query)
        : menu.path,
    );
  }

  const breadcrumbItems = useMemo(() => {
    const { enable, hideOnlyOne, showHome, showIcon } = preferences.breadcrumb;
    if (!enable) {
      return [];
    }
    const metas = matches
      .map((match) => (match.handle as RouteHandle | undefined)?.meta)
      .filter(
        (item): item is RouteMeta => !!item?.title && !item.hideInBreadcrumb,
      );
    // 只有一级时隐藏（breadcrumb.hideOnlyOne）
    if (hideOnlyOne && metas.length <= 1) {
      return [];
    }
    const items: { title: ReactNode }[] = [];
    if (showHome) {
      // Vue 侧首页项是纯图标（mdi:home-outline）+ path '/'，无文字
      items.push({
        title: (
          <span
            className="text-primary flex cursor-pointer items-center"
            onClick={() => navigate('/')}
          >
            <WalnutIcon className="size-3.5" icon="mdi:home-outline" />
          </span>
        ),
      });
    }
    metas.forEach((item, index) => {
      const isLast = index === metas.length - 1;
      items.push({
        title: (
          <span
            className={
              isLast
                ? 'flex items-center gap-1'
                : 'text-primary flex cursor-pointer items-center gap-1'
            }
          >
            {showIcon && item.icon ? (
              <WalnutIcon className="size-3.5" icon={item.icon as string} />
            ) : null}
            {$t(item.title)}
          </span>
        ),
      });
    });
    return items;
  }, [matches, preferences.breadcrumb, preferences.app.defaultHomePath, navigate]);

  // 非默认侧边导航布局走 MultiLayout（功能版多布局）；默认形态保持原渲染以守住像素门禁
  if (layout !== 'sidebar-nav' && !mobile) {
    return <MultiLayout />;
  }

  return (
    <Layout className="h-screen overflow-hidden">
      {mobile && <Drawer title={preferences.app.name} open={mobileOpen} width={280} onClose={() => setMobileOpen(false)} styles={{ body: { padding: 0 } }}>
        <WalnutMenu menus={accessMenus} mode="inline" selectedKeys={[activePath]} openKeys={openKeys} onOpenChange={handleOpenChange} onMenuSelect={menu => { handleMenuSelect(menu); setMobileOpen(false); }} />
      </Drawer>}
      {showSidebar ? (
        <ConfigProvider theme={semiDarkSidebar ? semiDarkTheme : undefined}>
        {/* ⚠️ 不要写 theme="light"：它会让 antd 用实例级 token 强制白底，
            盖掉 ConfigProvider 里 siderBg: var(--sidebar)，暗色主题下侧边栏会变成白的。
            底色统一由 useAntdComponentTokens 的 Layout.siderBg 提供。 */}
        <Sider
          className={`${semiDarkSidebar ? 'dark ' : ''}bg-sidebar border-border border-r`}
          collapsed={effectiveCollapsed}
          collapsedWidth={preferences.sidebar.collapseWidth}
          collapsible
          onCollapse={toggleCollapsed}
          onMouseEnter={() => {
            if (collapsed && preferences.sidebar.expandOnHover) setHoverExpand(true);
          }}
          onMouseLeave={() => setHoverExpand(false)}
          style={{ position: 'relative' }}
          trigger={null}
          width={preferences.sidebar.width}
        >
          {preferences.sidebar.draggable && !effectiveCollapsed ? (
            <div
              className="walnut-sidebar-dragger"
              onMouseDown={handleSidebarDragStart}
            />
          ) : null}
          <div className="flex h-full flex-col">
            {preferences.logo.enable ? (
              <div className="flex h-[50px] shrink-0 items-center gap-2 overflow-hidden px-3">
                <img
                  alt="logo"
                  className="size-8 shrink-0"
                  src={preferences.logo.source}
                />
                {effectiveCollapsed && !preferences.sidebar.collapsedShowTitle ? null : (
                  <span className="text-foreground truncate text-lg font-semibold">
                    {preferences.app.name}
                  </span>
                )}
              </div>
            ) : null}
            <div
              className="flex-1 overflow-x-hidden overflow-y-auto"
            >
              <WalnutMenu
                inlineCollapsed={effectiveCollapsed}
                menus={accessMenus}
                mode="inline"
                onMenuSelect={handleMenuSelect}
                onOpenChange={handleOpenChange}
                openKeys={effectiveCollapsed ? [] : openKeys}
                selectedKeys={[activePath]}
                style={{ background: 'transparent', borderInlineEnd: 'none' }}
              />
            </div>
            {/*
              侧边栏底栏（对应 Vue 侧 layout.vue 的 extra 区）：
              左 = 折叠按钮（sidebar.collapsedButton），右 = 偏好设置固定入口（sidebar.fixedButton）。
            */}
            <div className="flex h-9 shrink-0 items-center justify-between px-3">
              {preferences.widget.sidebarToggle && preferences.sidebar.collapsedButton ? (
                <WalnutIconButton onClick={() => mobile ? setMobileOpen(true) : toggleCollapsed()}>
                  {effectiveCollapsed ? (
                    <ChevronsRight className="size-4" />
                  ) : (
                    <ChevronsLeft className="size-4" />
                  )}
                </WalnutIconButton>
              ) : (
                <span />
              )}
              {preferences.sidebar.fixedButton ? <WalnutIconButton aria-label="pin-sidebar" class="h-6 w-6 rounded-sm bg-accent" onClick={() => updatePreferences({ sidebar: { expandOnHover: !preferences.sidebar.expandOnHover } })}>{preferences.sidebar.expandOnHover ? <Pin className="size-3.5" /> : <PinOff className="size-3.5" />}</WalnutIconButton> : null}
            </div>
          </div>
        </Sider>
        </ConfigProvider>
      ) : null}

      <Layout className="bg-background-deep">
        {showHeader ? (
          <ConfigProvider theme={semiDarkHeader ? semiDarkTheme : undefined}>
          <Header
            className={`${semiDarkHeader ? 'dark ' : ''}bg-header border-border flex items-center justify-between gap-2 border-b px-3`}
            style={{
              height: preferences.header.height,
              // antd 6 的 header line-height 固定取 headerHeight token（64px），
              // 不显式覆盖会让顶栏内的水平菜单继承 64px 行高而溢出
              lineHeight: `${preferences.header.height}px`,
              marginTop: headerOffset,
              transition: preferences.header.mode === 'static' ? 'none' : 'margin-top 180ms ease',
              // antd 6 scopes Layout tokens to .ant-layout, not a standalone Header.
              background: 'hsl(var(--header))',
              color: 'hsl(var(--foreground))',
            }}
          >
            <div className="flex min-w-0 flex-1 items-center gap-2">
              {/*
                折叠按钮的图标与按钮形态对齐 Vue 侧 layout-ui：
                `WalnutIconButton`（无边框图标按钮）+ Iconify `ep:fold` / `ep:expand`。
                此前用 antd `Button type="text"` + lucide 箭头，字形和按钮盒子都不对。
              */}
              {(showSidebar || mobile) && preferences.widget.sidebarToggle ? (
                <WalnutIconButton
                  aria-label="toggle-sidebar"
                  class="my-0 mr-1 rounded-md"
                  onClick={() => mobile ? setMobileOpen(true) : toggleCollapsed()}
                >
                  {collapsed ? <EpExpand /> : <EpFold />}
                </WalnutIconButton>
              ) : null}
              {preferences.widget.refresh ? (
                <WalnutIconButton
                  aria-label="refresh"
                  class="my-0 mr-1 rounded-md"
                  onClick={refresh}
                >
                  <RotateCw className="size-4" />
                </WalnutIconButton>
              ) : null}
              {!mobile && breadcrumbItems.length > 0 ? (
                <span
                  className={
                    preferences.breadcrumb.styleType === 'background'
                      ? 'walnut-breadcrumb-background'
                      : undefined
                  }
                >
                  <Breadcrumb items={breadcrumbItems} />
                </span>
              ) : null}
            </div>
            <div className="flex shrink-0 items-center">
              {/* 顺序对齐 Vue 侧 header.vue 的 rightSlots */}
              <TenantToggle />{preferences.widget.globalSearch ? <SearchButton /> : null}
              {preferences.app.enablePreferences && preferencesButtonPosition.header ? (
                <PreferencesButton />
              ) : null}
              {!mobile && preferences.widget.themeToggle ? <ThemeToggle /> : null}
              {!mobile && preferences.widget.languageToggle ? <LanguageToggle /> : null}
              {!mobile && preferences.widget.fullscreen ? <FullscreenToggle /> : null}
              <HeaderTools /><UserDropdown />
            </div>
          </Header>
          </ConfigProvider>
        ) : null}

        {showTabbar ? <WalnutTabs /> : null}

        <ContentArea />

        {/* 底栏 + 版权（footer.enable / fixed / copyright.enable） */}
        {preferences.footer.enable ? (
          <footer
            className={`bg-header border-border text-muted-foreground flex-center shrink-0 border-t text-xs ${
              preferences.footer.fixed ? 'absolute bottom-0 left-0 right-0 z-10' : ''
            }`}
            style={{ height: preferences.footer.height }}
          >
            {preferences.copyright.enable ? (
              <span>
                {preferences.copyright.date}{' '}
                {preferences.copyright.companySiteLink ? (
                  <a
                    className="hover:text-foreground"
                    href={preferences.copyright.companySiteLink}
                    rel="noreferrer"
                    target="_blank"
                  >
                    {preferences.copyright.companyName}
                  </a>
                ) : (
                  preferences.copyright.companyName
                )}
                {preferences.copyright.icp ? (
                  <a
                    className="ml-2 hover:text-foreground"
                    href={preferences.copyright.icpLink || '#'}
                    rel="noreferrer"
                    target="_blank"
                  >
                    {preferences.copyright.icp}
                  </a>
                ) : null}
              </span>
            ) : null}
          </footer>
        ) : null}
      </Layout>
    </Layout>
  );
}
