import { forwardRef, useImperativeHandle, useState } from 'react';

import type { DescriptionsProps } from 'antd';

import type { OperationLog } from '@/api/monitor/operlog/model';

import { useWalnutDrawer } from '@/core/popup-ui';
import { DictEnum } from '@/core/constants';
import { Descriptions, Tag } from 'antd';

import { useDictStore } from '@/store/dict';
import {
  renderDict,
  renderHttpMethodTag,
  renderJsonPreview,
} from '@/utils/render';

export interface OperationPreviewDrawerHandle {
  open: (record: OperationLog) => void;
}

/**
 * 操作日志详情抽屉（对应 Vue 侧 monitor/operlog/operation-preview-drawer.vue）。
 *
 * 与 Vue 版的差异：
 * - `shallowRef` → `useState`（React 无需区分深浅响应）。
 * - Descriptions 的 item 字段 Vue 用 `content`，antd React 用 `children`。
 * - `:classes="{ label: ... }"` → `styles={{ label: ... }}`（antd 6 语义化样式）。
 * - 条件项（errorMsg / jsonResult 为空时不显示）用 `.filter(Boolean)` 保留原逻辑。
 */
export const OperationPreviewDrawer =
  forwardRef<OperationPreviewDrawerHandle>(
    function OperationPreviewDrawer(_props, ref) {
      // 订阅字典 store：字典异步到达后重渲染
      useDictStore((state) => state.map);
      const [currentLog, setCurrentLog] = useState<null | OperationLog>(null);

      const [BasicDrawer, drawerApi] = useWalnutDrawer({
        footer: false,
        onClosed: () => setCurrentLog(null),
        onOpenChange: (isOpen) => {
          if (!isOpen) return;
          const { record } = drawerApi.getData<{ record: OperationLog }>();
          setCurrentLog(record);
        },
      });

      useImperativeHandle(
        ref,
        () => ({
          open: (record) => {
            drawerApi.setData({ record });
            drawerApi.open();
          },
        }),
        [drawerApi],
      );

      const items: DescriptionsProps['items'] = (() => {
        if (!currentLog) return [];
        const data = currentLog;
        const actionInfo = `账号: ${data.operName} / ${data.deptName} / ${data.operIp} / ${data.operLocation}`;
        return [
          { children: data.id, label: '日志编号' },
          {
            children: renderDict(data.status, DictEnum.SYS_COMMON_STATUS),
            label: '操作结果',
          },
          {
            children: (
              <div className="flex items-center gap-2">
                <Tag>{data.title}</Tag>
                {renderDict(data.businessType, DictEnum.SYS_OPER_TYPE)}
              </div>
            ),
            label: '操作模块',
          },
          { children: actionInfo, label: '操作信息' },
          {
            children: (
              <div className="flex items-center gap-2">
                {renderHttpMethodTag(data.requestMethod)}
                {data.operUrl}
              </div>
            ),
            label: '请求信息',
          },
          data.errorMsg
            ? {
                children: (
                  <span className="font-semibold text-red-600">
                    {data.errorMsg}
                  </span>
                ),
                label: '异常信息',
              }
            : undefined,
          { children: data.method, label: '方法' },
          {
            children: (
              <div className="max-h-[300px] overflow-y-auto">
                {renderJsonPreview(data.operParam)}
              </div>
            ),
            label: '请求参数',
          },
          data.jsonResult
            ? {
                children: (
                  <div className="max-h-[300px] overflow-y-auto">
                    {renderJsonPreview(data.jsonResult)}
                  </div>
                ),
                label: '响应参数',
              }
            : undefined,
          { children: `${data.costTime} ms`, label: '请求耗时' },
          { children: `${data.operTime}`, label: '操作时间' },
        ].filter(Boolean) as DescriptionsProps['items'];
      })();

      return (
        <BasicDrawer class="w-[600px]" title="查看日志">
          {currentLog ? (
            <Descriptions
              bordered
              column={1}
              items={items}
              size="small"
              styles={{ label: { minWidth: 120 } }}
            />
          ) : null}
        </BasicDrawer>
      );
    },
  );

export default OperationPreviewDrawer;
