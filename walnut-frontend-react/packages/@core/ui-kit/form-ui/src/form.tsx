import type { ReactNode } from 'react';

import type { FormApi } from './form-api';
import type { FormSchema } from './types';

import { useStore } from '@vben-core/shared/store';
import { cn } from '@vben-core/shared/utils';

import { getComponent } from './config';

const LABEL_BASE =
  'text-sm leading-none font-medium peer-disabled:cursor-not-allowed peer-disabled:opacity-70';

function isEventLike(value: any): boolean {
  return (
    value !== null &&
    typeof value === 'object' &&
    'target' in value &&
    value.target !== null &&
    typeof value.target === 'object'
  );
}

function unwrapEvent(value: any): any {
  if (!isEventLike(value)) return value;
  const target = value.target;
  return target.type === 'checkbox' ? target.checked : target.value;
}

export interface FormRendererProps {
  class?: string;
  className?: string;
  formApi: FormApi;
  onSubmit?: (values: Record<string, any>) => void;
}

/**
 * 表单渲染组件（对应 Vue 侧 form-render/form.vue + form-field.vue）。
 *
 * 逐字复刻布局 class：
 * - 网格容器：`flex gap-x-4 flex-col grid gap-y-1.5` + wrapperClass
 * - 表单项：`relative flex flex-row items-center pb-4`
 * - label：`flex leading-6 mr-2 shrink-0 justify-end` + 宽度=labelWidth，必填星号 `mr-0.5 text-destructive`
 * - 控件容器：`flex-auto p-px` > `relative flex w-full items-center`
 */
export function FormRenderer({
  class: classNameProp,
  className,
  formApi,
  onSubmit,
}: FormRendererProps) {
  const { errors, values } = useStore(formApi.getStore());
  const options = formApi.latestOptions;
  const { commonConfig = {}, compact = false, schema = [], wrapperClass } =
    options;
  const hideLabelGlobal = commonConfig.hideLabel ?? false;
  const labelWidthGlobal = commonConfig.labelWidth ?? 100;
  const commonComponentProps = commonConfig.componentProps ?? {};

  const gridClass = cn(
    'flex gap-x-4 flex-col grid gap-y-1.5',
    wrapperClass,
    classNameProp,
    className,
  );

  const renderField = (field: FormSchema): ReactNode => {
    const deps = field.dependencies;
    // dependencies.if 为 false 时字段不渲染（与 Vue 的 v-if 一致）
    if (deps?.if && !deps.if(values)) {
      return null;
    }
    const isShow = deps?.show ? deps.show(values) : true;

    const FieldComponent =
      typeof field.component === 'string'
        ? getComponent(field.component)
        : field.component;

    if (!FieldComponent) {
      console.warn(`[form-ui] 组件未注册: ${String(field.component)}`);
      return null;
    }

    const dynamicProps =
      typeof field.componentProps === 'function'
        ? field.componentProps(values)
        : (field.componentProps ?? {});
    const computedProps = { ...commonComponentProps, ...dynamicProps };

    const isDisabled = deps?.disabled ? deps.disabled(values) : false;
    const normalizedRequired =
      (deps?.required ? deps.required(values) : false) ||
      field.rules === 'required' ||
      field.rules === 'selectRequired' ||
      (Array.isArray(field.rules)
        ? field.rules.some((rule) => rule.required)
        : typeof field.rules === 'object' && field.rules !== null
          ? (field.rules as any).required
          : false);

    const hideLabel = field.hideLabel ?? hideLabelGlobal;
    const labelWidth = field.labelWidth ?? labelWidthGlobal;
    const valuePropName = field.modelPropName ?? 'value';
    const error = errors[field.fieldName];
    const value = values[field.fieldName];

    const handleChange = (arg: any) => {
      const next = unwrapEvent(arg);
      formApi.setFieldValue(field.fieldName, next);
      computedProps.onChange?.(arg);
    };

    return (
      <div
        className={cn(
          'relative flex flex-row items-center',
          compact ? 'pb-2' : 'pb-4',
          normalizedRequired && 'form-is-required',
          error && 'form-valid-error',
          field.formItemClass,
          !isShow && 'hidden',
        )}
        key={field.fieldName}
      >
        {hideLabel ? null : (
          <label
            className={cn(
              LABEL_BASE,
              'flex leading-6 mr-2 shrink-0 justify-end items-center',
            )}
            style={{ width: `${labelWidth}px` }}
          >
            {normalizedRequired && !commonConfig.hideRequiredMark ? (
              <span className="mr-0.5 text-destructive">*</span>
            ) : null}
            {field.label}
          </label>
        )}
        <div className="flex-auto p-px">
          <div className="relative flex w-full items-center">
            <FieldComponent
              {...computedProps}
              {...{ [valuePropName]: value }}
              disabled={isDisabled || computedProps.disabled}
              onChange={handleChange}
            />
          </div>
          {error ? (
            <p className="text-destructive text-[0.8rem] absolute bottom-[-4px]">
              {error}
            </p>
          ) : null}
        </div>
      </div>
    );
  };

  return (
    <form
      className={gridClass}
      onSubmit={(event) => {
        event.preventDefault();
        void (async () => {
          const { valid, values: formValues } = await formApi.validate();
          if (valid) onSubmit?.(formValues);
        })();
      }}
    >
      {schema.map((field) => renderField(field))}
    </form>
  );
}
