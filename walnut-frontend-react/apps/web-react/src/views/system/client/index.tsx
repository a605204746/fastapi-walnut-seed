import { useRef } from 'react';

import type { Client } from '@/api/system/client/model';

import { $t } from '@vben/locales';
import { DEFAULT_CLIENT_ID, EnableStatus } from '@vben/constants';
import { Button, Popconfirm, Space } from 'antd';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  clientChangeStatus,
  clientExport,
  clientList,
  clientRemove,
} from '@/api/system/client';
import { Access, useAccess } from '@/components/access/Access';
import { ApiSwitch } from '@/components/global/api-switch';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';

import { ClientDrawer, type ClientDrawerHandle } from './client-drawer';
import { columns, querySchema } from './data';

/**
 * 客户端管理页（对应 Vue 侧 views/system/client/index.vue）。
 *
 * 两处特殊约束（与 Vue 侧一致）：
 * - `checkMethod` 让 id === 1 的内置客户端**不可勾选**，避免被批量删除
 * - 状态开关对内置客户端或无 `system:client:edit` 权限时**禁用而非隐藏**
 *   （PC 端被停用会直接登录不了，报「认证权限类型已禁用」）
 */
export default function ClientPage() {
  useDictStore((state) => state.map);
  const drawerRef = useRef<ClientDrawerHandle>(null);
  const { hasAccessByCodes } = useAccess();

  const formOptions = {
    commonConfig: {
      componentProps: { allowClear: true },
      labelWidth: 80,
    },
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
  };

  const gridOptions = {
    checkboxConfig: {
      // 内置客户端（id === 1）不允许勾选
      checkMethod: ({ row }: { row: Client }) => row?.id !== 1,
      highlight: true,
      reserve: true,
    },
    columns,
    height: 'auto',
    id: 'system-client-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          return await clientList({
            pageNum: page.currentPage,
            pageSize: page.pageSize,
            ...formValues,
          });
        },
      },
    },
    rowConfig: { keyField: 'id' },
  };

  const [BasicTable, tableApi] = useGrid({ formOptions, gridOptions });

  const { buildExportFileName, exportBlob, exportLoading } =
    useBlobExport(clientExport);

  async function handleExport() {
    const formValues = await tableApi.formApi?.getValues();
    const fileName = buildExportFileName('客户端数据');
    await exportBlob({ data: formValues, fileName });
  }

  async function handleDelete(row: Client) {
    await clientRemove([row.id]);
    await tableApi.query();
  }

  function handleMultiDelete() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: Client) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认删除选中的${ids.length}条记录吗？`,
      okType: 'danger',
      onOk: async () => {
        await clientRemove(ids);
        await tableApi.query();
      },
      title: $t('pages.common.tip'),
    });
  }

  async function handleChangeStatus(checked: boolean, row: Client) {
    await clientChangeStatus({
      clientId: row.clientId,
      status: checked ? EnableStatus.Enable : EnableStatus.Disable,
    });
  }

  return (
    <Page autoContentHeight>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Space>
              <Access code={['system:client:edit']}>
                <Button
                  onClick={(e) => {
                    e.stopPropagation();
                    drawerRef.current?.openEdit((row as Client).id);
                  }}
                  size="small"
                  type="link"
                >
                  {$t('pages.common.edit')}
                </Button>
              </Access>
              <Popconfirm
                disabled={(row as Client).id === 1}
                onConfirm={() => handleDelete(row as Client)}
                placement="left"
                title="确认删除？"
              >
                <Access code={['system:client:remove']}>
                  <Button
                    danger
                    disabled={(row as Client).id === 1}
                    onClick={(e) => e.stopPropagation()}
                    size="small"
                    type="link"
                  >
                    {$t('pages.common.delete')}
                  </Button>
                </Access>
              </Popconfirm>
            </Space>
          ),
          status: ({ row }) => (
            <ApiSwitch
              api={(checked) => handleChangeStatus(checked, row as Client)}
              disabled={
                (row as Client).id === DEFAULT_CLIENT_ID ||
                !hasAccessByCodes(['system:client:edit'])
              }
              onReload={() => void tableApi.query()}
              value={(row as Client).status === EnableStatus.Enable}
            />
          ),
        }}
        tableTitle="客户端列表"
        toolbarTools={() => (
          <Space>
            <Access code={['system:client:export']}>
              <Button
                disabled={exportLoading}
                loading={exportLoading}
                onClick={handleExport}
              >
                {$t('pages.common.export')}
              </Button>
            </Access>
            <Access code={['system:client:remove']}>
              <Button
                danger
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={handleMultiDelete}
                type="primary"
              >
                {$t('pages.common.delete')}
              </Button>
            </Access>
            <Access code={['system:client:add']}>
              <Button
                onClick={() => drawerRef.current?.openAdd()}
                type="primary"
              >
                {$t('pages.common.add')}
              </Button>
            </Access>
          </Space>
        )}
      />
      <ClientDrawer onReload={() => void tableApi.query()} ref={drawerRef} />
    </Page>
  );
}
