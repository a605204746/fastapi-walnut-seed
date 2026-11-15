import type { ReactNode } from 'react';

import { cn } from '@vben-core/shared/utils';
import { useCallback, useRef, useState } from 'react';

export interface VbenTooltipProps {
  /** 提示内容 */
  children?: ReactNode;
  contentClass?: string;
  delayDuration?: number;
  disabled?: boolean;
  side?: 'bottom' | 'left' | 'right' | 'top';
  /** 触发元素 */
  trigger?: ReactNode;
}

const sideClass: Record<NonNullable<VbenTooltipProps['side']>, string> = {
  bottom: 'left-1/2 top-full mt-1 -translate-x-1/2',
  left: 'right-full top-1/2 mr-1 -translate-y-1/2',
  right: 'left-full top-1/2 ml-1 -translate-y-1/2',
  top: 'bottom-full left-1/2 mb-1 -translate-x-1/2',
};

/**
 * 对应 Vue 侧 `components/tooltip/tooltip.vue`（基于 reka-ui Tooltip）。
 *
 * PoC 用轻量自绘实现（hover 延迟 + 绝对定位气泡），不引入 Radix：
 * 默认态只渲染 trigger，气泡仅在 hover 后出现，因此不影响像素回归门禁的默认截图。
 * 气泡样式沿用 Vben 的 `bg-accent text-popover-foreground rounded-md`。
 */
export function VbenTooltip({
  children,
  contentClass,
  delayDuration = 0,
  disabled = false,
  side = 'right',
  trigger,
}: VbenTooltipProps) {
  const [open, setOpen] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const show = useCallback(() => {
    if (disabled) return;
    timer.current = setTimeout(() => setOpen(true), delayDuration);
  }, [delayDuration, disabled]);

  const hide = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    setOpen(false);
  }, []);

  return (
    <span
      className="relative inline-flex"
      onMouseEnter={show}
      onMouseLeave={hide}
    >
      {trigger}
      {open && children ? (
        <span
          className={cn(
            'side-content bg-accent text-popover-foreground rounded-md',
            'absolute z-50 px-3 py-1.5 text-sm whitespace-nowrap shadow-md',
            sideClass[side],
            contentClass,
          )}
          role="tooltip"
        >
          {children}
        </span>
      ) : null}
    </span>
  );
}
