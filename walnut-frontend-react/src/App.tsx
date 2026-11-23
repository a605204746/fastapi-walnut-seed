import { useEffect, useMemo } from 'react';

import { useAntdComponentTokens, useAntdDesignTokens } from '@/core/hooks';
import { usePreferences } from '@/core/preferences';
import { App as AntdApp, ConfigProvider, theme as antdTheme } from 'antd';
import enUS from 'antd/locale/en_US';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import { RouterProvider } from 'react-router';

import { router } from './router';
import { useButtonEffects } from './hooks/use-button-effects';
import { ErrorBoundary } from './components/error-boundary';
import { initMessageApi, type MessageApiLike } from './services/message';

/**
 * 把 antd App.useApp() 的 message/modal/notification 实例注册到全局桥接，
 * 供命令式代码（请求拦截器、store、导出）使用（对应 Vue 侧 window.message/modal）。
 */
function MessageBridge() {
  const app = AntdApp.useApp();
  useEffect(() => {
    initMessageApi(app as unknown as MessageApiLike);
  }, [app]);
  return null;
}

/**
 * 应用根组件（对应 Vue 侧 apps/web-antd/src/app.vue）。
 * 单一事实来源：preferences → CSS 变量 → useAntdDesignTokens → antd ConfigProvider，
 * 保证自绘组件（Tailwind/shadcn）与 antd 组件同色。
 */
export function App() {
  useButtonEffects();
  const { isDark, preferences } = usePreferences();
  const token = useAntdDesignTokens();
  // Layout / Menu 的组件级 token：修掉「浅色主题下顶栏是 antd 默认深蓝黑」的问题，
  // 详见 useAntdComponentTokens 的注释（Tailwind 有层样式打不过 antd 无层样式）
  const components = useAntdComponentTokens();

  const themeConfig = useMemo(() => {
    const algorithm = [
      isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    ];
    if (preferences.app.compact) {
      algorithm.push(antdTheme.compactAlgorithm);
    }
    return { algorithm, components, token };
  }, [components, isDark, preferences.app.compact, token]);

  const isEn = preferences.app.locale === 'en-US';

  useEffect(() => {
    dayjs.locale(isEn ? 'en' : 'zh-cn');
  }, [isEn]);

  return (
    <ConfigProvider
      locale={isEn ? enUS : zhCN}
      theme={themeConfig}
      wave={{ disabled: preferences.theme.buttonWaveMode === 'Disabled' }}
    >
      <AntdApp>
        <MessageBridge />
        <ErrorBoundary>
          <RouterProvider router={router} />
        </ErrorBoundary>
      </AntdApp>
    </ConfigProvider>
  );
}
