import { useState } from 'react';
import { Button, Popconfirm, Space } from 'antd';
import { useNavigate } from 'react-router';
import { ADMIN_ROLE_KEY, SUPERADMIN_ROLE_ID, SUPERADMIN_ROLE_KEY } from '@vben/constants';
import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import { roleChangeStatus, roleExport, roleList, roleRemove } from '@/api/system/role';
import { Access, useAccess } from '@/components/access/Access';
import { ApiSwitch } from '@/components/global/api-switch';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';
import { columns, querySchema } from './data';
import { RoleEditor } from './role-editor';

export default function RolePage() {
  useDictStore((s) => s.map);
  const navigate = useNavigate();
  const access = useAccess();
  const [editor, setEditor] = useState<{ id?: number | string; scope?: boolean } | null>(null);
  const [BasicTable, tableApi] = useGrid({
    formOptions: {
      commonConfig: { labelWidth: 80, componentProps: { allowClear: true } },
      schema: querySchema(),
      wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
      fieldMappingTime: [
        [
          'createTime',
          ['params[beginTime]', 'params[endTime]'],
          ['YYYY-MM-DD 00:00:00', 'YYYY-MM-DD 23:59:59'],
        ],
      ],
    },
    gridOptions: {
      id: 'system-role-index',
      height: 'auto',
      columns,
      pagerConfig: {},
      checkboxConfig: {
        reserve: true,
        highlight: true,
        checkMethod: ({ row }) => row.id !== SUPERADMIN_ROLE_ID,
      },
      rowConfig: { keyField: 'id' },
      proxyConfig: {
        ajax: {
          query: ({ page }, values) =>
            roleList({ pageNum: page.currentPage, pageSize: page.pageSize, ...values }),
        },
      },
    },
  });
  const { exportBlob, exportLoading, buildExportFileName } = useBlobExport(roleExport);
  const reload = () => {
    void tableApi.query();
  };
  async function remove(ids: (number | string)[]) {
    await roleRemove(ids);
    reload();
  }
  return (
    <Page autoContentHeight>
      <BasicTable
        tableTitle="角色列表"
        toolbarTools={() => (
          <Space>
            <Access code={['system:role:export']}>
              <Button
                loading={exportLoading}
                onClick={async () =>
                  exportBlob({
                    data: await tableApi.formApi?.getValues(),
                    fileName: buildExportFileName('角色数据'),
                  })
                }
              >
                导出
              </Button>
            </Access>
            <Access code={['system:role:remove']}>
              <Button
                danger
                type="primary"
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={() => {
                  const ids = tableApi.grid.getCheckboxRecords().map((row) => row.id);
                  getMessageApi()?.modal.confirm({
                    title: '提示',
                    content: `确认删除选中的${ids.length}条记录吗？`,
                    okType: 'danger',
                    onOk: () => remove(ids),
                  });
                }}
              >
                删除
              </Button>
            </Access>
            <Access code={['system:role:add']}>
              <Button type="primary" onClick={() => setEditor({})}>
                新增
              </Button>
            </Access>
          </Space>
        )}
        slots={{
          status: ({ row }) => (
            <ApiSwitch
              value={row.status === '0'}
              disabled={
                row.id === SUPERADMIN_ROLE_ID ||
                row.roleKey === ADMIN_ROLE_KEY ||
                !access.hasAccessByCodes(['system:role:edit'])
              }
              api={(checked) => roleChangeStatus({ id: row.id, status: checked ? '0' : '1' })}
              onReload={reload}
            />
          ),
          action: ({ row }) =>
            !row.superAdmin &&
            (row.roleKey !== ADMIN_ROLE_KEY || access.hasAccessByRoles([SUPERADMIN_ROLE_KEY])) ? (
              <Space>
                <Access code={['system:role:edit']}>
                  <Button type="link" size="small" onClick={() => setEditor({ id: row.id })}>
                    编辑
                  </Button>
                  <Button
                    type="link"
                    size="small"
                    onClick={() => setEditor({ id: row.id, scope: true })}
                  >
                    权限
                  </Button>
                  <Button
                    type="link"
                    size="small"
                    onClick={() => navigate(`/system/role-auth/user/${row.id}`)}
                  >
                    分配
                  </Button>
                </Access>
                <Access code={['system:role:remove']}>
                  <Popconfirm title="确认删除？" onConfirm={() => remove([row.id])}>
                    <Button danger type="link" size="small">
                      删除
                    </Button>
                  </Popconfirm>
                </Access>
              </Space>
            ) : null,
        }}
      />
      {editor && <RoleEditor {...editor} onClose={() => setEditor(null)} onSaved={reload} />}
    </Page>
  );
}
