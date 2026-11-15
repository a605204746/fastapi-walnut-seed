import type { ButtonHTMLAttributes, ReactNode } from 'react';

import { cn } from '@vben-core/shared/utils';
import { LoaderCircle } from 'lucide-react';

import { type ButtonVariants, type ButtonVariantSize, buttonVariants } from '../ui/button';
import { VbenTooltip } from './tooltip';

export interface VbenButtonProps
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
export function VbenButton({
  class: className,
  children,
  disabled = false,
  loading = false,
  size = 'default',
  variant = 'default',
  ...rest
}: VbenButtonProps) {
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

export interface VbenIconButtonProps extends VbenButtonProps {
  tooltip?: ReactNode;
  tooltipDelayDuration?: number;
  tooltipSide?: 'bottom' | 'left' | 'right' | 'top';
}

/**
 * 对应 Vue 侧 `icon-button.vue`：variant=icon / size=icon / rounded-full，
 * 传入 tooltip 时包裹 VbenTooltip。
 */
export function VbenIconButton({
  class: className,
  tooltip,
  tooltipDelayDuration = 200,
  tooltipSide = 'bottom',
  variant = 'icon',
  ...rest
}: VbenIconButtonProps) {
  const button = (
    <VbenButton
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
    <VbenTooltip
      delayDuration={tooltipDelayDuration}
      side={tooltipSide}
      trigger={button}
    >
      {tooltip}
    </VbenTooltip>
  );
}
