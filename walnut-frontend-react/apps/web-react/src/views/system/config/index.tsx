import { useRef } from 'react';

import type { SysConfig } from '@/api/system/config/model';

import { $t } from '@walnut/locales';
import { Button, Popconfirm, Space } from 'antd';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  configExport,
  configList,
  configRefreshCache,
  configRemove,
} from '@/api/system/config';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';

import { ConfigModal, type ConfigModalHandle } from './config-modal';
import { columns, querySchema } from './data';

/**
 * 参数配置页（对应 Vue 侧 views/system/config/index.vue）。
 * 模式：Page > useGrid(查询表单 + 工具栏 + 表格 + 分页) + ConfigModal。
 */
export default function ConfigPage() {
  // 订阅字典 store：字典异步到达后重渲染（查询下拉 + 表格 DictTag）
  useDictStore((state) => state.map);
  const modalRef = useRef<ConfigModalHandle>(null);

  const formOptions = {
    commonConfig: {
      componentProps: { allowClear: true },
      labelWidth: 80,
    },
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
    id: 'system-config-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          return await configList({
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
    useBlobExport(configExport);

  async function handleExport() {
    const formValues = await tableApi.formApi?.getValues();
    const fileName = buildExportFileName('参数配置');
    await exportBlob({ data: formValues, fileName });
  }

  async function handleDelete(row: SysConfig) {
    await configRemove([row.id]);
    await tableApi.query();
  }

  function handleMultiDelete() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: SysConfig) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认删除选中的${ids.length}条记录吗？`,
      okType: 'danger',
      onOk: async () => {
        await configRemove(ids);
        await tableApi.query();
      },
      title: '提示',
    });
  }

  async function handleRefreshCache() {
    await configRefreshCache();
    await tableApi.query();
  }

  return (
    <Page autoContentHeight>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Space>
              <Access code={['system:config:edit']}>
                <Button
                  onClick={(e) => {
                    e.stopPropagation();
                    modalRef.current?.openEdit(row.id);
                  }}
                  size="small"
                  type="link"
                >
                  {$t('pages.common.edit')}
                </Button>
              </Access>
              <Access code={['system:config:remove']}>
                <Popconfirm
                  onConfirm={() => handleDelete(row)}
                  placement="left"
                  title="确认删除？"
                >
                  <Button danger onClick={(e) => e.stopPropagation()} size="small" type="link">
                    {$t('pages.common.delete')}
                  </Button>
                </Popconfirm>
              </Access>
            </Space>
          ),
        }}
        tableTitle="参数列表"
        toolbarTools={() => (
          <Space>
            <Button onClick={handleRefreshCache}>刷新缓存</Button>
            <Access code={['system:config:export']}>
              <Button
                disabled={exportLoading}
                loading={exportLoading}
                onClick={handleExport}
              >
                {$t('pages.common.export')}
              </Button>
            </Access>
            <Access code={['system:config:remove']}>
              <Button
                danger
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={handleMultiDelete}
                type="primary"
              >
                {$t('pages.common.delete')}
              </Button>
            </Access>
            <Access code={['system:config:add']}>
              <Button onClick={() => modalRef.current?.openAdd()} type="primary">
                {$t('pages.common.add')}
              </Button>
            </Access>
          </Space>
        )}
      />
      <ConfigModal onReload={() => void tableApi.query()} ref={modalRef} />
    </Page>
  );
}
