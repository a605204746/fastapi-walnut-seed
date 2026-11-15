import type { ComponentType } from 'react';

import type { FormProps } from './types';
import type { FormRendererProps } from './form';

import { useCallback, useRef } from 'react';

import { FormRenderer } from './form';
import { FormApi } from './form-api';

/** 对外暴露的 Form 组件 props（formApi 由 hook 内部注入，调用方无需传） */
export type VbenFormComponentProps = Omit<FormRendererProps, 'formApi'>;

/**
 * 对应 Vue 侧 `use-vben-form.ts`：保留 `useVbenForm(options) → [Form, formApi]` 签名，
 * 使 32 处使用点（含 config 页查询表单 / 弹窗表单）机械搬运。
 *
 * options 每次渲染同步进 formApi（支持动态 schema，如登录页租户/验证码字段联动），
 * 但不重置已填值。
 */
export function useVbenForm(
  options: FormProps,
): [ComponentType<VbenFormComponentProps>, FormApi] {
  const formApiRef = useRef<FormApi | undefined>(undefined);
  if (!formApiRef.current) {
    formApiRef.current = new FormApi(options);
  }
  const formApi = formApiRef.current;
  formApi.syncOptions(options);

  const Form = useCallback(
    (props: VbenFormComponentProps) => (
      <FormRenderer {...props} formApi={formApi} />
    ),
    [formApi],
  );

  return [Form, formApi];
}
