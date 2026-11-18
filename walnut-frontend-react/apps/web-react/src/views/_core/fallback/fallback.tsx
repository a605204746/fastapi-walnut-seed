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
 * 兜底页容器（对应 Vue 侧 `packages/effects/common-ui/src/ui/fallback/fallback.vue`）。
 *
 * 与 Vue 版的差异：Vue 侧每个状态配一张手绘 SVG 插画（清单 §6.2 的 894 行 B 类文件），
 * 这里先用 antd `Result` 自带图形；插画 SVG → JSX 的机械转换留到阶段 ⑥-b。
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
        status={status}
        subTitle={description}
        title={title}
      />
    </div>
  );
}

export default Fallback;
