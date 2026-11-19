import type { FormSchema } from '@vben-core/form-ui';
import type { GridColumn } from '@/adapter/grid';
import { DictEnum } from '@vben/constants';
import { getPopupContainer } from '@vben/utils';

import { getDictOptions } from '@/utils/dict';
import { renderDict } from '@/utils/render';

export const querySchema: () => FormSchema[] = () => [
  {
    component: 'Input',
    fieldName: 'noticeTitle',
    label: '公告标题',
  },
  {
    component: 'Input',
    fieldName: 'createByName',
    label: '创建人',
  },
  {
    component: 'Select',
    componentProps: {
      getPopupContainer,
      options: getDictOptions(DictEnum.SYS_NOTICE_TYPE),
    },
    fieldName: 'noticeType',
    label: '公告类型',
  },
];

export const columns: GridColumn[] = [
  { type: 'checkbox', width: 60 },
  {
    title: '公告标题',
    field: 'noticeTitle',
    align: 'left',
  },
  {
    title: '公告类型',
    field: 'noticeType',
    width: 120,
    slots: {
      default: ({ row }) => {
        return renderDict(row.noticeType, DictEnum.SYS_NOTICE_TYPE);
      },
    },
  },
  {
    title: '状态',
    field: 'status',
    width: 120,
    slots: {
      default: ({ row }) => {
        return renderDict(row.status, DictEnum.SYS_NOTICE_STATUS);
      },
    },
  },
  {
    title: '创建人',
    field: 'createByName',
    width: 150,
  },
  {
    title: '创建时间',
    field: 'createTime',
  },
  {
    field: 'action',
    fixed: 'right',
    slots: { default: 'action' },
    title: '操作',
    resizable: false,
    width: 'auto',
  },
];
