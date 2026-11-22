import { useRef, useState } from 'react';

import type { MenuProps } from 'antd';

import type { User } from '@/api/system/user/model';

import { $t } from '@walnut/locales';
import { EnableStatus, SUPERADMIN_USER_ID } from '@walnut/constants';
import { usePreferences } from '@walnut/preferences';
import { Avatar, Button, Dropdown, Popconfirm, Space } from 'antd';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  userExport,
  userList,
  userRemove,
  userStatusChange,
} from '@/api/system/user';
import { Access, useAccess } from '@/components/access/Access';
import { ApiSwitch } from '@/components/global/api-switch';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';

import { columns, querySchema } from './data';
import { DeptTree } from './dept-tree';
import { UserDrawer, type UserDrawerHandle } from './user-drawer';
import {
  UserImportModal,
  type UserImportModalHandle,
} from './user-import-modal';
import { UserInfoModal, type UserInfoModalHandle } from './user-info-modal';
import {
  UserResetPwdModal,
  type UserResetPwdModalHandle,
} from './user-reset-pwd-modal';

/**
 * 用户管理页（对应 Vue 侧 views/system/user/index.vue，329 行）。
 *
 * 布局与 post 页一致：左 DeptTree(260px) + 右表格；额外有头像列、状态开关、
 * 「更多」下拉（用户信息 / 重置密码）与导入弹窗。
 *
 * 与 Vue 版的差异：
 * - `tableApi.formApi.submitForm()` → `tableApi.reload()`。React 的 FormApi 没有
 *   vee-validate 的 submitForm；reload 同样是「回到第一页 + 带当前表单值重查」。
 * - `headerCellConfig` / `cellConfig` 的行高配置未移植（vxe 专有），
 *   antd 侧由 `size="small"` 决定，差异列入 B 阶段像素补齐。
 * - 超管行（id === SUPERADMIN_USER_ID）不渲染操作列，与 Vue 一致。
 */
