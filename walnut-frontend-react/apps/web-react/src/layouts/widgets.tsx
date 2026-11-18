import { useEffect, useState } from 'react';

import {
  COLOR_PRESETS,
  updatePreferences,
  usePreferences,
} from '@vben/preferences';
import { SUPPORT_LANGUAGES } from '@vben/constants';
import { loadLocaleMessages } from '@vben/locales';
import {
  VbenDropdownRadioMenu,
  VbenIconButton,
} from '@vben-core/shadcn-ui';
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

  function toggleTheme() {
    updatePreferences({ theme: { mode: isDark ? 'light' : 'dark' } });
  }

  return (
    <VbenIconButton
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
    </VbenIconButton>
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
          <VbenIconButton
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
          </VbenIconButton>
        ))}
      </div>
      <VbenIconButton>
        <Palette className="size-4 text-primary" />
      </VbenIconButton>
    </div>
  );
}

/** 认证页布局切换（对应 widgets/layout-toggle.vue） */
export function AuthenticationLayoutToggle() {
  const { authPanelCenter, authPanelLeft, authPanelRight, preferences } =
    usePreferences();

  const menus = [
    { icon: PanelLeft, label: '左对齐', value: 'panel-left' },
    { icon: InspectionPanel, label: '居中', value: 'panel-center' },
    { icon: PanelRight, label: '右对齐', value: 'panel-right' },
  ];

  function handleUpdate(value: string) {
    updatePreferences({ app: { authPageLayout: value as any } });
  }

  return (
    <VbenDropdownRadioMenu
      menus={menus}
      modelValue={preferences.app.authPageLayout}
      onUpdate:modelValue={handleUpdate}
    >
      <VbenIconButton>
        {authPanelRight ? <PanelRight className="size-4" /> : null}
        {authPanelLeft ? <PanelLeft className="size-4" /> : null}
        {authPanelCenter ? <InspectionPanel className="size-4" /> : null}
      </VbenIconButton>
    </VbenDropdownRadioMenu>
  );
}

/** 语言切换（对应 widgets/language-toggle.vue） */
export function LanguageToggle() {
  const { preferences } = usePreferences();

  async function handleUpdate(value: string) {
    updatePreferences({ app: { locale: value as any } });
    await loadLocaleMessages(value as any);
  }

  const menus = SUPPORT_LANGUAGES.map((item) => ({
    label: item.label,
    value: item.value,
  }));

  return (
    <VbenDropdownRadioMenu
      menus={menus}
      modelValue={preferences.app.locale}
      onUpdate:modelValue={handleUpdate}
    >
      <VbenIconButton class="hover:animate-[shrink_0.3s_ease-in-out]">
        <Languages className="size-4 text-foreground" />
      </VbenIconButton>
    </VbenDropdownRadioMenu>
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
    <VbenIconButton onClick={toggle}>
      {isFullscreen ? (
        <Minimize className="size-4" />
      ) : (
        <Maximize className="size-4" />
      )}
    </VbenIconButton>
  );
}
