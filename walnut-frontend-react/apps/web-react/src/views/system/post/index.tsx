import { useRef, useState } from 'react';

import type { Post } from '@/api/system/post/model';

import { $t } from '@vben/locales';
import { Button, Popconfirm, Space } from 'antd';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  postDeptTreeSelect,
  postExport,
  postList,
  postRemove,
} from '@/api/system/post';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';
import { DeptTree } from '@/views/system/user/dept-tree';

import { columns, querySchema } from './data';
import { PostDrawer, type PostDrawerHandle } from './post-drawer';

/**
 * 岗位管理页（对应 Vue 侧 views/system/post/index.vue，451 行含 data.ts）。
 *
 * 布局：`Page(contentClass="flex gap-[8px] w-full")` > 左侧 DeptTree(260px) +
 * 右侧 BasicTable(flex-1)，与 Vue 侧完全一致。
 */
export default function PostPage() {
  // 订阅字典 store：字典异步到达后重渲染（查询下拉 + 表格 DictTag）
  useDictStore((state) => state.map);
  const drawerRef = useRef<PostDrawerHandle>(null);

  const [selectDeptId, setSelectDeptId] = useState<string[]>([]);
  /**
   * 查询闭包读的是 ref 而不是 state：`handleDeptSelect` 里「设置选中 → 立刻 reload」
   * 是同步连续动作，React 的 state 更新是异步的，只读 state 会拿到旧值。
   * Vue 侧 `selectDeptId.value = keys` 是同步生效的，所以那边不需要这一层。
   */
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
    // 重置时一并清空左侧部门树的选中（对应 Vue 侧 formOptions.handleReset）
    handleReset: async () => {
      updateSelectDeptId([]);
      await tableApi.formApi?.resetForm();
      await tableApi.reload();
    },
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
  };

  const gridOptions = {
    checkboxConfig: { highlight: true, reserve: true },
    columns,
    height: 'auto',
    id: 'system-post-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          // 部门树选择处理：单选一个部门时才带 belongDeptId
          const deptIds = selectDeptIdRef.current;
          if (deptIds.length === 1) {
            formValues.belongDeptId = deptIds[0];
          } else {
            delete formValues.belongDeptId;
          }
          return await postList({
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
    useBlobExport(postExport);

  async function handleExport() {
    const formValues = await tableApi.formApi?.getValues();
    const fileName = buildExportFileName('岗位数据');
    await exportBlob({ data: formValues, fileName });
  }

  async function handleDelete(row: Post) {
    await postRemove([row.id]);
    await tableApi.query();
  }

  function handleMultiDelete() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: Post) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认删除选中的${ids.length}条记录吗？`,
      okType: 'danger',
      onOk: async () => {
        await postRemove(ids);
        await tableApi.query();
      },
      title: $t('pages.common.tip'),
    });
  }

  function handleDeptSelect(keys: string[]) {
    updateSelectDeptId(keys);
    void tableApi.reload();
  }

  return (
    <Page autoContentHeight contentClass="flex w-full gap-[8px]">
      <DeptTree
        api={postDeptTreeSelect}
        className="w-[260px]"
        onChange={updateSelectDeptId}
        onReload={() => void tableApi.reload()}
        onSelect={handleDeptSelect}
        value={selectDeptId}
      />
      <BasicTable
        class="flex-1 overflow-hidden"
        slots={{
          action: ({ row }) => (
            <Space>
              <Access code={['system:post:edit']}>
                <Button
                  onClick={(e) => {
                    e.stopPropagation();
                    drawerRef.current?.openEdit(row.id);
                  }}
                  size="small"
                  type="link"
                >
                  {$t('pages.common.edit')}
                </Button>
              </Access>
              <Popconfirm
                onConfirm={() => handleDelete(row)}
                placement="left"
                title="确认删除？"
              >
                <Access code={['system:post:remove']}>
                  <Button
                    danger
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
        }}
        tableTitle="岗位列表"
        toolbarTools={() => (
          <Space>
            <Access code={['system:post:export']}>
              <Button
                disabled={exportLoading}
                loading={exportLoading}
                onClick={handleExport}
              >
                {$t('pages.common.export')}
              </Button>
            </Access>
            <Access code={['system:post:remove']}>
              <Button
                danger
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={handleMultiDelete}
                type="primary"
              >
                {$t('pages.common.delete')}
              </Button>
            </Access>
            <Access code={['system:post:add']}>
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
      <PostDrawer onReload={() => void tableApi.query()} ref={drawerRef} />
    </Page>
  );
}
