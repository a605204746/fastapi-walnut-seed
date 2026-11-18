import { setDrawerTextResolver } from '@vben-core/popup-ui';

import { $t } from '@/locales';

/**
 * 抽屉全局装配（对应 Vue 侧在 bootstrap 里调用 `setDefaultDrawerProps` 的那一步）。
 *
 * `@core/ui-kit/popup-ui` 是 UI 库无关层，不能依赖 `@vben/locales`，因此底部按钮
 * 文案由这里注入解析器（见 `drawer.tsx` 的 `setDrawerTextResolver` 说明）。
 * 解析器在渲染期调用，语言切换后自动生效。
 */
export function setupDrawer() {
  setDrawerTextResolver((key) => {
    switch (key) {
      case 'cancel': {
        return $t('common.cancel');
      }
      case 'confirm': {
        return $t('common.confirm');
      }
      case 'loading': {
        return $t('common.loading');
      }
    }
  });
}
