import type { ReactNode } from 'react';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { LayoutType } from '@/core/typings';
import type { SupportedLanguagesType } from '@/core/locales';

import {
  BUILT_IN_THEME_PRESETS,
  clearCache,
  getPreferences,
  resetPreferences,
  updateCustomPreferences,
  updatePreferences,
  usePreferences,
} from '@/core/preferences';
import { SUPPORT_LANGUAGES } from '@/core/constants';
import { $t, loadLocaleMessages } from '@/core/locales';
import { useTimezoneStore } from '@/core/stores';
import { convertToHsl, TinyColor } from '@/core/utils';
import {
  NumberField,
  NumberFieldContent,
  NumberFieldDecrement,
  NumberFieldIncrement,
  NumberFieldInput,
  ToggleGroup,
  ToggleGroupItem,
  WalnutButton,
  WalnutIconButton,
  WalnutSegmented,
} from '@/core/shadcn-ui';
import {
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
import { Drawer } from 'antd';

import { useAuthStore } from '@/store/auth';
import { getMessageApi } from '@/services/message';

import {
  Block,
  CheckboxItem,
  CustomFields,
  InputItem,
  NumberItem,
  SelectItem,
  SwitchItem,
  TipIcon,
  ToggleItem,
} from './preferences-blocks';
import {
  ContentCompact,
  ContentWide,
  FullContent,
  HeaderMixedNav,
  HeaderNav,
  HeaderSidebarNav,
  MixedNav,
  SidebarMixedNav,
  SidebarNav,
} from './preferences-icons';

/**
 * 系统设置（偏好设置抽屉）—— 1:1 复刻 Vue 侧
 * `widgets/preferences/preferences-drawer.vue` + 全部 blocks。
 *
 * 结构：Drawer(384px) + title/subtitle + extra(重置/钉住/关闭) +
 * WalnutSegmented 四页签 外观→布局→快捷键→通用（扩展字段存在时追加自定义页签）
 * + footer(复制偏好/清空缓存退出)。
 *
 * 与 Vue 的对齐原则：
 * - 控件全部使用同源 shadcn-ui 原语（见 preferences-blocks.tsx），像素一致。
 * - 禁用逻辑逐条对齐 Vue 各 block（布局模式 / 父开关 / 明暗模式）。
 * - 「有改动」判定 / 复制内容均使用与默认值的 diff（mergedDiffPreference）。
 */

let openPreferencesFn: null | (() => void) = null;

/** 命令式打开偏好设置抽屉（顶栏齿轮 / 侧边栏固定按钮 / 悬浮按钮共用） */
export function openPreferences() {
  openPreferencesFn?.();
}

export function PreferencesButton() {
  return (
    <WalnutIconButton onClick={openPreferences}>
      <Settings className="size-4" />
    </WalnutIconButton>
  );
}

/** 右侧边缘悬浮的设置入口（preferencesButtonPosition = fixed 时显示，对应 Vue preferences.vue 默认插槽按钮） */
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

/* ---------------- 工具 ---------------- */

/**
 * 等价 @vueuse/core useThrottleFn(fn, 300, true, true)：
 * leading + trailing 节流，用于自定义主题取色器的连续 input。
 */
function useThrottledCallback<A extends unknown[]>(
  fn: (...args: A) => void,
  delay = 300,
) {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const lastRef = useRef(0);
  const timerRef = useRef<null | ReturnType<typeof setTimeout>>(null);
  const pendingRef = useRef<A | null>(null);

  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    },
    [],
  );

  return useCallback(
    (...args: A) => {
      const now = Date.now();
      const remaining = delay - (now - lastRef.current);
      pendingRef.current = args;
      if (remaining <= 0) {
        if (timerRef.current) {
          clearTimeout(timerRef.current);
          timerRef.current = null;
        }
        lastRef.current = now;
        pendingRef.current = null;
        fnRef.current(...args);
      } else if (!timerRef.current) {
        timerRef.current = setTimeout(() => {
          lastRef.current = Date.now();
          timerRef.current = null;
          if (pendingRef.current) fnRef.current(...pendingRef.current);
          pendingRef.current = null;
        }, remaining);
      }
    },
    [delay],
  );
}

/** 内置主题名：preset.type 为连字符（deep-blue），locale 键为驼峰（deepBlue） */
function builtinLabel(type: string): string {
  const key = type.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
  return $t(`preferences.theme.builtin.${key}`);
}

/* ---------------- 预览卡（对应 Vue outline-box 卡片 + 真实 SVG 示意图） ---------------- */

function OutlineCard({
  active,
  children,
  label,
  labelHover = false,
  onClick,
  tip,
}: {
  active: boolean;
  children: ReactNode;
  label: string;
  labelHover?: boolean;
  onClick: () => void;
  tip?: string;
}) {
  return (
    <div className="flex w-25 cursor-pointer flex-col" onClick={onClick}>
      <div className={`outline-box flex-center ${active ? 'outline-box-active' : ''}`}>
        {children}
      </div>
      <div
        className={`text-muted-foreground mt-2 text-center text-xs ${
          labelHover ? 'flex-center hover:text-foreground' : ''
        }`}
      >
        {label}
        <TipIcon tip={tip} />
      </div>
    </div>
  );
}

