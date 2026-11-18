import type { ReactNode } from 'react';

import { useEffect, useMemo, useRef, useState } from 'react';

import type { LayoutType } from '@vben-core/typings';
import type { SupportedLanguagesType } from '@vben/locales';

import {
  BUILT_IN_THEME_PRESETS,
  DEFAULT_TIME_ZONE_OPTIONS,
  clearCache,
  resetPreferences,
  updatePreferences,
  usePreferences,
} from '@vben/preferences';
import { SUPPORT_LANGUAGES } from '@vben/constants';
import { $t, loadLocaleMessages } from '@vben/locales';
import { convertToHsl, TinyColor } from '@vben/utils';
import {
  ToggleGroup,
  ToggleGroupItem,
  VbenIconButton,
} from '@vben-core/shadcn-ui';
import {
  CircleHelp,
  Copy,
  MoonStar,
  Pin,
  PinOff,
  RotateCw,
  Settings,
  Sun,
  SunMoon,
  UserRoundPen,
  X,
} from 'lucide-react';
import { Button, Drawer, Input, InputNumber, Segmented, Select, Switch, Tooltip } from 'antd';

import { useAuthStore } from '@/store/auth';
import { getMessageApi } from '@/services/message';

/**
 * 系统设置（偏好设置抽屉）—— 与 Vue 侧 `preferences-drawer.vue` 同构。
 *
 * 结构：Drawer(384px) + title/subtitle + extra(重置/钉住/关闭) +
 * 四页签 外观→布局→快捷键→通用 + footer(复制偏好/清空缓存退出)。
 *
 * 与 Vue 的对齐原则：
 * - 每个控件要么真实接到消费端，要么禁用 + tip 说明原因（不允许「点了没反应」）。
 * - 依赖外壳 1:1（B1/B3/B5）尚未迁移组件的开关，统一禁用 + `preferences.pendingTip`。
 * - 「有改动」判定 / 复制内容均使用与默认值的 diff（`usePreferences().diffPreference`），
 *   与 Vue 的 `mergedDiffPreference` 语义一致。
 */

let openPreferencesFn: null | (() => void) = null;

/** 命令式打开偏好设置抽屉（顶栏齿轮 / 侧边栏固定按钮 / 悬浮按钮共用） */
export function openPreferences() {
  openPreferencesFn?.();
}

export function PreferencesButton() {
  return (
    <VbenIconButton onClick={openPreferences}>
      <Settings className="size-4" />
    </VbenIconButton>
  );
}

/** 右侧边缘悬浮的设置入口（preferencesButtonPosition = fixed 时显示，对应 Vue layout.vue） */
export function PreferencesFixedButton() {
  return (
    <button
      className="bg-primary text-primary-foreground fixed top-1/2 right-0 flex size-10 -translate-y-1/2 cursor-pointer items-center justify-center rounded-l-lg border-none"
      onClick={openPreferences}
      style={{ zIndex: 100 }}
      title={$t('preferences.title')}
      type="button"
    >
      <Settings className="size-5" />
    </button>
  );
}

/* ---------------- 行组件（对齐 Vue blocks/*-item.vue） ---------------- */

function Block({ children, title }: { children: ReactNode; title: string }) {
  return (
    <div className="flex flex-col py-4">
      <h3 className="mb-3 leading-none font-semibold tracking-tight">{title}</h3>
      {children}
    </div>
  );
}

function TipIcon({ tip }: { tip?: string }) {
  if (!tip) return null;
  return (
    <Tooltip title={tip}>
      <CircleHelp className="ml-1 size-3 cursor-help" />
    </Tooltip>
  );
}

function SwitchItem({
  checked,
  disabled = false,
  label,
  onChange,
  shortcut,
  tip,
}: {
  checked: boolean;
  disabled?: boolean;
  label: string;
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
        onClick={(_, e) => e.stopPropagation()}
        size="small"
      />
    </div>
  );
}

