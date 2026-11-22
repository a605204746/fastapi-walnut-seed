import { DEFAULT_TENANT_ID } from '@walnut/constants';
import { create } from 'zustand';

import { authBinding } from '@/api/core/auth';

export interface BindItem {
  description: string;
  icon: string;
  source: string;
  title: string;
}

/**
 * 登录页 tenantId 的共享状态（对应 Vue 侧 createGlobalState 的 useLoginTenantId）。
 * 个人中心也会用到，故用模块级 store 共享。
 */
export const useLoginTenantStore = create<{
  loginTenantId: string;
  setLoginTenantId(id: string): void;
}>((set) => ({
  loginTenantId: DEFAULT_TENANT_ID,
  setLoginTenantId: (id) => set({ loginTenantId: id }),
}));

/** 绑定授权（对应 Vue 侧 handleAuthBinding） */
export async function handleAuthBinding(source: string) {
  const { loginTenantId } = useLoginTenantStore.getState();
  const href = await authBinding(source, loginTenantId);
  window.location.href = href;
}

/**
 * 账号绑定 list（对应 Vue 侧 accountBindList）。
 * 图标改用 public/icons 下的 svg（Vue 侧为 iconify + svg 混合），
 * 视觉等价；github 由 fa-github-alt 换为 github.svg，像素门禁会量化差异。
 */
export const accountBindList: BindItem[] = [
  {
    description: '绑定Github账号',
    icon: '/icons/github.svg',
    source: 'github',
    title: 'Github',
  },
  {
    description: '绑定MaxKey账号',
    icon: '/icons/max-key.svg',
    source: 'maxkey',
    title: 'MaxKey',
  },
  {
    description: '绑定topiam账号',
    icon: '/icons/topiam.svg',
    source: 'topiam',
    title: 'TopIAM',
  },
  {
    description: '绑定wechat账号',
    icon: '/icons/wechat.svg',
    source: 'wechat',
    title: 'Wechat',
  },
];
