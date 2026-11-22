import type { FormProps, FormSchema } from '@walnut-core/form-ui';

import { setFormRuleMessages, useWalnutForm } from '@walnut-core/form-ui';

import { $t } from '@/locales';

/**
 * 表单适配层（对应 Vue 侧 `adapter/form.ts`，56 行）。
 *
 * Vue 基线做三件事：表单 setup 函数注册 `baseModelPropName` / `modelPropNameMap`、
 * 用 `defineRules` 注册两条国际化必填规则、再导出包了泛型的 `useWalnutForm`。
 *
 * React 侧的差异：
 * - **不需要 `modelPropNameMap`**：React 统一 `value` + `onChange` 受控，
 *   没有 Vue 的 `v-model:value` / `v-model:checked` / `v-model:fileList` 之分。
 * - `defineRules` → 注入式文案解析器（`@core/*` 不能依赖 `@walnut/locales`，
 *   与 popup-ui 的 `setDrawerTextResolver` 同一套路）。
 * - 不需要泛型包装：React 的 `FormSchema.component` 已是 `ComponentType | string`。
 */

export type WalnutFormSchema = FormSchema;
export type WalnutFormProps = FormProps;

/** 各页面 `data.ts` 里 schema 工厂函数的统一类型（对应 Vue 侧同名导出） */
export type FormSchemaGetter = () => FormSchema[];

/**
 * 注册必填校验的国际化文案。
 *
 * 词典：`ui.formRules.required` = `请输入{0}`、`ui.formRules.selectRequired` = `请选择{0}`。
 * 必须在 bootstrap 阶段调用一次；解析器在校验时执行，语言切换后自动生效。
 */
export function initSetupWalnutForm() {
  setFormRuleMessages((key, label) =>
    $t(
      key === 'required'
        ? 'ui.formRules.required'
        : 'ui.formRules.selectRequired',
      [label],
    ),
  );
}

export { useWalnutForm };
