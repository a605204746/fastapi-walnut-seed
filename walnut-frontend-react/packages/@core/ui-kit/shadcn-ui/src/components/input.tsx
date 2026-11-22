import type { InputHTMLAttributes } from 'react';

import { cn } from '@walnut-core/shared/utils';

export interface InputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'className' | 'onChange'> {
  class?: string;
  onChange?: (value: string) => void;
}

/**
 * 对应 Vue 侧 `ui/input/Input.vue`。
 * Vue 的 scoped style（focus 内描边 primary、隐藏浏览器自带 spin/reveal 按钮）
 * 这里用 Tailwind 任意变体等价表达。
 */
export function Input({
  class: className,
  onChange,
  ...rest
}: InputProps) {
  return (
    <input
      {...rest}
      className={cn(
        'border-input bg-background ring-offset-background placeholder:text-muted-foreground/50 focus-visible:outline-none flex h-10 w-full rounded-md border px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:cursor-not-allowed disabled:opacity-50',
        'focus-visible:shadow-[inset_0_0_0_1px_hsl(var(--primary))]',
        '[&::-ms-reveal]:hidden [&::-ms-clear]:hidden [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none',
        className,
      )}
      onChange={(e) => onChange?.(e.target.value)}
    />
  );
}