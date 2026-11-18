import type { ReactNode } from 'react';

import { useEffect, useState } from 'react';

import { $t } from '@vben/locales';
import { Alert, Input, Modal } from 'antd';

import { getMessageApi } from '@/services/message';

export interface ConfirmModalProps {
  /** 需要输入的确认文案，默认「确认删除」 */
  confirmText?: string;
  /** 校验通过后的回调 */
  onValidated?: () => Promise<void> | void;
  /** 输入框占位文案 */
  placeholder?: string;
  title?: string;
}

interface ConfirmInputState {
  setError: (message: string) => void;
  value: string;
}

/**
 * 确认输入框（对应 Vue 侧 utils/modal.tsx 里 `content: () => h(defineComponent(...))`
 * 的那段内联组件）。用独立组件承载 state，因为 antd 的命令式 confirm 只接受
 * 一个静态 ReactNode，拿不到内部输入值 —— 通过 `state` 这个可变对象桥接出去。
 */
function ConfirmDeleteInput({
  confirmText,
  placeholder,
  state,
}: {
  confirmText: string;
  placeholder: string;
  state: ConfirmInputState;
}) {
  const [value, setValue] = useState('');
  const [error, setError] = useState('');

  // 把最新值与 setError 暴露给外层 onOk 校验使用
  useEffect(() => {
    state.value = value;
    state.setError = setError;
  });

  return (
    <div className="flex flex-col gap-[8px]">
      <Alert message="确认删除后将无法恢复，请谨慎操作！" type="error" />
      <div>
        <Input
          onChange={(e) => {
            setValue(e.target.value);
            if (error) setError('');
          }}
          placeholder={placeholder}
          status={error ? 'error' : undefined}
          value={value}
        />
        {error ? (
          <div className="text-destructive mt-1 text-xs">{error}</div>
        ) : null}
      </div>
      <span className="text-muted-foreground text-xs">
        请输入「{confirmText}」以确认操作
      </span>
    </div>
  );
}

/**
 * 带二次输入校验的删除确认弹窗（对应 Vue 侧 `utils/modal.tsx` 的 confirmDeleteModal）。
 *
 * 被 `monitor/logininfor`、`monitor/operlog` 的「清空」按钮使用：这类不可逆操作
 * 要求用户手动输入「确认删除」才放行。
 *
 * 与 Vue 版的差异：
 * - `window.modal.confirm` → `getMessageApi()?.modal.confirm`，未初始化时回退到
 *   antd 静态 `Modal.confirm`（拿不到 ConfigProvider 主题，但不会崩）。
 * - Vue 用 antd Form + rules 做校验；React 侧命令式弹窗里挂 Form 需要额外的
 *   ref 桥接，改为直接比对输入值 + 受控 error 态，行为等价。
 * - 校验不通过时 `onOk` 抛错，antd 会保持弹窗打开（与 Vue 的 validate 失败一致）。
 */
export function confirmDeleteModal(props: ConfirmModalProps = {}) {
  const confirmText = props.confirmText ?? '确认删除';
  const placeholder = props.placeholder ?? `输入'${confirmText}'`;

  const state: ConfirmInputState = { setError: () => {}, value: '' };

  const content: ReactNode = (
    <ConfirmDeleteInput
      confirmText={confirmText}
      placeholder={placeholder}
      state={state}
    />
  );

  const config = {
    centered: true,
    content,
    okButtonProps: { danger: true, type: 'primary' as const },
    onOk: async () => {
      if (state.value !== confirmText) {
        state.setError('校验不通过');
        throw new Error('校验不通过');
      }
      await props.onValidated?.();
    },
    title: props.title ?? $t('pages.common.tip'),
  };

  const modal = getMessageApi()?.modal;
  if (modal) {
    modal.confirm(config);
  } else {
    Modal.confirm(config);
  }
}
