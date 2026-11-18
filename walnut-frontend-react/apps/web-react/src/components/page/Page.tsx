import type { CSSProperties, ReactNode } from 'react';

import { usePreferences } from '@vben/preferences';
import { CSS_VARIABLE_LAYOUT_CONTENT_HEIGHT } from '@vben-core/shared/constants';
import { cn } from '@vben-core/shared/utils';

export interface PageProps {
  autoContentHeight?: boolean;
  children?: ReactNode;
  contentClass?: string;
  description?: ReactNode;
  footerClass?: string;
  headerClass?: string;
  heightOffset?: number | string;
  title?: ReactNode;
}

/**
 * 页面容器（对应 Vue 侧 common-ui/src/components/page/page.vue）。
 *
 * PoC 简化：config 页无 title/description/footer，autoContentHeight 直接用
 * `--vben-content-height` 计算内容高度（该变量由 BasicLayout 的内容区写入并继承）。
 */
export function Page({
  autoContentHeight = false,
  children,
  contentClass,
  description,
  heightOffset = 0,
  title,
}: PageProps) {
  const { preferences } = usePreferences();
  const hasHeader = title || description;

  const contentStyle: CSSProperties = autoContentHeight
    ? {
        height: `calc(var(${CSS_VARIABLE_LAYOUT_CONTENT_HEIGHT}) - ${
          typeof heightOffset === 'number' ? `${heightOffset}px` : heightOffset
        })`,
        overflowY: 'auto',
      }
    : {};

  // 内容紧凑模式（app.contentCompact = compact）：限宽 + 居中，对应 Vue 侧 content 偏好
  const compactStyle: CSSProperties =
    preferences.app.contentCompact === 'compact'
      ? { marginInline: 'auto', maxWidth: preferences.app.contentCompactWidth }
      : {};

  return (
    <div className="relative flex min-h-full flex-col" style={compactStyle}>
      {hasHeader ? (
        <div className="border-border bg-card relative flex items-end border-b px-6 py-4">
          <div className="flex-auto">
            {title ? (
              <div className="mb-2 flex text-lg font-semibold">{title}</div>
            ) : null}
            {description ? (
              <p className="text-muted-foreground">{description}</p>
            ) : null}
          </div>
        </div>
      ) : null}
      <div className={cn('h-full p-4', contentClass)} style={contentStyle}>
        {children}
      </div>
    </div>
  );
}
