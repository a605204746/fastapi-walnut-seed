import type { ReactNode } from 'react';

import { LOGIN_PATH } from '@vben/constants';
import { useAccessStore } from '@vben/stores';
import {
  createBrowserRouter,
  Navigate,
  useLocation,
} from 'react-router';

import { AuthLayout } from '@/layouts/AuthLayout';
import { BasicLayout } from '@/layouts/BasicLayout';
import { LoginPage } from '@/views/_core/authentication/login';
import { ConfigPage } from '@/views/system/config';

/**
 * 布局路由守卫（对应 Vue 侧 router/guard.ts 的 token 检查）。
 * 无 token 时跳登录页并带上 redirect；完整动态菜单 + patchRoutes 在阶段 ④ 落地。
 */
function RequireAuth({ children }: { children: ReactNode }) {
  const token = useAccessStore((state) => state.accessToken);
  const location = useLocation();
  if (!token) {
    const redirect = encodeURIComponent(
      location.pathname + location.search,
    );
    return <Navigate replace to={`${LOGIN_PATH}?redirect=${redirect}`} />;
  }
  return <>{children}</>;
}

function NotFound() {
  return (
    <div className="flex-center h-screen flex-col gap-2">
      <div className="text-2xl font-semibold">404</div>
      <div className="text-muted-foreground text-sm">页面不存在</div>
    </div>
  );
}

/**
 * PoC 路由：静态挂载登录页与 system/config 页。
 * 后端动态菜单驱动（backMenuToRouteObjects + patchRoutes）在阶段 ④ 接入。
 */
export const router = createBrowserRouter([
  {
    children: [{ element: <LoginPage />, path: 'login' }],
    path: '/auth',
    element: <AuthLayout />,
  },
  {
    children: [
      { element: <Navigate replace to="/system/config" />, index: true },
      { element: <ConfigPage />, path: 'system/config' },
    ],
    element: (
      <RequireAuth>
        <BasicLayout />
      </RequireAuth>
    ),
    path: '/',
  },
  { element: <NotFound />, path: '*' },
]);