export default function UserPage() {
  useDictStore((state) => state.map);
  const { preferences } = usePreferences();
  const { hasAccessByCodes } = useAccess();

  const drawerRef = useRef<UserDrawerHandle>(null);
  const importModalRef = useRef<UserImportModalHandle>(null);
  const infoModalRef = useRef<UserInfoModalHandle>(null);
  const resetPwdModalRef = useRef<UserResetPwdModalHandle>(null);

  // 左边部门树用
  const [selectDeptId, setSelectDeptId] = useState<string[]>([]);
  /** 查询闭包读 ref，原因同 post 页（state 更新异步） */
  const selectDeptIdRef = useRef<string[]>([]);

  function updateSelectDeptId(keys: string[]) {
    selectDeptIdRef.current = keys;
    setSelectDeptId(keys);
  }

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
    handleReset: async () => {
      updateSelectDeptId([]);
      await tableApi.formApi?.resetForm();
      await tableApi.reload();
    },
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3',
  };

  const gridOptions = {
    checkboxConfig: {
      // 超管账号不允许勾选（避免被批量删除）
      // ⚠️ `User.id` 类型标注是 string，但后端 seed 实际下发数字，
      // 因此统一 Number() 后再比（Vue 侧 row 是 any，所以没暴露这个类型不一致）
      checkMethod: ({ row }: { row: User }) =>
        Number(row?.id) !== SUPERADMIN_USER_ID,
      highlight: true,
      reserve: true,
    },
    columns,
    height: 'auto',
    id: 'system-user-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          // 部门树选择处理
          const deptIds = selectDeptIdRef.current;
          if (deptIds.length === 1) {
            formValues.deptId = deptIds[0];
          } else {
            delete formValues.deptId;
          }
          return await userList({
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
    useBlobExport(userExport);

  async function handleExport() {
    const formValues = await tableApi.formApi?.getValues();
    const fileName = buildExportFileName('用户信息');
    await exportBlob({ data: formValues, fileName });
  }

  async function handleDelete(row: User) {
    await userRemove([row.id]);
    await tableApi.query();
  }

  function handleMultiDelete() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: User) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认删除选中的${ids.length}条记录吗？`,
      okType: 'danger',
      onOk: async () => {
        await userRemove(ids);
        await tableApi.query();
      },
      title: $t('pages.common.tip'),
    });
  }

  async function handleChangeStatus(checked: boolean, row: User) {
    await userStatusChange({
      id: row.id,
      status: checked ? EnableStatus.Enable : EnableStatus.Disable,
    });
  }

  function handleDeptSelect(keys: string[]) {
    updateSelectDeptId(keys);
    void tableApi.reload();
  }

  async function handleDeptReload() {
    await tableApi.formApi?.resetForm();
    await tableApi.reload();
  }

  const menuItems: MenuProps['items'] = [
    { key: 'info', label: '用户信息' },
    ...(hasAccessByCodes(['system:user:resetPwd'])
      ? [{ key: 'resetPwd', label: '重置密码' }]
      : []),
  ];

  function handleMenuClick(key: string, row: User) {
    switch (key) {
      case 'info': {
        infoModalRef.current?.open(row.id);
        break;
      }
      case 'resetPwd': {
        resetPwdModalRef.current?.open(row);
        break;
      }
    }
  }

  return (
    <Page autoContentHeight>
      <div className="flex h-full gap-[8px]">
        <DeptTree
          className="w-[260px]"
          onChange={updateSelectDeptId}
          onReload={() => void handleDeptReload()}
          onSelect={handleDeptSelect}
          value={selectDeptId}
        />
        <BasicTable
          class="flex-1 overflow-hidden"
          slots={{
            action: ({ row }) =>
              Number((row as User).id) === SUPERADMIN_USER_ID ? null : (
                <>
                  <Space>
                    <Access code={['system:user:edit']}>
                      <Button
                        onClick={(e) => {
                          e.stopPropagation();
                          drawerRef.current?.openEdit((row as User).id);
                        }}
                        size="small"
                        type="link"
                      >
                        {$t('pages.common.edit')}
                      </Button>
                    </Access>
                    <Access code={['system:user:remove']}>
                      <Popconfirm
                        onConfirm={() => handleDelete(row as User)}
                        placement="left"
                        title="确认删除？"
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
                  <Dropdown
                    menu={{
                      items: menuItems,
                      onClick: (info) =>
                        handleMenuClick(String(info.key), row as User),
                    }}
                    placement="bottomRight"
                  >
                    <Button size="small" type="link">
                      {$t('pages.common.more')}
                    </Button>
                  </Dropdown>
                </>
              ),
            avatar: ({ row }) => (
              // 可能要判断空字符串情况，所以没有使用 ??
              <Avatar
                src={(row as User).avatar || preferences.app.defaultAvatar}
              />
            ),
            status: ({ row }) => (
              <ApiSwitch
                api={(checked) => handleChangeStatus(checked, row as User)}
                disabled={
                  Number((row as User).id) === SUPERADMIN_USER_ID ||
                  !hasAccessByCodes(['system:user:edit'])
                }
                onReload={() => void tableApi.query()}
                value={(row as User).status === EnableStatus.Enable}
              />
            ),
          }}
          tableTitle="用户列表"
          toolbarTools={() => (
            <Space>
              <Access code={['system:user:export']}>
                <Button
                  disabled={exportLoading}
                  loading={exportLoading}
                  onClick={handleExport}
                >
                  {$t('pages.common.export')}
                </Button>
              </Access>
              <Access code={['system:user:import']}>
                <Button onClick={() => importModalRef.current?.open()}>
                  {$t('pages.common.import')}
                </Button>
              </Access>
              <Access code={['system:user:remove']}>
                <Button
                  danger
                  disabled={!vxeCheckboxChecked(tableApi)}
                  onClick={handleMultiDelete}
                  type="primary"
                >
                  {$t('pages.common.delete')}
                </Button>
              </Access>
              <Access code={['system:user:add']}>
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
      </div>
      <UserImportModal
        onReload={() => void tableApi.query()}
        ref={importModalRef}
      />
      <UserDrawer onReload={() => void tableApi.query()} ref={drawerRef} />
      <UserInfoModal ref={infoModalRef} />
      <UserResetPwdModal
        onReload={() => void tableApi.query()}
        ref={resetPwdModalRef}
      />
    </Page>
  );
}
