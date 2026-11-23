import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import type { Dept } from '@/api/system/dept/model';

import { $t } from '@/core/locales';
import { useWalnutForm, type FormSchema } from '@/core/form-ui';
import { useWalnutDrawer } from '@/core/popup-ui';
import { cloneDeep } from '@/core/shared/utils';
import { addFullName, listToTree } from '@/core/utils';

import {
  deptAdd,
  deptInfo,
  deptList,
  deptNodeList,
  deptUpdate,
} from '@/api/system/dept';
import { listUserByDeptId } from '@/api/system/user';
import { useDictStore } from '@/store/dict';
import { defaultFormValueGetter, useBeforeCloseDiff } from '@/utils/popup';

import { drawerSchema } from './data';

export interface DeptDrawerHandle {
  openAdd: () => void;
  openEdit: (id: number | string) => void;
  /** 在指定部门下新增子部门 */
  openSubAdd: (parentId: number | string) => void;
}

export interface DeptDrawerProps {
  onReload: () => void;
}

/** 抽屉打开时携带的数据（对应 Vue 侧的 DrawerProps 接口） */
interface DrawerPayload {
  id?: number | string;
  update: boolean;
}

/**
 * `listToTree` 会给每个节点补 `children: []`，antd TreeSelect 会把空数组
 * 当成「有子节点」而渲染展开箭头，这里统一剪掉。
 */
function pruneEmptyChildren(nodes: any[]) {
  for (const node of nodes) {
    const children = node.children;
    if (Array.isArray(children) && children.length > 0) {
      pruneEmptyChildren(children);
    } else {
      delete node.children;
    }
  }
}

/**
 * 部门新增/编辑抽屉（对应 Vue 侧 views/system/dept/dept-drawer.vue）。
 *
 * ⚠️ `isUpdate` 来自 payload 的 `update` 字段而**不是**「有没有 id」：
 * 「新增子部门」同时带 id（父部门）和 `update: false`，两者语义不同。
 */
export const DeptDrawer = forwardRef<DeptDrawerHandle, DeptDrawerProps>(
  function DeptDrawer({ onReload }, ref) {
    // 订阅字典 store：字典异步到达后重渲染，让 RadioGroup 选项显示出来
    useDictStore((state) => state.map);
    const [isUpdate, setIsUpdate] = useState(false);
    /**
     * `isUpdate` 的同步镜像。`onOpenChange` 里 `setIsUpdate(update)` 之后紧接着就要
     * 调 `initDeptSelect()` 读这个值，而 React 的 state 更新是异步的、闭包里读到的
     * 还是旧值；Vue 侧 `isUpdate.value = update` 是同步生效的，所以那边不需要这层。
     */
    const isUpdateRef = useRef(false);

    function setUpdateFlag(value: boolean) {
      isUpdateRef.current = value;
      setIsUpdate(value);
    }

    const [BasicForm, formApi] = useWalnutForm({
      commonConfig: {
        componentProps: { className: 'w-full' },
        formItemClass: 'col-span-2',
        labelWidth: 80,
      },
      schema: drawerSchema() as FormSchema[],
      showDefaultActions: false,
      wrapperClass: 'grid-cols-2',
    });

    /** 拉部门树；exclude=true 时取全量（新增），否则取 deptId 的子树（编辑） */
    async function loadDeptTree(
      deptId?: number | string,
      exclude = false,
    ): Promise<Dept[]> {
      const ret = await (!deptId || exclude ? deptList({}) : deptNodeList(deptId));
      // 注意两个 helper 的泛型口径不一致（沿袭 Vue 侧）：
      // `listToTree<T>` 返回 `T[]`（T 是**节点**类型），`treeToList<T>` 返回 `T`（T 是**数组**类型）
      const treeData = listToTree<Dept>((ret ?? []) as any[], {
        id: 'id',
        pid: 'parentId',
      });
      pruneEmptyChildren(treeData);
      // 添加部门全名，如 xx / xx / xx
      addFullName(treeData, 'deptName', ' / ');
      return treeData;
    }

    async function initDeptSelect(deptId?: number | string) {
      // 需要动态更新 TreeSelect 组件，这里允许为空
      const treeData = await loadDeptTree(deptId, !isUpdateRef.current);
      await formApi.updateSchema([
        {
          componentProps: {
            fieldNames: { label: 'deptName', value: 'id' },
            showSearch: true,
            treeData,
            treeDefaultExpandAll: true,
            treeLine: { showLeafIcon: false },
            // 筛选的字段
            treeNodeFilterProp: 'deptName',
            // 选中后显示在输入框的值
            treeNodeLabelProp: 'fullName',
          },
          fieldName: 'parentId',
        },
      ]);
    }

    /** 部门负责人下拉，仅更新时可选 */
    async function initDeptUsers(deptId: number | string) {
      const ret = await listUserByDeptId(deptId);
      const users = ret ?? [];
      const options = users.map((user) => ({
        label: `${user.userName} | ${user.nickName}`,
        value: user.id,
      }));
      await formApi.updateSchema([
        {
          componentProps: {
            disabled: users.length === 0,
            options,
            placeholder:
              users.length === 0 ? '该部门暂无用户' : '请选择部门负责人',
          },
          fieldName: 'leader',
        },
      ]);
    }

    async function setLeaderOptions() {
      await formApi.updateSchema([
        {
          componentProps: {
            disabled: true,
            options: [],
            placeholder: '仅在更新时可选部门负责人',
          },
          fieldName: 'leader',
        },
      ]);
    }

    const { markInitialized, onBeforeClose, resetInitialized } =
      useBeforeCloseDiff({
        currentGetter: defaultFormValueGetter(formApi),
        initializedGetter: defaultFormValueGetter(formApi),
      });

    const [BasicDrawer, drawerApi] = useWalnutDrawer({
      onBeforeClose,
      onClosed: handleClosed,
      onConfirm: handleConfirm,
      onOpenChange: async (isOpen) => {
        if (!isOpen) return;
        drawerApi.drawerLoading(true);

        const { id, update } = drawerApi.getData<DrawerPayload>();
        setUpdateFlag(update);

        if (id) {
          await formApi.setFieldValue('parentId', id);
          if (update) {
            const record = await deptInfo(id);
            await formApi.setValues(record as unknown as Record<string, any>);
          }
        }

        await (update && id ? initDeptUsers(id) : setLeaderOptions());
        // 部门选择下拉框
        await initDeptSelect(id);
        await markInitialized();

        drawerApi.drawerLoading(false);
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
          ? deptUpdate(data as Partial<Dept>)
          : deptAdd(data as Partial<Dept>));
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
        <BasicForm />
      </BasicDrawer>
    );
  },
);

export default DeptDrawer;
