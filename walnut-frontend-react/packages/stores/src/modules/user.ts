import { create } from 'zustand';

interface BasicUserInfo {
  [key: string]: any;
  /** 头像 */
  avatar: string;
  /** 邮箱 */
  email: string;
  /** 用户权限 */
  permissions: string[];
  /** 用户昵称 */
  realName: string;
  /** 用户角色 */
  roles: string[];
  /** 用户id */
  userId: number | string;
  /** 用户名 */
  username: string;
}

interface UserState {
  /** 用户信息 */
  userInfo: BasicUserInfo | null;
  /** 用户角色 */
  userRoles: string[];
}

interface UserActions {
  setUserInfo(userInfo: BasicUserInfo | null): void;
  setUserRoles(roles: string[]): void;
  $reset(): void;
}

export type UserStore = UserState & UserActions;

const initialState: UserState = {
  userInfo: null,
  userRoles: [],
};

/**
 * @zh_CN 用户信息相关（对应 Vue 侧 packages/stores/src/modules/user.ts）。
 */
export const useUserStore = create<UserStore>()((set) => ({
  ...initialState,
  $reset: () => set({ ...initialState }),
  setUserInfo: (userInfo) =>
    set({ userInfo, userRoles: userInfo?.roles ?? [] }),
  setUserRoles: (roles) => set({ userRoles: roles }),
}));
