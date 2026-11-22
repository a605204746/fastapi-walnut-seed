import { useWalnutForm, type FormSchema } from '@walnut-core/form-ui';

import { userUpdatePassword } from '@/api/system/profile';
import { getMessageApi } from '@/services/message';
import { useAuthStore } from '@/store/auth';

/** 密码长度区间（对应 Vue 侧 zod 的 min(5).max(20)） */
function lengthRule(value: any): boolean | string {
  const text = String(value ?? '');
  if (text.length < 5) return '密码长度不能少于5个字符';
  if (text.length > 20) return '密码长度不能超过20个字符';
  return true;
}

const formSchema: FormSchema[] = [
  {
    component: 'InputPassword',
    fieldName: 'oldPassword',
    label: '旧密码',
    rules: [
      { message: '请输入密码', required: true },
      { validator: lengthRule },
    ],
  },
  {
    component: 'InputPassword',
    dependencies: {
      rules: (values) => [
        { message: '请输入新密码', required: true },
        { validator: lengthRule },
        {
          validator: (value: any) =>
            value !== values.oldPassword || '新旧密码不能相同',
        },
      ],
      triggerFields: ['newPassword', 'oldPassword'],
    },
    fieldName: 'newPassword',
    label: '新密码',
  },
  {
    component: 'InputPassword',
    dependencies: {
      rules: (values) => [
        { message: '请输入确认密码', required: true },
        { validator: lengthRule },
        {
          validator: (value: any) =>
            value === values.newPassword || '新密码和确认密码不一致',
        },
      ],
      triggerFields: ['newPassword', 'confirmPassword'],
    },
    fieldName: 'confirmPassword',
    label: '确认密码',
  },
];

/**
 * 安全设置 = 修改密码（对应 Vue 侧 views/_core/profile/components/secure-setting.vue）。
 *
 * 行为逐条对齐：确认弹窗 → `userUpdatePassword`（接口加密）→ **强制登出**。
 * 提交时剔除 confirmPassword 字段。
 */
export function SecureSetting() {
  const logout = useAuthStore((state) => state.logout);

  const [BasicForm, formApi] = useWalnutForm({
    commonConfig: { labelWidth: 90 },
    schema: formSchema,
    showDefaultActions: false,
  });

  async function handleSubmit() {
    const { valid, values } = await formApi.validate();
    if (!valid) return;
    getMessageApi()?.modal.confirm({
      content: '确认修改密码吗？',
      onOk: async () => {
        const { confirmPassword: _confirm, ...data } = values;
        await userUpdatePassword(data as any);
        await logout();
      },
      title: '提示',
    });
  }

  return (
    <div className="mt-[16px] md:w-full lg:w-1/2 2xl:w-2/5">
      <BasicForm />
      <div className="flex justify-end">
        <button
          className="bg-primary text-primary-foreground rounded-md px-4 py-1.5 text-sm"
          onClick={() => void handleSubmit()}
          type="button"
        >
          修改密码
        </button>
      </div>
    </div>
  );
}

export default SecureSetting;
