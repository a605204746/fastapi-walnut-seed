import type { ReactNode } from 'react';

import type { DrawerApi } from './drawer-api';

import { useStore } from '@/core/shared/store';
import { Button, Drawer } from 'antd';

export interface DrawerRendererProps {
  api: DrawerApi;
  children?: ReactNode;
  class?: string;
  className?: string;
  title?: ReactNode;
  /** 显式尺寸；不传则从 `class` 的 `w-[600px]` / `h-[300px]` 解析 */
  width?: number | string;
}

/**
 * 底部按钮文案解析器。
 *
 * `@core/*` 是 UI 库无关层，**不能**依赖 `@walnut/locales`（会破坏分层、且有循环依赖风险）。
 * Vue 基线用其 composables 包的 `useSimpleLocale()` 解决同一问题，React 侧没有
 * composables 包，改为由 app 在 bootstrap 时注入解析器：
 *
 * ```ts
 * setDrawerTextResolver((key) => $t(key === 'cancel' ? 'common.cancel' : 'common.confirm'));
 * ```
 *
 * 解析器在**渲染期**调用，因此语言切换后自动生效，无需重新注入。
 */
export type DrawerTextKey = 'cancel' | 'confirm' | 'loading';

const FALLBACK_TEXT: Record<DrawerTextKey, string> = {
  cancel: 'Cancel',
  confirm: 'Confirm',
  loading: 'Loading...',
};

let textResolver: (key: DrawerTextKey) => string = (key) => FALLBACK_TEXT[key];

export function setDrawerTextResolver(
  resolver: (key: DrawerTextKey) => string,
) {
  textResolver = resolver;
}

/** 从 `w-[600px]` 解析横向抽屉宽度 */
function parseWidthFromClass(className?: string): number | undefined {
  if (!className) return undefined;
  const matched = className.match(/w-\[(\d+)px\]/);
  return matched ? Number(matched[1]) : undefined;
}

/** 从 `h-[300px]` 解析纵向抽屉高度 */
function parseHeightFromClass(className?: string): number | undefined {
  if (!className) return undefined;
  const matched = className.match(/h-\[(\d+)px\]/);
  return matched ? Number(matched[1]) : undefined;
}

/**
 * 抽屉渲染组件（对应 Vue 侧 popup-ui/src/drawer/drawer.vue 339 行，D 类：改用 antd Drawer）。
 *
 * 与同目录 `modal.tsx` 刻意保持同构：状态由 `DrawerApi` 的 TanStack Store 驱动、
 * `useStore` 订阅、`loading` 分支的写法一致，便于两者一起维护。
 *
 * ⚠️ antd 6 的 API 变更（与 Vue 侧 antdv-next 不同，照抄会报 deprecation）：
 * - `destroyOnClose` → **`destroyOnHidden`**
 * - `maskClosable` → **`mask={{ closable }}`**（`MaskConfig` 还支持 `enabled` / `blur`）
 * - `width`/`height` 仍可用，但 antd 6 推荐 `size`
 */
export function DrawerRenderer({
  api,
  children,
  class: classProp,
  className,
  title,
  width,
}: DrawerRendererProps) {
  const state = useStore(api.store);
  const mergedClass = classProp ?? className;
  const isHorizontal = state.placement === 'left' || state.placement === 'right';
  const size =
    width ??
    (isHorizontal
      ? (parseWidthFromClass(mergedClass) ?? 520)
      : (parseHeightFromClass(mergedClass) ?? 378));
  const locked = !!state.submitting;

  const footer = state.footer ? (
    <div className="flex justify-end gap-2">
      {state.showCancelButton ? (
        <Button disabled={locked} onClick={() => api.onCancel()}>
          {state.cancelText ?? textResolver('cancel')}
        </Button>
      ) : null}
      {state.showConfirmButton ? (
        <Button
          disabled={state.confirmDisabled}
          loading={locked || state.confirmLoading}
          onClick={() => api.onConfirm()}
          type="primary"
        >
          {state.confirmText ?? textResolver('confirm')}
        </Button>
      ) : null}
    </div>
  ) : null;

  return (
    <Drawer
      afterOpenChange={(open) => (open ? api.onOpened() : api.onClosed())}
      closable={state.header ? (state.closable && !locked) : false}
      destroyOnHidden={state.destroyOnClose}
      footer={footer}
      keyboard={!!state.closeOnPressEscape && !locked}
      mask={{
        closable: !!state.closeOnClickModal && !locked,
        enabled: state.modal !== false,
      }}
      onClose={() => api.onCancel()}
      open={!!state.isOpen}
      placement={state.placement}
      title={state.header ? (title ?? state.title) : null}
      zIndex={state.zIndex}
      {...(isHorizontal ? { width: size } : { height: size })}
    >
      {state.loading ? (
        <div className="flex-center min-h-[120px]">
          <span className="text-muted-foreground">{textResolver('loading')}</span>
        </div>
      ) : (
        children
      )}
    </Drawer>
  );
}
