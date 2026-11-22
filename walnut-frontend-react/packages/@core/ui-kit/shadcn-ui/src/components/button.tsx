import type { ButtonHTMLAttributes, ReactNode } from 'react';

import { cn } from '@walnut-core/shared/utils';
import { LoaderCircle } from 'lucide-react';

import { type ButtonVariants, type ButtonVariantSize, buttonVariants } from '../ui/button';
import { WalnutTooltip } from './tooltip';

export interface WalnutButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className'> {
  class?: string;
  className?: string;
  loading?: boolean;
  size?: ButtonVariantSize;
  variant?: ButtonVariants;
}

/**
 * 对应 Vue 侧 `@core/ui-kit/shadcn-ui/src/components/button/button.vue`。
 * 默认 as=button，class = buttonVariants({variant,size}) + props.class。
 */
export function WalnutButton({
  class: className,
  children,
  disabled = false,
  loading = false,
  size = 'default',
  variant = 'default',
  ...rest
}: WalnutButtonProps) {
  const isDisabled = disabled || loading;
  return (
    <button
      {...rest}
      className={cn(buttonVariants({ size, variant }), className)}
      disabled={isDisabled}
      type={rest.type ?? 'button'}
    >
      {loading ? (
        <LoaderCircle className="text-md mr-2 size-4 shrink-0 animate-spin" />
      ) : null}
      {children}
    </button>
  );
}

export interface WalnutIconButtonProps extends WalnutButtonProps {
  tooltip?: ReactNode;
  tooltipDelayDuration?: number;
  tooltipSide?: 'bottom' | 'left' | 'right' | 'top';
}

/**
 * 对应 Vue 侧 `icon-button.vue`：variant=icon / size=icon / rounded-full，
 * 传入 tooltip 时包裹 WalnutTooltip。
 */
export function WalnutIconButton({
  class: className,
  tooltip,
  tooltipDelayDuration = 200,
  tooltipSide = 'bottom',
  variant = 'icon',
  ...rest
}: WalnutIconButtonProps) {
  const button = (
    <WalnutButton
      {...rest}
      class={cn('rounded-full', className)}
      size="icon"
      variant={variant}
    />
  );

  if (!tooltip) {
    return button;
  }

  return (
    <WalnutTooltip
      delayDuration={tooltipDelayDuration}
      side={tooltipSide}
      trigger={button}
    >
      {tooltip}
    </WalnutTooltip>
  );
}