function ToggleItem({
  disabled = false,
  items,
  label,
  onChange,
  tip,
  value,
}: {
  disabled?: boolean;
  items: { label: string; value: string }[];
  label: string;
  onChange: (value: string) => void;
  tip?: string;
  value: string;
}) {
  return (
    <div
      className={`flex w-full items-center justify-between rounded-md p-2 hover:bg-accent ${
        disabled ? 'pointer-events-none opacity-50' : ''
      }`}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
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

/** 多选按钮组（对应 Vue checkbox-item.vue 的 VbenCheckButtonGroup） */
function CheckboxItem({
  disabled = false,
  items,
  label,
  onChange,
  values,
}: {
  disabled?: boolean;
  items: { label: string; value: string }[];
  label: string;
  onChange: (values: string[]) => void;
  values: string[];
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-1 hover:bg-accent ${
        disabled ? 'pointer-events-none opacity-50' : ''
      }`}
    >
      <span className="text-sm">{label}</span>
      <ToggleGroup
        class="gap-2"
        modelValue={values}
        size="sm"
        type="multiple"
        onUpdate:modelValue={(next) => onChange(next as string[])}
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

function NumberItem({
  disabled = false,
  label,
  max,
  min,
  onChange,
  step = 1,
  tip,
  value,
}: {
  disabled?: boolean;
  label: string;
  max: number;
  min: number;
  onChange: (value: number) => void;
  step?: number;
  tip?: string;
  value: number;
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-2.5 ${
        disabled ? 'pointer-events-none opacity-50' : ''
      }`}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
      <InputNumber
        disabled={disabled}
        max={max}
        min={min}
        onChange={(next) => next !== null && onChange(next)}
        size="small"
        step={step}
        style={{ width: 90 }}
        value={value}
      />
    </div>
  );
}

function SelectItem({
  disabled = false,
  label,
  onChange,
  options,
  tip,
  value,
}: {
  disabled?: boolean;
  label: string;
  onChange: (value: string) => void;
  options: { label: string; value: string }[];
  tip?: string;
  value: string;
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-2.5 ${
        disabled ? 'pointer-events-none opacity-50' : ''
      }`}
    >
      <span className="flex items-center text-sm">
        {label}
        <TipIcon tip={tip} />
      </span>
      <Select
        disabled={disabled}
        onChange={onChange}
        options={options}
        size="small"
        style={{ width: 150 }}
        value={value}
      />
    </div>
  );
}

function InputItem({
  disabled = false,
  label,
  onChange,
  value,
}: {
  disabled?: boolean;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <div
      className={`my-1 flex w-full items-center justify-between rounded-md px-2 py-2.5 ${
        disabled ? 'pointer-events-none opacity-50' : ''
      }`}
    >
      <span className="text-sm">{label}</span>
      <Input
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        size="small"
        style={{ width: 150 }}
        value={value}
      />
    </div>
  );
}

/* ---------------- 预览卡（布局 / 内容 / 动画，对应 Vue outline-box 卡片） ---------------- */

const P = 'var(--primary)';
const G = 'hsl(var(--accent))';
const D = 'hsl(var(--border))';

function Preview({ children }: { children: ReactNode }) {
  return (
    <svg className="h-14 w-full" viewBox="0 0 100 70">
      <rect fill={G} height="70" rx="4" width="100" />
      {children}
    </svg>
  );
}

const LAYOUT_PREVIEWS: Record<LayoutType, ReactNode> = {
  'full-content': <Preview>{null}</Preview>,
  'header-mixed-nav': (
    <Preview>
      <rect fill={P} height="14" width="46" />
      <rect fill={D} height="14" width="54" x="46" />
    </Preview>
  ),
  'header-nav': (
    <Preview>
      <rect fill={P} height="14" width="100" />
    </Preview>
  ),
  'header-sidebar-nav': (
    <Preview>
      <rect fill={P} height="14" width="100" />
      <rect fill={D} height="56" width="24" y="14" />
    </Preview>
  ),
  'mixed-nav': (
    <Preview>
      <rect fill={P} height="14" width="100" />
      <rect fill={D} height="56" width="16" y="14" />
      <rect fill={D} height="56" width="12" x="16" y="14" />
    </Preview>
  ),
  'sidebar-mixed-nav': (
    <Preview>
      <rect fill={D} height="70" width="12" />
      <rect fill={P} height="70" width="14" x="12" />
    </Preview>
  ),
  'sidebar-nav': (
    <Preview>
      <rect fill={P} height="70" width="24" />
    </Preview>
  ),
};

function OutlineCard({
  active,
  children,
  label,
  onClick,
  tip,
}: {
  active: boolean;
  children: ReactNode;
  label: string;
  onClick: () => void;
  tip?: string;
}) {
  return (
    <div className="flex w-25 cursor-pointer flex-col" onClick={onClick}>
      <div className={`outline-box flex-center ${active ? 'outline-box-active' : ''}`}>
        {children}
      </div>
      <div className="text-muted-foreground hover:text-foreground mt-2 flex-center text-center text-xs">
        {label}
        <TipIcon tip={tip} />
      </div>
    </div>
  );
}

function LayoutPreset({ onChange, value }: { onChange: (v: LayoutType) => void; value: LayoutType }) {
  const presets: { name: string; tip: string; type: LayoutType }[] = [
    { name: $t('preferences.vertical'), tip: $t('preferences.verticalTip'), type: 'sidebar-nav' },
    { name: $t('preferences.twoColumn'), tip: $t('preferences.twoColumnTip'), type: 'sidebar-mixed-nav' },
    { name: $t('preferences.horizontal'), tip: $t('preferences.horizontalTip'), type: 'header-nav' },
    { name: $t('preferences.headerSidebarNav'), tip: $t('preferences.headerSidebarNavTip'), type: 'header-sidebar-nav' },
    { name: $t('preferences.mixedMenu'), tip: $t('preferences.mixedMenuTip'), type: 'mixed-nav' },
    { name: $t('preferences.headerTwoColumn'), tip: $t('preferences.headerTwoColumnTip'), type: 'header-mixed-nav' },
    { name: $t('preferences.fullContent'), tip: $t('preferences.fullContentTip'), type: 'full-content' },
  ];
  return (
    <div className="flex w-full flex-wrap gap-5">
      {presets.map((p) => (
        <OutlineCard active={value === p.type} key={p.type} label={p.name} onClick={() => onChange(p.type)} tip={p.tip}>
          {LAYOUT_PREVIEWS[p.type]}
        </OutlineCard>
      ))}
    </div>
  );
}

/* ---------------- 主题模式卡片 / 内置主题网格 ---------------- */

function ModeCards({ onChange, value }: { onChange: (v: string) => void; value: string }) {
  const presets = [
    { icon: Sun, label: $t('preferences.theme.light'), name: 'light' },
    { icon: MoonStar, label: $t('preferences.theme.dark'), name: 'dark' },
    { icon: SunMoon, label: $t('preferences.followSystem'), name: 'auto' },
  ];
  return (
    <div className="flex w-full flex-wrap justify-between">
      {presets.map((p) => (
        <div className="flex cursor-pointer flex-col" key={p.name} onClick={() => onChange(p.name)}>
          <div className={`outline-box flex-center py-4 ${value === p.name ? 'outline-box-active' : ''}`}>
            <p.icon className="mx-9 size-5" />
          </div>
          <div className="text-muted-foreground mt-2 text-center text-xs">{p.label}</div>
        </div>
      ))}
    </div>
  );
}

/** 取内置主题在指定明暗下的主色（Vue builtin.vue 的 watch 逻辑） */
function presetPrimary(type: string, isDark: boolean): string {
  const preset = BUILT_IN_THEME_PRESETS.find((item) => item.type === type);
  if (!preset) return '';
  const color = isDark ? preset.darkPrimaryColor || preset.primaryColor : preset.primaryColor;
  return color || preset.color;
}

/** 内置主题名：preset.type 为连字符（deep-blue），locale 键为驼峰（deepBlue），需转换 */
function builtinLabel(type: string): string {
  const key = type.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
  return $t(`preferences.theme.builtin.${key}`);
}

function BuiltinGrid({
  customColor,
  isDark,
  onCustomColor,
  onSelect,
  value,
}: {
  customColor: string;
  isDark: boolean;
  onCustomColor: (hsl: string) => void;
  onSelect: (type: string) => void;
  value: string;
}) {
  const colorInputRef = useRef<HTMLInputElement>(null);
  return (
    <div className="flex w-full flex-wrap justify-between">
      {BUILT_IN_THEME_PRESETS.filter((p) => p.type !== 'custom').map((preset) => (
        <div className="flex cursor-pointer flex-col" key={preset.type} onClick={() => onSelect(preset.type)}>
          <div className={`outline-box flex-center cursor-pointer ${value === preset.type ? 'outline-box-active' : ''}`}>
            <div className="mx-9 my-2 size-5 rounded-md" style={{ backgroundColor: preset.color }} />
          </div>
          <div className="text-muted-foreground my-2 text-center text-xs">
            {builtinLabel(preset.type)}
          </div>
        </div>
      ))}
      <div className="flex cursor-pointer flex-col" onClick={() => colorInputRef.current?.click()}>
        <div className={`outline-box flex-center cursor-pointer ${value === 'custom' ? 'outline-box-active' : ''}`}>
          <div className="relative flex-center size-5 rounded-sm">
            <UserRoundPen className="absolute z-1 size-5 opacity-60" />
            <input
              className="absolute inset-0 opacity-0"
              onChange={(e) => onCustomColor(convertToHsl(e.target.value))}
              onClick={(e) => e.stopPropagation()}
              ref={colorInputRef}
              type="color"
              value={new TinyColor(customColor || '#000000').toHexString()}
            />
          </div>
        </div>
        <div className="text-muted-foreground my-2 text-center text-xs">
          {$t('preferences.theme.builtin.custom')}
        </div>
      </div>
    </div>
  );
}

/* ---------------- 抽屉本体 ---------------- */

type TabKey = 'appearance' | 'general' | 'layout' | 'shortcutKey';

const IS_WINDOWS = navigator.platform.toLowerCase().includes('win');

export function PreferencesDrawer() {
  const { diffCustomPreference, diffPreference, isDark, preferences } = usePreferences();
  const logout = useAuthStore((state) => state.logout);
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<TabKey>('appearance');
  const [sticky, setSticky] = useState(preferences.app.enableStickyPreferencesNavigationBar);

  useEffect(() => {
    openPreferencesFn = () => setOpen(true);
    return () => {
      openPreferencesFn = null;
    };
  }, []);

  const {
    app, breadcrumb, copyright, footer, header, navigation,
    shortcutKeys, sidebar, tabbar, theme, transition, widget,
  } = preferences;

  // Vue theme.vue 的 watch：semiDarkSidebar 关闭时联动关闭 sub
  useEffect(() => {
    if (!theme.semiDarkSidebar && theme.semiDarkSidebarSub) {
      updatePreferences({ theme: { semiDarkSidebarSub: false } });
    }
  }, [theme.semiDarkSidebar, theme.semiDarkSidebarSub]);

  // Vue builtin.vue 的 watch：builtinType / isDark 变化时重算主色（跳过首帧，避免覆盖持久化值）
  const skipThemeWatch = useRef(true);
  useEffect(() => {
    if (skipThemeWatch.current) {
      skipThemeWatch.current = false;
      return;
    }
    if (theme.builtinType !== 'custom') {
      updatePreferences({ theme: { colorPrimary: presetPrimary(theme.builtinType, isDark) } });
    }
  }, [theme.builtinType, isDark]);

  // 与 Vue mergedDiffPreference 一致：与默认值的 diff（而非会话快照）
  const mergedDiff = useMemo(() => {
    const result: Record<string, unknown> = {};
    if (diffPreference) Object.assign(result, diffPreference);
    if (diffCustomPreference) result.custom = diffCustomPreference;
    return Object.keys(result).length > 0 ? result : undefined;
  }, [diffPreference, diffCustomPreference]);
  const hasDiff = Boolean(mergedDiff);

  const pendingTip = $t('preferences.pendingTip');

  async function handleCopy() {
    await navigator.clipboard.writeText(JSON.stringify(mergedDiff, null, 2));
    getMessageApi()?.notification.success({
      description: $t('preferences.copyPreferencesSuccess'),
      message: $t('preferences.copyPreferencesSuccessTitle'),
    });
  }

  async function handleClearCache() {
    await resetPreferences();
    clearCache();
    await logout();
  }

  async function handleReset() {
    if (!hasDiff) return;
    await resetPreferences();
    await loadLocaleMessages(preferences.app.locale);
  }

  // 侧边栏「显示按钮」checkbox 与两个布尔偏好的双向映射（Vue sidebar.vue onMounted/handleCheckboxChange）
  const sidebarButtons = [
    ...(sidebar.collapsedButton ? ['collapsed'] : []),
    ...(sidebar.fixedButton ? ['fixed'] : []),
  ];
  function handleSidebarButtons(values: string[]) {
    updatePreferences({
      sidebar: {
        collapsedButton: values.includes('collapsed'),
        fixedButton: values.includes('fixed'),
      },
    });
  }

  const sidebarDisabled = !sidebar.enable;
  const breadcrumbDisabled = !breadcrumb.enable;
  const tabbarDisabled = !tabbar.enable;
  const copyrightDisabled = !footer.enable || !copyright.enable;

  return (
    <Drawer
      extra={
        <div className="flex items-center">
          <Tooltip title={$t('preferences.resetTip')}>
            <VbenIconButton class="relative" disabled={!hasDiff} onClick={() => void handleReset()}>
              {hasDiff ? <span className="bg-primary absolute top-0.5 right-0.5 size-2 rounded-sm" /> : null}
              <RotateCw className="size-4" />
            </VbenIconButton>
          </Tooltip>
          <Tooltip
            title={
              sticky
                ? $t('preferences.disableStickyPreferencesNavigationBar')
                : $t('preferences.enableStickyPreferencesNavigationBar')
            }
          >
            <VbenIconButton
              class="relative"
              onClick={() => {
                const next = !sticky;
                setSticky(next);
                updatePreferences({ app: { enableStickyPreferencesNavigationBar: next } });
              }}
            >
              {sticky ? <PinOff className="size-4" /> : <Pin className="size-4" />}
            </VbenIconButton>
          </Tooltip>
          <VbenIconButton onClick={() => setOpen(false)}>
            <X className="size-4" />
          </VbenIconButton>
        </div>
      }
      footer={
        <div className="flex gap-2 px-4">
          {app.enableCopyPreferences ? (
            <Button block disabled={!hasDiff} icon={<Copy className="size-3" />} onClick={() => void handleCopy()} size="small">
              {$t('preferences.copyPreferences')}
            </Button>
          ) : null}
          <Button block disabled={!hasDiff} onClick={() => void handleClearCache()} size="small" type="text">
            {$t('preferences.clearAndLogout')}
          </Button>
        </div>
      }
      closable={false}
      onClose={() => setOpen(false)}
      open={open}
      styles={{ body: { padding: '0 16px' } }}
      title={
        <div className="flex items-baseline gap-2">
          <span>{$t('preferences.title')}</span>
          <span className="text-muted-foreground text-xs font-normal">{$t('preferences.subtitle')}</span>
        </div>
      }
      size={384}
    >
      <div className={sticky ? 'bg-background sticky -top-3 z-50 py-2' : 'py-2'}>
        <Segmented
          block
          className="vben-pref-segmented"
          onChange={(value) => setTab(value as TabKey)}
          options={[
            { label: $t('preferences.appearance'), value: 'appearance' },
            { label: $t('preferences.layout'), value: 'layout' },
            { label: $t('preferences.shortcutKeys.title'), value: 'shortcutKey' },
            { label: $t('preferences.general'), value: 'general' },
          ]}
          value={tab}
        />
      </div>

      {tab === 'appearance' ? (
        <div>
          <Block title={$t('preferences.theme.title')}>
            <ModeCards onChange={(v) => updatePreferences({ theme: { mode: v as any } })} value={theme.mode} />
            <div className="mt-6">
              <SwitchItem
                checked={theme.semiDarkSidebar}
                disabled={theme.mode === 'dark' || app.layout === 'header-nav' || app.layout === 'full-content'}
                label={$t('preferences.theme.darkSidebar')}
                onChange={(c) => updatePreferences({ theme: { semiDarkSidebar: c } })}
                tip={$t('preferences.theme.darkSidebarTip')}
              />
            </div>
            <SwitchItem
              checked={theme.semiDarkSidebarSub}
              disabled={
                theme.mode === 'dark' ||
                (app.layout !== 'header-mixed-nav' && app.layout !== 'sidebar-mixed-nav') ||
                !theme.semiDarkSidebar
              }
              label={$t('preferences.theme.darkSidebarSub')}
              onChange={(c) => updatePreferences({ theme: { semiDarkSidebarSub: c } })}
              tip={$t('preferences.theme.darkSidebarSubTip')}
            />
            <SwitchItem
              checked={theme.semiDarkHeader}
              disabled={theme.mode === 'dark'}
              label={$t('preferences.theme.darkHeader')}
              onChange={(c) => updatePreferences({ theme: { semiDarkHeader: c } })}
            />
          </Block>

          <Block title={$t('preferences.theme.builtin.title')}>
            <BuiltinGrid
              customColor={theme.colorPrimary}
              isDark={isDark}
              onCustomColor={(hsl) => updatePreferences({ theme: { builtinType: 'custom', colorPrimary: hsl } })}
              onSelect={(type) =>
                updatePreferences({ theme: { builtinType: type as any, colorPrimary: presetPrimary(type, isDark) } })
              }
              value={theme.builtinType}
            />
          </Block>

          <Block title={$t('preferences.theme.radius')}>
            <ToggleGroup
              class="gap-2"
              modelValue={theme.radius}
              size="sm"
              type="single"
              onUpdate:modelValue={(n) => updatePreferences({ theme: { radius: n as string } })}
              variant="outline"
            >
              {['0', '0.25', '0.5', '0.75', '1'].map((v) => (
                <ToggleGroupItem class="h-7 w-16 rounded-sm data-[state=on]:bg-primary data-[state=on]:text-primary-foreground" key={v} value={v}>
                  {v}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </Block>

          <Block title={$t('preferences.theme.buttonWaveMode')}>
            <ToggleGroup
              class="gap-2"
              modelValue={theme.buttonWaveMode}
              size="sm"
              type="single"
              onUpdate:modelValue={(n) => updatePreferences({ theme: { buttonWaveMode: n as any } })}
              variant="outline"
            >
              {(['Default', 'Disabled', 'Happy', 'Inset', 'Shake'] as const).map((v) => (
                <ToggleGroupItem
                  class="h-7 rounded-sm px-2 data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
                  disabled={v !== 'Default' && v !== 'Disabled'}
                  key={v}
                  value={v}
                >
                  {v}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </Block>

          <Block title={$t('preferences.theme.fontSize')}>
            <div className="flex w-full flex-col gap-4">
              <div className="flex items-center gap-2">
                <InputNumber
                  max={22}
                  min={15}
                  onChange={(n) => n !== null && updatePreferences({ theme: { fontSize: n } })}
                  style={{ width: '100%' }}
                  value={theme.fontSize}
                />
                <span className="text-muted-foreground text-xs whitespace-nowrap">px</span>
              </div>
              <div className="text-muted-foreground text-xs">{$t('preferences.theme.fontSizeTip')}</div>
            </div>
          </Block>

          <Block title={$t('preferences.other')}>
            <SwitchItem checked={app.colorWeakMode} label={$t('preferences.theme.weakMode')} onChange={(c) => updatePreferences({ app: { colorWeakMode: c } })} />
            <SwitchItem checked={app.colorGrayMode} label={$t('preferences.theme.grayMode')} onChange={(c) => updatePreferences({ app: { colorGrayMode: c } })} />
          </Block>
        </div>
      ) : null}

      {tab === 'layout' ? (
        <div>
          <Block title={$t('preferences.layout')}>
            <LayoutPreset onChange={(v) => updatePreferences({ app: { layout: v } })} value={app.layout} />
          </Block>

          <Block title={$t('preferences.content')}>
            <div className="flex w-full gap-5">
              <OutlineCard active={app.contentCompact === 'wide'} label={$t('preferences.wide')} onClick={() => updatePreferences({ app: { contentCompact: 'wide' } })}>
                <Preview><rect fill={D} height="50" width="84" x="8" y="10" /></Preview>
              </OutlineCard>
              <OutlineCard active={app.contentCompact === 'compact'} label={$t('preferences.compact')} onClick={() => updatePreferences({ app: { contentCompact: 'compact' } })}>
                <Preview><rect fill={D} height="50" width="52" x="24" y="10" /></Preview>
              </OutlineCard>
            </div>
          </Block>

          <Block title={$t('preferences.sidebar.title')}>
            <SwitchItem checked={sidebar.enable} label={$t('preferences.sidebar.visible')} onChange={(c) => updatePreferences({ sidebar: { enable: c } })} />
            <SwitchItem checked={sidebar.draggable} disabled={sidebarDisabled} label={$t('preferences.sidebar.draggable')} onChange={(c) => updatePreferences({ sidebar: { draggable: c } })} />
            <SwitchItem checked={sidebar.collapsed} disabled={sidebarDisabled} label={$t('preferences.sidebar.collapsed')} onChange={(c) => updatePreferences({ sidebar: { collapsed: c } })} />
            <SwitchItem checked={sidebar.expandOnHover} disabled={sidebarDisabled || !sidebar.collapsed} label={$t('preferences.sidebar.expandOnHover')} onChange={(c) => updatePreferences({ sidebar: { expandOnHover: c } })} tip={$t('preferences.sidebar.expandOnHoverTip')} />
            <SwitchItem checked={sidebar.collapsedShowTitle} disabled={sidebarDisabled || !sidebar.collapsed} label={$t('preferences.sidebar.collapsedShowTitle')} onChange={(c) => updatePreferences({ sidebar: { collapsedShowTitle: c } })} />
            <SwitchItem checked={sidebar.autoActivateChild} disabled label={$t('preferences.sidebar.autoActivateChild')} onChange={(c) => updatePreferences({ sidebar: { autoActivateChild: c } })} tip={pendingTip} />
            <CheckboxItem
              disabled={sidebarDisabled}
              items={[
                { label: $t('preferences.sidebar.buttonCollapsed'), value: 'collapsed' },
                { label: $t('preferences.sidebar.buttonFixed'), value: 'fixed' },
              ]}
              label={$t('preferences.sidebar.buttons')}
              onChange={handleSidebarButtons}
              values={sidebarButtons}
            />
            <NumberItem disabled={sidebarDisabled} label={$t('preferences.sidebar.width')} max={320} min={160} onChange={(v) => updatePreferences({ sidebar: { width: v } })} step={10} value={sidebar.width} />
          </Block>

          <Block title={$t('preferences.header.title')}>
            <SwitchItem checked={header.enable} label={$t('preferences.header.visible')} onChange={(c) => updatePreferences({ header: { enable: c } })} />
            <SelectItem
              disabled
              label={$t('preferences.header.mode')}
              onChange={(v) => updatePreferences({ header: { mode: v as any } })}
              options={[
                { label: $t('preferences.header.modeStatic'), value: 'static' },
                { label: $t('preferences.header.modeFixed'), value: 'fixed' },
                { label: $t('preferences.header.modeAuto'), value: 'auto' },
                { label: $t('preferences.header.modeAutoScroll'), value: 'auto-scroll' },
              ]}
              tip={pendingTip}
              value={header.mode}
            />
            <ToggleItem
              disabled
              items={[
                { label: $t('preferences.header.menuAlignStart'), value: 'start' },
                { label: $t('preferences.header.menuAlignCenter'), value: 'center' },
                { label: $t('preferences.header.menuAlignEnd'), value: 'end' },
              ]}
              label={$t('preferences.header.menuAlign')}
              onChange={(v) => updatePreferences({ header: { menuAlign: v as any } })}
              tip={pendingTip}
              value={header.menuAlign}
            />
          </Block>

          <Block title={$t('preferences.navigationMenu.title')}>
            <ToggleItem
              items={[
                { label: $t('preferences.rounded'), value: 'rounded' },
                { label: $t('preferences.plain'), value: 'plain' },
              ]}
              label={$t('preferences.navigationMenu.style')}
              onChange={(v) => updatePreferences({ navigation: { styleType: v as any } })}
              value={navigation.styleType}
            />
            <SwitchItem checked={navigation.split} disabled label={$t('preferences.navigationMenu.split')} onChange={(c) => updatePreferences({ navigation: { split: c } })} tip={pendingTip} />
            <SwitchItem checked={navigation.accordion} label={$t('preferences.navigationMenu.accordion')} onChange={(c) => updatePreferences({ navigation: { accordion: c } })} />
          </Block>

          <Block title={$t('preferences.breadcrumb.title')}>
            <SwitchItem checked={breadcrumb.enable} label={$t('preferences.breadcrumb.enable')} onChange={(c) => updatePreferences({ breadcrumb: { enable: c } })} />
            <SwitchItem checked={breadcrumb.hideOnlyOne} disabled={breadcrumbDisabled} label={$t('preferences.breadcrumb.hideOnlyOne')} onChange={(c) => updatePreferences({ breadcrumb: { hideOnlyOne: c } })} />
            <SwitchItem checked={breadcrumb.showIcon} disabled={breadcrumbDisabled} label={$t('preferences.breadcrumb.icon')} onChange={(c) => updatePreferences({ breadcrumb: { showIcon: c } })} />
            <SwitchItem checked={breadcrumb.showHome} disabled={breadcrumbDisabled || !breadcrumb.showIcon} label={$t('preferences.breadcrumb.home')} onChange={(c) => updatePreferences({ breadcrumb: { showHome: c } })} />
            <ToggleItem
              disabled={breadcrumbDisabled}
              items={[
                { label: $t('preferences.normal'), value: 'normal' },
                { label: $t('preferences.breadcrumb.background'), value: 'background' },
              ]}
              label={$t('preferences.breadcrumb.style')}
              onChange={(v) => updatePreferences({ breadcrumb: { styleType: v as any } })}
              value={breadcrumb.styleType}
            />
          </Block>

          <Block title={$t('preferences.tabbar.title')}>
            <SwitchItem checked={tabbar.enable} label={$t('preferences.tabbar.enable')} onChange={(c) => updatePreferences({ tabbar: { enable: c } })} />
            <SwitchItem checked={tabbar.persist} disabled={tabbarDisabled} label={$t('preferences.tabbar.persist')} onChange={(c) => updatePreferences({ tabbar: { persist: c } })} />
            <SwitchItem checked={tabbar.visitHistory} disabled label={$t('preferences.tabbar.visitHistory')} onChange={(c) => updatePreferences({ tabbar: { visitHistory: c } })} tip={pendingTip} />
            <NumberItem disabled={tabbarDisabled} label={$t('preferences.tabbar.maxCount')} max={30} min={0} onChange={(v) => updatePreferences({ tabbar: { maxCount: v } })} step={5} tip={$t('preferences.tabbar.maxCountTip')} value={tabbar.maxCount} />
            <SwitchItem checked={tabbar.draggable} disabled={tabbarDisabled} label={$t('preferences.tabbar.draggable')} onChange={(c) => updatePreferences({ tabbar: { draggable: c } })} />
            <SwitchItem checked={tabbar.wheelable} disabled={tabbarDisabled} label={$t('preferences.tabbar.wheelable')} onChange={(c) => updatePreferences({ tabbar: { wheelable: c } })} tip={$t('preferences.tabbar.wheelableTip')} />
            <SwitchItem checked={tabbar.middleClickToClose} disabled={tabbarDisabled} label={$t('preferences.tabbar.middleClickClose')} onChange={(c) => updatePreferences({ tabbar: { middleClickToClose: c } })} />
            <SwitchItem checked={tabbar.showIcon} disabled={tabbarDisabled} label={$t('preferences.tabbar.icon')} onChange={(c) => updatePreferences({ tabbar: { showIcon: c } })} />
            <SwitchItem checked={tabbar.showMore} disabled={tabbarDisabled} label={$t('preferences.tabbar.showMore')} onChange={(c) => updatePreferences({ tabbar: { showMore: c } })} />
            <SwitchItem checked={tabbar.showMaximize} disabled={tabbarDisabled} label={$t('preferences.tabbar.showMaximize')} onChange={(c) => updatePreferences({ tabbar: { showMaximize: c } })} />
            <SwitchItem checked={tabbar.showRefresh} disabled={tabbarDisabled} label={$t('preferences.widget.refresh')} onChange={(c) => updatePreferences({ tabbar: { showRefresh: c } })} />
            <SelectItem
              disabled={tabbarDisabled}
              label={$t('preferences.tabbar.styleType.title')}
              onChange={(v) => updatePreferences({ tabbar: { styleType: v as any } })}
              options={['chrome', 'card', 'plain', 'brisk'].map((v) => ({ label: $t(`preferences.tabbar.styleType.${v}`), value: v }))}
              value={tabbar.styleType}
            />
          </Block>

          <Block title={$t('preferences.widget.title')}>
            <SwitchItem checked={widget.globalSearch} disabled label={$t('preferences.widget.globalSearch')} onChange={(c) => updatePreferences({ widget: { globalSearch: c } })} tip={pendingTip} />
            <SwitchItem checked={widget.themeToggle} label={$t('preferences.widget.themeToggle')} onChange={(c) => updatePreferences({ widget: { themeToggle: c } })} />
            <SwitchItem checked={widget.languageToggle} label={$t('preferences.widget.languageToggle')} onChange={(c) => updatePreferences({ widget: { languageToggle: c } })} />
            <SwitchItem checked={widget.fullscreen} label={$t('preferences.widget.fullscreen')} onChange={(c) => updatePreferences({ widget: { fullscreen: c } })} />
            <SwitchItem checked={widget.notification} disabled label={$t('preferences.widget.notification')} onChange={(c) => updatePreferences({ widget: { notification: c } })} tip={pendingTip} />
            <SwitchItem checked={widget.lockScreen} disabled label={$t('preferences.widget.lockScreen')} onChange={(c) => updatePreferences({ widget: { lockScreen: c } })} tip={pendingTip} />
            <SwitchItem checked={widget.sidebarToggle} label={$t('preferences.widget.sidebarToggle')} onChange={(c) => updatePreferences({ widget: { sidebarToggle: c } })} />
            <SwitchItem checked={widget.refresh} label={$t('preferences.widget.refresh')} onChange={(c) => updatePreferences({ widget: { refresh: c } })} />
            <SwitchItem checked={widget.timezone} disabled label={$t('preferences.widget.timezone')} onChange={(c) => updatePreferences({ widget: { timezone: c } })} tip={pendingTip} />
            <SelectItem
              label={$t('preferences.position.title')}
              onChange={(v) => updatePreferences({ app: { preferencesButtonPosition: v as any } })}
              options={[
                { label: $t('preferences.position.auto'), value: 'auto' },
                { label: $t('preferences.position.header'), value: 'header' },
                { label: $t('preferences.position.fixed'), value: 'fixed' },
                { label: $t('preferences.position.userDropdown'), value: 'user-dropdown' },
              ]}
              value={app.preferencesButtonPosition}
            />
          </Block>

          <Block title={$t('preferences.footer.title')}>
            <SwitchItem checked={footer.enable} label={$t('preferences.footer.visible')} onChange={(c) => updatePreferences({ footer: { enable: c } })} />
            <SwitchItem checked={footer.fixed} disabled={!footer.enable} label={$t('preferences.footer.fixed')} onChange={(c) => updatePreferences({ footer: { fixed: c } })} />
          </Block>

          {copyright.settingShow ? (
            <Block title={$t('preferences.copyright.title')}>
              <SwitchItem checked={copyright.enable} disabled={!footer.enable} label={$t('preferences.copyright.enable')} onChange={(c) => updatePreferences({ copyright: { enable: c } })} />
              <InputItem disabled={copyrightDisabled} label={$t('preferences.copyright.companyName')} onChange={(v) => updatePreferences({ copyright: { companyName: v } })} value={copyright.companyName} />
              <InputItem disabled={copyrightDisabled} label={$t('preferences.copyright.companySiteLink')} onChange={(v) => updatePreferences({ copyright: { companySiteLink: v } })} value={copyright.companySiteLink} />
              <InputItem disabled={copyrightDisabled} label={$t('preferences.copyright.date')} onChange={(v) => updatePreferences({ copyright: { date: v } })} value={copyright.date} />
              <InputItem disabled={copyrightDisabled} label={$t('preferences.copyright.icp')} onChange={(v) => updatePreferences({ copyright: { icp: v } })} value={copyright.icp} />
              <InputItem disabled={copyrightDisabled} label={$t('preferences.copyright.icpLink')} onChange={(v) => updatePreferences({ copyright: { icpLink: v } })} value={copyright.icpLink} />
            </Block>
          ) : null}
        </div>
      ) : null}

      {tab === 'shortcutKey' ? (
        <Block title={$t('preferences.shortcutKeys.global')}>
          <SwitchItem checked={shortcutKeys.enable} label={$t('preferences.shortcutKeys.title')} onChange={(c) => updatePreferences({ shortcutKeys: { enable: c } })} />
          <SwitchItem checked={shortcutKeys.globalSearch} disabled label={$t('preferences.shortcutKeys.search')} onChange={(c) => updatePreferences({ shortcutKeys: { globalSearch: c } })} shortcut={<>{IS_WINDOWS ? 'Ctrl' : '⌘'} <kbd>K</kbd></>} tip={pendingTip} />
          <SwitchItem checked={shortcutKeys.globalLogout} disabled={!shortcutKeys.enable} label={$t('preferences.shortcutKeys.logout')} onChange={(c) => updatePreferences({ shortcutKeys: { globalLogout: c } })} shortcut={<>{IS_WINDOWS ? 'Alt' : '⌥'} Q</>} />
          <SwitchItem checked={shortcutKeys.globalLockScreen} disabled label={$t('ui.widgets.lockScreen.title')} onChange={(c) => updatePreferences({ shortcutKeys: { globalLockScreen: c } })} shortcut={<>{IS_WINDOWS ? 'Alt' : '⌥'} L</>} tip={pendingTip} />
          <SwitchItem checked={shortcutKeys.globalEscape} disabled={!shortcutKeys.enable} label={$t('preferences.shortcutKeys.escape')} onChange={(c) => updatePreferences({ shortcutKeys: { globalEscape: c } })} shortcut={<>Esc</>} />
        </Block>
      ) : null}

      {tab === 'general' ? (
        <div>
          <Block title={$t('preferences.general')}>
            <SelectItem
              label={$t('preferences.language')}
              onChange={async (value) => {
                const locale = value as SupportedLanguagesType;
                updatePreferences({ app: { locale } });
                await loadLocaleMessages(locale);
              }}
              options={SUPPORT_LANGUAGES.map((i) => ({ label: i.label, value: i.value }))}
              value={app.locale}
            />
            <SelectItem
              disabled
              label={$t('preferences.timezone')}
              onChange={(v) => updatePreferences({ app: { timezone: v } })}
              options={DEFAULT_TIME_ZONE_OPTIONS.map((i) => ({ label: i.label, value: i.timezone }))}
              tip={pendingTip}
              value={app.timezone}
            />
            <SwitchItem checked={app.dynamicTitle} label={$t('preferences.dynamicTitle')} onChange={(c) => updatePreferences({ app: { dynamicTitle: c } })} />
            <SwitchItem
              checked={app.watermark}
              label={$t('preferences.watermark')}
              onChange={(c) => {
                updatePreferences({ app: { watermark: c } });
                if (!c) updatePreferences({ app: { watermarkContent: '' } });
              }}
            />
            {app.watermark ? (
              <InputItem label={$t('preferences.watermarkContent')} onChange={(v) => updatePreferences({ app: { watermarkContent: v } })} value={app.watermarkContent} />
            ) : null}
            <SwitchItem checked={app.enableCheckUpdates} disabled label={$t('preferences.checkUpdates')} onChange={(c) => updatePreferences({ app: { enableCheckUpdates: c } })} tip={pendingTip} />
            <SwitchItem checked={app.enableCopyPreferences} label={$t('preferences.enableCopyPreferences')} onChange={(c) => updatePreferences({ app: { enableCopyPreferences: c } })} />
          </Block>

          <Block title={$t('preferences.animation.title')}>
            <SwitchItem checked={transition.progress} label={$t('preferences.animation.progress')} onChange={(c) => updatePreferences({ transition: { progress: c } })} />
            <SwitchItem checked={transition.loading} label={$t('preferences.animation.loading')} onChange={(c) => updatePreferences({ transition: { loading: c } })} />
            <SwitchItem checked={transition.enable} label={$t('preferences.animation.transition')} onChange={(c) => updatePreferences({ transition: { enable: c } })} />
            {transition.enable ? (
              <div className="mt-3 mb-2 flex justify-between gap-3 px-2">
                {(['fade', 'fade-slide', 'fade-up', 'fade-down'] as const).map((name) => (
                  <div
                    className={`outline-box p-2 ${transition.name === name ? 'outline-box-active' : ''}`}
                    key={name}
                    onClick={() => updatePreferences({ transition: { name } })}
                  >
                    <div className={`${name}-slow h-10 w-12 rounded-md bg-primary`} />
                  </div>
                ))}
              </div>
            ) : null}
          </Block>
        </div>
      ) : null}
    </Drawer>
  );
}
