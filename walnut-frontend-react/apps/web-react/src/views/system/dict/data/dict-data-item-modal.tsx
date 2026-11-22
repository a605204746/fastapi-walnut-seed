import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import type { DictData } from '@/api/system/dict/dict-data-model';

import { $t } from '@walnut/locales';
import { useWalnutForm, type FormSchema } from '@walnut-core/form-ui';
import { useWalnutModal } from '@walnut-core/popup-ui';
import { cloneDeep } from '@walnut-core/shared/utils';

import {
  dictDataAdd,
  dictDataUpdate,
  dictDetailInfo,
} from '@/api/system/dict/dict-data';
import { useDictStore } from '@/store/dict';
import { defaultFormValueGetter, useBeforeCloseDiff } from '@/utils/popup';

import { modalSchema } from './data';

export interface DictDataItemModalHandle {
  open: (payload: { dictType: string; id?: number | string }) => void;
}

export interface DictDataItemModalProps {
  onReload: () => void;
}

/**
 * 字典数据新增/编辑弹窗（对应 Vue 侧 dict/data/dict-data-item-modal.vue）。
 *
 * `dictType` 由父组件（字典数据面板）通过 setData 传入，提交时固定写回，
 * 用户不能改所属字典类型。
 */
export const DictDataItemModal = forwardRef<
  DictDataItemModalHandle,
  DictDataItemModalProps
>(function DictDataItemModal({ onReload }, ref) {
  // 订阅字典 store：sys_yes_no 选项异步到达后重渲染
  useDictStore((state) => state.map);
  const [isUpdate, setIsUpdate] = useState(false);
  /** 同步镜像，供 handleConfirm 读取（React state 更新是异步的） */
  const stateRef = useRef({ dictType: '', isUpdate: false });

  const [BasicForm, formApi] = useWalnutForm({
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

  const [BasicModal, modalApi] = useWalnutModal({
    fullscreenButton: false,
    onBeforeClose,
    onClosed: handleClosed,
    onConfirm: handleConfirm,
    onOpenChange: async (isOpen) => {
      if (!isOpen) return;
      modalApi.modalLoading(true);

      const { dictType, id } = modalApi.getData<{
        dictType: string;
        id?: number | string;
      }>();
      stateRef.current = { dictType, isUpdate: !!id };
      setIsUpdate(!!id);

      if (id) {
        const record = await dictDetailInfo(id);
        await formApi.setValues(record as unknown as Record<string, any>);
      }
      await markInitialized();

      modalApi.modalLoading(false);
    },
  });

  useImperativeHandle(
    ref,
    () => ({
      open: (payload) => {
        modalApi.setData(payload);
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
      // 字典类型固定为当前打开的字典类型
      data.dictType = stateRef.current.dictType;
      await (stateRef.current.isUpdate
        ? dictDataUpdate(data as Partial<DictData>)
        : dictDataAdd(data as Partial<DictData>));
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

export default DictDataItemModal;
