import type { ResultProps } from 'antd';

import { getPreferences, usePreferences } from '@/core/preferences';

import { Button, Result } from 'antd';
import { useNavigate } from 'react-router';

import { $t } from '@/locales';

interface FallbackProps {
  description?: string;
  /** 是否显示「返回首页」按钮 */
  showBack?: boolean;
  status: ResultProps['status'];
  title?: string;
}

// 与 Vue fallback.vue 的 getDefaultTitle/getDefaultDescription 对等
const TITLE_KEYS: Record<string, string> = {
  '403': 'ui.fallback.forbidden',
  '404': 'ui.fallback.pageNotFound',
  '500': 'ui.fallback.internalError',
  'coming-soon': 'ui.fallback.comingSoon',
  offline: 'ui.fallback.offlineError',
};
const DESC_KEYS: Record<string, string> = {
  '403': 'ui.fallback.forbiddenDesc',
  '404': 'ui.fallback.pageNotFoundDesc',
  '500': 'ui.fallback.internalErrorDesc',
  offline: 'ui.fallback.offlineErrorDesc',
};

/**
 * 兜底页容器，复用 Vue 端原始 SVG 插画及主题令牌。
 * 未显式传 title/description 时按 status 取 $t 文案（Vue 同语义）。
 */
export function Fallback({
  description,
  showBack = true,
  status,
  title,
}: FallbackProps) {
  const navigate = useNavigate();
  // 订阅偏好：locale 变化时重译默认文案
  usePreferences();
  const key = String(status);
  const resolvedTitle = title ?? (TITLE_KEYS[key] ? $t(TITLE_KEYS[key]) : undefined);
  const resolvedDescription =
    description ?? (DESC_KEYS[key] ? $t(DESC_KEYS[key]) : undefined);

  return (
    <div className="flex-center h-full w-full">
      <Result
        extra={
          showBack ? (
            <Button
              onClick={() =>
                navigate(getPreferences().app.defaultHomePath, { replace: true })
              }
              type="primary"
            >
              {$t('common.backToHome')}
            </Button>
          ) : undefined
        }
        icon={<img className="mx-auto h-64 max-w-full" src={`/fallback/icon-${status}.svg`} alt="" />}
        status={status}
        subTitle={resolvedDescription}
        title={resolvedTitle}
      />
    </div>
  );
}

export default Fallback;
