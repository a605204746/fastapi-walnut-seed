import { useEffect, useState } from 'react';

import { $t } from '@/core/locales';
import { Modal, Switch } from 'antd';

import { getMessageApi } from '@/services/message';

export interface ApiSwitchProps {
  /** 状态变更接口；抛错则回滚开关 */
  api: (checked: boolean) => PromiseLike<void>;
  /** 选中的文本，默认 i18n「启用」 */
  checkedText?: string;
  /** 更新前是否弹窗确认，默认 false */
  confirm?: boolean;
  /** 确认弹窗内容，参数是更新后的值 */
  confirmText?: (checked: boolean) => string;
  disabled?: boolean;
  /** 更新成功后回调（对应 Vue 侧 emit('reload')） */
  onReload?: () => void;
  /** 未选中的文本，默认 i18n「禁用」 */
  unCheckedText?: string;
  /** 受控值（对应 Vue 侧 `v-model:value`） */
  value?: boolean;
}

/**
 * 带接口调用的状态开关（对应 Vue 侧 `components/global/api-switch.vue`）。
 *
 * 语义逐条对齐：切换即调接口 → 成功 emit reload → 失败**回滚**到原状态；
 * `confirm` 为 true 时先弹确认框，取消也回滚；变更事件要 stopPropagation，
 * 否则会跟表格的行选中/行点击冲突。
 *
 * 与 Vue 版的差异：
 * - `defineModel('value')` → 受控 prop `value` + 内部 state 镜像。内部 state 是
 *   必需的：乐观切换和失败回滚都要能脱离父组件立即生效。
 * - `checkedText` 用 `$t` 在**渲染期**求值（Vue 侧用 computed 达到同样效果），
 *   语言切换后自动更新。
 * - `window.modal.confirm` → `getMessageApi()?.modal.confirm`，未初始化时回退 antd 静态方法。
 */
export function ApiSwitch({
  api,
  checkedText,
  confirm = false,
  confirmText,
  disabled,
  onReload,
  unCheckedText,
  value = false,
}: ApiSwitchProps) {
  const [checked, setChecked] = useState(value);
  const [loading, setLoading] = useState(false);

  // 父组件的数据刷新（翻页 / reload）后同步外部值
  useEffect(() => {
    setChecked(value);
  }, [value]);

  async function callApi(next: boolean, lastStatus: boolean) {
    try {
      setLoading(true);
      await api(next);
      onReload?.();
    } catch {
      setChecked(lastStatus);
    } finally {
      setLoading(false);
    }
  }

  function confirmUpdate(next: boolean, lastStatus: boolean) {
    const content = confirmText
      ? confirmText(next)
      : '确认要更新状态吗？';
    const config = {
      centered: true,
      content,
      onCancel: () => setChecked(lastStatus),
      onOk: async () => {
        await callApi(next, lastStatus);
      },
      title: $t('pages.common.tip'),
    };
    const modal = getMessageApi()?.modal;
    if (modal) {
      modal.confirm(config);
    } else {
      Modal.confirm(config);
    }
  }

  function handleChange(next: boolean, event: any) {
    // 阻止事件冒泡，否则会跟行选中/行点击冲突
    event?.stopPropagation?.();
    const lastStatus = !next;
    setChecked(next);
    if (confirm) {
      confirmUpdate(next, lastStatus);
      return;
    }
    void callApi(next, lastStatus);
  }

  return (
    <Switch
      checked={checked}
      checkedChildren={checkedText ?? $t('pages.common.enable')}
      disabled={disabled}
      loading={loading}
      onChange={handleChange}
      unCheckedChildren={unCheckedText ?? $t('pages.common.disable')}
    />
  );
}

export default ApiSwitch;
