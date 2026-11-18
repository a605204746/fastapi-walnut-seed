import { useEffect, useRef, useState } from 'react';

import { usePreferences } from '@vben/preferences';
import { Spin } from 'antd';
import { useLocation } from 'react-router';

/**
 * 页面切换反馈（preferences.transition.progress / loading）。
 * 对应 Vue 侧 nprogress 顶部进度条 + 内容区 Loading。
 * 只在「后续」路由切换时显示，跳过首次挂载（初始加载不闪遮罩，
 * 也保证像素门禁截图不受瞬时遮罩影响）。
 */
export function RouteFeedback() {
  const { preferences } = usePreferences();
  const location = useLocation();
  const [active, setActive] = useState(false);
  const skipFirst = useRef(true);

  useEffect(() => {
    if (skipFirst.current) {
      skipFirst.current = false;
      return;
    }
    setActive(true);
    const timer = setTimeout(() => setActive(false), 450);
    return () => clearTimeout(timer);
  }, [location.pathname]);

  const { loading, progress } = preferences.transition;
  if (!active) return null;

  return (
    <>
      {progress ? <div className="vben-progress-bar" /> : null}
      {loading ? (
        <div className="bg-background/60 absolute inset-0 z-50 flex items-center justify-center">
          <Spin />
        </div>
      ) : null}
    </>
  );
}
