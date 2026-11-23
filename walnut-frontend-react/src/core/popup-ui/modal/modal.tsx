import type { ReactNode } from 'react';

import type { ModalApi } from './modal-api';

import { useEffect } from 'react';

import { useStore } from '@/core/shared/store';
import { Modal } from 'antd';

export interface ModalRendererProps {
  api: ModalApi;
  children?: ReactNode;
  class?: string;
  className?: string;
  title?: ReactNode;
  width?: number | string;
}

/** 从 `w-[550px]` 这样的 class 中解析像素宽度，映射到 antd Modal 的 width */
function parseWidthFromClass(className?: string): number | undefined {
  if (!className) return undefined;
  const matched = className.match(/w-\[(\d+)px\]/);
  return matched ? Number(matched[1]) : undefined;
}

/**
 * 弹窗渲染组件（对应 Vue 侧 popup-ui/src/modal/modal.vue，D 类：改用 antd Modal）。
 * 状态由 ModalApi 的 TanStack Store 驱动，useStore 订阅。
 */
export function ModalRenderer({
  api,
  children,
  class: classProp,
  className,
  title,
  width,
}: ModalRendererProps) {
  const state = useStore(api.store);
  const mergedClass = classProp ?? className;
  const modalWidth = width ?? parseWidthFromClass(mergedClass) ?? 520;

  // antd Modal 没有 Drawer 的 afterOpenChange，用 isOpen 副作用补上 onOpened
  // 回调（字典数据面板靠它在每次打开时按 dictType 重新加载）。
  useEffect(() => {
    if (state.isOpen) api.onOpened();
  }, [api, state.isOpen]);

  return (
    <Modal
      afterClose={() => api.onClosed()}
      centered={state.centered}
      confirmLoading={state.submitting || state.confirmLoading}
      // ⚠️ antd 6 把 `destroyOnClose` 改名为 `destroyOnHidden`（旧名已弃用并会告警）。
      // state 字段名仍保留 `destroyOnClose` 以与 Vue 侧 DrawerState/ModalState 对齐，
      // 只在这里映射到新的 antd prop（drawer.tsx 同样处理）。
      destroyOnHidden={state.destroyOnClose}
      footer={state.footer ? undefined : null}
      // antd 6：`maskClosable` 已弃用，改用 `mask.closable`
      mask={{ closable: state.closeOnClickModal !== false }}
      okButtonProps={{ disabled: state.confirmDisabled }}
      onCancel={() => api.onCancel()}
      onOk={() => api.onConfirm()}
      open={!!state.isOpen}
      title={title ?? state.title}
      width={modalWidth}
    >
      {state.loading ? (
        <div className="flex-center min-h-[120px]">
          <span className="text-muted-foreground">加载中...</span>
        </div>
      ) : (
        children
      )}
    </Modal>
  );
}
