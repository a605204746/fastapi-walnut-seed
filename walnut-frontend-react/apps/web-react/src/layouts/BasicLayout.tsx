import type { CSSProperties } from 'react';

import type {
  MenuRecordRaw,
  RouteHandle,
  RouteMeta,
} from '@vben-core/typings';

import { useEffect, useMemo, useRef, useState } from 'react';

import { updatePreferences, usePreferences } from '@vben/preferences';
import { useAccessStore, useUserStore } from '@vben/stores';
import { findMenuByPath, setObjToUrlParams } from '@vben/utils';
import { isHttpUrl } from '@vben-core/shared/utils';
import { Avatar, Breadcrumb, Button, Dropdown, Layout } from 'antd';
import {
  ArrowLeftFromLine,
  ArrowRightFromLine,
  LogOut,
} from 'lucide-react';
import { Outlet, useLocation, useMatches, useNavigate } from 'react-router';

import { VbenMenu } from '@/adapter/menu';
import { VbenTabs } from '@/adapter/tabs';
import { $t } from '@/locales';
import { mergeMatchMeta } from '@/router/route-tree';
import { useAuthStore } from '@/store/auth';

import { LanguageToggle, ThemeToggle } from './widgets';

const { Content, Header, Sider } = Layout;

/**
 * 用户下拉（对应 Vue 侧 widgets/user-dropdown.vue，304 行）。
 *
 * ⑥-a 只提供退出登录；个人中心 / 锁屏 / 文档 / 消息通知等入口随对应视图与
 * widget 在 ⑥-b、⑦、⑨ 补齐——现在挂上去只会得到 404。
 */
function UserDropdown() {
  const userInfo = useUserStore((state) => state.userInfo);
  const logout = useAuthStore((state) => state.logout);

  return (
    <Dropdown
      menu={{
        items: [
          {
            danger: true,
            icon: <LogOut className="size-4" />,
            key: 'logout',
            label: $t('common.logout'),
          },
        ],
        onClick: ({ key }) => {
          if (key === 'logout') {
            void logout();
          }
        },
      }}
      trigger={['click']}
    >
      <div className="hover:bg-accent flex-center cursor-pointer gap-2 rounded-md px-2 py-1">
        <Avatar size={24} src={userInfo?.avatar || undefined}>
          {(userInfo?.realName ?? 'U').charAt(0)}
        </Avatar>
        <span className="text-foreground text-sm">{userInfo?.realName}</span>
      </div>
    </Dropdown>
  );
}

/**
 * 后台外壳（对应 Vue 侧 packages/effects/layouts/src/basic/layout.vue）。
 *
 * ⚠️ ⑥-a 的范围是「可用外壳」：antd `Layout` 的侧边栏 + 顶栏 + 内容区，
 * 单一 `sidebar-nav` 布局形态。6 种布局模式、偏好设置抽屉、多页签、全局搜索、
 * 锁屏、`<Activity>` 页签缓存都在 ⑥-b（见清单 §15.5）。
 *
 * 折叠按钮暂放顶栏左侧（antd 惯例）；vben 是贴在侧边栏右缘的悬浮按钮，⑥-b 对齐。
 */
