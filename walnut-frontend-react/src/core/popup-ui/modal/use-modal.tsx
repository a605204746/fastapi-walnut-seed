import type { ComponentType } from 'react';

import type { ModalApi, ModalApiOptions } from './modal-api';
import type { ModalRendererProps } from './modal';

import { useCallback, useRef } from 'react';

import { ModalRenderer } from './modal';
import { ModalApi as ModalApiClass } from './modal-api';

/**
 * 对应 Vue 侧 `use-modal.ts`：保留 `useWalnutModal(options) → [Modal, modalApi]` 签名。
 *
 * PoC 说明：Vue 侧的 `connectedComponent` 父子挂载机制在 React 下改由
 * 「弹窗组件自持 useWalnutModal + useWalnutForm，父组件通过 ref 命令式打开」实现，
 * 因此这里的 `connectedComponent` 选项被接受但忽略。
 */
/** 对外暴露的 Modal 组件 props（api 由 hook 内部注入） */
export type WalnutModalComponentProps = Omit<ModalRendererProps, 'api'>;

export function useWalnutModal(
  options: ModalApiOptions = {},
): [ComponentType<WalnutModalComponentProps>, ModalApi] {
  const apiRef = useRef<ModalApi | undefined>(undefined);
  if (!apiRef.current) {
    apiRef.current = new ModalApiClass(options);
  }
  // 每次渲染同步最新回调，避免闭包过期
  apiRef.current.updateCallbacks(options);
  const api = apiRef.current;

  const Modal = useCallback(
    (props: WalnutModalComponentProps) => (
      <ModalRenderer {...props} api={api} />
    ),
    [api],
  );

  return [Modal, api];
}
