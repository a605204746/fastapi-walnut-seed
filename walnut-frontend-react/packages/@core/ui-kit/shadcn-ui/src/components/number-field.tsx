import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from 'react';

import { cn } from '@walnut-core/shared/utils';
import { Minus, Plus } from 'lucide-react';
import { createContext, useContext, useEffect, useState } from 'react';

interface NumberFieldContextValue {
  commit: (value: number) => void;
  disabled: boolean;
  max?: number;
  min?: number;
  step: number;
  value?: number;
}

const NumberFieldContext = createContext<NumberFieldContextValue | null>(null);

function useNumberField() {
  const context = useContext(NumberFieldContext);
  if (!context) {
    throw new Error('NumberField components must be used within NumberField');
  }
  return context;
}

function clamp(value: number, min?: number, max?: number) {
  let result = value;
  if (min !== undefined && result < min) result = min;
  if (max !== undefined && result > max) result = max;
  return result;
}

export interface NumberFieldProps {
  children?: ReactNode;
  class?: string;
  disabled?: boolean;
  max?: number;
  min?: number;
  modelValue?: number;
  step?: number;
  'onUpdate:modelValue'?: (value: number) => void;
}

/** 对应 Vue 侧 `ui/number-field/NumberField.vue`（reka-ui NumberFieldRoot）。 */
export function NumberField({
  children,
  class: className,
  disabled = false,
  max,
  min,
  modelValue,
  step = 1,
  'onUpdate:modelValue': onUpdate,
}: NumberFieldProps) {
  const commit = (value: number) => {
    if (!Number.isFinite(value)) return;
    onUpdate?.(clamp(value, min, max));
  };
  return (
    <NumberFieldContext.Provider
      value={{ commit, disabled, max, min, step, value: modelValue }}
    >
      <div className={cn('grid gap-1.5', className)}>{children}</div>
    </NumberFieldContext.Provider>
  );
}

/** 对应 `NumberFieldContent.vue`：相对定位容器，左右留出步进按钮内边距。 */
export function NumberFieldContent({
  children,
  class: className,
}: {
  children?: ReactNode;
  class?: string;
}) {
  return (
    <div
      className={cn(
        'relative [&>[data-slot=input]]:pl-5 [&>[data-slot=input]]:pr-5',
        className,
      )}
    >
      {children}
    </div>
  );
}

export interface NumberFieldInputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'className' | 'onChange' | 'value'> {
  class?: string;
}

/** 对应 `NumberFieldInput.vue`：居中数字输入，失焦 / 回车时 clamp 提交。 */
export function NumberFieldInput({ class: className, ...rest }: NumberFieldInputProps) {
  const { commit, disabled, value } = useNumberField();
  const [text, setText] = useState(value === undefined ? '' : String(value));

  useEffect(() => {
    setText(value === undefined ? '' : String(value));
  }, [value]);

  function handleCommit() {
    const parsed = Number.parseFloat(text);
    if (Number.isFinite(parsed)) {
      commit(parsed);
    } else {
      setText(value === undefined ? '' : String(value));
    }
  }

  return (
    <input
      {...rest}
      className={cn(
        'border-input placeholder:text-muted-foreground focus-visible:ring-ring flex h-9 w-full rounded-md border bg-transparent py-1 text-center text-sm shadow-xs transition-colors focus-visible:ring-1 focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50',
        '[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none',
        className,
      )}
      data-slot="input"
      disabled={disabled || rest.disabled}
      inputMode="decimal"
      onBlur={handleCommit}
      onChange={(e) => setText(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') handleCommit();
        rest.onKeyDown?.(e);
      }}
      type="text"
      value={text}
    />
  );
}

type StepButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className'> & {
  class?: string;
};

/** 对应 `NumberFieldDecrement.vue`：左缘 Minus 步进按钮。 */
export function NumberFieldDecrement({ class: className, ...rest }: StepButtonProps) {
  const { commit, disabled, min, step, value } = useNumberField();
  const atBound = value !== undefined && min !== undefined && value <= min;
  return (
    <button
      {...rest}
      aria-label="decrement"
      className={cn(
        'absolute top-1/2 left-0 -translate-y-1/2 p-3 disabled:cursor-not-allowed disabled:opacity-20',
        className,
      )}
      data-slot="decrement"
      disabled={disabled || atBound}
      onClick={() => commit((value ?? 0) - step)}
      type="button"
    >
      <Minus className="h-4 w-4" />
    </button>
  );
}

/** 对应 `NumberFieldIncrement.vue`：右缘 Plus 步进按钮。 */
export function NumberFieldIncrement({ class: className, ...rest }: StepButtonProps) {
  const { commit, disabled, max, step, value } = useNumberField();
  const atBound = value !== undefined && max !== undefined && value >= max;
  return (
    <button
      {...rest}
      aria-label="increment"
      className={cn(
        'absolute top-1/2 right-0 -translate-y-1/2 p-3 disabled:cursor-not-allowed disabled:opacity-20',
        className,
      )}
      data-slot="increment"
      disabled={disabled || atBound}
      onClick={() => commit((value ?? 0) + step)}
      type="button"
    >
      <Plus className="h-4 w-4" />
    </button>
  );
}