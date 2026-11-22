import type { AppRouteObject, MenuRecordRaw } from '@walnut-core/typings';

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

import { storeNamespace } from '../namespace';

type AccessToken = null | string;

interface AccessState {
  /** 权限码 */
  accessCodes: string[];
  /** 可访问的菜单列表 */
  accessMenus: MenuRecordRaw[];
  /** 可访问的路由列表 */
  accessRoutes: AppRouteObject[];
  /** 登录 accessToken */
  accessToken: AccessToken;
  /** 是否已经检查过权限 */
  isAccessChecked: boolean;
  /** 是否锁屏状态 */
  isLockScreen: boolean;
  /** 锁屏密码 */
  lockScreenPassword?: string;
  /** 登录是否过期 */
  loginExpired: boolean;
  /** refresh accessToken */
  refreshToken: AccessToken;
}

interface AccessActions {
  getMenuByPath(path: string): MenuRecordRaw | undefined;
  lockScreen(password: string): void;
  setAccessCodes(codes: string[]): void;
  setAccessMenus(menus: MenuRecordRaw[]): void;
  setAccessRoutes(routes: AppRouteObject[]): void;
  setAccessToken(token: AccessToken): void;
  setIsAccessChecked(isAccessChecked: boolean): void;
  setLoginExpired(loginExpired: boolean): void;
  setRefreshToken(token: AccessToken): void;
  unlockScreen(): void;
  $reset(): void;
}

export type AccessStore = AccessState & AccessActions;

const initialState: AccessState = {
  accessCodes: [],
  accessMenus: [],
  accessRoutes: [],
  accessToken: null,
  isAccessChecked: false,
  isLockScreen: false,
  lockScreenPassword: undefined,
  loginExpired: false,
  refreshToken: null,
};

function findMenu(
  menus: MenuRecordRaw[],
  path: string,
): MenuRecordRaw | undefined {
  for (const menu of menus) {
    if (menu.path === path) {
      return menu;
    }
    if (menu.children) {
      const matched = findMenu(menu.children, path);
      if (matched) {
        return matched;
      }
    }
  }
  return undefined;
}

/**
 * @zh_CN 访问权限相关（对应 Vue 侧 packages/stores/src/modules/access.ts）。
 *
 * 组件内：`const { accessToken } = useAccessStore();`（自动订阅）
 * 命令式（请求拦截器等）：`useAccessStore.getState().accessToken`
 */
export const useAccessStore = create<AccessStore>()(
  persist(
    (set, get) => ({
      ...initialState,
      $reset: () => set({ ...initialState }),
      getMenuByPath: (path) => findMenu(get().accessMenus, path),
      lockScreen: (password) =>
        set({ isLockScreen: true, lockScreenPassword: password }),
      setAccessCodes: (codes) => set({ accessCodes: codes }),
      setAccessMenus: (menus) => set({ accessMenus: menus }),
      setAccessRoutes: (routes) => set({ accessRoutes: routes }),
      setAccessToken: (token) => set({ accessToken: token }),
      setIsAccessChecked: (isAccessChecked) => set({ isAccessChecked }),
      setLoginExpired: (loginExpired) => set({ loginExpired }),
      setRefreshToken: (token) => set({ refreshToken: token }),
      unlockScreen: () =>
        set({ isLockScreen: false, lockScreenPassword: undefined }),
    }),
    {
      skipHydration: true, name: `${storeNamespace.prefix}-core-access`,
      // 与 Vue 侧 persist.pick 一致：只持久化这几个字段
      partialize: (state) => ({
        accessCodes: state.accessCodes,
        accessToken: state.accessToken,
        isLockScreen: state.isLockScreen,
        lockScreenPassword: state.lockScreenPassword,
        refreshToken: state.refreshToken,
      }),
      version: 1,
    },
  ),
);
