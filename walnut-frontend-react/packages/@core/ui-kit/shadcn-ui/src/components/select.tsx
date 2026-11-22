import type { ButtonHTMLAttributes, ReactNode, RefObject } from 'react';

import { cn } from '@walnut-core/shared/utils';
import { Check, ChevronDown } from 'lucide-react';
import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import { createPortal } from 'react-dom';

interface SelectContextValue {
  contentRef: RefObject<HTMLDivElement | null>;
  disabled: boolean;
  labels: Map<string, ReactNode>;
  onChange: (value: string) => void;
  open: boolean;
  registerLabel: (value: string, label: ReactNode) => void;
  setOpen: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
  value?: string;
}

const SelectContext = createContext<SelectContextValue | null>(null);

function useSelect() {
  const context = useContext(SelectContext);
  if (!context) {
    throw new Error('Select components must be used within Select');
  }
  return context;
}

export interface SelectProps {
  children?: ReactNode;
  disabled?: boolean;
  modelValue?: string;
  'onUpdate:modelValue'?: (value: string) => void;
}

/**
 * 对应 Vue 侧 `ui/select`（reka-ui Select）。
 * 轻量自绘：trigger + body portal 弹层（popper 定位），
 * class 逐字复刻 SelectTrigger/SelectContent/SelectItem，保证观感一致。
 * 弹层关闭时仅 display:none（不卸载），SelectValue 依赖 item 注册的 label。
 */
export function Select({
  children,
  disabled = false,
  modelValue,
  'onUpdate:modelValue': onUpdate,
}: SelectProps) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const contentRef = useRef<HTMLDivElement | null>(null);
  const labelsRef = useRef(new Map<string, ReactNode>());
  const [, forceRender] = useState(0);

  useEffect(() => {
    if (!open) return;
    function onDocumentMouseDown(e: MouseEvent) {
      const target = e.target as Node;
      if (
        triggerRef.current?.contains(target) ||
        contentRef.current?.contains(target)
      ) {
        return;
      }
      setOpen(false);
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false);
    }
    document.addEventListener('mousedown', onDocumentMouseDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onDocumentMouseDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  return (
    <SelectContext.Provider
      value={{
        contentRef,
        disabled,
        labels: labelsRef.current,
        onChange: (value) => onUpdate?.(value),
        open,
        registerLabel: (value, label) => {
          if (labelsRef.current.get(value) !== label) {
            labelsRef.current.set(value, label);
            forceRender((n) => n + 1);
          }
        },
        setOpen,
        triggerRef,
        value: modelValue,
      }}
    >
      {children}
    </SelectContext.Provider>
  );
}

export interface SelectTriggerProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className'> {
  class?: string;
}

/** 对应 `SelectTrigger.vue`。 */
export function SelectTrigger({
  children,
  class: className,
  ...rest
}: SelectTriggerProps) {
  const { disabled, open, setOpen, triggerRef } = useSelect();
  return (
    <button
      {...rest}
      aria-expanded={open}
      aria-haspopup="listbox"
      className={cn(
        'border-input ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:shadow-[inset_0_0_0_1px_hsl(var(--primary))] flex h-10 w-full items-center justify-between rounded-md border bg-transparent px-3 py-2 text-sm whitespace-nowrap shadow-xs disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1',
        className,
      )}
      disabled={disabled || rest.disabled}
      onClick={() => setOpen(!open)}
      onKeyDown={(e) => {
        if (e.key === 'ArrowDown' || e.key === 'Enter') {
          e.preventDefault();
          setOpen(true);
        }
        rest.onKeyDown?.(e);
      }}
      ref={triggerRef}
      type="button"
    >
      {children}
      <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
    </button>
  );
}

/** 对应 `SelectValue.vue`：展示选中项 label（item 注册）或 placeholder。 */
export function SelectValue({ placeholder }: { placeholder?: string }) {
  const { labels, value } = useSelect();
  const label = value === undefined ? undefined : labels.get(value);
  if (label === undefined && !placeholder) {
    return <span>{value}</span>;
  }
  return (
    <span className={label === undefined ? 'text-muted-foreground' : undefined}>
      {label ?? placeholder}
    </span>
  );
}

/** 对应 `SelectContent.vue`：body portal + popper 定位 + 边框阴影弹层。 */
export function SelectContent({
  children,
  class: className,
}: {
  children?: ReactNode;
  class?: string;
}) {
  const { contentRef, open, triggerRef } = useSelect();
  const [rect, setRect] = useState<DOMRect | null>(null);

  useEffect(() => {
    if (open && triggerRef.current) {
      setRect(triggerRef.current.getBoundingClientRect());
    }
  }, [open, triggerRef]);

  return createPortal(
    <div
      className={cn(
        'border-border bg-popover text-popover-foreground relative max-h-96 min-w-32 overflow-hidden rounded-md border shadow-md',
        className,
      )}
      data-state={open ? 'open' : 'closed'}
      ref={contentRef}
      role="listbox"
      style={{
        display: open ? undefined : 'none',
        left: rect?.left ?? 0,
        minWidth: rect?.width,
        position: 'fixed',
        top: (rect?.bottom ?? 0) + 4,
        zIndex: 2000,
      }}
    >
      <div className="max-h-96 w-full overflow-y-auto p-1">{children}</div>
    </div>,
    document.body,
  );
}

export interface SelectItemProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className' | 'value'> {
  class?: string;
  value: string;
}

/** 对应 `SelectItem.vue`：右侧 Check 指示器 + 选中态。 */
export function SelectItem({
  children,
  class: className,
  value,
  ...rest
}: SelectItemProps) {
  const { onChange, registerLabel, setOpen, value: current } = useSelect();
  const selected = current === value;

  useEffect(() => {
    registerLabel(value, children);
  }, [value, children, registerLabel]);

  return (
    <button
      {...rest}
      aria-selected={selected}
      className={cn(
        'focus:bg-accent focus:text-accent-foreground relative flex w-full cursor-default items-center rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden select-none data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
        'hover:bg-accent hover:text-accent-foreground',
        className,
      )}
      onClick={() => {
        onChange(value);
        setOpen(false);
      }}
      role="option"
      type="button"
    >
      <span className="absolute right-2 flex h-3.5 w-3.5 items-center justify-center">
        {selected ? <Check className="h-4 w-4" /> : null}
      </span>
      {children}
    </button>
  );
}