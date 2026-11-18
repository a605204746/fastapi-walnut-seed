import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import type { Client } from '@/api/system/client/model';

import { $t } from '@vben/locales';
import { useVbenForm, type FormSchema } from '@vben-core/form-ui';
import { useVbenDrawer } from '@vben-core/popup-ui';
import { cloneDeep } from '@vben-core/shared/utils';

import { clientAdd, clientInfo, clientUpdate } from '@/api/system/client';
import { useDictStore } from '@/store/dict';
import { defaultFormValueGetter, useBeforeCloseDiff } from '@/utils/popup';

import { drawerSchema } from './data';

export interface ClientDrawerHandle {
  openAdd: () => void;
  openEdit: (id: number | string) => void;
}

export interface ClientDrawerProps {
  onReload: () => void;
}

/**
 * 客户端新增/编辑抽屉（对应 Vue 侧 views/system/client/client-drawer.vue）。
 *
 * 关键业务约束（与 Vue 侧逐条对齐）：
 * - 编辑态下 `clientId` 才显示，`clientKey` / `clientSecret` 禁用（不可改）
 * - `id === 1` 的内置客户端，`status` 字段禁用（不能被停用，否则直接登录不了）
 * - 新增态下 `status` 必须可用
 */
export const ClientDrawer = forwardRef<ClientDrawerHandle, ClientDrawerProps>(
  function ClientDrawer({ onReload }, ref) {
    // 订阅字典 store：授权类型 / 设备类型 / 状态选项异步到达后重渲染
    useDictStore((state) => state.map);
    const [isUpdate, setIsUpdate] = useState(false);
    const isUpdateRef = useRef(false);

    function setUpdateFlag(value: boolean) {
      isUpdateRef.current = value;
      setIsUpdate(value);
    }

    const [BasicForm, formApi] = useVbenForm({
      commonConfig: {
        componentProps: { className: 'w-full' },
        formItemClass: 'col-span-2',
      },
      layout: 'vertical',
      schema: drawerSchema() as FormSchema[],
      showDefaultActions: false,
      wrapperClass: 'grid-cols-2 gap-x-4',
    });

    function setupForm(update: boolean) {
      void formApi.updateSchema([
        {
          dependencies: {
            show: () => update,
            triggerFields: [''],
          },
          fieldName: 'clientId',
        },
        {
          componentProps: { disabled: update },
          fieldName: 'clientKey',
        },
        {
          componentProps: { disabled: update },
          fieldName: 'clientSecret',
        },
      ]);
    }

    /** 生成状态字段 schema 的函数（对应 Vue 侧 getStatusSchema） */
    function getStatusSchema(disabled: boolean) {
      return [{ componentProps: { disabled }, fieldName: 'status' }];
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
        setUpdateFlag(updating);
        // 初始化字段可见性与禁用态
        setupForm(updating);

        if (updating && id) {
          const record = await clientInfo(id);
          // 不能禁用 id 为 1 的记录
          await formApi.updateSchema(getStatusSchema(record.id === 1));
          await formApi.setValues(record as unknown as Record<string, any>);
        } else {
          // 新增模式：确保状态字段可用
          await formApi.updateSchema(getStatusSchema(false));
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
        await (isUpdateRef.current
          ? clientUpdate(data as Partial<Client>)
          : clientAdd(data as Partial<Client>));
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

export default ClientDrawer;
