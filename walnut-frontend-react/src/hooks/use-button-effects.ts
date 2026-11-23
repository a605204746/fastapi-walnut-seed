import { useEffect } from 'react';
import { usePreferences } from '@/core/preferences';
export function useButtonEffects() {
  const { preferences } = usePreferences();
  const mode = preferences.theme.buttonWaveMode;
  useEffect(() => {
    if (mode === 'Default' || mode === 'Disabled') return;
    function click(event: MouseEvent) {
      const target =
        event.target instanceof Element
          ? event.target.closest<HTMLElement>('button:not(:disabled)')
          : null;
      if (!target) return;
      const effects = {
        Happy: [{ transform: 'scale(1)' }, { transform: 'scale(1.08)' }, { transform: 'scale(1)' }],
        Inset: [
          { boxShadow: 'inset 0 0 0 0 hsl(var(--primary) / .5)' },
          { boxShadow: 'inset 0 0 0 12px transparent' },
        ],
        Shake: [
          { transform: 'translateX(0)' },
          { transform: 'translateX(-4px)' },
          { transform: 'translateX(4px)' },
          { transform: 'translateX(0)' },
        ],
      };
      if (mode in effects)
        target.animate(effects[mode as keyof typeof effects], {
          duration: 300,
          easing: 'ease-out',
        });
    }
    document.addEventListener('click', click);
    return () => document.removeEventListener('click', click);
  }, [mode]);
}
