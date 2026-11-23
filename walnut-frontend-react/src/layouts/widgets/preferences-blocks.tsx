import type { ReactNode } from 'react';

import type {
  AnyCustomPreferencesField,
  CustomPreferencesRecord,
} from '@/core/preferences';

import { $t } from '@/core/locales';
import {
  Input,
  NumberField,
  NumberFieldContent,
  NumberFieldDecrement,
  NumberFieldIncrement,
  NumberFieldInput,
  Select,
  SelectContent,
  SelectItem as SelectItemPrimitive,
  SelectTrigger,
  SelectValue,
  Switch,
  ToggleGroup,
  ToggleGroupItem,
  WalnutCheckButtonGroup,
  WalnutTooltip,
} from '@/core/shadcn-ui';
import { CircleHelp, CircleX } from 'lucide-react';

/**
 * 偏好设置抽屉的行组件，逐一对应 Vue 侧
 * `widgets/preferences/blocks/{block,switch-item,toggle-item,checkbox-item,
 * number-field-item,select-item,input-item}.vue` 与 `custom/custom.vue`。
 * 控件全部使用与 Vue 同源的 shadcn-ui 原语，保证像素一致。
 */

/** 对应 block.vue */
export function Block({ children, title }: { children: ReactNode; title: string }) {
  return (
    <div className="flex flex-col py-4">
      <h3 className="mb-3 leading-none font-semibold tracking-tight">{title}</h3>
      {children}
    </div>
  );
}

/** 对应各 item 的 WalnutTooltip + CircleHelp，tip 按 \n 分行（Vue <p v-for>） */
export function TipIcon({ tip }: { tip?: string }) {
  if (!tip) return null;
  return (
    <WalnutTooltip
      side="bottom"
      trigger={<CircleHelp className="ml-1 size-3 cursor-help" />}
    >
      {tip.split('\n').map((line, index) => (
        <p key={index}>{line}</p>
      ))}
    </WalnutTooltip>
  );
}

/** 对应 switch-item.vue：整行可点切换 + shadcn Switch + shortcut 插槽 */
export function SwitchItem({
  checked,
  disabled = false,
  label,
  onChange,
  shortcut,
  tip,
}: {
  checked: boolean;
  disabled?: boolean;
  label: ReactNode;
  onChange: (checked: boolean) => void;
  shortcut?: ReactNode;
  tip?: string;
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-2.5 hover:bg-accent ${
        disabled ? 'pointer-events-none opacity-50' : 'cursor-pointer'
      }`}
      onClick={() => !disabled && onChange(!checked)}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
      {shortcut ? (
        <span className="mr-2 ml-auto text-xs opacity-60">{shortcut}</span>
      ) : null}
      <Switch
        checked={checked}
        disabled={disabled}
        onChange={onChange}
        onClick={(e) => e.stopPropagation()}
      />
    </div>
  );
}

/** 对应 toggle-item.vue：单选 ToggleGroup（outline，选中 primary） */
export function ToggleItem({
  disabled = false,
  items,
  label,
  onChange,
  value,
}: {
  disabled?: boolean;
  items: { label: string; value: string }[];
  label: ReactNode;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <div
      className={`flex w-full items-center justify-between rounded-md p-2 hover:bg-accent ${
        disabled ? 'pointer-events-none opacity-50' : ''
      }`}
    >
      <span className="text-sm">{label}</span>
      <ToggleGroup
        class="gap-2"
        modelValue={value}
        size="sm"
        type="single"
        onUpdate:modelValue={(next) => onChange(next as string)}
        variant="outline"
      >
        {items.map((item) => (
          <ToggleGroupItem
            class="h-7 rounded-sm data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
            key={item.value}
            value={item.value}
          >
            {item.label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}

/** 对应 checkbox-item.vue：WalnutCheckButtonGroup 多选（h-8 w-41.25） */
export function CheckboxItem({
  disabled = false,
  items,
  label,
  onChange,
  tip,
  values,
}: {
  disabled?: boolean;
  items: { label: string; value: string }[];
  label: ReactNode;
  onChange: (values: string[]) => void;
  tip?: string;
  values: string[];
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-1 ${
        tip ? '' : 'hover:bg-accent'
      } ${disabled ? 'pointer-events-none opacity-50' : ''}`}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
      <WalnutCheckButtonGroup
        class="h-8 w-41.25"
        disabled={disabled}
        modelValue={values}
        multiple
        options={items}
        onUpdate:modelValue={(next) => onChange((next as string[]) ?? [])}
      />
    </div>
  );
}

