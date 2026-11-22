import type { ButtonHTMLAttributes, ReactNode } from 'react';

import { cn } from '@walnut-core/shared/utils';
import { createContext, useContext } from 'react';

/**
 * 复刻 Vue 侧 `ui/toggle/toggle.ts` 的 toggleVariants（cva）。
 */
const toggleBase =
  'inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors hover:bg-muted hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 data-[state=on]:bg-accent data-[state=on]:text-accent-foreground';

const toggleSizeMap = {
  default: 'h-9 px-3',
  lg: 'h-10 px-3',
  sm: 'h-8 px-2',
} as const;

const toggleVariantMap = {
  default: 'bg-transparent',
  outline:
    'border border-input bg-transparent shadow-sm hover:bg-accent hover:text-accent-foreground',
} as const;

export type ToggleVariant = keyof typeof toggleVariantMap;
export type ToggleSize = keyof typeof toggleSizeMap;

export function toggleVariants({
  class: className,
  size = 'default',
  variant = 'default',
}: { class?: string; size?: ToggleSize; variant?: ToggleVariant } = {}) {
  return cn(toggleBase, toggleVariantMap[variant], toggleSizeMap[size], className);
}

interface ToggleGroupContextValue {
  size: ToggleSize;
  toggle(value: string): void;
  value: string | string[] | undefined;
  variant: ToggleVariant;
}

const ToggleGroupContext = createContext<ToggleGroupContextValue | null>(null);

export interface ToggleGroupProps {
  children?: ReactNode;
  class?: string;
  modelValue?: string | string[];
  size?: ToggleSize;
  type?: 'multiple' | 'single';
  variant?: ToggleVariant;
  'onUpdate:modelValue'?: (value: string | string[]) => void;
}

/**
 * 对应 Vue 侧 `ui/toggle-group`。type=single 单选、multiple 多选。
 */
export function ToggleGroup({
  children,
  class: className,
  modelValue,
  size = 'default',
  type = 'single',
  variant = 'default',
  'onUpdate:modelValue': onUpdate,
}: ToggleGroupProps) {
  const toggle = (value: string) => {
    if (type === 'single') {
      onUpdate?.(value);
      return;
    }
    const current = Array.isArray(modelValue) ? modelValue : [];
    onUpdate?.(
      current.includes(value)
        ? current.filter((item) => item !== value)
        : [...current, value],
    );
  };

  return (
    <ToggleGroupContext.Provider value={{ size, toggle, value: modelValue, variant }}>
      <div className={cn('flex items-center', className)} role="group">
        {children}
      </div>
    </ToggleGroupContext.Provider>
  );
}

export interface ToggleGroupItemProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className' | 'value'> {
  class?: string;
  size?: ToggleSize;
  value: string;
  variant?: ToggleVariant;
}

export function ToggleGroupItem({
  class: className,
  children,
  size,
  value,
  variant,
  ...rest
}: ToggleGroupItemProps) {
  const context = useContext(ToggleGroupContext);
  const isActive = Array.isArray(context?.value)
    ? context.value.includes(value)
    : context?.value === value;

  return (
    <button
      {...rest}
      className={cn(
        toggleVariants({
          size: context?.size ?? size,
          variant: context?.variant ?? variant,
        }),
        className,
      )}
      data-state={isActive ? 'on' : 'off'}
      onClick={() => context?.toggle(value)}
      type={rest.type ?? 'button'}
    >
      {children}
    </button>
  );
}
