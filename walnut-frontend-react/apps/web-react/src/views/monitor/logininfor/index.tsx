import { useRef } from 'react';

import type { LoginLog } from '@/api/monitor/logininfo/model';

import { $t } from '@walnut/locales';
import { Button, Popconfirm, Space } from 'antd';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  loginInfoClean,
  loginInfoExport,
  loginInfoList,
  loginInfoRemove,
  userUnlock,
} from '@/api/monitor/logininfo';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';
import { confirmDeleteModal } from '@/utils/modal';

import { columns, querySchema } from './data';
import {
  LoginInfoModal,
  type LoginInfoModalHandle,
} from './login-info-modal';

/**
 * 登录日志页（对应 Vue 侧 views/monitor/logininfor/index.vue）。
 *
 * 与 Vue 版的差异：Vue 用 `gridEvents.checkboxChange` 维护 `canUnlock` 这个 ref，
 * React 侧直接在 `toolbarTools` 渲染期从 store 读选中行计算 —— GridView 已经
 * 订阅了 `selectedKeys`，选中变化会触发工具栏重渲染，不需要额外的事件通道。
 */
export default function LoginInfoPage() {
  useDictStore((state) => state.map);
  const modalRef = useRef<LoginInfoModalHandle>(null);

  const formOptions = {
    commonConfig: {
      componentProps: { allowClear: true },
      labelWidth: 80,
    },
    // 日期选择格式化
    fieldMappingTime: [
      [
        'dateTime',
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
    id: 'monitor-logininfo-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          return await loginInfoList({
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

  const { buildExportFileName, exportBlob, exportLoading } = useBlobExport(
    loginInfoExport,
  );

  async function handleExport() {
    const formValues = await tableApi.formApi?.getValues();
    const fileName = buildExportFileName('登录日志');
    await exportBlob({ data: formValues, fileName });
  }

  function handleClear() {
    confirmDeleteModal({
      onValidated: async () => {
        await loginInfoClean();
        await tableApi.reload();
      },
    });
  }

  async function handleDelete(row: LoginLog) {
    await loginInfoRemove([row.id]);
    await tableApi.query();
  }

  function handleMultiDelete() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: LoginLog) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认删除选中的${ids.length}条记录吗？`,
      okType: 'danger',
      onOk: async () => {
        await loginInfoRemove(ids);
        await tableApi.query();
      },
      title: $t('pages.common.tip'),
    });
  }

  async function handleUnlock() {
    const records = tableApi.grid.getCheckboxRecords();
    if (records.length !== 1) return;
    const { userName } = records[0] as LoginLog;
    await userUnlock(userName);
    await tableApi.query();
    tableApi.grid.clearCheckboxRow();
  }

  return (
    <Page autoContentHeight>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Space>
              <Button
                onClick={(e) => {
                  e.stopPropagation();
                  modalRef.current?.open(row as LoginLog);
                }}
                size="small"
                type="link"
              >
                {$t('pages.common.info')}
              </Button>
              <Access code={['monitor:logininfor:remove']}>
                <Popconfirm
                  onConfirm={() => handleDelete(row)}
                  placement="left"
                  title="确认删除?"
                >
                  <Button
                    danger
                    onClick={(e) => e.stopPropagation()}
                    size="small"
                    type="link"
                  >
                    {$t('pages.common.delete')}
                  </Button>
                </Popconfirm>
              </Access>
            </Space>
          ),
        }}
        tableTitle="登录日志列表"
        toolbarTools={() => {
          // 「解锁」只在恰好选中一条且该条为锁定态（status === '1'）时可用
          const records = tableApi.grid.getCheckboxRecords();
          const canUnlock =
            records.length === 1 && (records[0] as LoginLog)?.status === '1';
          return (
            <Space>
              <Access code={['monitor:logininfor:remove']}>
                <Button onClick={handleClear}>
                  {$t('pages.common.clear')}
                </Button>
              </Access>
              <Access code={['monitor:logininfor:export']}>
                <Button
                  disabled={exportLoading}
                  loading={exportLoading}
                  onClick={handleExport}
                >
                  {$t('pages.common.export')}
                </Button>
              </Access>
              <Access code={['monitor:logininfor:remove']}>
                <Button
                  danger
                  disabled={!vxeCheckboxChecked(tableApi)}
                  onClick={handleMultiDelete}
                  type="primary"
                >
                  {$t('pages.common.delete')}
                </Button>
              </Access>
              <Access code={['monitor:logininfor:unlock']}>
                <Button
                  disabled={!canUnlock}
                  onClick={handleUnlock}
                  type="primary"
                >
                  {$t('pages.common.unlock')}
                </Button>
              </Access>
            </Space>
          );
        }}
      />
      <LoginInfoModal ref={modalRef} />
    </Page>
  );
}
