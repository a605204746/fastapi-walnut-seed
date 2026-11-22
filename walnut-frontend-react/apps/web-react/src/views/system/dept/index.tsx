import { useRef } from 'react';

import type { Dept } from '@/api/system/dept/model';

import { $t } from '@walnut/locales';
import { Button, Popconfirm, Space } from 'antd';

import { useGrid } from '@/adapter/grid';
import { deptList, deptRemove } from '@/api/system/dept';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { useDictStore } from '@/store/dict';

import { columns, querySchema } from './data';
import { DeptDrawer, type DeptDrawerHandle } from './dept-drawer';

/**
 * 部门管理页（对应 Vue 侧 views/system/dept/index.vue）。
 *
 * 树形表格：后端返回平铺列表，`treeConfig.transform` 让适配层组树；
 * 不分页（`pagerConfig: { enabled: false }`）；查询完成后默认展开全部。
 *
 * 与 Vue 版的差异：Vue 靠 `eachTree(getData(), item => item.expand = true)` 在每行上
 * 打标记 + `gridEvents.toggleTreeExpand` 同步箭头点击；React 侧展开态集中在
 * 适配层 store 的 `expandedKeys` 里，箭头点击和双击行都由适配层自己维护，
 * 页面只需要 `querySuccess` 里调一次 `setAllTreeExpand(true)`。
 */
export default function DeptPage() {
  useDictStore((state) => state.map);
  const drawerRef = useRef<DeptDrawerHandle>(null);

  const formOptions = {
    commonConfig: {
      componentProps: { allowClear: true },
      labelWidth: 80,
    },
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
  };

  const gridOptions = {
    columns,
    height: 'auto',
    id: 'system-dept-index',
    // 部门是树形结构，不分页
    pagerConfig: { enabled: false },
    proxyConfig: {
      ajax: {
        query: async (
          _params: unknown,
          formValues: Record<string, any> = {},
        ) => {
          const resp = await deptList({ ...formValues });
          return { rows: resp ?? [] };
        },
        // 默认展开全部（对应 Vue 侧 querySuccess 里的 eachTree + setExpandOrCollapse）
        querySuccess: () => {
          tableApi.grid.setAllTreeExpand(true);
        },
      },
    },
    rowConfig: { keyField: 'id' },
    treeConfig: {
      parentField: 'parentId',
      rowField: 'id',
      transform: true,
    },
  };

  const [BasicTable, tableApi] = useGrid({ formOptions, gridOptions });

  function handleAdd() {
    drawerRef.current?.openAdd();
  }

  function handleSubAdd(row: Dept) {
    drawerRef.current?.openSubAdd(row.id);
  }

  function handleEdit(record: Dept) {
    drawerRef.current?.openEdit(record.id);
  }

  async function handleDelete(row: Dept) {
    await deptRemove(row.id);
    await tableApi.query();
  }

  /** 全部展开/折叠 */
  function setExpandOrCollapse(expand: boolean) {
    tableApi.grid.setAllTreeExpand(expand);
  }

  return (
    <Page autoContentHeight>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Space>
              <Access code={['system:dept:edit']}>
                <Button
                  onClick={(e) => {
                    e.stopPropagation();
                    handleEdit(row as Dept);
                  }}
                  size="small"
                  type="link"
                >
                  {$t('pages.common.edit')}
                </Button>
              </Access>
              <Access code={['system:dept:add']}>
                <Button
                  onClick={(e) => {
                    e.stopPropagation();
                    handleSubAdd(row as Dept);
                  }}
                  size="small"
                  style={{ color: 'green' }}
                  type="link"
                >
                  {$t('pages.common.add')}
                </Button>
              </Access>
              <Access code={['system:dept:remove']}>
                <Popconfirm
                  onConfirm={() => handleDelete(row as Dept)}
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
          ),
        }}
        tableTitle="部门列表"
        tableTitleHelp="双击展开/收起子菜单"
        toolbarTools={() => (
          <Space>
            <Button onClick={() => setExpandOrCollapse(false)}>
              {$t('pages.common.collapse')}
            </Button>
            <Button onClick={() => setExpandOrCollapse(true)}>
              {$t('pages.common.expand')}
            </Button>
            <Access code={['system:dept:add']}>
              <Button onClick={handleAdd} type="primary">
                {$t('pages.common.add')}
              </Button>
            </Access>
          </Space>
        )}
      />
      <DeptDrawer onReload={() => void tableApi.query()} ref={drawerRef} />
    </Page>
  );
}
