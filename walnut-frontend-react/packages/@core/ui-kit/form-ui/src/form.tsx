import type { ReactNode } from 'react';

import type { FormApi } from './form-api';
import type { FormSchema } from './types';

import { useStore } from '@vben-core/shared/store';
import { cn } from '@vben-core/shared/utils';

import { getComponent } from './config';
import { normalizeRules } from './form-api';

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
  const {
    commonConfig = {},
    compact = false,
    layout = 'horizontal',
    schema = [],
    wrapperClass,
  } = options;
  const isVertical = layout === 'vertical';
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
    const computedProps: Record<string, any> = {
      ...commonComponentProps,
      ...dynamicProps,
    };

    const isDisabled = deps?.disabled ? deps.disabled(values) : false;
    // dependencies.rules 提供时覆盖 field.rules（动态校验，如菜单的路由地址）
    const effectiveRules = deps?.rules ? deps.rules(values) : field.rules;
    const normalizedRequired =
      (deps?.required ? deps.required(values) : false) ||
      normalizeRules(effectiveRules).some((rule) => rule.required);

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

    /**
     * renderComponentContent（对应 Vue 侧同名属性）：
     * - 具名插槽（addonBefore / addonAfter / prefix …）→ 转成同名 prop 合并给控件
     * - `default` 插槽 → **替换整个控件**（角色页把 TreeSelect 换成自绘面板就是这么做的）
     *
     * 值可以是节点，也可以是函数。函数的入参是**最终 props**（含 value / onChange /
     * disabled），与 Vue 的插槽作用域 `slotProps` 对齐 —— 否则 `default` 替换出来的
     * 控件会拿不到受控值。
     */
    let defaultSlot:
      | ((attrs: Record<string, any>) => ReactNode)
      | ReactNode
      | undefined;
    if (field.renderComponentContent) {
      const content = field.renderComponentContent(values);
      for (const [slot, node] of Object.entries(content)) {
        if (slot === 'default') {
          defaultSlot = node;
        } else {
          computedProps[slot] =
            typeof node === 'function' ? node(computedProps) : node;
        }
      }
    }

    const finalProps: Record<string, any> = {
      ...computedProps,
      [valuePropName]: value,
      disabled: isDisabled || computedProps.disabled,
      onChange: handleChange,
    };

    const contentOverride: null | ReactNode =
      defaultSlot === undefined
        ? null
        : typeof defaultSlot === 'function'
          ? defaultSlot(finalProps)
          : defaultSlot;

    return (
      <div
        className={cn(
          'relative flex',
          isVertical ? 'flex-col items-start' : 'flex-row items-center',
          compact ? 'pb-2' : 'pb-4',
          normalizedRequired && 'form-is-required',
          error && 'form-valid-error',
          // 全局 formItemClass 在前、字段级在后，保证字段级能覆盖（tailwind-merge 语义）
          commonConfig.formItemClass,
          field.formItemClass,
          !isShow && 'hidden',
        )}
        key={field.fieldName}
      >
        {hideLabel ? null : (
          <label
            className={cn(
              LABEL_BASE,
              'flex leading-6 items-center',
              isVertical
                ? 'mb-1 justify-start'
                : 'mr-2 shrink-0 justify-end',
            )}
            style={isVertical ? undefined : { width: `${labelWidth}px` }}
          >
            {normalizedRequired && !commonConfig.hideRequiredMark ? (
              <span className="mr-0.5 text-destructive">*</span>
            ) : null}
            {field.label}
            {/*
              help 用原生 title 提示：form-ui 是 UI 库无关层（package.json 里没有 antd），
              不能引 antd Tooltip。视觉与 Vue 的问号图标不等价，属 B 阶段的像素补齐项。
            */}
            {field.help ? (
              <span
                className="text-muted-foreground ml-1 cursor-help text-xs"
                title={field.help}
              >
                (?)
              </span>
            ) : null}
          </label>
        )}
        <div className={cn('p-px', isVertical ? 'w-full' : 'flex-auto')}>
          <div className="relative flex w-full items-center">
            {contentOverride ?? <FieldComponent {...finalProps} />}
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
