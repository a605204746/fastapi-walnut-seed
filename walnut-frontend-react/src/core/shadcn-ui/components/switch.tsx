import type { ButtonHTMLAttributes } from 'react';

import { cn } from '@/core/shared/utils';

export interface SwitchProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className' | 'onChange'> {
  checked?: boolean;
  class?: string;
  onChange?: (checked: boolean) => void;
}

/**
 * 对应 Vue 侧 `ui/switch/Switch.vue`（reka-ui SwitchRoot/SwitchThumb）。
 * class 逐字复刻，保证与 Vue 像素一致：h-5 w-9 轨道 + h-4 w-4 圆点，
 * 选中 translate-x-4，轨道色 primary / input。
 */
export function Switch({
  checked = false,
  class: className,
  disabled,
  onChange,
  onClick,
  ...rest
}: SwitchProps) {
  const state = checked ? 'checked' : 'unchecked';
  return (
    <button
      {...rest}
      aria-checked={checked}
      className={cn(
        'peer focus-visible:ring-ring focus-visible:ring-offset-background data-[state=checked]:bg-primary data-[state=unchecked]:bg-input inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent shadow-xs transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      data-state={state}
      disabled={disabled}
      onClick={(e) => {
        onClick?.(e);
        if (!e.defaultPrevented) onChange?.(!checked);
      }}
      role="switch"
      type="button"
    >
      <span
        className="bg-background pointer-events-none block h-4 w-4 rounded-full shadow-lg ring-0 transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0"
        data-state={state}
      />
    </button>
  );
}