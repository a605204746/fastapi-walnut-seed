import { useRef, useState } from 'react';

import type { Menu } from '@/api/system/menu/model';

import { $t } from '@walnut/locales';
import { listToTree, treeToList } from '@walnut/utils';
import { Button, Popconfirm, Space, Switch, Tooltip } from 'antd';

import { pruneEmptyChildren, useGrid } from '@/adapter/grid';
import { menuCascadeRemove, menuList, menuRemove } from '@/api/system/menu';
import { Access, useAccess } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { useDictStore } from '@/store/dict';
import { Fallback } from '@/views/_core/fallback/fallback';

import { columns, querySchema } from './data';
import { MenuDrawer, type MenuDrawerHandle } from './menu-drawer';

/**
 * 菜单管理页（对应 Vue 侧 views/system/menu/index.vue，262 行）。
 *
 * 与 Vue 版的差异：
 * - **树在页面侧组**：`treeConfig.transform: false`，query 里自己 `listToTree`，
 *   与 Vue 一致。额外多一步 `pruneEmptyChildren` —— `listToTree` 给叶子补的
 *   `children: []` 会让 antd 渲染出多余的展开箭头（vxe 靠 `hasChildField` 判断，
 *   antd 靠 children 真值判断，这是两个表格库的语义差）。
 * - Vue 的 `treeConfig.lazy + loadMethod: ({row}) => row.children ?? []` **不迁移**：
 *   loadMethod 直接返回已有的 children，本质是个空操作（数据本来就在全量树里）。
 * - `hasChildren` 字段同理不再计算，它只服务于 vxe 的懒加载展开箭头。
 * - `gridEvents.cellDblclick` / `toggleTreeExpand` 由适配层内部统一处理，页面无需订阅。
 */
export default function MenuPage() {
  useDictStore((state) => state.map);
  const drawerRef = useRef<MenuDrawerHandle>(null);
  const { hasAccessByRoles } = useAccess();

  /** 是否级联删除 */
  const [cascadingDeletion, setCascadingDeletion] = useState(false);
  const cascadingRef = useRef(false);

  function setCascading(value: boolean) {
    cascadingRef.current = value;
    setCascadingDeletion(value);
  }

  /**
   * 与后台逻辑相同：只有租户管理和超级管理能访问菜单管理。
   * 注意：只有超管才能对菜单进行「增删改」操作（Vue 侧原文重复了三遍）。
   */
  const isAdmin = hasAccessByRoles(['admin', 'superadmin']);

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
    id: 'system-menu-index',
    pagerConfig: { enabled: false },
    proxyConfig: {
      ajax: {
        query: async (
          _params: unknown,
          formValues: Record<string, any> = {},
        ) => {
          const resp = await menuList({ ...formValues });
          // 手动转为树结构
          const treeData = listToTree<Menu>((resp ?? []) as any[], {
            id: 'id',
            pid: 'parentId',
          });
          pruneEmptyChildren(treeData);
          return { rows: treeData };
        },
      },
    },
    rowClassName: 'hover:cursor-pointer',
    rowConfig: { keyField: 'id' },
    treeConfig: {
      parentField: 'parentId',
      // 刷新接口后记录展开行的情况
      reserve: true,
      rowField: 'id',
      // 页面侧已组好树，不需要适配层再转
      transform: false,
    },
  };

  const [BasicTable, tableApi] = useGrid({ formOptions, gridOptions });

  function handleAdd() {
    drawerRef.current?.openAdd();
  }

  function handleSubAdd(row: Menu) {
    drawerRef.current?.openSubAdd(row.id);
  }

  function handleEdit(record: Menu) {
    drawerRef.current?.openEdit(record.id);
  }

  async function handleDelete(row: Menu) {
    if (cascadingRef.current) {
      // 级联删除
      const menuAndChildren = treeToList<Menu[]>([row], { id: 'id' });
      await menuCascadeRemove(menuAndChildren.map((item) => item.id));
    } else {
      // 单删除
      await menuRemove([row.id]);
    }
    await tableApi.query();
  }

  function removeConfirmTitle(row: Menu) {
    const menuName = $t(row.menuName);
    if (!cascadingDeletion) {
      return `是否确认删除 [${menuName}] ?`;
    }
    const menuAndChildren = treeToList<Menu[]>([row], { id: 'id' });
    if (menuAndChildren.length === 1) {
      return `是否确认删除 [${menuName}] ?`;
    }
    return `是否确认删除 [${menuName}] 及 [${
      menuAndChildren.length - 1
    }]个子项目 ?`;
  }

  /** 全部展开/折叠 */
  function setExpandOrCollapse(expand: boolean) {
    tableApi.grid.setAllTreeExpand(expand);
  }

  if (!isAdmin) {
    return (
      <Fallback
        description="您没有菜单管理的访问权限"
        showBack={false}
        status="403"
      />
    );
  }

  return (
    <Page autoContentHeight>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Space>
              <Access code={['system:menu:edit']} role={['superadmin']}>
                <Button
                  onClick={(e) => {
                    e.stopPropagation();
                    handleEdit(row as Menu);
                  }}
                  size="small"
                  type="link"
                >
                  {$t('pages.common.edit')}
                </Button>
              </Access>
              {/* 「按钮类型」无法再添加子菜单 */}
              {(row as Menu).menuType === 'F' ? null : (
                <Access code={['system:menu:add']} role={['superadmin']}>
                  <Button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleSubAdd(row as Menu);
                    }}
                    size="small"
                    style={{ color: 'green' }}
                    type="link"
                  >
                    {$t('pages.common.add')}
                  </Button>
                </Access>
              )}
              <Access code={['system:menu:remove']} role={['superadmin']}>
                <Popconfirm
                  onConfirm={() => handleDelete(row as Menu)}
                  placement="left"
                  title={removeConfirmTitle(row as Menu)}
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
        tableTitle="菜单列表"
        tableTitleHelp="双击展开/收起子菜单"
        toolbarTools={() => (
          <Space>
            {/*
              Access 放在 Tooltip 外层：antd Tooltip 需要给子元素挂 ref，
              而 <Access> 渲染的是 Fragment，包在里面会拿不到 DOM 节点。
              可见性与 Vue 侧一致（无权限时两者都不渲染）。
            */}
            <Access code={['system:menu:remove']} role={['superadmin']}>
              <Tooltip title="删除菜单以及子菜单">
                <span className="mr-2 flex items-center">
                  <span className="mr-2 text-sm text-[#666666]">级联删除</span>
                  <Switch
                    checked={cascadingDeletion}
                    onChange={setCascading}
                  />
                </span>
              </Tooltip>
            </Access>

            <Button onClick={() => setExpandOrCollapse(false)}>
              {$t('pages.common.collapse')}
            </Button>

            <Access code={['system:menu:add']} role={['superadmin']}>
              <Button onClick={handleAdd} type="primary">
                {$t('pages.common.add')}
              </Button>
            </Access>
          </Space>
        )}
      />
      <MenuDrawer onReload={() => void tableApi.query()} ref={drawerRef} />
    </Page>
  );
}
