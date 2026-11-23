import type { ReactNode } from 'react';

import { usePreferences } from '@/core/preferences';
import { cn } from '@/core/shared/utils';

import { WalnutSlogan } from '@/components/walnut-slogan/WalnutSlogan';

import './authentication.css';
import { AuthenticationFormView } from './AuthenticationFormView';
import { Copyright } from './Copyright';
import { Toolbar, type ToolbarType } from './Toolbar';

export interface AuthenticationLayoutProps {
  appName?: string;
  children?: ReactNode;
  copyright?: boolean;
  logo?: string;
  logoDark?: string;
  pageDescription?: string;
  pageTitle?: string;
  slogan?: ReactNode;
  toolbar?: boolean;
  toolbarList?: ToolbarType[];
}

/**
 * 认证页布局（对应 Vue 侧 layouts/src/authentication/authentication.vue）。
 * 三种面板布局（left/center/right）由 preferences.app.authPageLayout 驱动，
 * 背景渐变 / 网格 / slogan 逐字复刻。
 */
export function AuthenticationLayout({
  appName = '',
  children,
  copyright = true,
  logo = '',
  logoDark = '',
  pageDescription = '',
  pageTitle = '',
  slogan,
  toolbar = true,
  toolbarList = ['color', 'language', 'layout', 'theme'],
}: AuthenticationLayoutProps) {
  const { authPanelCenter, authPanelLeft, authPanelRight, isDark, preferences } =
    usePreferences();

  const logoSrc = isDark && logoDark ? logoDark : logo;

  const copyrightNode =
    copyright && preferences.copyright.enable ? (
      <Copyright
        companyName={preferences.copyright.companyName}
        companySiteLink={preferences.copyright.companySiteLink}
        date={preferences.copyright.date}
        icp={preferences.copyright.icp}
        icpLink={preferences.copyright.icpLink}
      />
    ) : null;

  const sloganNode = slogan ?? <WalnutSlogan className="animate-float" />;

  return (
    <div
      className={cn(
        isDark ? 'dark' : '',
        // Vue 侧 min-h-full 依赖全高 app shell；React 这里直接用 h-screen
        // 保证 slogan 面板 / 表单面板撑满视口（表单垂直居中、copyright 贴底）。
        'flex h-screen flex-1 overflow-x-hidden select-none',
      )}
    >
      {toolbar ? <Toolbar toolbarList={toolbarList} /> : null}

      {authPanelLeft ? (
        <AuthenticationFormView
          className="min-h-full w-2/5 flex-1"
          copyright={copyrightNode}
          dataSide="left"
        >
          {children}
        </AuthenticationFormView>
      ) : null}

      {logoSrc || appName ? (
        <div className="absolute top-0 left-0 z-10 flex flex-1">
          <div className="mt-4 ml-4 flex flex-1 items-center text-foreground sm:top-6 sm:left-6 lg:text-foreground">
            {logoSrc ? (
              <img alt={appName} className="mr-2" key={logoSrc} src={logoSrc} width={42} />
            ) : null}
            {appName ? (
              <p className="m-0 text-xl font-medium">{appName}</p>
            ) : null}
          </div>
        </div>
      ) : null}

      {!authPanelCenter ? (
        <div className="relative hidden w-0 flex-1 lg:block">
          <div className="absolute inset-0 size-full bg-[#f6f1e9] dark:bg-[#0b0e13]">
            <div className="login-background absolute top-0 left-0 size-full"></div>
            <div className="slogan-grid absolute inset-0"></div>
            <div
              className={cn(
                'mr-20 flex-col-center h-full',
                authPanelLeft && 'enter-x',
                authPanelRight && '-enter-x',
              )}
            >
              {sloganNode}
              <div className="mt-8 text-2xl font-semibold tracking-[0.06em] text-foreground lg:text-3xl">
                {pageTitle}
              </div>
              <div className="mt-3 text-sm tracking-[0.12em] dark:text-muted-foreground">
                {pageDescription}
              </div>
            </div>
          </div>
        </div>
      ) : null}

      {authPanelCenter ? (
        <div className="relative flex-center w-full">
          <div className="login-background absolute top-0 left-0 size-full"></div>
          <AuthenticationFormView
            className="w-full rounded-3xl pb-20 shadow-float shadow-primary/5 md:w-2/3 md:bg-background lg:w-1/2 xl:w-[36%]"
            copyright={copyrightNode}
            dataSide="bottom"
          >
            {children}
          </AuthenticationFormView>
        </div>
      ) : null}

      {authPanelRight ? (
        <AuthenticationFormView
          className="min-h-full w-2/5 flex-1"
          copyright={copyrightNode}
          dataSide="right"
        >
          {children}
        </AuthenticationFormView>
      ) : null}
    </div>
  );
}
