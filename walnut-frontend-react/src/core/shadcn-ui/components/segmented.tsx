import type { ReactNode } from 'react';

import { cn } from '@/core/shared/utils';

export interface SegmentedItem {
  label: string;
  value: string;
}

export interface WalnutSegmentedProps {
  children?: ReactNode;
  class?: string;
  defaultValue?: string;
  modelValue?: string;
  tabs?: SegmentedItem[];
  'onUpdate:modelValue'?: (value: string) => void;
}

/**
 * 对应 Vue 侧 `components/segmented/segmented.vue` + `tabs-indicator.vue`。
 * grid 等分页签 + 绝对定位滑动指示块（translateX 按索引百分比，300ms 过渡），
 * 选中页签 font-bold + text-primary，外观与 Vue 基线 Segmented 一致。
 * children 为当前页签内容（对应 TabsContent，mt-2）。
 */
export function WalnutSegmented({
  children,
  class: className,
  defaultValue = '',
  modelValue,
  tabs = [],
  'onUpdate:modelValue': onUpdate,
}: WalnutSegmentedProps) {
  const active = modelValue ?? (defaultValue || tabs[0]?.value);
  const activeIndex = Math.max(
    0,
    tabs.findIndex((tab) => tab.value === active),
  );
  const count = Math.max(1, tabs.length);

  return (
    <div>
      <div
        className={cn(
          'bg-muted text-muted-foreground inline-flex h-9 items-center justify-center rounded-md p-1',
          'bg-accent relative grid w-full outline! outline-2!',
          className,
        )}
        role="tablist"
        style={{ gridTemplateColumns: `repeat(${count}, minmax(0, 1fr))` }}
      >
        <div
          className="absolute bottom-0 left-0 z-10 h-full rounded-full px-0 py-1 pr-0.5 transition-[width,transform] duration-300"
          style={{
            transform: `translateX(${activeIndex * 100}%)`,
            width: `${(100 / count).toFixed(0)}%`,
          }}
        >
          <div className="bg-background text-foreground inline-flex h-full w-full items-center justify-center rounded-md px-3 py-1 text-sm font-medium whitespace-nowrap" />
        </div>
        {tabs.map((tab) => (
          <button
            aria-selected={tab.value === active}
            className={cn(
              'hover:text-primary z-20 inline-flex items-center justify-center rounded-md px-3 py-1 text-sm font-medium whitespace-nowrap disabled:pointer-events-none disabled:opacity-50',
              tab.value === active && 'font-bold! text-primary',
            )}
            key={tab.value}
            onClick={() => onUpdate?.(tab.value)}
            role="tab"
            type="button"
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div className="mt-2">{children}</div>
    </div>
  );
}