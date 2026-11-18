import { useRef } from 'react';

import type { User } from '@/api/system/user/model';

import { $t } from '@vben/locales';
import { Button, Popconfirm, Space } from 'antd';
import { useParams } from 'react-router';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  roleAllocatedList,
  roleAuthCancel,
  roleAuthCancelAll,
} from '@/api/system/role';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';

import { columns, querySchema } from './data';
import {
  RoleAssignDrawer,
  type RoleAssignDrawerHandle,
} from './role-assign-drawer';

/**
 * 分配用户页（对应 Vue 侧 views/system/role-assign/index.vue）。
 *
 * 路由：后端菜单 `role-auth/user/:roleId` → `component: 'system/role/authUser'`，
 * 而 `system/role/authUser.tsx` 只是转发到本页（与 Vue 侧 authUser.vue 一致）。
 */
export default function RoleAssignPage() {
  const { roleId = '' } = useParams<{ roleId: string }>();
  const drawerRef = useRef<RoleAssignDrawerHandle>(null);

  const formOptions = {
    commonConfig: {
      componentProps: { allowClear: true },
      labelWidth: 80,
    },
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
  };

  const gridOptions = {
    checkboxConfig: { highlight: true, reserve: true },
    columns,
    height: 'auto',
    id: 'system-role-assign-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          return await roleAllocatedList({
            pageNum: page.currentPage,
            pageSize: page.pageSize,
            roleId,
            ...formValues,
          });
        },
      },
    },
    rowConfig: { keyField: 'id' },
  };

  const [BasicTable, tableApi] = useGrid({ formOptions, gridOptions });

  /** 取消授权单条记录 */
  async function handleAuthCancel(record: User) {
    await roleAuthCancel({ roleId, userId: record.id });
    await tableApi.query();
  }

  /** 批量取消授权 */
  function handleMultipleAuthCancel() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: User) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认取消选中的${ids.length}条授权记录吗？`,
      okType: 'danger',
      onOk: async () => {
        await roleAuthCancelAll(roleId, ids);
        await tableApi.query();
        tableApi.grid.clearCheckboxRow();
      },
      title: $t('pages.common.tip'),
    });
  }

  return (
    <Page autoContentHeight>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Popconfirm
              onConfirm={() => handleAuthCancel(row as User)}
              placement="left"
              title={`是否取消授权用户[${(row as User).userName} - ${
                (row as User).nickName
              }]?`}
            >
              <Access code={['system:role:remove']}>
                <Button
                  danger
                  onClick={(e) => e.stopPropagation()}
                  size="small"
                  type="link"
                >
                  取消授权
                </Button>
              </Access>
            </Popconfirm>
          ),
        }}
        tableTitle="已分配的用户列表"
        toolbarTools={() => (
          <Space>
            <Access code={['system:role:remove']}>
              <Button
                danger
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={handleMultipleAuthCancel}
                type="primary"
              >
                取消授权
              </Button>
            </Access>
            <Access code={['system:role:add']}>
              <Button
                onClick={() => drawerRef.current?.open()}
                type="primary"
              >
                {$t('pages.common.add')}
              </Button>
            </Access>
          </Space>
        )}
      />
      <RoleAssignDrawer
        onReload={() => void tableApi.query()}
        ref={drawerRef}
      />
    </Page>
  );
}
