import { forwardRef, useImperativeHandle, useState } from 'react';

import type { SysConfig } from '@/api/system/config/model';

import { useVbenForm, type FormSchema } from '@vben-core/form-ui';
import { useVbenModal } from '@vben-core/popup-ui';
import { cloneDeep } from '@vben-core/shared/utils';

import { configAdd, configInfo, configUpdate } from '@/api/system/config';
import { useDictStore } from '@/store/dict';

import { modalSchema } from './data';

export interface ConfigModalHandle {
  openAdd: () => void;
  openEdit: (id: number | string) => void;
}

export interface ConfigModalProps {
  onReload: () => void;
}

/**
 * 参数配置新增/编辑弹窗（对应 Vue 侧 views/system/config/config-modal.vue）。
 *
 * Vue 的 connectedComponent 父子挂载在 React 下改为「组件自持 modal+form，
 * 父组件通过 ref 命令式打开」，弹窗宽度 w-[550px] 与 labelWidth 80 保持一致。
 */
export const ConfigModal = forwardRef<ConfigModalHandle, ConfigModalProps>(
  function ConfigModal({ onReload }, ref) {
    // 订阅字典 store：字典异步到达后重渲染，让 RadioGroup 选项显示出来
    useDictStore((state) => state.map);
    const [isUpdate, setIsUpdate] = useState(false);

    const [BasicForm, formApi] = useVbenForm({
      commonConfig: { labelWidth: 80 },
      schema: modalSchema() as FormSchema[],
      showDefaultActions: false,
    });

    const [BasicModal, modalApi] = useVbenModal({
      onClosed: async () => {
        await formApi.resetForm();
      },
      onConfirm: handleConfirm,
      onOpenChange: async (isOpen) => {
        if (!isOpen) return;
        modalApi.modalLoading(true);
        const { id } = modalApi.getData<{ id?: number | string }>();
        const updating = !!id;
        setIsUpdate(updating);
        if (updating && id) {
          const record = await configInfo(id);
          await formApi.setValues(record as unknown as Record<string, any>);
        }
        modalApi.modalLoading(false);
      },
    });

    useImperativeHandle(
      ref,
      () => ({
        openAdd: () => {
          modalApi.setData({});
          modalApi.open();
        },
        openEdit: (id) => {
          modalApi.setData({ id });
          modalApi.open();
        },
      }),
      [modalApi],
    );

    async function handleConfirm() {
      try {
        modalApi.lock(true);
        const { valid } = await formApi.validate();
        if (!valid) return;
        const data = cloneDeep(await formApi.getValues());
        await (isUpdate
          ? configUpdate(data as Partial<SysConfig>)
          : configAdd(data as Partial<SysConfig>));
        onReload();
        await modalApi.close();
      } catch (error) {
        console.error(error);
      } finally {
        modalApi.lock(false);
      }
    }

    return (
      <BasicModal class="w-[550px]" title={isUpdate ? '编辑' : '新增'}>
        <BasicForm />
      </BasicModal>
    );
  },
);

export default ConfigModal;
