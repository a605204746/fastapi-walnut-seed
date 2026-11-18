import { forwardRef, useImperativeHandle, useState } from 'react';

import type { Post } from '@/api/system/post/model';

import { $t } from '@vben/locales';
import { useVbenForm, type FormSchema } from '@vben-core/form-ui';
import { useVbenDrawer } from '@vben-core/popup-ui';
import { cloneDeep } from '@vben-core/shared/utils';
import { addFullName } from '@vben/utils';

import { postAdd, postInfo, postUpdate } from '@/api/system/post';
import { getDeptTree } from '@/api/system/user';
import { useDictStore } from '@/store/dict';
import { defaultFormValueGetter, useBeforeCloseDiff } from '@/utils/popup';

import { drawerSchema } from './data';

export interface PostDrawerHandle {
  openAdd: () => void;
  openEdit: (id: number | string) => void;
}

export interface PostDrawerProps {
  onReload: () => void;
}

/**
 * 岗位新增/编辑抽屉（对应 Vue 侧 views/system/post/post-drawer.vue）。
 *
 * 这是 A2 阶段 8 个抽屉页的**样板**：Vue 的 `connectedComponent` 父子挂载改为
 * 「组件自持 drawer + form，父组件通过 ref 命令式打开」，其余逻辑逐行对齐，
 * 包括 `useBeforeCloseDiff` 的「有改动时关闭前二次确认」。
 */
export const PostDrawer = forwardRef<PostDrawerHandle, PostDrawerProps>(
  function PostDrawer({ onReload }, ref) {
    // 订阅字典 store：字典异步到达后重渲染，让 RadioGroup 选项显示出来
    useDictStore((state) => state.map);
    const [isUpdate, setIsUpdate] = useState(false);

    const [BasicForm, formApi] = useVbenForm({
      commonConfig: {
        componentProps: { className: 'w-full' },
        formItemClass: 'col-span-2',
        labelWidth: 80,
      },
      schema: drawerSchema() as FormSchema[],
      showDefaultActions: false,
      wrapperClass: 'grid-cols-2',
    });

    /** 拉部门树并注入 deptId 的 TreeSelect（对应 Vue 侧 setupDeptSelect） */
    async function setupDeptSelect() {
      const deptTree = await getDeptTree();
      // 选中后显示在输入框的值，即「父节点 / 子节点」
      addFullName(deptTree, 'label', ' / ');
      await formApi.updateSchema([
        {
          componentProps: {
            fieldNames: { label: 'label', value: 'id' },
            treeData: deptTree,
            treeDefaultExpandAll: true,
            treeLine: { showLeafIcon: false },
            // 选中后显示在输入框的值
            treeNodeLabelProp: 'fullName',
          },
          fieldName: 'deptId',
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
        const { id } = drawerApi.getData<{ id?: number | string }>();
        const updating = !!id;
        setIsUpdate(updating);
        // 初始化部门树下拉
        await setupDeptSelect();
        // 编辑态：拉详情并回填
        if (updating && id) {
          const record = await postInfo(id);
          await formApi.setValues(record as unknown as Record<string, any>);
        }
        await markInitialized();
        drawerApi.drawerLoading(false);
      },
    });

    useImperativeHandle(
      ref,
      () => ({
        openAdd: () => {
          drawerApi.setData({});
          drawerApi.open();
        },
        openEdit: (id) => {
          drawerApi.setData({ id });
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
        await (isUpdate
          ? postUpdate(data as Partial<Post>)
          : postAdd(data as Partial<Post>));
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

export default PostDrawer;
