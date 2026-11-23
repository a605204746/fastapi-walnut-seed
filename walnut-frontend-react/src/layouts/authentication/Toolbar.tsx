import { usePreferences } from '@/core/preferences';
import { cn } from '@/core/shared/utils';

import {
  AuthenticationColorToggle,
  AuthenticationLayoutToggle,
  LanguageToggle,
  ThemeToggle,
} from '../widgets';

export type ToolbarType = 'color' | 'language' | 'layout' | 'theme';

export interface ToolbarProps {
  toolbarList?: ToolbarType[];
}

/** 对应 Vue 侧 layouts/src/authentication/toolbar.vue */
export function Toolbar({
  toolbarList = ['color', 'language', 'layout', 'theme'],
}: ToolbarProps) {
  const { preferences } = usePreferences();
  const showColor = toolbarList.includes('color');
  const showLayout = toolbarList.includes('layout');
  const showLanguage = toolbarList.includes('language');
  const showTheme = toolbarList.includes('theme');

  return (
    <div
      className={cn(
        'absolute top-4 right-2 z-10 flex-center',
        toolbarList.length > 1 && 'rounded-3xl bg-accent px-3 py-1',
      )}
    >
      <div className="hidden md:flex">
        {showColor ? <AuthenticationColorToggle /> : null}
        {showLayout ? <AuthenticationLayoutToggle /> : null}
      </div>
      {showLanguage && preferences.widget.languageToggle ? (
        <LanguageToggle />
      ) : null}
      {showTheme && preferences.widget.themeToggle ? <ThemeToggle /> : null}
    </div>
  );
}
