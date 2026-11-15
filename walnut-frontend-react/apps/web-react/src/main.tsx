// 根入口会经 css/global.css 带进 @vben/tailwind-config/theme（Tailwind 4 入口）
import '@vben-core/design';

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { loadLocaleMessages, setupI18n } from '@vben/locales';
import { getPreferences, initPreferences } from '@vben/preferences';
import { initStores } from '@vben/stores';
import { unmountGlobalLoading } from '@vben/utils';

import { setupComponents } from './adapter/component';
import { App } from './App';
import { appConfig } from './lib/config';
import { loadAppMessages } from './locales';
import { overridesPreferences } from './preferences';
import './styles.css';

async function bootstrap() {
  // 与 Vue 侧 bootstrap.ts 的顺序一致：i18n → store → preferences → 挂载
  await setupI18n({
    defaultLocale: 'zh-CN',
    loadMessages: loadAppMessages,
    missingWarn: true,
  });

  initStores({ namespace: appConfig.namespace });

  await initPreferences({
    namespace: appConfig.namespace,
    overrides: overridesPreferences,
  });

  // 语言以偏好设置为准（用户上次选择的会被 localStorage 恢复）
  await loadLocaleMessages(getPreferences().app.locale);

  // 注册表单组件映射表（componentMap）
  setupComponents();

  createRoot(document.querySelector('#root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );

  // 与 Vue 侧 bootstrap.ts 一致：挂载后移除首屏 loading 闪屏
  unmountGlobalLoading();
}

bootstrap();