const LAYOUT_ICONS: Record<LayoutType, () => ReactNode> = {
  'full-content': FullContent,
  'header-mixed-nav': HeaderMixedNav,
  'header-nav': HeaderNav,
  'header-sidebar-nav': HeaderSidebarNav,
  'mixed-nav': MixedNav,
  'sidebar-mixed-nav': SidebarMixedNav,
  'sidebar-nav': SidebarNav,
};

/** 对应 blocks/layout/layout.vue：7 种布局预设 + tip */
function LayoutPreset() {
  const { preferences } = usePreferences();
  const value = preferences.app.layout;
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
      {presets.map((preset) => {
        const Icon = LAYOUT_ICONS[preset.type];
        return (
          <OutlineCard
            active={value === preset.type}
            key={preset.type}
            label={preset.name}
            labelHover
            onClick={() => updatePreferences({ app: { layout: preset.type } })}
            tip={preset.tip}
          >
            <Icon />
          </OutlineCard>
        );
      })}
    </div>
  );
}

/** 对应 blocks/layout/content.vue：宽 / 紧凑（ContentWide 即 HeaderNav 示意图） */
function ContentPreset() {
  const { preferences } = usePreferences();
  const value = preferences.app.contentCompact;
  const presets = [
    { icon: ContentWide, name: $t('preferences.wide'), type: 'wide' as const },
    { icon: ContentCompact, name: $t('preferences.compact'), type: 'compact' as const },
  ];
  return (
    <div className="flex w-full gap-5">
      {presets.map((preset) => (
        <OutlineCard
          active={value === preset.type}
          key={preset.type}
          label={preset.name}
          onClick={() => updatePreferences({ app: { contentCompact: preset.type } })}
        >
          <preset.icon />
        </OutlineCard>
      ))}
    </div>
  );
}

/* ---------------- 外观页签 ---------------- */

