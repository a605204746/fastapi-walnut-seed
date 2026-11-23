import { useRef } from 'react';

import type { PageQuery } from '@/api/common';
import type { OperationLog } from '@/api/monitor/operlog/model';

import { $t } from '@/core/locales';
import { Button, Space } from 'antd';

import { addSortParams, useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  operLogClean,
  operLogDelete,
  operLogExport,
  operLogList,
} from '@/api/monitor/operlog';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';
import { confirmDeleteModal } from '@/utils/modal';

import { columns, querySchema } from './data';
import {
  OperationPreviewDrawer,
  type OperationPreviewDrawerHandle,
} from './operation-preview-drawer';

/**
 * 操作日志页（对应 Vue 侧 views/monitor/operlog/index.vue）。
 *
 * 与 Vue 版的差异：Vue 用 `gridEvents.sortChange: () => tableApi.query()` 触发重查，
 * React 侧的排序变化由 `adapter/grid` 内部的 Table `onChange` 统一处理
 * （`sortConfig.remote: true` 时自动回到第一页重新查询），页面侧不需要事件通道。
 */
export default function OperLogPage() {
  useDictStore((state) => state.map);
  const drawerRef = useRef<OperationPreviewDrawerHandle>(null);

  const formOptions = {
    commonConfig: {
      componentProps: { allowClear: true },
      labelWidth: 80,
    },
    // 日期选择格式化
    fieldMappingTime: [
      [
        'createTime',
        ['params[beginTime]', 'params[endTime]'],
        ['YYYY-MM-DD 00:00:00', 'YYYY-MM-DD 23:59:59'],
      ],
    ] as [string, [string, string], [string, string]][],
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
  };

  const gridOptions = {
    checkboxConfig: { highlight: true, reserve: true },
    columns,
    height: 'auto',
    id: 'monitor-operlog-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          {
            page,
            sorts,
          }: {
            page: { currentPage: number; pageSize: number };
            sorts: { field: string; order: 'asc' | 'desc' }[];
          },
          formValues: Record<string, any> = {},
        ) => {
          const params: PageQuery = {
            pageNum: page.currentPage,
            pageSize: page.pageSize,
            ...formValues,
          };
          // 添加排序参数（orderByColumn / isAsc）
          addSortParams(params, sorts);
          return await operLogList(params);
        },
      },
    },
    rowConfig: { keyField: 'id' },
    sortConfig: {
      // 支持多字段排序
      multiple: true,
      // 远程排序
      remote: true,
    },
  };

  const [BasicTable, tableApi] = useGrid({ formOptions, gridOptions });

  const { buildExportFileName, exportBlob, exportLoading } = useBlobExport(
    operLogExport,
  );

  async function handleExport() {
    const formValues = await tableApi.formApi?.getValues();
    const fileName = buildExportFileName('操作日志');
    await exportBlob({ data: formValues, fileName });
  }

  /** 清空全部日志（不可逆，需二次输入确认） */
  function handleClear() {
    confirmDeleteModal({
      onValidated: async () => {
        await operLogClean();
        await tableApi.reload();
      },
    });
  }

  function handleDelete() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: OperationLog) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认删除选中的${ids.length}条操作日志吗？`,
      okType: 'danger',
      onOk: async () => {
        await operLogDelete(ids);
        await tableApi.query();
      },
      title: $t('pages.common.tip'),
    });
  }

  return (
    <Page autoContentHeight>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Access code={['monitor:operlog:list']}>
              <Button
                onClick={(e) => {
                  e.stopPropagation();
                  drawerRef.current?.open(row as OperationLog);
                }}
                size="small"
                type="link"
              >
                {$t('pages.common.preview')}
              </Button>
            </Access>
          ),
        }}
        tableTitle="操作日志列表"
        toolbarTools={() => (
          <Space>
            <Access code={['monitor:operlog:remove']}>
              <Button onClick={handleClear}>{$t('pages.common.clear')}</Button>
            </Access>
            <Access code={['monitor:operlog:export']}>
              <Button
                disabled={exportLoading}
                loading={exportLoading}
                onClick={handleExport}
              >
                {$t('pages.common.export')}
              </Button>
            </Access>
            <Access code={['monitor:operlog:remove']}>
              <Button
                danger
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={handleDelete}
                type="primary"
              >
                {$t('pages.common.delete')}
              </Button>
            </Access>
          </Space>
        )}
      />
      <OperationPreviewDrawer ref={drawerRef} />
    </Page>
  );
}
