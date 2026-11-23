// 根入口会经 css/global.css 带进 @walnut/tailwind-config/theme（Tailwind 4 入口）
import '@/core/design';

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { loadLocaleMessages, setupI18n } from '@/core/locales';
import { getPreferences, initPreferences } from '@/core/preferences';
import { initStores } from '@/core/stores';
import { unmountGlobalLoading } from '@/core/utils';

import { setupComponents } from './adapter/component';
import { setupDrawer } from './adapter/drawer';
import { initSetupWalnutForm } from './adapter/form';
import { App } from './App';
import { initAppStores } from './store/setup';
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
  initAppStores(appConfig.namespace);

  await initPreferences({
    namespace: appConfig.namespace,
    overrides: overridesPreferences,
  });

  // 语言以偏好设置为准（用户上次选择的会被 localStorage 恢复）
  await loadLocaleMessages(getPreferences().app.locale);

  // 注册表单组件映射表（componentMap）
  setupComponents();

  // 注册必填校验的国际化文案（对应 Vue 基线表单 setup 的 defineRules）
  initSetupWalnutForm();

  // 注入抽屉底部按钮的 i18n 文案（@core 层不能依赖 @walnut/locales）
  setupDrawer();

  createRoot(document.querySelector('#root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );

  // 与 Vue 侧 bootstrap.ts 一致：挂载后移除首屏 loading 闪屏
  unmountGlobalLoading();
}

bootstrap();
