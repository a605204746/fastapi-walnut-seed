import { forwardRef, useImperativeHandle, useState } from 'react';

import type { DescriptionsProps } from 'antd';

import type { ResetPwdParam, User } from '@/api/system/user/model';

import { useWalnutForm, type FormSchema } from '@/core/form-ui';
import { useWalnutModal } from '@/core/popup-ui';
import { Descriptions } from 'antd';

import { userResetPassword } from '@/api/system/user';

export interface UserResetPwdModalHandle {
  open: (record: User) => void;
}

export interface UserResetPwdModalProps {
  onReload: () => void;
}

/** 密码长度校验（对应 Vue 侧 `z.string().min(5).max(20)`） */
const PASSWORD_MESSAGE = '密码长度为5 - 20';

function validatePassword(value: any): boolean | string {
  if (value === undefined || value === null || value === '') {
    return PASSWORD_MESSAGE;
  }
  const text = String(value);
  if (text.length < 5 || text.length > 20) {
    return PASSWORD_MESSAGE;
  }
  return true;
}

/**
 * 重置密码弹窗（对应 Vue 侧 views/system/user/user-reset-pwd-modal.vue，115 行）。
 * 上半部分只读展示用户信息，下半部分是密码表单。
 * `closeOnClickModal: false`：防止误点遮罩丢失输入。
 */
export const UserResetPwdModal = forwardRef<
  UserResetPwdModalHandle,
  UserResetPwdModalProps
>(function UserResetPwdModal({ onReload }, ref) {
  const [currentUser, setCurrentUser] = useState<null | User>(null);

  const [BasicForm, formApi] = useWalnutForm({
    commonConfig: { labelWidth: 80 },
    schema: [
      {
        component: 'Input',
        dependencies: {
          show: () => false,
          triggerFields: [''],
        },
        fieldName: 'id',
        label: '用户ID',
        rules: 'required',
      },
      {
        component: 'InputPassword',
        componentProps: {
          placeholder: '请输入新的密码, 密码长度为5 - 20',
        },
        fieldName: 'password',
        label: '新的密码',
        rules: { validator: validatePassword },
      },
    ] as FormSchema[],
    showDefaultActions: false,
  });

  const [BasicModal, modalApi] = useWalnutModal({
    closeOnClickModal: false,
    fullscreenButton: false,
    onClosed: handleClosed,
    onConfirm: handleSubmit,
    onOpenChange: async (isOpen) => {
      if (!isOpen) return;
      modalApi.modalLoading(true);

      const { record } = modalApi.getData<{ record: User }>();
      setCurrentUser(record);
      await formApi.setValues({ id: record.id });

      modalApi.modalLoading(false);
    },
  });

  useImperativeHandle(
    ref,
    () => ({
      open: (record) => {
        modalApi.setData({ record });
        modalApi.open();
      },
    }),
    [modalApi],
  );

  async function handleSubmit() {
    try {
      modalApi.modalLoading(true);
      const { valid } = await formApi.validate();
      if (!valid) return;
      const data = await formApi.getValues();
      await userResetPassword(data as ResetPwdParam);
      onReload();
      await handleClosed();
    } catch (error) {
      console.error(error);
    } finally {
      modalApi.modalLoading(false);
    }
  }

  async function handleClosed() {
    await modalApi.close();
    await formApi.resetForm();
    setCurrentUser(null);
  }

  const items: DescriptionsProps['items'] = currentUser
    ? [
        { children: currentUser.id, label: '用户ID' },
        { children: currentUser.userName, label: '用户名' },
        { children: currentUser.nickName, label: '昵称' },
      ]
    : [];

  return (
    <BasicModal title="重置密码">
      <div className="flex flex-col gap-[12px]">
        <Descriptions bordered column={1} items={items} size="small" />
        <BasicForm />
      </div>
    </BasicModal>
  );
});

export default UserResetPwdModal;
