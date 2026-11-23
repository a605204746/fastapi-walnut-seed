import type { FormSchemaGetter } from '@/adapter/form';

import type { GridColumn } from '@/adapter/grid';

import { DictEnum } from '@/core/constants';

import { getDictOptions } from '@/utils/dict';
import {
  renderBrowserIcon,
  renderDict,
  renderOsIcon,
} from '@/utils/render';

/** 登录日志的查询表单 / 表格列（对应 Vue 侧 monitor/logininfor/data.tsx） */

export const querySchema: FormSchemaGetter = () => [
  {
    component: 'Input',
    fieldName: 'ipaddr',
    label: 'IP地址',
  },
  {
    component: 'Input',
    fieldName: 'userName',
    label: '用户账号',
  },
  {
    component: 'Select',
    componentProps: {
      options: getDictOptions(DictEnum.SYS_COMMON_STATUS),
    },
    fieldName: 'status',
    label: '登录状态',
  },
  {
    component: 'RangePicker',
    fieldName: 'dateTime',
    label: '登录日期',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    field: 'userName',
    title: '用户账号',
  },
  {
    field: 'clientKey',
    title: '登录平台',
  },
  {
    field: 'ipaddr',
    title: 'IP地址',
  },
  {
    field: 'loginLocation',
    title: 'IP地点',
    width: 200,
  },
  {
    field: 'browser',
    slots: {
      default: ({ row }) => (
        <div className="flex items-center justify-center gap-[6px]">
          {renderBrowserIcon(row.browser, 'shrink-0')}
          {row.browser}
        </div>
      ),
    },
    title: '浏览器',
  },
  {
    field: 'os',
    slots: {
      default: ({ row }) => {
        // "Windows 10 or Windows Server 2016" 太长了，分割一下；详情里依旧能看到完整值
        let value = row.os as string;
        if (value) {
          const split = value.split(' or ');
          if (split.length === 2) {
            value = split[0]!;
          }
        }
        return (
          <div className="flex items-center justify-center gap-[6px]">
            {renderOsIcon(row.os, 'shrink-0')}
            {value}
          </div>
        );
      },
    },
    title: '系统',
  },
  {
    field: 'status',
    slots: {
      default: ({ row }) => renderDict(row.status, DictEnum.SYS_COMMON_STATUS),
    },
    title: '登录结果',
  },
  {
    field: 'msg',
    title: '信息',
  },
  {
    field: 'loginTime',
    title: '日期',
  },
  {
    field: 'action',
    fixed: 'right',
    slots: { default: 'action' },
    title: '操作',
    width: 150,
  },
];
