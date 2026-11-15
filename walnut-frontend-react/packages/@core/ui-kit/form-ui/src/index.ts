export * from './config';
export * from './form';
export * from './form-api';
export * from './types';
export * from './use-vben-form';

// 兼容 Vue 侧命名（views/data.ts 引用 VbenFormSchema / VbenFormProps）
export type { FormProps as VbenFormProps, FormSchema as VbenFormSchema } from './types';
