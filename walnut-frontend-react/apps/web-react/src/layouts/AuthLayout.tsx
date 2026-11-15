import { useMemo } from 'react';

import { $t } from '@vben/locales';
import { usePreferences } from '@vben/preferences';
import { ConfigProvider, theme as antdTheme } from 'antd';
import { Outlet } from 'react-router';

import { AuthenticationLayout } from './authentication/AuthenticationLayout';

/**
 * 认证页外层布局（对应 Vue 侧 apps/web-antd/src/layouts/auth.vue）。
 *
 * 关键点：登录页单独使用核桃品牌色（琥珀），嵌套 ConfigProvider 只作用于认证页，
 * 后台管理端仍跟随全局主题色。light=#b4692e / dark=#e8a752。
 */
export function AuthLayout() {
  const { isDark, preferences } = usePreferences();

  const authTheme = useMemo(
    () => ({
      algorithm: isDark
        ? [antdTheme.darkAlgorithm]
        : [antdTheme.defaultAlgorithm],
      token: {
        colorLink: isDark ? '#e8a752' : '#b4692e',
        colorPrimary: isDark ? '#e8a752' : '#b4692e',
      },
    }),
    [isDark],
  );

  return (
    <ConfigProvider theme={authTheme}>
      <AuthenticationLayout
        appName={preferences.app.name}
        logo={preferences.logo.source}
        logoDark={preferences.logo.sourceDark}
        pageDescription={$t('authentication.pageDesc')}
        pageTitle={$t('authentication.pageTitle')}
      >
        <Outlet />
      </AuthenticationLayout>
    </ConfigProvider>
  );
}
