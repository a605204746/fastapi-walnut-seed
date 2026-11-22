import { useEffect, useState } from 'react';

import type { UserProfile } from '@/api/system/profile/model';

import { DictEnum } from '@walnut/constants';
import { useUserStore } from '@walnut/stores';
import { useWalnutForm, type FormSchema } from '@walnut-core/form-ui';

import { userProfileUpdate } from '@/api/system/profile';
import { useAuthStore } from '@/store/auth';
import { useDictStore } from '@/store/dict';
import { getDictOptions } from '@/utils/dict';

export interface BaseSettingProps {
  profile: UserProfile;
  /** 更新成功后重载左侧资料卡（对应 Vue 侧 emitter.emit('updateProfile')） */
  onProfileUpdate?: () => Promise<void> | void;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const PHONE_RE = /^1[3-9]\d{9}$/;

function buildSchema(): FormSchema[] {
  return [
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
      component: 'Input',
      fieldName: 'nickName',
      label: '昵称',
      rules: 'required',
    },
    {
      component: 'Input',
      fieldName: 'email',
      label: '邮箱',
      // Vue 侧 z.string().email()：空串同样不通过
      rules: [
        {
          message: '请输入正确的邮箱',
          validator: (value) => EMAIL_RE.test(String(value ?? '')),
        },
      ],
    },
    {
      component: 'RadioGroup',
      componentProps: {
        buttonStyle: 'solid',
        options: getDictOptions(DictEnum.SYS_USER_SEX),
        optionType: 'button',
      },
      defaultValue: '0',
      fieldName: 'sex',
      label: '性别',
      rules: 'required',
    },
    {
      component: 'Input',
      fieldName: 'phonenumber',
      label: '电话',
      rules: [
        {
          message: '请输入正确的电话',
          validator: (value) => PHONE_RE.test(String(value ?? '')),
        },
      ],
    },
  ];
}

/**
 * 基本设置（对应 Vue 侧 views/_core/profile/components/base-setting.vue）。
 *
 * ⚠️ 早前误搬了 common-ui 的演示版 ProfileBaseSetting（mock 角色 + 假成功），
 * 实际业务版是 id/昵称/邮箱/性别/电话 五字段，提交走 PUT /system/user/profile，
 * 成功后刷新全局 userInfo 并通知父组件重载左侧资料卡。
 */
export function BaseSetting({ profile, onProfileUpdate }: BaseSettingProps) {
  // 订阅字典 store：性别选项异步到达后重渲染（schema 每次渲染重建）
  useDictStore((state) => state.map);
  const [submitting, setSubmitting] = useState(false);
  const fetchUserInfo = useAuthStore((state) => state.fetchUserInfo);
  const setUserInfo = useUserStore((state) => state.setUserInfo);

  const [BasicForm, formApi] = useWalnutForm({
    commonConfig: { labelWidth: 60 },
    schema: buildSchema(),
    showDefaultActions: false,
  });

  useEffect(() => {
    const user = profile?.user;
    if (!user) return;
    void formApi.setValues({
      email: user.email,
      id: user.id,
      nickName: user.nickName,
      phonenumber: user.phonenumber,
      sex: user.sex,
    });
  }, [formApi, profile]);

  async function handleSubmit() {
    const { valid } = await formApi.validate();
    if (!valid) return;
    try {
      setSubmitting(true);
      const values = await formApi.getValues();
      await userProfileUpdate(values);
      // 更新store
      const userInfo = await fetchUserInfo();
      setUserInfo(userInfo);
      // 左边reload
      await onProfileUpdate?.();
    } catch (error) {
      console.error(error);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mt-[16px] md:w-full lg:w-1/2 2xl:w-2/5">
      <BasicForm />
      <div className="flex justify-end">
        <button
          className="bg-primary text-primary-foreground rounded-md px-4 py-1.5 text-sm disabled:opacity-60"
          disabled={submitting}
          onClick={() => void handleSubmit()}
          type="button"
        >
          更新信息
        </button>
      </div>
    </div>
  );
}

export default BaseSetting;