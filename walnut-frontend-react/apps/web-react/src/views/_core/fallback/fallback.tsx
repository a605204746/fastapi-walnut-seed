import type { ResultProps } from 'antd';

import { getPreferences } from '@vben/preferences';

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

/**
 * 兜底页容器，复用 Vue 端原始 SVG 插画及主题令牌。
 */
export function Fallback({
  description,
  showBack = true,
  status,
  title,
}: FallbackProps) {
  const navigate = useNavigate();

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
        subTitle={description}
        title={title}
      />
    </div>
  );
}

export default Fallback;