/** 对应 number-field-item.vue：NumberField 左右步进按钮（w-41.25） */
export function NumberItem({
  disabled = false,
  label,
  max,
  min,
  onChange,
  placeholder,
  step = 1,
  tip,
  value,
}: {
  disabled?: boolean;
  label: ReactNode;
  max?: number;
  min?: number;
  onChange: (value: number) => void;
  placeholder?: string;
  step?: number;
  tip?: string;
  value?: number;
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-1 ${
        tip ? '' : 'hover:bg-accent'
      } ${disabled ? 'pointer-events-none opacity-50' : ''}`}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
      <NumberField
        class="w-41.25"
        disabled={disabled}
        max={max}
        min={min}
        modelValue={value}
        step={step}
        onUpdate:modelValue={onChange}
      >
        <NumberFieldContent>
          <NumberFieldDecrement />
          <NumberFieldInput placeholder={placeholder} />
          <NumberFieldIncrement />
        </NumberFieldContent>
      </NumberField>
    </div>
  );
}

/** 对应 select-item.vue：shadcn Select（trigger h-8 w-41.25） */
export function SelectItem({
  disabled = false,
  label,
  onChange,
  options,
  placeholder,
  tip,
  value,
}: {
  disabled?: boolean;
  label: ReactNode;
  onChange: (value: string) => void;
  options: { label: string; value: string }[];
  placeholder?: string;
  tip?: string;
  value: string;
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-1 ${
        tip ? '' : 'hover:bg-accent'
      } ${disabled ? 'pointer-events-none opacity-50' : ''}`}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
      <Select
        disabled={disabled}
        modelValue={value}
        onUpdate:modelValue={onChange}
      >
        <SelectTrigger class="h-8 w-41.25">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {options.map((item) => (
            <SelectItemPrimitive key={item.value} value={item.value}>
              {item.label}
            </SelectItemPrimitive>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

/** 对应 input-item.vue：shadcn Input + 有值时 CircleX 一键清空 */
export function InputItem({
  disabled = false,
  label,
  onChange,
  placeholder,
  tip,
  value,
}: {
  disabled?: boolean;
  label: ReactNode;
  onChange: (value: string) => void;
  placeholder?: string;
  tip?: string;
  value: string;
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-1 ${
        tip ? '' : 'hover:bg-accent'
      } ${disabled ? 'pointer-events-none opacity-50' : ''}`}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
      <div className="relative">
        <Input
          class="h-8 w-41.25"
          disabled={disabled}
          onChange={onChange}
          placeholder={placeholder}
          value={value}
        />
        {value ? (
          <CircleX
            className="text-foreground/60 hover:text-foreground absolute top-1/2 right-2 size-3 -translate-y-1/2 transform cursor-pointer"
            onClick={() => onChange('')}
          />
        ) : null}
      </div>
    </div>
  );
}

/** 对应 custom/custom.vue：按扩展字段 schema 渲染 switch/number/select/input */
export function CustomFields({
  fields,
  onUpdate,
  values,
}: {
  fields: ReadonlyArray<AnyCustomPreferencesField>;
  onUpdate: (updates: CustomPreferencesRecord) => void;
  values: CustomPreferencesRecord;
}) {
  function resolveNumber(value: unknown) {
    return typeof value === 'number' && Number.isFinite(value)
      ? value
      : undefined;
  }
  return (
    <>
      {fields.map((field) => {
        const label = $t(field.label);
        const tip = field.tip ? $t(field.tip) : '';
        const placeholder = field.placeholder ? $t(field.placeholder) : '';
        const props = field.componentProps ?? {};
        switch (field.component) {
          case 'number': {
            return (
              <NumberItem
                disabled={field.disabled}
                key={field.key}
                label={label}
                max={props.max}
                min={props.min}
                onChange={(value) => onUpdate({ [field.key]: value })}
                placeholder={placeholder}
                step={props.step}
                tip={tip}
                value={resolveNumber(values[field.key])}
              />
            );
          }
          case 'select': {
            return (
              <SelectItem
                disabled={field.disabled}
                key={field.key}
                label={label}
                onChange={(value) => onUpdate({ [field.key]: value })}
                options={field.options.map((option) => ({
                  label: $t(option.label),
                  value: option.value,
                }))}
                placeholder={placeholder}
                tip={tip}
                value={String(values[field.key] ?? '')}
              />
            );
          }
          case 'switch': {
            return (
              <SwitchItem
                checked={Boolean(values[field.key])}
                disabled={field.disabled}
                key={field.key}
                label={label}
                onChange={(checked) => onUpdate({ [field.key]: checked })}
                tip={tip}
              />
            );
          }
          default: {
            return (
              <InputItem
                disabled={field.disabled}
                key={field.key}
                label={label}
                onChange={(value) => onUpdate({ [field.key]: value })}
                placeholder={placeholder}
                tip={tip}
                value={String(values[field.key] ?? '')}
              />
            );
          }
        }
      })}
    </>
  );
}