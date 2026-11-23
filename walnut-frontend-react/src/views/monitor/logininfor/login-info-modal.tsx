import { forwardRef, useImperativeHandle, useState } from 'react';

import type { DescriptionsProps } from 'antd';

import type { LoginLog } from '@/api/monitor/logininfo/model';

import { useWalnutModal } from '@/core/popup-ui';
import { DictEnum } from '@/core/constants';
import { Descriptions } from 'antd';

import { useDictStore } from '@/store/dict';
import {
  renderBrowserIcon,
  renderDict,
  renderOsIcon,
} from '@/utils/render';

export interface LoginInfoModalHandle {
  open: (record: LoginLog) => void;
}

/**
 * 登录日志详情弹窗（对应 Vue 侧 monitor/logininfor/login-info-modal.vue）。
 *
 * `footer: false` + `fullscreenButton: false` 走 useWalnutModal 的 options
 * （React 侧 ModalRendererProps 只暴露 title/width/class，其余状态在 ModalState 里）。
 * Descriptions 的 item 字段名与 antdv-next 不同：Vue 用 `content`，antd React 用 `children`。
 */
export const LoginInfoModal = forwardRef<LoginInfoModalHandle>(
  function LoginInfoModal(_props, ref) {
    // 订阅字典 store：字典异步到达后重渲染，让状态 Tag 显示出来
    useDictStore((state) => state.map);
    const [loginInfo, setLoginInfo] = useState<LoginLog | undefined>();

    const [BasicModal, modalApi] = useWalnutModal({
      footer: false,
      fullscreenButton: false,
      onClosed: () => setLoginInfo(undefined),
      onOpenChange: (isOpen) => {
        if (!isOpen) return;
        setLoginInfo(modalApi.getData<LoginLog>());
      },
    });

    useImperativeHandle(
      ref,
      () => ({
        open: (record) => {
          modalApi.setData(record);
          modalApi.open();
        },
      }),
      [modalApi],
    );

    const items: DescriptionsProps['items'] = (() => {
      if (!loginInfo) return [];
      const data = loginInfo;
      // "Windows 10 or Windows Server 2016" 太长，列表里已截断，详情保留完整值
      return [
        {
          children: renderDict(data.status, DictEnum.SYS_COMMON_STATUS),
          label: '登录状态',
        },
        {
          children: data.clientKey?.toLowerCase(),
          label: '登录平台',
        },
        {
          children: `账号: ${data.userName} / ${data.ipaddr} / ${data.loginLocation}`,
          label: '账号信息',
        },
        {
          children: data.loginTime,
          label: '登录时间',
        },
        {
          children: (
            <span
              className={
                data.status === '0' ? 'font-semibold' : 'font-semibold text-red-500'
              }
            >
              {data.msg}
            </span>
          ),
          label: '登录信息',
        },
        {
          children: (
            <div className="flex items-center gap-[6px]">
              {renderOsIcon(data.os, 'shrink-0')}
              {data.os}
            </div>
          ),
          label: '登录设备',
        },
        {
          children: (
            <div className="flex items-center gap-[6px]">
              {renderBrowserIcon(data.browser, 'shrink-0')}
              {data.browser}
            </div>
          ),
          label: '浏览器',
        },
      ];
    })();

    return (
      <BasicModal class="w-[550px]" title="登录日志">
        {loginInfo ? (
          <Descriptions bordered column={1} items={items} size="small" />
        ) : null}
      </BasicModal>
    );
  },
);

export default LoginInfoModal;
