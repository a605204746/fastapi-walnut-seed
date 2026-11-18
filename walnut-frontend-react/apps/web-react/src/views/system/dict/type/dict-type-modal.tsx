import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import type { DictType } from '@/api/system/dict/dict-type-model';

import { $t } from '@vben/locales';
import { useVbenForm, type FormSchema } from '@vben-core/form-ui';
import { useVbenModal } from '@vben-core/popup-ui';
import { cloneDeep } from '@vben-core/shared/utils';

import {
  dictTypeAdd,
  dictTypeInfo,
  dictTypeUpdate,
} from '@/api/system/dict/dict-type';
import { defaultFormValueGetter, useBeforeCloseDiff } from '@/utils/popup';

import { modalSchema } from './data';

export interface DictTypeModalHandle {
  openAdd: () => void;
  openEdit: (id: number | string) => void;
}

export interface DictTypeModalProps {
  onReload: () => void;
}

/**
 * 字典类型新增/编辑弹窗（对应 Vue 侧 dict/type/dict-type-modal.vue）。
 * `layout: 'vertical'` + `labelWidth: 100`，与 Vue 侧一致。
 */
export const DictTypeModal = forwardRef<
  DictTypeModalHandle,
  DictTypeModalProps
>(function DictTypeModal({ onReload }, ref) {
  const [isUpdate, setIsUpdate] = useState(false);
  const isUpdateRef = useRef(false);

  function setUpdateFlag(value: boolean) {
    isUpdateRef.current = value;
    setIsUpdate(value);
  }

  const [BasicForm, formApi] = useVbenForm({
    commonConfig: { labelWidth: 100 },
    layout: 'vertical',
    schema: modalSchema() as FormSchema[],
    showDefaultActions: false,
  });

  const { markInitialized, onBeforeClose, resetInitialized } =
    useBeforeCloseDiff({
      currentGetter: defaultFormValueGetter(formApi),
      initializedGetter: defaultFormValueGetter(formApi),
    });

  const [BasicModal, modalApi] = useVbenModal({
    fullscreenButton: false,
    onBeforeClose,
    onClosed: handleClosed,
    onConfirm: handleConfirm,
    onOpenChange: async (isOpen) => {
      if (!isOpen) return;
      modalApi.modalLoading(true);

      const { id } = modalApi.getData<{ id?: number | string }>();
      setUpdateFlag(!!id);
      if (id) {
        const record = await dictTypeInfo(id);
        await formApi.setValues(record as unknown as Record<string, any>);
      }
      await markInitialized();

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
      await (isUpdateRef.current
        ? dictTypeUpdate(data as Partial<DictType>)
        : dictTypeAdd(data as Partial<DictType>));
      resetInitialized();
      onReload();
      await modalApi.close();
    } catch (error) {
      console.error(error);
    } finally {
      modalApi.lock(false);
    }
  }

  async function handleClosed() {
    await formApi.resetForm();
    resetInitialized();
  }

  return (
    <BasicModal
      title={isUpdate ? $t('pages.common.edit') : $t('pages.common.add')}
    >
      <BasicForm />
    </BasicModal>
  );
});

export default DictTypeModal;
