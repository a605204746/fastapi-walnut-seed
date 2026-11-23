import { useEffect, useState } from 'react';
import { updatePreferences } from '@/core/preferences';
export function useIsMobile() {
  const [mobile, setMobile] = useState(() => window.matchMedia('(max-width: 767px)').matches);
  useEffect(() => {
    const media = window.matchMedia('(max-width: 767px)');
    const update = () => {
      setMobile(media.matches);
      updatePreferences({ app: { isMobile: media.matches } });
    };
    update();
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }, []);
  return mobile;
}
