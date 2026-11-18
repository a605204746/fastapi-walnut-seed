import { forwardRef, useImperativeHandle } from 'react';

import { useVbenDrawer } from '@vben-core/popup-ui';
import { useParams } from 'react-router';

import { useGrid } from '@/adapter/grid';
import { roleSelectAll, roleUnallocatedList } from '@/api/system/role';

import { columns, querySchema } from './data';

export interface RoleAssignDrawerHandle {
  open: () => void;
}

export interface RoleAssignDrawerProps {
  onReload: () => void;
}

/**
 * 「选择用户」抽屉（对应 Vue 侧 role-assign/role-assign-drawer.vue）。
 *
 * 抽屉里嵌一张未分配用户表格，勾选后确定即批量授权。
 * `destroyOnClose: true` 保证每次打开都重新拉数据。
 *
 * Vue 的 `connectedComponent` 父子挂载改为「组件自持 drawer + grid，父组件通过
 * ref 命令式打开」，与 config-modal / post-drawer 的范式一致。
 */
export const RoleAssignDrawer = forwardRef<
  RoleAssignDrawerHandle,
  RoleAssignDrawerProps
>(function RoleAssignDrawer({ onReload }, ref) {
  const { roleId = '' } = useParams<{ roleId: string }>();

  const formOptions = {
    commonConfig: {
      labelWidth: 80,
    },
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3',
  };

  const gridOptions = {
    checkboxConfig: { highlight: true, reserve: true },
    // 抽屉里的表格不需要操作列
    columns: columns.filter((item) => item.field !== 'action'),
    height: 'auto',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          return await roleUnallocatedList({
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

  const [BasicDrawer, drawerApi] = useVbenDrawer({
    destroyOnClose: true,
    onCancel: handleReset,
    onConfirm: handleSubmit,
  });

  useImperativeHandle(
    ref,
    () => ({
      open: () => {
        drawerApi.setData({});
        drawerApi.open();
      },
    }),
    [drawerApi],
  );

  async function handleSubmit() {
    const records = tableApi.grid.getCheckboxRecords();
    const userIds = records.map((item: any) => item.id);
    if (userIds.length > 0) {
      await roleSelectAll(roleId, userIds);
    }
    handleReset();
    onReload();
  }

  function handleReset() {
    void drawerApi.close();
  }

  return (
    <BasicDrawer class="w-[800px]" title="选择用户">
      <BasicTable />
    </BasicDrawer>
  );
});

export default RoleAssignDrawer;
