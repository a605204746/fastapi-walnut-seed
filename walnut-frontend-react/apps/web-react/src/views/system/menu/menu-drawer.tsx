import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import type { Menu } from '@/api/system/menu/model';

import { $t } from '@vben/locales';
import { useVbenForm, type FormSchema } from '@vben-core/form-ui';
import { useVbenDrawer } from '@vben-core/popup-ui';
import { cloneDeep } from '@vben-core/shared/utils';
import { addFullName, getPopupContainer, listToTree } from '@vben/utils';
import { Skeleton } from 'antd';

import { pruneEmptyChildren } from '@/adapter/grid';
import { menuAdd, menuInfo, menuList, menuUpdate } from '@/api/system/menu';
import { useDictStore } from '@/store/dict';
import { defaultFormValueGetter, useBeforeCloseDiff } from '@/utils/popup';

import { drawerSchema } from './data';

export interface MenuDrawerHandle {
  openAdd: () => void;
  openEdit: (id: number | string) => void;
  openSubAdd: (parentId: number | string) => void;
}

export interface MenuDrawerProps {
  onReload: () => void;
}

/** 抽屉打开时携带的数据（对应 Vue 侧的 ModalProps 接口） */
interface DrawerPayload {
  id?: number | string;
  update: boolean;
}

/**
 * 菜单新增/编辑抽屉（对应 Vue 侧 views/system/menu/menu-drawer.vue，179 行）。
 *
 * 与 Vue 版的差异：
 * - Vue 的 `<template #remark>` 插槽（拓展字段下面那段说明文字）改由 schema 的
 *   `renderComponentContent.default` 承载，见 `data.tsx`。
 * - `loading` 期间 Vue 用 `v-show` 藏表单 + Skeleton 占位；React 侧同样二选一渲染。
 * - 组树后需要 `pruneEmptyChildren`：`listToTree` 会给叶子补 `children: []`，
 *   antd TreeSelect 会把它当成可展开节点。
 */
export const MenuDrawer = forwardRef<MenuDrawerHandle, MenuDrawerProps>(
  function MenuDrawer({ onReload }, ref) {
    // 订阅字典 store：菜单状态 / 显示状态选项异步到达后重渲染
    useDictStore((state) => state.map);
    const [isUpdate, setIsUpdate] = useState(false);
    const [loading, setLoading] = useState(false);
    const isUpdateRef = useRef(false);

    function setUpdateFlag(value: boolean) {
      isUpdateRef.current = value;
      setIsUpdate(value);
    }

    const [BasicForm, formApi] = useVbenForm({
      commonConfig: {
        componentProps: { className: 'w-full' },
        formItemClass: 'col-span-2',
        labelWidth: 90,
      },
      schema: drawerSchema() as FormSchema[],
      showDefaultActions: false,
      wrapperClass: 'grid-cols-2',
    });

    async function setupMenuSelect() {
      const menuArray = await menuList();
      // support i18n
      (menuArray ?? []).forEach((item) => {
        item.menuName = $t(item.menuName);
      });
      /**
       * 这里需要过滤掉按钮类型，不允许在按钮下添加数据
       */
      const filteredList = (menuArray ?? []).filter(
        (item) => item.menuType !== 'F',
      );
      const menuTree = listToTree<Menu>(filteredList as any[], {
        id: 'id',
        pid: 'parentId',
      });
      pruneEmptyChildren(menuTree);
      const fullMenuTree = [
        {
          children: menuTree,
          id: 0,
          menuName: $t('menu.root'),
        },
      ];
      addFullName(fullMenuTree, 'menuName', ' / ');

      await formApi.updateSchema([
        {
          componentProps: {
            fieldNames: {
              label: 'menuName',
              value: 'id',
            },
            getPopupContainer,
            // 设置弹窗滚动高度，默认 256
            listHeight: 300,
            showSearch: true,
            treeData: fullMenuTree,
            treeDefaultExpandAll: false,
            // 默认展开的树节点
            treeDefaultExpandedKeys: [0],
            treeLine: { showLeafIcon: false },
            // 筛选的字段
            treeNodeFilterProp: 'menuName',
            treeNodeLabelProp: 'fullName',
          },
          fieldName: 'parentId',
        },
      ]);
    }

    const { markInitialized, onBeforeClose, resetInitialized } =
      useBeforeCloseDiff({
        currentGetter: defaultFormValueGetter(formApi),
        initializedGetter: defaultFormValueGetter(formApi),
      });

    const [BasicDrawer, drawerApi] = useVbenDrawer({
      onBeforeClose,
      onClosed: handleClosed,
      onConfirm: handleConfirm,
      onOpenChange: async (isOpen) => {
        if (!isOpen) return;
        drawerApi.drawerLoading(true);
        setLoading(true);

        const { id, update } = drawerApi.getData<DrawerPayload>();
        setUpdateFlag(update);

        if (id) {
          await formApi.setFieldValue('parentId', id);
          // 并行获取菜单树选择和菜单信息
          const [record] = await Promise.all([
            update ? menuInfo(id) : null,
            setupMenuSelect(),
          ] as const);
          if (record) {
            await formApi.setValues(record as unknown as Record<string, any>);
          }
        } else {
          // 加载菜单树选择
          await setupMenuSelect();
        }
        await markInitialized();

        drawerApi.drawerLoading(false);
        setLoading(false);
      },
    });

    useImperativeHandle(
      ref,
      () => ({
        openAdd: () => {
          drawerApi.setData({ update: false } satisfies DrawerPayload);
          drawerApi.open();
        },
        openEdit: (id) => {
          drawerApi.setData({ id, update: true } satisfies DrawerPayload);
          drawerApi.open();
        },
        openSubAdd: (parentId) => {
          drawerApi.setData({
            id: parentId,
            update: false,
          } satisfies DrawerPayload);
          drawerApi.open();
        },
      }),
      [drawerApi],
    );

    async function handleConfirm() {
      try {
        drawerApi.lock(true);
        const { valid } = await formApi.validate();
        if (!valid) return;
        const data = cloneDeep(await formApi.getValues());
        await (isUpdateRef.current
          ? menuUpdate(data as Partial<Menu>)
          : menuAdd(data as Partial<Menu>));
        resetInitialized();
        onReload();
        await drawerApi.close();
      } catch (error) {
        console.error(error);
      } finally {
        drawerApi.lock(false);
      }
    }

    async function handleClosed() {
      await formApi.resetForm();
      resetInitialized();
    }

    return (
      <BasicDrawer
        class="w-[600px]"
        title={isUpdate ? $t('pages.common.edit') : $t('pages.common.add')}
      >
        {loading ? <Skeleton active /> : <BasicForm />}
      </BasicDrawer>
    );
  },
);

export default MenuDrawer;
