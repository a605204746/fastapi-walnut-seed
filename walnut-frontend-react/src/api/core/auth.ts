import { appConfig } from '@/lib/config';
import { request } from '@/lib/request';

export type GrantType = 'password' | 'sms' | 'social' | 'email' | 'xcx';

export namespace AuthApi {
  export interface BaseLoginParams {
    clientId?: string;
    grantType: GrantType;
    tenantId: string;
  }

  export interface OAuthLoginParams extends BaseLoginParams {
    socialCode: string;
    socialState: string;
    source: string;
  }

  export interface SimpleLoginParams extends BaseLoginParams {
    code?: string;
    password: string;
    username: string;
    uuid?: string;
  }

  export type LoginParams = OAuthLoginParams | SimpleLoginParams;

  export interface LoginResult {
    access_token: string;
    client_id: string;
    expire_in: number;
  }
}

/** 登录 */
export async function loginApi(data: AuthApi.LoginParams) {
  return request.post<AuthApi.LoginResult>(
    '/auth/login',
    { ...data, clientId: appConfig.clientId },
    { encrypt: true },
  );
}

/** 用户登出 */
export function doLogout() {
  return request.post<void>('/auth/logout');
}

/** 关闭 sse 连接 */
export function seeConnectionClose() {
  if (!appConfig.sseEnable) {
    return Promise.resolve();
  }
  return request.get<void>('/resource/sse/close');
}

export interface TenantOption {
  companyName: string;
  domain?: string;
  tenantId: string;
}

export interface TenantResp {
  tenantEnabled: boolean;
  voList: TenantOption[];
}

/** 获取租户列表 下拉框使用 */
export function tenantList() {
  return request.get<TenantResp>('/auth/tenant/list');
}

/** 绑定第三方账号，返回跳转 url */
export function authBinding(source: string, tenantId: string) {
  return request.get<string>(`/auth/binding/${source}`, {
    params: { domain: window.location.host, tenantId },
  });
}

/** oauth 授权回调 */
export function authCallback(data: AuthApi.OAuthLoginParams) {
  return request.post<void>('/auth/social/callback', data);
}

/**
 * 取消绑定
 * @param id id
 */
export function authUnbinding(id: string) {
  return request.deleteWithMsg<void>(`/auth/unlock/${id}`);
}
