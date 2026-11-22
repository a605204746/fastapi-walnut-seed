import { cn } from '@walnut-core/shared/utils';

/**
 * 按钮 variant / size 的 class 表。
 *
 * 逐字复刻 Vue 侧 `@core/ui-kit/shadcn-ui/src/ui/button/button.ts` 的 cva 定义，
 * 保证像素一致。这里用手写函数替代 class-variance-authority（输出等价），
 * 以免为一个 PoC 引入额外依赖。
 */
const buttonBase =
  'inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50';

const buttonSizeMap = {
  default: 'h-9 px-4 py-2',
  icon: 'h-8 w-8 rounded-sm px-1 text-lg',
  lg: 'h-10 rounded-md px-4',
  sm: 'h-8 rounded-md px-2 text-xs',
  xs: 'h-8 w-8 rounded-sm px-1 text-xs',
} as const;

const buttonVariantMap = {
  default: 'bg-primary text-primary-foreground shadow hover:bg-primary/90',
  destructive:
    'bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive-hover',
  ghost: 'hover:bg-accent hover:text-accent-foreground',
  heavy: 'hover:bg-heavy hover:text-heavy-foreground',
  icon: 'hover:bg-accent hover:text-accent-foreground text-foreground/80',
  link: 'text-primary underline-offset-4 hover:underline',
  outline:
    'border border-input bg-background shadow-sm hover:bg-accent hover:text-accent-foreground',
  secondary:
    'bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/80',
} as const;

export type ButtonVariants = keyof typeof buttonVariantMap;
export type ButtonVariantSize = keyof typeof buttonSizeMap;

export interface ButtonVariantsOptions {
  class?: string;
  size?: ButtonVariantSize;
  variant?: ButtonVariants;
}

export function buttonVariants({
  class: className,
  size = 'default',
  variant = 'default',
}: ButtonVariantsOptions = {}) {
  return cn(buttonBase, buttonVariantMap[variant], buttonSizeMap[size], className);
}
