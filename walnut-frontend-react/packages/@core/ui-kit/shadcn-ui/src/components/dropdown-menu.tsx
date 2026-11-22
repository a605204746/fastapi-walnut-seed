import type { ComponentType, ReactNode } from 'react';

import { cn } from '@walnut-core/shared/utils';
import { useEffect, useRef, useState } from 'react';

export interface WalnutDropdownMenuItem {
  disabled?: boolean;
  handler?: (data: any) => void;
  /** 图标组件（React 组件类型） */
  icon?: ComponentType<{ class?: string; className?: string }>;
  label: string;
  separator?: boolean;
  value: string;
}

export interface WalnutDropdownRadioMenuProps {
  /** 触发元素（默认插槽） */
  children?: ReactNode;
  menus: WalnutDropdownMenuItem[];
  modelValue?: string;
  'onUpdate:modelValue'?: (value: string) => void;
}

/**
 * 对应 Vue 侧 `components/dropdown-menu/dropdown-radio-menu.vue`。
 *
 * PoC 用轻量自绘下拉（点击展开 + 点击外部关闭），class 逐字复刻：
 * content: `z-popup border-border bg-popover text-popover-foreground ... rounded-md border p-1 shadow-md`
 * item: `focus:bg-accent ... rounded-sm px-2 py-1.5 text-sm ...`
 * 默认态只渲染 trigger，不影响像素回归门禁。
 */
export function WalnutDropdownRadioMenu({
  children,
  menus,
  modelValue,
  'onUpdate:modelValue': onUpdate,
}: WalnutDropdownRadioMenuProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onClickOutside = (event: MouseEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [open]);

  return (
    <div className="relative inline-flex" ref={containerRef}>
      <div
        className="flex items-center gap-1"
        onClick={() => setOpen((value) => !value)}
        role="button"
        tabIndex={-1}
      >
        {children}
      </div>
      {open ? (
        <div
          className={cn(
            'z-popup border-border bg-popover text-popover-foreground',
            'absolute top-full left-0 z-50 mt-1 min-w-32 overflow-hidden rounded-md border p-1 shadow-md',
          )}
        >
          {menus.map((menu) => {
            const Icon = menu.icon;
            const active = menu.value === modelValue;
            return (
              <div
                className={cn(
                  'focus:bg-accent focus:text-accent-foreground relative flex cursor-default items-center rounded-sm px-2 py-1.5 text-sm transition-colors select-none',
                  'text-foreground/80 mb-1 cursor-pointer',
                  active && 'bg-accent text-accent-foreground',
                )}
                key={menu.value}
                onClick={() => {
                  onUpdate?.(menu.value);
                  menu.handler?.(menu.value);
                  setOpen(false);
                }}
                role="menuitemradio"
              >
                {Icon ? (
                  <Icon className="mr-2 size-4" />
                ) : (
                  <span
                    className={cn(
                      'mr-2 size-1.5 rounded-full',
                      active && 'bg-foreground',
                    )}
                  ></span>
                )}
                {menu.label}
              </div>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
