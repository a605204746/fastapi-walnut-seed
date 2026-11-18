import { useMemo } from 'react';

import { convertToRgb } from '@vben-core/shared/color';
import { usePreferences } from '@vben/preferences';

/**
 * 把设计系统的 CSS 变量读回来，转成 antd 的 design token。
 *
 * 单一事实来源是 `document.documentElement` 上的 CSS 变量：
 * `preferences` 变化 → `updateCSSVariables()` 写入 CSS 变量（含切换 `.dark` class）
 * → 本 hook 用 `getComputedStyle` 读回 → 喂给 antd `ConfigProvider`。
 * 这样自绘组件（Tailwind / shadcn）与 antd 组件天然同色，不必两边各维护一份色板。
 *
 * 对应 Vue 侧 packages/effects/hooks/src/use-design-tokens.ts 的 useAntdDesignTokens。
 */
export function useAntdDesignTokens() {
  const { preferences } = usePreferences();
  const { theme } = preferences;

  return useMemo(() => {
    const rootStyles = getComputedStyle(document.documentElement);

    // CSS 变量存的是 HSL 三元组（如 `215 100% 54%`）。
    // ⚠️ 不能直接拼成 `hsl(215 100% 54%)` 交给 antd：这种 CSS Color 4 空格语法
    // antd 的颜色解析器会读偏（实测 hsl(215 100% 54%) → rgb(20,103,220)，
    // 而真实值是 rgb(20,118,255)），导致 antd 组件与自绘组件色差。
    // 统一转成 rgb() 再传，Vue 侧的 useNaiveDesignTokens 也是这么做的。
    const readColor = (variable: string) => {
      const triplet = rootStyles.getPropertyValue(variable).trim();
      // 变量缺失时返回 undefined，让 antd 用自己的默认值，而不是退化成黑色
      return triplet ? convertToRgb(`hsl(${triplet})`) : undefined;
    };

    // --radius 单位是 rem，antd 的 borderRadius 单位是 px（1rem = 16px）
    const radius = Number.parseFloat(
      rootStyles.getPropertyValue('--radius'),
    );
    // ⚠️ 不要把 theme.fontSize 喂给 antd 的 fontSize token：Vue 侧的
    // useAntdDesignTokens 只喂 borderRadius + 颜色，字号偏好只作用于自绘组件
    // （--font-size-base / --menu-font-size）。喂了会让 antd 组件整体变大，
    // 像素门禁实测 login 差异从 0.277% 跳到 1.325%（回归）。

    return {
      borderRadius: Number.isFinite(radius) ? radius * 16 : undefined,
      colorBgBase: readColor('--background'),
      colorBgContainer: readColor('--card'),
      colorBgElevated: readColor('--popover'),
      colorBgLayout: readColor('--background-deep'),
      colorBgMask: readColor('--overlay'),
      colorBorder: readColor('--border'),
      colorBorderSecondary: readColor('--border'),
      colorError: readColor('--destructive'),
      colorInfo: readColor('--primary'),
      colorPrimary: readColor('--primary'),
      colorSuccess: readColor('--success'),
      colorTextBase: readColor('--foreground'),
      colorWarning: readColor('--warning'),
      // 调整基础弹层层级，避免下拉等组件被弹窗或最大化状态下的表格遮挡
      zIndexPopupBase: 2000,
    };
    // 只在 theme 变化时重读：CSS 变量由 updateCSSVariables 同步写好，
    // 而它正是被 theme 分支触发的。
  }, [theme]);
}

/**
 * antd **组件级** token（与 {@link useAntdDesignTokens} 的全局 token 配套）。
 *
 * ⚠️ 为什么必须有这一段：antd `Layout` 的 `headerBg` / `siderBg` 默认是
 * `#001529`（深蓝黑），而外壳写的是 Tailwind 的 `bg-header` / `bg-sidebar`。
 * Tailwind v4 的 utilities 在 `@layer utilities` 里，antd 的 CSS-in-JS **不在任何层**，
 * 按 cascade 规则无层样式打赢有层样式 —— 于是浅色主题下顶栏一直是深蓝黑
 * （侧边栏因为碰巧给了 `theme="light"` 才幸免）。这类「Tailwind 背景色 vs antd
 * 组件默认背景色」的冲突无法靠加 class 解决，只能走 antd 自己的 token 通道。
 */
export function useAntdComponentTokens() {
  const { preferences } = usePreferences();
  const { theme } = preferences;

  return useMemo(() => {
    const rootStyles = getComputedStyle(document.documentElement);
    const readColor = (variable: string) => {
      const triplet = rootStyles.getPropertyValue(variable).trim();
      return triplet ? convertToRgb(`hsl(${triplet})`) : undefined;
    };

    return {
      Layout: {
        bodyBg: readColor('--background-deep'),
        headerBg: readColor('--header'),
        siderBg: readColor('--sidebar'),
        triggerBg: readColor('--sidebar-deep'),
        triggerColor: readColor('--foreground'),
      },
      Menu: {
        // 侧边栏菜单底色跟随 sidebar，避免 antd Menu 自带白底盖住半暗侧边栏
        darkItemBg: readColor('--sidebar'),
        itemBg: 'transparent',
        subMenuItemBg: 'transparent',
      },
    };
    // 理由同 useAntdDesignTokens
  }, [theme]);
}
