import { useEffect, useState } from 'react';

import { LOGIN_PATH } from '@vben/constants';
import { useAccessStore } from '@vben/stores';

import { Button, Result, Spin } from 'antd';
import { Navigate, useLocation } from 'react-router';

import { $t } from '@/locales';
import { ensureAccessRoutes } from '@/router/guard';

import { BasicLayout } from './BasicLayout';

/**
 * Root 路由组件：鉴权 + 动态路由 bootstrap + 后台外壳。
 *
 * 对应 Vue 侧 `router/guard.ts` 的 `setupAccessGuard`。差异在于 Vue 用
 * `router.beforeEach` 全局前置守卫，React Router 7 没有等价物——守卫只能落在 Root
 * 路由的组件里。Root 带一条静态 `*` 子路由，因此**任何**路径都会先经过本组件，
 * 未登录时重定向到登录页、已登录时先把动态路由 patch 进 Root 再渲染 Outlet。
 */
export function RootLayout() {
  const accessToken = useAccessStore((state) => state.accessToken);
  const isAccessChecked = useAccessStore((state) => state.isAccessChecked);
  const location = useLocation();
  const [error, setError] = useState<null | string>(null);
  const [retryToken, setRetryToken] = useState(0);

  useEffect(() => {
    if (!accessToken || isAccessChecked) {
      return;
    }
    let cancelled = false;
    setError(null);
    ensureAccessRoutes().catch((cause: unknown) => {
      if (!cancelled) {
        setError(cause instanceof Error ? cause.message : String(cause));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [accessToken, isAccessChecked, retryToken]);

  if (!accessToken) {
    const redirect = encodeURIComponent(location.pathname + location.search);
    return <Navigate replace to={`${LOGIN_PATH}?redirect=${redirect}`} />;
  }

  // 菜单拉取失败（后端不可用 / token 失效）时给出可重试的错误态，
  // 否则用户只会看到永久 loading。token 失效的场景由请求拦截器负责跳登录。
  if (error) {
    return (
      <div className="flex-center h-screen">
        <Result
          extra={
            <Button
              onClick={() => setRetryToken((token) => token + 1)}
              type="primary"
            >
              {$t('common.refresh')}
            </Button>
          }
          status="error"
          subTitle={error}
        />
      </div>
    );
  }

  if (!isAccessChecked) {
    return (
      <div className="flex-center h-screen">
        <Spin size="large" />
      </div>
    );
  }

  return <BasicLayout />;
}