/** 对应 blocks/theme/theme.vue：明暗卡片 + 3 个半暗开关（含 watch 联动） */
function ThemePanel() {
  const { layout, preferences } = usePreferences();
  const { theme } = preferences;

  // Vue watch(themeSemiDarkSidebar)：关闭时联动关闭 sub
  const prevSemiDarkSidebar = useRef(theme.semiDarkSidebar);
  useEffect(() => {
    if (prevSemiDarkSidebar.current && !theme.semiDarkSidebar) {
      updatePreferences({ theme: { semiDarkSidebarSub: false } });
    }
    prevSemiDarkSidebar.current = theme.semiDarkSidebar;
  }, [theme.semiDarkSidebar]);

  const presets = [
    { icon: Sun, label: $t('preferences.theme.light'), name: 'light' },
    { icon: MoonStar, label: $t('preferences.theme.dark'), name: 'dark' },
    { icon: SunMoon, label: $t('preferences.followSystem'), name: 'auto' },
  ];
  return (
    <div className="flex w-full flex-wrap justify-between">
      {presets.map((preset) => (
        <div
          className="flex cursor-pointer flex-col"
          key={preset.name}
          onClick={() => updatePreferences({ theme: { mode: preset.name as 'auto' | 'dark' | 'light' } })}
        >
          <div className={`outline-box flex-center py-4 ${theme.mode === preset.name ? 'outline-box-active' : ''}`}>
            <preset.icon className="mx-9 size-5" />
          </div>
          <div className="text-muted-foreground mt-2 text-center text-xs">{preset.label}</div>
        </div>
      ))}

      <div className="mt-6 w-full">
        <SwitchItem
          checked={theme.semiDarkSidebar}
          disabled={theme.mode === 'dark' || layout === 'header-nav' || layout === 'full-content'}
          label={$t('preferences.theme.darkSidebar')}
          onChange={(c) => updatePreferences({ theme: { semiDarkSidebar: c } })}
          tip={$t('preferences.theme.darkSidebarTip')}
        />
      </div>
      <SwitchItem
        checked={theme.semiDarkSidebarSub}
        disabled={
          theme.mode === 'dark' ||
          (layout !== 'header-mixed-nav' && layout !== 'sidebar-mixed-nav') ||
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
    </div>
  );
}

/**
 * 对应 blocks/theme/builtin.vue：全部内置主题（custom 在末尾）+ 取色器。
 * watch([builtinType, isDark]) 对所有 preset 生效（custom 的 color 为空 → 清空主色）；
 * 取色器 input 经 300ms 节流只更新 colorPrimary；custom 内层点击只打开取色器。
 */
function BuiltinGrid() {
  const { isDark, preferences } = usePreferences();
  const { builtinType, colorPrimary } = preferences.theme;
  const colorInputRef = useRef<HTMLInputElement>(null);

  const updateColorPrimary = useThrottledCallback((hsl: string) => {
    updatePreferences({ theme: { colorPrimary: hsl } });
  }, 300);

  // Vue watch 非 immediate：仅在 builtinType / isDark 变化时重算（用 prev 对比，兼容 StrictMode）
  const prevBuiltinType = useRef(builtinType);
  const prevIsDark = useRef(isDark);
  useEffect(() => {
    if (prevBuiltinType.current === builtinType && prevIsDark.current === isDark) {
      return;
    }
    prevBuiltinType.current = builtinType;
    prevIsDark.current = isDark;
    const preset = BUILT_IN_THEME_PRESETS.find((item) => item.type === builtinType);
    if (preset) {
      const primaryColor = isDark
        ? preset.darkPrimaryColor || preset.primaryColor
        : preset.primaryColor;
      updatePreferences({ theme: { colorPrimary: primaryColor || preset.color } });
    }
  }, [builtinType, isDark]);

  const inputValue = new TinyColor(colorPrimary || '').toHexString();

  return (
    <div className="flex w-full flex-wrap justify-between">
      {BUILT_IN_THEME_PRESETS.map((preset) => (
        <div
          className="flex cursor-pointer flex-col"
          key={preset.type}
          onClick={() => updatePreferences({ theme: { builtinType: preset.type } })}
        >
          <div
            className={`group outline-box flex-center cursor-pointer ${
              builtinType === preset.type ? 'outline-box-active' : ''
            }`}
          >
            {preset.type === 'custom' ? (
              <div
                className="size-full px-9 py-2"
                onClick={(e) => {
                  e.stopPropagation();
                  colorInputRef.current?.click();
                }}
              >
                <div className="relative flex-center size-5 rounded-sm">
                  <UserRoundPen className="absolute z-1 size-5 opacity-60 group-hover:opacity-100" />
                  <input
                    className="absolute inset-0 opacity-0"
                    onChange={(e) => updateColorPrimary(convertToHsl(e.target.value))}
                    ref={colorInputRef}
                    type="color"
                    value={inputValue}
                  />
                </div>
              </div>
            ) : (
              <div className="mx-9 my-2 size-5 rounded-md" style={{ backgroundColor: preset.color }} />
            )}
          </div>
          <div className="text-muted-foreground my-2 text-center text-xs">
            {builtinLabel(preset.type)}
          </div>
        </div>
      ))}
    </div>
  );
}

/** 对应 blocks/theme/font-size.vue：全宽 NumberField + px + tip，clamp 15..22 */
function FontSizeControl() {
  const { preferences } = usePreferences();
  const fontSize = preferences.theme.fontSize;

  // Vue watch immediate：越界即夹取
  useEffect(() => {
    if (fontSize < 15) {
      updatePreferences({ theme: { fontSize: 15 } });
    } else if (fontSize > 22) {
      updatePreferences({ theme: { fontSize: 22 } });
    }
  }, [fontSize]);

  return (
    <div className="flex w-full flex-col gap-4">
      <div className="flex items-center gap-2">
        <NumberField
          class="w-full"
          max={22}
          min={15}
          modelValue={fontSize}
          step={1}
          onUpdate:modelValue={(value) => updatePreferences({ theme: { fontSize: value } })}
        >
          <NumberFieldContent>
            <NumberFieldDecrement />
            <NumberFieldInput />
            <NumberFieldIncrement />
          </NumberFieldContent>
        </NumberField>
        <span className="text-muted-foreground text-xs whitespace-nowrap">px</span>
      </div>
      <div className="text-muted-foreground text-xs">{$t('preferences.theme.fontSizeTip')}</div>
    </div>
  );
}

function AppearancePanel() {
  const { preferences } = usePreferences();
  const { app, theme } = preferences;
  return (
    <>
      <Block title={$t('preferences.theme.title')}>
        <ThemePanel />
      </Block>
      <Block title={$t('preferences.theme.builtin.title')}>
        <BuiltinGrid />
      </Block>
      <Block title={$t('preferences.theme.radius')}>
        <ToggleGroup
          class="gap-2"
          modelValue={theme.radius}
          size="sm"
          type="single"
          onUpdate:modelValue={(next) => updatePreferences({ theme: { radius: next as string } })}
          variant="outline"
        >
          {['0', '0.25', '0.5', '0.75', '1'].map((value) => (
            <ToggleGroupItem
              class="h-7 w-16 rounded-sm data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
              key={value}
              value={value}
            >
              {value}
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
          onUpdate:modelValue={(next) => updatePreferences({ theme: { buttonWaveMode: next as typeof theme.buttonWaveMode } })}
          variant="outline"
        >
          {(['Default', 'Disabled', 'Happy', 'Inset', 'Shake'] as const).map((value) => (
            <ToggleGroupItem
              class="h-7 rounded-sm px-2 data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
              key={value}
              value={value}
            >
              {value}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </Block>
      <Block title={$t('preferences.theme.fontSize')}>
        <FontSizeControl />
      </Block>
      <Block title={$t('preferences.other')}>
        <SwitchItem
          checked={app.colorWeakMode}
          label={$t('preferences.theme.weakMode')}
          onChange={(c) => updatePreferences({ app: { colorWeakMode: c } })}
        />
        <SwitchItem
          checked={app.colorGrayMode}
          label={$t('preferences.theme.grayMode')}
          onChange={(c) => updatePreferences({ app: { colorGrayMode: c } })}
        />
      </Block>
    </>
  );
}

/* ---------------- 布局页签 ---------------- */

function LayoutPanel() {
  const {
    isFullContent,
    isHeaderNav,
    isHeaderSidebarNav,
    isMixedNav,
    isSideMixedNav,
    isSideMode,
    isSideNav,
    preferences,
  } = usePreferences();
  const {
    app, breadcrumb, copyright, footer, header, navigation,
    sidebar, tabbar, widget,
  } = preferences;

  // Vue preferences-drawer.vue showBreadcrumbConfig
  const showBreadcrumbConfig =
    !isFullContent && !isMixedNav && !isHeaderNav && header.enable;
  const breadcrumbDisabled =
    !showBreadcrumbConfig || !(isSideNav || isSideMixedNav || isHeaderSidebarNav);
  const breadcrumbItemDisabled = !breadcrumb.enable || breadcrumbDisabled;

  const sidebarDisabled = !sidebar.enable || !isSideMode;
  const tabbarDisabled = !tabbar.enable;
  const copyrightItemDisabled = !footer.enable || !copyright.enable;

  // Vue sidebar.vue：两个布尔偏好 ↔ checkbox 组的双向映射
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

  return (
    <>
      <Block title={$t('preferences.layout')}>
        <LayoutPreset />
      </Block>
      <Block title={$t('preferences.content')}>
        <ContentPreset />
      </Block>

      <Block title={$t('preferences.sidebar.title')}>
        <SwitchItem
          checked={sidebar.enable}
          disabled={!isSideMode}
          label={$t('preferences.sidebar.visible')}
          onChange={(c) => updatePreferences({ sidebar: { enable: c } })}
        />
        <SwitchItem
          checked={sidebar.draggable}
          disabled={sidebarDisabled}
          label={$t('preferences.sidebar.draggable')}
          onChange={(c) => updatePreferences({ sidebar: { draggable: c } })}
        />
        <SwitchItem
          checked={sidebar.collapsed}
          disabled={sidebarDisabled}
          label={$t('preferences.sidebar.collapsed')}
          onChange={(c) => updatePreferences({ sidebar: { collapsed: c } })}
        />
        <SwitchItem
          checked={sidebar.expandOnHover}
          disabled={sidebarDisabled || !sidebar.collapsed}
          label={$t('preferences.sidebar.expandOnHover')}
          onChange={(c) => updatePreferences({ sidebar: { expandOnHover: c } })}
          tip={$t('preferences.sidebar.expandOnHoverTip')}
        />
        <SwitchItem
          checked={sidebar.collapsedShowTitle}
          disabled={sidebarDisabled || !sidebar.collapsed}
          label={$t('preferences.sidebar.collapsedShowTitle')}
          onChange={(c) => updatePreferences({ sidebar: { collapsedShowTitle: c } })}
        />
        <SwitchItem
          checked={sidebar.autoActivateChild}
          disabled={
            !sidebar.enable ||
            !['sidebar-mixed-nav', 'mixed-nav', 'header-mixed-nav'].includes(app.layout) ||
            !isSideMode
          }
          label={$t('preferences.sidebar.autoActivateChild')}
          onChange={(c) => updatePreferences({ sidebar: { autoActivateChild: c } })}
          tip={$t('preferences.sidebar.autoActivateChildTip')}
        />
        <CheckboxItem
          items={[
            { label: $t('preferences.sidebar.buttonCollapsed'), value: 'collapsed' },
            { label: $t('preferences.sidebar.buttonFixed'), value: 'fixed' },
          ]}
          label={$t('preferences.sidebar.buttons')}
          onChange={handleSidebarButtons}
          values={sidebarButtons}
        />
        <NumberItem
          disabled={sidebarDisabled}
          label={$t('preferences.sidebar.width')}
          max={320}
          min={160}
          onChange={(v) => updatePreferences({ sidebar: { width: v } })}
          step={10}
          value={sidebar.width}
        />
      </Block>

      <Block title={$t('preferences.header.title')}>
        <SwitchItem
          checked={header.enable}
          disabled={isFullContent}
          label={$t('preferences.header.visible')}
          onChange={(c) => updatePreferences({ header: { enable: c } })}
        />
        <SelectItem
          disabled={!header.enable}
          label={$t('preferences.mode')}
          onChange={(v) => updatePreferences({ header: { mode: v as typeof header.mode } })}
          options={[
            { label: $t('preferences.header.modeStatic'), value: 'static' },
            { label: $t('preferences.header.modeFixed'), value: 'fixed' },
            { label: $t('preferences.header.modeAuto'), value: 'auto' },
            { label: $t('preferences.header.modeAutoScroll'), value: 'auto-scroll' },
          ]}
          value={header.mode}
        />
        <ToggleItem
          disabled={!header.enable}
          items={[
            { label: $t('preferences.header.menuAlignStart'), value: 'start' },
            { label: $t('preferences.header.menuAlignCenter'), value: 'center' },
            { label: $t('preferences.header.menuAlignEnd'), value: 'end' },
          ]}
          label={$t('preferences.header.menuAlign')}
          onChange={(v) => updatePreferences({ header: { menuAlign: v as typeof header.menuAlign } })}
          value={header.menuAlign}
        />
      </Block>

      <Block title={$t('preferences.navigationMenu.title')}>
        <ToggleItem
          disabled={isFullContent}
          items={[
            { label: $t('preferences.rounded'), value: 'rounded' },
            { label: $t('preferences.plain'), value: 'plain' },
          ]}
          label={$t('preferences.navigationMenu.style')}
          onChange={(v) => updatePreferences({ navigation: { styleType: v as typeof navigation.styleType } })}
          value={navigation.styleType}
        />
        <SwitchItem
          checked={navigation.split}
          disabled={!isMixedNav || isFullContent}
          label={$t('preferences.navigationMenu.split')}
          onChange={(c) => updatePreferences({ navigation: { split: c } })}
          tip={$t('preferences.navigationMenu.splitTip')}
        />
        <SwitchItem
          checked={navigation.accordion}
          disabled={isFullContent}
          label={$t('preferences.navigationMenu.accordion')}
          onChange={(c) => updatePreferences({ navigation: { accordion: c } })}
        />
      </Block>

      <Block title={$t('preferences.breadcrumb.title')}>
        <SwitchItem
          checked={breadcrumb.enable}
          disabled={breadcrumbDisabled}
          label={$t('preferences.breadcrumb.enable')}
          onChange={(c) => updatePreferences({ breadcrumb: { enable: c } })}
        />
        <SwitchItem
          checked={breadcrumb.hideOnlyOne}
          disabled={breadcrumbItemDisabled}
          label={$t('preferences.breadcrumb.hideOnlyOne')}
          onChange={(c) => updatePreferences({ breadcrumb: { hideOnlyOne: c } })}
        />
        <SwitchItem
          checked={breadcrumb.showIcon}
          disabled={breadcrumbItemDisabled}
          label={$t('preferences.breadcrumb.icon')}
          onChange={(c) => updatePreferences({ breadcrumb: { showIcon: c } })}
        />
        <SwitchItem
          checked={breadcrumb.showHome}
          disabled={breadcrumbItemDisabled || !breadcrumb.showIcon}
          label={$t('preferences.breadcrumb.home')}
          onChange={(c) => updatePreferences({ breadcrumb: { showHome: c } })}
        />
        <ToggleItem
          disabled={breadcrumbItemDisabled}
          items={[
            { label: $t('preferences.normal'), value: 'normal' },
            { label: $t('preferences.breadcrumb.background'), value: 'background' },
          ]}
          label={$t('preferences.breadcrumb.style')}
          onChange={(v) => updatePreferences({ breadcrumb: { styleType: v as typeof breadcrumb.styleType } })}
          value={breadcrumb.styleType}
        />
      </Block>

      <Block title={$t('preferences.tabbar.title')}>
        <SwitchItem
          checked={tabbar.enable}
          label={$t('preferences.tabbar.enable')}
          onChange={(c) => updatePreferences({ tabbar: { enable: c } })}
        />
        <SwitchItem
          checked={tabbar.persist}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.persist')}
          onChange={(c) => updatePreferences({ tabbar: { persist: c } })}
        />
        <SwitchItem
          checked={tabbar.visitHistory}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.visitHistory')}
          onChange={(c) => updatePreferences({ tabbar: { visitHistory: c } })}
          tip={$t('preferences.tabbar.visitHistoryTip')}
        />
        <NumberItem
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.maxCount')}
          max={30}
          min={0}
          onChange={(v) => updatePreferences({ tabbar: { maxCount: v } })}
          step={5}
          tip={$t('preferences.tabbar.maxCountTip')}
          value={tabbar.maxCount}
        />
        <SwitchItem
          checked={tabbar.draggable}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.draggable')}
          onChange={(c) => updatePreferences({ tabbar: { draggable: c } })}
        />
        <SwitchItem
          checked={tabbar.wheelable}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.wheelable')}
          onChange={(c) => updatePreferences({ tabbar: { wheelable: c } })}
          tip={$t('preferences.tabbar.wheelableTip')}
        />
        <SwitchItem
          checked={tabbar.middleClickToClose}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.middleClickClose')}
          onChange={(c) => updatePreferences({ tabbar: { middleClickToClose: c } })}
        />
        <SwitchItem
          checked={tabbar.showIcon}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.icon')}
          onChange={(c) => updatePreferences({ tabbar: { showIcon: c } })}
        />
        <SwitchItem
          checked={tabbar.showMore}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.showMore')}
          onChange={(c) => updatePreferences({ tabbar: { showMore: c } })}
        />
        <SwitchItem
          checked={tabbar.showMaximize}
          disabled={tabbarDisabled}
          label={$t('preferences.tabbar.showMaximize')}
          onChange={(c) => updatePreferences({ tabbar: { showMaximize: c } })}
        />
        <SelectItem
          label={$t('preferences.tabbar.styleType.title')}
          onChange={(v) => updatePreferences({ tabbar: { styleType: v as typeof tabbar.styleType } })}
          options={['chrome', 'plain', 'card', 'brisk'].map((v) => ({
            label: $t(`preferences.tabbar.styleType.${v}`),
            value: v,
          }))}
          value={tabbar.styleType}
        />
      </Block>

      <Block title={$t('preferences.widget.title')}>
        <SwitchItem checked={widget.globalSearch} label={$t('preferences.widget.globalSearch')} onChange={(c) => updatePreferences({ widget: { globalSearch: c } })} />
        <SwitchItem checked={widget.themeToggle} label={$t('preferences.widget.themeToggle')} onChange={(c) => updatePreferences({ widget: { themeToggle: c } })} />
        <SwitchItem checked={widget.languageToggle} label={$t('preferences.widget.languageToggle')} onChange={(c) => updatePreferences({ widget: { languageToggle: c } })} />
        <SwitchItem checked={widget.fullscreen} label={$t('preferences.widget.fullscreen')} onChange={(c) => updatePreferences({ widget: { fullscreen: c } })} />
        <SwitchItem checked={widget.notification} label={$t('preferences.widget.notification')} onChange={(c) => updatePreferences({ widget: { notification: c } })} />
        <SwitchItem checked={widget.lockScreen} label={$t('preferences.widget.lockScreen')} onChange={(c) => updatePreferences({ widget: { lockScreen: c } })} />
        <SwitchItem checked={widget.sidebarToggle} label={$t('preferences.widget.sidebarToggle')} onChange={(c) => updatePreferences({ widget: { sidebarToggle: c } })} />
        <SwitchItem checked={widget.refresh} label={$t('preferences.widget.refresh')} onChange={(c) => updatePreferences({ widget: { refresh: c } })} />
        <SwitchItem checked={widget.timezone} label={$t('preferences.widget.timezone')} onChange={(c) => updatePreferences({ widget: { timezone: c } })} />
        <SelectItem
          label={$t('preferences.position.title')}
          onChange={(v) => updatePreferences({ app: { preferencesButtonPosition: v as typeof app.preferencesButtonPosition } })}
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
        <SwitchItem
          checked={footer.enable}
          label={$t('preferences.footer.visible')}
          onChange={(c) => updatePreferences({ footer: { enable: c } })}
        />
        <SwitchItem
          checked={footer.fixed}
          disabled={!footer.enable}
          label={$t('preferences.footer.fixed')}
          onChange={(c) => updatePreferences({ footer: { fixed: c } })}
        />
      </Block>

      {copyright.settingShow ? (
        <Block title={$t('preferences.copyright.title')}>
          <SwitchItem
            checked={copyright.enable}
            disabled={!footer.enable}
            label={$t('preferences.copyright.enable')}
            onChange={(c) => updatePreferences({ copyright: { enable: c } })}
          />
          <InputItem disabled={copyrightItemDisabled} label={$t('preferences.copyright.companyName')} onChange={(v) => updatePreferences({ copyright: { companyName: v } })} value={copyright.companyName} />
          <InputItem disabled={copyrightItemDisabled} label={$t('preferences.copyright.companySiteLink')} onChange={(v) => updatePreferences({ copyright: { companySiteLink: v } })} value={copyright.companySiteLink} />
          <InputItem disabled={copyrightItemDisabled} label={$t('preferences.copyright.date')} onChange={(v) => updatePreferences({ copyright: { date: v } })} value={copyright.date} />
          <InputItem disabled={copyrightItemDisabled} label={$t('preferences.copyright.icp')} onChange={(v) => updatePreferences({ copyright: { icp: v } })} value={copyright.icp} />
          <InputItem disabled={copyrightItemDisabled} label={$t('preferences.copyright.icpLink')} onChange={(v) => updatePreferences({ copyright: { icpLink: v } })} value={copyright.icpLink} />
        </Block>
      ) : null}
    </>
  );
}

/* ---------------- 快捷键页签 ---------------- */

const IS_WINDOWS = navigator.platform.toLowerCase().includes('win');

/** 对应 blocks/shortcut-keys/global.vue：4 个子开关全部受总开关控制 */
function ShortcutKeyPanel() {
  const { preferences } = usePreferences();
  const { shortcutKeys } = preferences;
  const altView = IS_WINDOWS ? 'Alt' : '⌥';
  return (
    <Block title={$t('preferences.shortcutKeys.global')}>
      <SwitchItem
        checked={shortcutKeys.enable}
        label={$t('preferences.shortcutKeys.title')}
        onChange={(c) => updatePreferences({ shortcutKeys: { enable: c } })}
      />
      <SwitchItem
        checked={shortcutKeys.globalSearch}
        disabled={!shortcutKeys.enable}
        label={$t('preferences.shortcutKeys.search')}
        onChange={(c) => updatePreferences({ shortcutKeys: { globalSearch: c } })}
        shortcut={<>{IS_WINDOWS ? 'Ctrl' : '⌘'} <kbd>K</kbd></>}
      />
      <SwitchItem
        checked={shortcutKeys.globalLogout}
        disabled={!shortcutKeys.enable}
        label={$t('preferences.shortcutKeys.logout')}
        onChange={(c) => updatePreferences({ shortcutKeys: { globalLogout: c } })}
        shortcut={<>{altView} Q</>}
      />
      <SwitchItem
        checked={shortcutKeys.globalLockScreen}
        disabled={!shortcutKeys.enable}
        label={$t('ui.widgets.lockScreen.title')}
        onChange={(c) => updatePreferences({ shortcutKeys: { globalLockScreen: c } })}
        shortcut={<>{altView} L</>}
      />
      <SwitchItem
        checked={shortcutKeys.globalEscape}
        disabled={!shortcutKeys.enable}
        label={$t('preferences.shortcutKeys.escape')}
        onChange={(c) => updatePreferences({ shortcutKeys: { globalEscape: c } })}
        shortcut={<>Esc</>}
      />
    </Block>
  );
}

/* ---------------- 通用页签 ---------------- */

/**
 * 对应 blocks/general/general.vue：挂载时拉取时区选项，
 * 并把 timezone store 的当前值同步回偏好（与 Vue onMounted 一致）。
 */
function GeneralPanel() {
  const { preferences } = usePreferences();
  const { app, transition } = preferences;
  const [timezoneOptions, setTimezoneOptions] = useState<{ label: string; value: string }[]>([]);

  useEffect(() => {
    let active = true;
    void (async () => {
      const store = useTimezoneStore.getState();
      const options = await store.getTimezoneOptions();
      if (!active) return;
      setTimezoneOptions(options);
      if (store.timezone) {
        updatePreferences({ app: { timezone: store.timezone } });
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  return (
    <>
      <Block title={$t('preferences.general')}>
        <SelectItem
          label={$t('preferences.language')}
          onChange={async (value) => {
            const locale = value as SupportedLanguagesType;
            // 先加载语言包再更新偏好，保证重渲染时 $t 已是新语言
            await loadLocaleMessages(locale);
            updatePreferences({ app: { locale } });
          }}
          options={SUPPORT_LANGUAGES.map((item) => ({ label: item.label, value: item.value }))}
          value={app.locale}
        />
        <SelectItem
          label={$t('preferences.timezone')}
          onChange={(v) => updatePreferences({ app: { timezone: v } })}
          options={timezoneOptions}
          value={app.timezone}
        />
        <SwitchItem
          checked={app.dynamicTitle}
          label={$t('preferences.dynamicTitle')}
          onChange={(c) => updatePreferences({ app: { dynamicTitle: c } })}
        />
        <SwitchItem
          checked={app.watermark}
          label={$t('preferences.watermark')}
          onChange={(c) => {
            updatePreferences({ app: { watermark: c } });
            if (!c) updatePreferences({ app: { watermarkContent: '' } });
          }}
        />
        {app.watermark ? (
          <InputItem
            label={$t('preferences.watermarkContent')}
            onChange={(v) => updatePreferences({ app: { watermarkContent: v } })}
            placeholder={$t('preferences.watermarkContent')}
            value={app.watermarkContent}
          />
        ) : null}
        <SwitchItem
          checked={app.enableCheckUpdates}
          label={$t('preferences.checkUpdates')}
          onChange={(c) => updatePreferences({ app: { enableCheckUpdates: c } })}
        />
        <SwitchItem
          checked={app.enableCopyPreferences}
          label={$t('preferences.enableCopyPreferences')}
          onChange={(c) => updatePreferences({ app: { enableCopyPreferences: c } })}
        />
      </Block>

      <Block title={$t('preferences.animation.title')}>
        <SwitchItem
          checked={transition.progress}
          label={$t('preferences.animation.progress')}
          onChange={(c) => updatePreferences({ transition: { progress: c } })}
        />
        <SwitchItem
          checked={transition.loading}
          label={$t('preferences.animation.loading')}
          onChange={(c) => updatePreferences({ transition: { loading: c } })}
        />
        <SwitchItem
          checked={transition.enable}
          label={$t('preferences.animation.transition')}
          onChange={(c) => updatePreferences({ transition: { enable: c } })}
        />
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
    </>
  );
}

/* ---------------- 自定义页签（preferencesExtension） ---------------- */

function CustomPanel() {
  const { customPreferences, preferencesExtension } = usePreferences();
  const title = preferencesExtension?.title || preferencesExtension?.tabLabel;
  return (
    <Block title={title ? $t(title) : ''}>
      <CustomFields
        fields={preferencesExtension?.fields ?? []}
        onUpdate={updateCustomPreferences}
        values={customPreferences}
      />
    </Block>
  );
}

/* ---------------- 抽屉本体 ---------------- */

type TabKey = 'appearance' | 'custom' | 'general' | 'layout' | 'shortcutKey';

export function PreferencesDrawer() {
  const { diffCustomPreference, diffPreference, preferences, preferencesExtension } =
    usePreferences();
  const logout = useAuthStore((state) => state.logout);
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<TabKey>('appearance');

  useEffect(() => {
    openPreferencesFn = () => setOpen(true);
    return () => {
      openPreferencesFn = null;
    };
  }, []);

  const { app } = preferences;
  const sticky = app.enableStickyPreferencesNavigationBar;

  // 与 Vue mergedDiffPreference 一致：与默认值的 diff（而非会话快照）
  const mergedDiff = useMemo(() => {
    const result: Record<string, unknown> = {};
    if (diffPreference) Object.assign(result, diffPreference);
    if (diffCustomPreference) result.custom = diffCustomPreference;
    return Object.keys(result).length > 0 ? result : undefined;
  }, [diffPreference, diffCustomPreference]);
  const hasDiff = Boolean(mergedDiff);

  const showCustomTab = (preferencesExtension?.fields.length ?? 0) > 0;
  // 不用 useMemo：$t 结果需随 locale 切换实时刷新（同 Vue computed tabs）
  const tabs = [
    { label: $t('preferences.appearance'), value: 'appearance' },
    { label: $t('preferences.layout'), value: 'layout' },
    { label: $t('preferences.shortcutKeys.title'), value: 'shortcutKey' },
    { label: $t('preferences.general'), value: 'general' },
  ];
  if (showCustomTab) {
    tabs.push({
      label: preferencesExtension?.tabLabel ? $t(preferencesExtension.tabLabel) : '',
      value: 'custom',
    });
  }

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
    const locale = getPreferences().app.locale;
    await loadLocaleMessages(locale);
    // 重置触发的重渲染发生在语言包就绪之前，这里再通知一次以重译 $t 文案
    updatePreferences({ app: { locale } });
  }

  return (
    <Drawer
      closable={false}
      extra={
        <div className="flex items-center">
          <WalnutIconButton
            class="relative"
            disabled={!hasDiff}
            onClick={() => void handleReset()}
            tooltip={$t('preferences.resetTip')}
          >
            {hasDiff ? (
              <span className="bg-primary absolute top-0.5 right-0.5 size-2 rounded-sm" />
            ) : null}
            <RotateCw className="size-4" />
          </WalnutIconButton>
          <WalnutIconButton
            class="relative"
            onClick={() =>
              updatePreferences({ app: { enableStickyPreferencesNavigationBar: !sticky } })
            }
            tooltip={
              sticky
                ? $t('preferences.disableStickyPreferencesNavigationBar')
                : $t('preferences.enableStickyPreferencesNavigationBar')
            }
          >
            {sticky ? <PinOff className="size-4" /> : <Pin className="size-4" />}
          </WalnutIconButton>
          <WalnutIconButton onClick={() => setOpen(false)}>
            <X className="size-4" />
          </WalnutIconButton>
        </div>
      }
      footer={
        <div className="flex w-full">
          {app.enableCopyPreferences ? (
            <WalnutButton
              class="mx-4 w-full"
              disabled={!hasDiff}
              onClick={() => void handleCopy()}
              size="sm"
              variant="default"
            >
              <Copy className="mr-2 size-3" />
              {$t('preferences.copyPreferences')}
            </WalnutButton>
          ) : null}
          <WalnutButton
            class="mr-4 w-full"
            disabled={!hasDiff}
            onClick={() => void handleClearCache()}
            size="sm"
            variant="ghost"
          >
            {$t('preferences.clearAndLogout')}
          </WalnutButton>
        </div>
      }
      onClose={() => setOpen(false)}
      open={open}
      styles={{
        body: { padding: '0 16px' },
        footer: { padding: '12px 0' },
        header: { padding: '16px' },
      }}
      title={
        <div>
          <div className="text-lg font-semibold">{$t('preferences.title')}</div>
          <div className="text-muted-foreground mt-1 text-xs font-normal">
            {$t('preferences.subtitle')}
          </div>
        </div>
      }
      width={384}
    >
      <WalnutSegmented
        class={sticky ? 'sticky-tabs-header' : undefined}
        modelValue={tab}
        tabs={tabs}
        onUpdate:modelValue={(value) => setTab(value as TabKey)}
      >
        {tab === 'appearance' ? <AppearancePanel /> : null}
        {tab === 'layout' ? <LayoutPanel /> : null}
        {tab === 'shortcutKey' ? <ShortcutKeyPanel /> : null}
        {tab === 'general' ? <GeneralPanel /> : null}
        {tab === 'custom' ? <CustomPanel /> : null}
      </WalnutSegmented>
    </Drawer>
  );
}