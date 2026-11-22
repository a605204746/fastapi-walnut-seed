import { useEffect, useState, type MouseEvent } from 'react';
import { flushSync } from 'react-dom';

import {
  COLOR_PRESETS,
  updatePreferences,
  usePreferences,
} from '@walnut/preferences';
import { SUPPORT_LANGUAGES } from '@walnut/constants';
import { $t, loadLocaleMessages } from '@walnut/locales';
import {
  WalnutDropdownRadioMenu,
  WalnutIconButton,
} from '@walnut-core/shadcn-ui';
import {
  InspectionPanel,
  Languages,
  Maximize,
  Minimize,
  Palette,
  PanelLeft,
  PanelRight,
} from 'lucide-react';

import './theme-button.css';

/** 主题切换按钮（对应 widgets/theme-toggle/theme-button.vue，type=icon） */
function ThemeButton() {
  const { isDark } = usePreferences();
  const theme = isDark ? 'light' : 'dark';

  /**
   * 1:1 复刻 Vue toggleTheme：支持 View Transition 时从点击点做 450ms 圆形扩散
   * （切暗收缩旧快照 old(root)，切亮扩张新快照 new(root)）；
   * 不支持或 prefers-reduced-motion 时直接切换。
   */
  function toggleTheme(event: MouseEvent<HTMLButtonElement>) {
    const nextDark = !isDark;
    const apply = () =>
      updatePreferences({ theme: { mode: nextDark ? 'dark' : 'light' } });
    // 运行时能力检测：旧浏览器没有 startViewTransition，直接切换
    const isAppearanceTransition =
      typeof document.startViewTransition === 'function' &&
      !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (!isAppearanceTransition) {
      apply();
      return;
    }
    const x = event.clientX;
    const y = event.clientY;
    const endRadius = Math.hypot(
      Math.max(x, window.innerWidth - x),
      Math.max(y, window.innerHeight - y),
    );
    // flushSync 等价 Vue nextTick：确保新主题 DOM 已提交后再截新快照
    const transition = document.startViewTransition(() => {
      flushSync(apply);
    });
    transition.ready.then(() => {
      const clipPath = [
        `circle(0px at ${x}px ${y}px)`,
        `circle(${endRadius}px at ${x}px ${y}px)`,
      ];
      const animate = document.documentElement.animate(
        {
          clipPath: nextDark ? [...clipPath].reverse() : clipPath,
        },
        {
          duration: 450,
          easing: 'ease-in',
          pseudoElement: nextDark
            ? '::view-transition-old(root)'
            : '::view-transition-new(root)',
        },
      );
      animate.onfinish = () => {
        transition.skipTransition();
      };
    });
  }

  return (
    <WalnutIconButton
      aria-label={theme}
      aria-live="polite"
      class={`theme-toggle is-${theme} cursor-pointer border-none bg-none hover:animate-[shrink_0.3s_ease-in-out] rounded-full`}
      onClick={toggleTheme}
      style={{ padding: '7px' }}
      variant="icon"
    >
      <svg aria-hidden="true" height="24" viewBox="0 0 24 24" width="24">
        <mask className="theme-toggle__moon" id="theme-toggle-moon">
          <rect fill="white" height="100%" width="100%" x="0" y="0" />
          <circle cx="40" cy="8" fill="black" r="11" />
        </mask>
        <circle
          className="theme-toggle__sun fill-foreground/90"
          cx="12"
          cy="12"
          id="sun"
          mask="url(#theme-toggle-moon)"
          r="11"
        />
        <g className="theme-toggle__sun-beams stroke-foreground/90 stroke-2">
          <line x1="12" x2="12" y1="1" y2="3" />
          <line x1="12" x2="12" y1="21" y2="23" />
          <line x1="4.22" x2="5.64" y1="4.22" y2="5.64" />
          <line x1="18.36" x2="19.78" y1="18.36" y2="19.78" />
          <line x1="1" x2="3" y1="12" y2="12" />
          <line x1="21" x2="23" y1="12" y2="12" />
          <line x1="4.22" x2="5.64" y1="19.78" y2="18.36" />
          <line x1="18.36" x2="19.78" y1="5.64" y2="4.22" />
        </g>
      </svg>
    </WalnutIconButton>
  );
}

