import type { ReactNode } from 'react';

import { cn } from '@walnut-core/shared/utils';
import { Circle, CircleCheckBig } from 'lucide-react';

import { buttonVariants } from '../ui/button';

export interface CheckButtonOption {
  label: ReactNode;
  value: string;
}

export interface WalnutCheckButtonGroupProps {
  allowClear?: boolean;
  btnClass?: string;
  class?: string;
  disabled?: boolean;
  gap?: number;
  maxCount?: number;
  modelValue?: string | string[];
  multiple?: boolean;
  onBtnClick?: (value: string | undefined) => void;
  options?: CheckButtonOption[];
  showIcon?: boolean;
  size?: 'large' | 'middle' | 'small';
  'onUpdate:modelValue'?: (value: string | string[] | undefined) => void;
}

const sizeMap = {
  large: { btn: 'h-9 px-3 text-sm', icon: 'mr-[0.4rem] size-4' },
  middle: { btn: 'h-8 px-2 text-xs', icon: 'mr-[0.2rem] size-3' },
  small: { btn: 'h-7 px-[0.4rem] text-[0.65rem]', icon: 'mr-[0.1rem] size-[0.65rem]' },
} as const;

/**
 * 对应 Vue 侧 `components/button/check-button-group.vue` + `button-group.vue`。
 * 选中项 variant=default + CircleCheckBig 图标，未选 outline + Circle；
 * no-gap 时按钮拼接（首尾圆角、中间去左边框），与 Vue scoped scss 等价。
 */
export function WalnutCheckButtonGroup({
  allowClear = false,
  btnClass,
  class: className,
  disabled = false,
  gap = 0,
  maxCount = 0,
  modelValue,
  multiple = false,
  onBtnClick,
  options = [],
  showIcon = true,
  size = 'middle',
  'onUpdate:modelValue': onUpdate,
}: WalnutCheckButtonGroupProps) {
  const inner: string[] = Array.isArray(modelValue)
    ? modelValue
    : modelValue === undefined
      ? []
      : [modelValue];
  const sizeClass = sizeMap[size];

  function handleClick(value: string) {
    if (multiple) {
      let next: string[];
      if (inner.includes(value)) {
        next = inner.filter((item) => item !== value);
      } else {
        next =
          maxCount > 0 && inner.length >= maxCount
            ? [...inner.slice(0, maxCount - 1), value]
            : [...inner, value];
      }
      onUpdate?.(next);
      onBtnClick?.(value);
      return;
    }
    if (allowClear && inner.includes(value)) {
      onUpdate?.(undefined);
      onBtnClick?.(undefined);
      return;
    }
    onUpdate?.(value);
    onBtnClick?.(value);
  }

  return (
    <div
      className={cn('inline-flex flex-wrap rounded-md', className)}
      style={{ gap: gap ? `${gap}px` : '0px' }}
    >
      {options.map((option, index) => {
        const checked = inner.includes(option.value);
        const first = index === 0;
        const last = index === options.length - 1;
        return (
          <button
            className={cn(
              buttonVariants({ size: 'sm', variant: checked ? 'default' : 'outline' }),
              sizeClass.btn,
              'border',
              gap === 0 && !first && 'border-l-0',
              gap === 0 && !first && !last && 'rounded-none',
              gap === 0 && first && !last && 'rounded-r-none',
              gap === 0 && last && !first && 'rounded-l-none',
              btnClass,
            )}
            disabled={disabled}
            key={option.value}
            onClick={() => handleClick(option.value)}
            type="button"
          >
            {showIcon ? (
              <span className={cn('flex items-center [&>svg]:h-full [&>svg]:w-full', sizeClass.icon)}>
                {checked ? <CircleCheckBig /> : <Circle />}
              </span>
            ) : null}
            {option.label}
          </button>
        );
      })}
    </div>
  );
}