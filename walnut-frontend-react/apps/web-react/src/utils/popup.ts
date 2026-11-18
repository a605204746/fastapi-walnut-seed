import type { FormApi } from '@vben-core/form-ui';

import { useRef } from 'react';

import { Modal } from 'antd';

import { $t } from '@/locales';
import { getMessageApi } from '@/services/message';

/**
 * 弹窗/抽屉相关的命令式工具（对应 Vue 侧 `utils/popup.ts`）。
 *
 * 8 个使用 `useVbenDrawer` 的业务页（dept / client / menu / post / role /
 * role-assign / user / operlog）都靠 `useBeforeCloseDiff` 实现「表单有改动时
 * 关闭前二次确认」，因此它是 A2 阶段的硬前置。
 *
 * 与 Vue 侧的差异：
 * - Vue 用全局 `window.modal.confirm`；React 侧走 `App.useApp()` 的桥接单例
 *   （`services/message.ts`），未初始化时回退到 antd 静态 `Modal.confirm`
 *   （拿不到 ConfigProvider 主题，但不会崩）。
 * - Vue 传的 `mask: { blur: false }` 在 antd 6 的 `ModalFuncProps` 上不存在
 *   （`mask` 被 Omit 掉了，只剩 `mask.closable`），故省略——它只影响遮罩模糊，
 *   不影响行为。
 * - Vue 的 `ref` 换成 `useRef`；本 hook 必须在组件/hook 内调用。
 */

type MaybePromise<T> = Promise<T> | T;

interface BeforeCloseDiffProps {
  /** 自定义比较函数，返回 true 表示「视为无改动」 */
  compare?: (init: string, current: string) => boolean;
  /** 当前值如何获取 */
  currentGetter: () => MaybePromise<string>;
  /** 初始值如何获取 */
  initializedGetter: () => MaybePromise<string>;
}

function confirmDiscard(): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const config = {
      cancelText: $t('common.cancel'),
      centered: true,
      content: $t('pages.common.beforeCloseTip'),
      okButtonProps: { danger: true },
      okText: $t('common.confirm'),
      onCancel: () => resolve(false),
      onOk: () => resolve(true),
      title: $t('pages.common.tip'),
    };
    const modal = getMessageApi()?.modal;
    if (modal) {
      modal.confirm(config);
    } else {
      Modal.confirm(config);
    }
  });
}

/**
 * 用于 Drawer/Modal：判断表单是否有变动来决定是否弹窗提示。
 *
 * 用法与 Vue 侧一致：
 * ```tsx
 * const { markInitialized, onBeforeClose, resetInitialized } = useBeforeCloseDiff({
 *   currentGetter: defaultFormValueGetter(formApi),
 *   initializedGetter: defaultFormValueGetter(formApi),
 * });
 * const [BasicDrawer, drawerApi] = useVbenDrawer({ onBeforeClose, ... });
 * ```
 */
export function useBeforeCloseDiff(props: BeforeCloseDiffProps) {
  const { compare, currentGetter, initializedGetter } = props;

  /** 记录初始值 JSON */
  const initialized = useRef('');
  /** 是否已初始化；为 false 时直接关闭、不弹窗 */
  const isInitialized = useRef(false);
  /** 每次渲染同步最新的 getter，避免闭包过期 */
  const getters = useRef({ compare, currentGetter, initializedGetter });
  getters.current = { compare, currentGetter, initializedGetter };

  /** 标记已完成初始化，后续关闭时需要对比 */
  async function markInitialized(data?: string) {
    initialized.current = data || (await getters.current.initializedGetter());
    isInitialized.current = true;
  }

  /** 重置初始化状态，需在 closed 前调用或打开窗口时调用 */
  function resetInitialized() {
    initialized.current = '';
    isInitialized.current = false;
  }

  /** 提供给 useVbenDrawer / useVbenModal 的 onBeforeClose，返回是否允许关闭 */
  async function onBeforeClose(): Promise<boolean> {
    // 还没初始化，直接允许关闭
    if (!isInitialized.current) {
      return true;
    }

    try {
      const current = await getters.current.currentGetter();
      const { compare: cmp } = getters.current;
      if (cmp) {
        if (cmp(initialized.current, current)) {
          isInitialized.current = false;
          return true;
        }
      } else if (current === initialized.current) {
        // 数据无变化，直接允许关闭
        return true;
      }

      const allowed = await confirmDiscard();
      if (allowed) {
        isInitialized.current = false;
      }
      return allowed;
    } catch (error) {
      console.error('Failed to compare data:', error);
      return true;
    }
  }

  return { markInitialized, onBeforeClose, resetInitialized };
}

/** 给 useVbenForm 使用的封装：把表单值序列化成 JSON 便于对比 */
export function defaultFormValueGetter(formApi: FormApi) {
  return async () => JSON.stringify(await formApi.getValues());
}