export function BasicLayout() {
  const { preferences } = usePreferences();
  const navigate = useNavigate();
  const location = useLocation();
  const matches = useMatches();
  const accessMenus = useAccessStore((state) => state.accessMenus);

  const contentRef = useRef<HTMLDivElement>(null);
  const [contentHeight, setContentHeight] = useState(0);
  const [openKeys, setOpenKeys] = useState<string[]>([]);

  // Page 的 autoContentHeight 依赖 --vben-content-height，由内容区实测后写入并继承下去
  useEffect(() => {
    const element = contentRef.current;
    if (!element) {
      return;
    }
    const update = () => setContentHeight(element.clientHeight);
    const observer = new ResizeObserver(update);
    observer.observe(element);
    update();
    return () => observer.disconnect();
  }, []);

  const meta = useMemo(() => mergeMatchMeta(matches), [matches]);

  // 带参详情页（如 /system/user-auth/role/:userId）用 meta.activePath 把高亮回指列表页
  const activePath = (meta?.activePath as string) || location.pathname;

  // 展开当前菜单的祖先链；用户手动展开的其它项保留不覆盖
  useEffect(() => {
    const parents = findMenuByPath(accessMenus, activePath)?.parents ?? [];
    if (parents.length === 0) {
      return;
    }
    setOpenKeys((prev) =>
      parents.every((key) => prev.includes(key))
        ? prev
        : [...new Set([...prev, ...parents])],
    );
  }, [accessMenus, activePath]);

  const collapsed = preferences.sidebar.collapsed;
  const showSidebar = preferences.sidebar.enable && !preferences.sidebar.hidden;
  const showHeader = preferences.header.enable && !preferences.header.hidden;

  function toggleCollapsed() {
    updatePreferences({ sidebar: { collapsed: !collapsed } });
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
    if (!preferences.breadcrumb.enable) {
      return [];
    }
    return matches
      .map((match) => (match.handle as RouteHandle | undefined)?.meta)
      .filter(
        (item): item is RouteMeta => !!item?.title && !item.hideInBreadcrumb,
      )
      .map((item) => ({ title: item.title }));
  }, [matches, preferences.breadcrumb.enable]);

  return (
    <Layout className="h-screen overflow-hidden">
      {showSidebar ? (
        <Sider
          className="bg-sidebar border-border border-r"
          collapsed={collapsed}
          collapsedWidth={preferences.sidebar.collapseWidth}
          collapsible
          onCollapse={toggleCollapsed}
          theme="light"
          trigger={null}
          width={preferences.sidebar.width}
        >
          <div className="flex h-full flex-col">
            {preferences.logo.enable ? (
              <div className="flex-center h-12 shrink-0 gap-2 overflow-hidden px-3">
                <img
                  alt="logo"
                  className="size-8 shrink-0"
                  src={preferences.logo.source}
                />
                {collapsed && !preferences.sidebar.collapsedShowTitle ? null : (
                  <span className="text-foreground truncate text-base font-semibold">
                    {preferences.app.name}
                  </span>
                )}
              </div>
            ) : null}
            <div className="flex-1 overflow-x-hidden overflow-y-auto">
              <VbenMenu
                inlineCollapsed={collapsed}
                menus={accessMenus}
                mode="inline"
                onMenuSelect={handleMenuSelect}
                onOpenChange={(keys) => setOpenKeys(keys as string[])}
                openKeys={collapsed ? [] : openKeys}
                selectedKeys={[activePath]}
                style={{ background: 'transparent', borderInlineEnd: 'none' }}
              />
            </div>
          </div>
        </Sider>
      ) : null}

      <Layout className="bg-background-deep">
        {showHeader ? (
          <Header
            className="bg-header border-border flex items-center justify-between gap-2 border-b px-3"
            style={{ height: preferences.header.height }}
          >
            <div className="flex min-w-0 flex-1 items-center gap-2">
              {showSidebar && preferences.widget.sidebarToggle ? (
                <Button
                  aria-label="toggle-sidebar"
                  icon={
                    collapsed ? (
                      <ArrowRightFromLine className="size-4" />
                    ) : (
                      <ArrowLeftFromLine className="size-4" />
                    )
                  }
                  onClick={toggleCollapsed}
                  type="text"
                />
              ) : null}
              {breadcrumbItems.length > 0 ? (
                <Breadcrumb items={breadcrumbItems} />
              ) : null}
            </div>
            <div className="flex shrink-0 items-center">
              {preferences.widget.themeToggle ? <ThemeToggle /> : null}
              {preferences.widget.languageToggle ? <LanguageToggle /> : null}
              <UserDropdown />
            </div>
          </Header>
        ) : null}

        {preferences.tabbar.enable ? <VbenTabs /> : null}

        <Content
          className="relative overflow-hidden"
          ref={contentRef}
          style={
            { '--vben-content-height': `${contentHeight}px` } as CSSProperties
          }
        >
          <div className="h-full overflow-y-auto">
            <Outlet />
          </div>
        </Content>
      </Layout>
    </Layout>
  );
}
