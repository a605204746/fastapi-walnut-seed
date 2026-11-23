import type { CSSProperties } from 'react';

import { useEffect, useState } from 'react';

import { usePreferences } from '@/core/preferences';
import { WalnutIconButton } from '@/core/shadcn-ui';
import { Layout, Watermark } from 'antd';
import { Minimize2 } from 'lucide-react';

import { useUiStore } from '@/store/ui';

import { RouteFeedback } from './widgets/route-feedback';
import { RouteCache } from './RouteCache';

/** Measure the mounted content node, including when a layout switch replaces it. */
export function ContentArea() {
  const { isDark, preferences } = usePreferences();
  const maximized = useUiStore((state) => state.maximized);
  const toggleMaximized = useUiStore((state) => state.toggleMaximized);
  const [element, setElement] = useState<HTMLElement | null>(null);
  const [height, setHeight] = useState(0);

  useEffect(() => {
    if (!element) return;
    const update = () => setHeight(element.clientHeight);
    const observer = new ResizeObserver(update);
    observer.observe(element);
    update();
    return () => observer.disconnect();
  }, [element]);

  return (
    <Layout.Content
      className="relative overflow-hidden"
      ref={setElement}
      style={{ '--walnut-content-height': `${height}px`, minHeight: 0 } as CSSProperties}
    >
      <Watermark
        content={preferences.app.watermark
          ? preferences.app.watermarkContent || preferences.app.name
          : undefined}
        font={{ color: isDark ? 'rgba(255,255,255,0.15)' : 'rgba(0,0,0,0.15)', fontSize: 16 }}
        gap={[100, 100]}
        rotate={-22}
        style={{ height: '100%' }}
      >
        <div
          className={`h-full overflow-y-auto ${preferences.transition.enable
            ? `page-transition page-transition--${preferences.transition.name}` : ''}`}
        >
          <RouteCache />
        </div>
      </Watermark>
      {maximized ? (
        <WalnutIconButton
          aria-label="restore-layout"
          class="bg-header absolute right-4 top-2 z-10 shadow-sm"
          onClick={toggleMaximized}
        >
          <Minimize2 className="size-4" />
        </WalnutIconButton>
      ) : null}
      <RouteFeedback />
    </Layout.Content>
  );
}
