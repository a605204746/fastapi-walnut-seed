import { useEffect, useMemo } from 'react';

import { useAntdDesignTokens } from '@vben/hooks';
import { usePreferences } from '@vben/preferences';
import { App as AntdApp, ConfigProvider, theme as antdTheme } from 'antd';
import enUS from 'antd/locale/en_US';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import { RouterProvider } from 'react-router';

import { router } from './router';
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
  const { isDark, preferences } = usePreferences();
  const token = useAntdDesignTokens();

  const themeConfig = useMemo(() => {
    const algorithm = [
      isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    ];
    if (preferences.app.compact) {
      algorithm.push(antdTheme.compactAlgorithm);
    }
    return { algorithm, token };
  }, [isDark, preferences.app.compact, token]);

  const isEn = preferences.app.locale === 'en-US';

  useEffect(() => {
    dayjs.locale(isEn ? 'en' : 'zh-cn');
  }, [isEn]);

  return (
    <ConfigProvider locale={isEn ? enUS : zhCN} theme={themeConfig}>
      <AntdApp>
        <MessageBridge />
        <RouterProvider router={router} />
      </AntdApp>
    </ConfigProvider>
  );
}