/** 主题色切换（对应 widgets/color-toggle.vue） */
export function AuthenticationColorToggle() {
  const { preferences } = usePreferences();

  function handleUpdate(colorPrimary: string, type: string) {
    updatePreferences({ theme: { builtinType: type as any, colorPrimary } });
  }

  return (
    <div className="group relative flex items-center overflow-hidden">
      <div className="flex w-0 overflow-hidden transition-all duration-500 ease-out group-hover:w-60">
        {COLOR_PRESETS.map((preset) => (
          <WalnutIconButton
            class="flex-center shrink-0"
            key={preset.color}
            onClick={() => handleUpdate(preset.color, preset.type)}
          >
            <div
              className="relative flex-center size-5 rounded-full hover:scale-110"
              style={{ backgroundColor: preset.color }}
            >
              {preferences.theme.builtinType === preset.type ? (
                <svg
                  className="size-3.5 text-white"
                  height="1em"
                  viewBox="0 0 15 15"
                  width="1em"
                >
                  <path
                    clipRule="evenodd"
                    d="M11.467 3.727c.289.189.37.576.181.865l-4.25 6.5a.625.625 0 0 1-.944.12l-2.75-2.5a.625.625 0 0 1 .841-.925l2.208 2.007l3.849-5.886a.625.625 0 0 1 .865-.181"
                    fill="currentColor"
                    fillRule="evenodd"
                  />
                </svg>
              ) : null}
            </div>
          </WalnutIconButton>
        ))}
      </div>
      <WalnutIconButton>
        <Palette className="size-4 text-primary" />
      </WalnutIconButton>
    </div>
  );
}

/** 认证页布局切换（对应 widgets/layout-toggle.vue） */
export function AuthenticationLayoutToggle() {
  const { authPanelCenter, authPanelLeft, authPanelRight, preferences } =
    usePreferences();

  const menus = [
    { icon: PanelLeft, label: $t('authentication.layout.alignLeft'), value: 'panel-left' },
    { icon: InspectionPanel, label: $t('authentication.layout.center'), value: 'panel-center' },
    { icon: PanelRight, label: $t('authentication.layout.alignRight'), value: 'panel-right' },
  ];

  function handleUpdate(value: string) {
    updatePreferences({ app: { authPageLayout: value as any } });
  }

  return (
    <WalnutDropdownRadioMenu
      menus={menus}
      modelValue={preferences.app.authPageLayout}
      onUpdate:modelValue={handleUpdate}
    >
      <WalnutIconButton>
        {authPanelRight ? <PanelRight className="size-4" /> : null}
        {authPanelLeft ? <PanelLeft className="size-4" /> : null}
        {authPanelCenter ? <InspectionPanel className="size-4" /> : null}
      </WalnutIconButton>
    </WalnutDropdownRadioMenu>
  );
}

/** 语言切换（对应 widgets/language-toggle.vue） */
export function LanguageToggle() {
  const { preferences } = usePreferences();

  async function handleUpdate(value: string) {
    // 先加载语言包再更新偏好：i18next 非响应式，只有偏好变化触发的
    // 重渲染才能读到新语言（Vue 侧 vue-i18n 响应式，顺序无关紧要）。
    // 旧顺序会导致页签/菜单等 $t 文案停留在旧语言。
    await loadLocaleMessages(value as any);
    updatePreferences({ app: { locale: value as any } });
  }

  const menus = SUPPORT_LANGUAGES.map((item) => ({
    label: item.label,
    value: item.value,
  }));

  return (
    <WalnutDropdownRadioMenu
      menus={menus}
      modelValue={preferences.app.locale}
      onUpdate:modelValue={handleUpdate}
    >
      <WalnutIconButton class="hover:animate-[shrink_0.3s_ease-in-out]">
        <Languages className="size-4 text-foreground" />
      </WalnutIconButton>
    </WalnutDropdownRadioMenu>
  );
}

/** 明暗主题切换（对应 widgets/theme-toggle/theme-toggle.vue，tooltip 默认禁用） */
export function ThemeToggle() {
  return <ThemeButton />;
}

/**
 * 全屏切换（对应 widgets/full-screen）。用原生 Fullscreen API，
 * 并监听 fullscreenchange 同步图标状态（退出全屏可能来自 Esc 键）。
 */
export function FullscreenToggle() {
  const [isFullscreen, setIsFullscreen] = useState(false);

  useEffect(() => {
    const onChange = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', onChange);
    return () => document.removeEventListener('fullscreenchange', onChange);
  }, []);

  function toggle() {
    if (document.fullscreenElement) {
      void document.exitFullscreen();
    } else {
      void document.documentElement.requestFullscreen();
    }
  }

  return (
    <WalnutIconButton onClick={toggle}>
      {isFullscreen ? (
        <Minimize className="size-4" />
      ) : (
        <Maximize className="size-4" />
      )}
    </WalnutIconButton>
  );
}
