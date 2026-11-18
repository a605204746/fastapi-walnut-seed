import { useEffect } from 'react';

import { useVbenForm, type FormSchema } from '@vben-core/form-ui';

import { getUserInfoApi } from '@/api/core/user';
import { getMessageApi } from '@/services/message';

const MOCK_ROLES_OPTIONS = [
  { label: '管理员', value: 'super' },
  { label: '用户', value: 'user' },
  { label: '测试', value: 'test' },
];

const formSchema: FormSchema[] = [
  { component: 'Input', fieldName: 'realName', label: '姓名' },
  { component: 'Input', fieldName: 'username', label: '用户名' },
  {
    component: 'Select',
    componentProps: { mode: 'tags', options: MOCK_ROLES_OPTIONS },
    fieldName: 'roles',
    label: '角色',
  },
  { component: 'Textarea', fieldName: 'introduction', label: '个人简介' },
];

/**
 * 基本设置（对应 Vue 侧 views/_core/profile/components/base-setting.vue +
 * common-ui 的 ProfileBaseSetting）。
 *
 * ⚠️ 与 Vue 侧完全一致地保留「演示性质」：角色选项是写死的 MOCK_ROLES_OPTIONS，
 * 提交只弹成功提示、不落库（Vue 的 ProfileBaseSetting 同样只 message.success）。
 * 不是迁移遗漏，是上游本来的行为。
 */
export function BaseSetting() {
  const [BasicForm, formApi] = useVbenForm({
    commonConfig: { labelWidth: 80 },
    schema: formSchema,
    showDefaultActions: false,
  });

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const data = await getUserInfoApi();
      if (!cancelled) {
        await formApi.setValues(data as unknown as Record<string, any>);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [formApi]);

  async function handleSubmit() {
    const { valid } = await formApi.validate();
    if (!valid) return;
    getMessageApi()?.message.success('更新成功');
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
          提交
        </button>
      </div>
    </div>
  );
}

export default BaseSetting;
