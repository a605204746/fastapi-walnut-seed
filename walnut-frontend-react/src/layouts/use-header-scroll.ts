import { useEffect, useRef, useState } from 'react';
import { usePreferences } from '@/core/preferences';
export function useHeaderScroll() {
  const { preferences } = usePreferences();
  const [offset, setOffset] = useState(0);
  const previous = useRef(0);
  const { mode, height } = preferences.header;
  useEffect(() => {
    setOffset(0);
    function scroll(event: Event) {
      const target = event.target;
      if (!(target instanceof HTMLElement) || !target.closest('main')) return;
      const top = target.scrollTop;
      if (mode === 'static') setOffset(-Math.min(top, height));
      else if (mode === 'auto-scroll') setOffset(top > height ? -height : 0);
      else if (mode === 'auto') setOffset(top > height && top > previous.current ? -height : 0);
      previous.current = top;
    }
    document.addEventListener('scroll', scroll, true);
    return () => document.removeEventListener('scroll', scroll, true);
  }, [mode, height]);
  return offset;
}
