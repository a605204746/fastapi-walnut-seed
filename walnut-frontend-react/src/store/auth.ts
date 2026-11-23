import type { UserInfo } from '@/core/types';

import type { AuthApi } from '@/api/core/auth';

import { doLogout, loginApi, seeConnectionClose } from '@/api/core/auth';
import { getUserInfoApi } from '@/api/core/user';
import { LOGIN_PATH } from '@/core/constants';
import { resetAllStores, useAccessStore, useUserStore } from '@/core/stores';
import { create } from 'zustand';

import { getMessageApi } from '@/services/message';
import { useDictStore } from './dict';
import { useTenantStore } from './tenant';

interface AuthState {
  loginLoading: boolean;
  authLogin(
    params: AuthApi.LoginParams,
  ): Promise<{ userInfo: null | UserInfo }>;
  fetchUserInfo(): Promise<UserInfo>;
  logout(redirect?: boolean): Promise<void>;
}

/**
 * 认证 store（对应 Vue 侧 store/auth.ts，Pinia → Zustand）。
 * 保留 authLogin / fetchUserInfo / logout 的语义与时序：
 * loginApi → setAccessToken → fetchUserInfo → setUserInfo + setAccessCodes。
 */
export const useAuthStore = create<AuthState>()((set, get) => ({
  loginLoading: false,

  async authLogin(params) {
    let userInfo: null | UserInfo = null;
    try {
      set({ loginLoading: true });
      const { access_token } = await loginApi(params);

      const accessStore = useAccessStore.getState();
      accessStore.setAccessToken(access_token);
      accessStore.setRefreshToken(access_token);

      userInfo = await get().fetchUserInfo();

      if (accessStore.loginExpired) {
        accessStore.setLoginExpired(false);
      }

      if (userInfo?.realName) {
        getMessageApi()?.notification.success({
          // ⚠️ antd 6 把 notification 的 `message` 改名为 `title`（`message` 已弃用并会告警）
          description: `欢迎回来:${userInfo.realName}`,
          title: '登录成功',
        });
      }
    } finally {
      set({ loginLoading: false });
    }
    return { userInfo };
  },

  async fetchUserInfo() {
    const backUserInfo = await getUserInfoApi();
    if (!backUserInfo) {
      throw new Error('获取用户信息失败.');
    }
    const { permissions = [], roles = [], user } = backUserInfo;
    const userInfo: UserInfo = {
      avatar: user.avatar ?? '',
      email: user.email ?? '',
      permissions,
      realName: user.nickName,
      roles,
      userId: user.id,
      username: user.userName,
    };
    useUserStore.getState().setUserInfo(userInfo);
    useAccessStore.getState().setAccessCodes(permissions);
    // 切换租户/重新登录时需要重新加载字典
    useDictStore.getState().resetCache();
    return userInfo;
  },

  async logout(redirect = true) {
    try {
      await Promise.all([seeConnectionClose(), doLogout()]);
    } catch (error) {
      console.error(error);
    } finally {
      resetAllStores();
      useTenantStore.getState().setSelected(undefined);
      useAccessStore.getState().setLoginExpired(false);
      if (redirect) {
        const current = window.location.pathname + window.location.search;
        window.location.assign(
          `${LOGIN_PATH}?redirect=${encodeURIComponent(current)}`,
        );
      }
    }
  },
}));
