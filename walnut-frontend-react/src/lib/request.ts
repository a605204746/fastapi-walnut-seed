import type { AxiosResponse, InternalAxiosRequestConfig } from 'axios';

import {
  BUSINESS_SUCCESS_CODE,
  LOGIN_PATH,
  UNAUTHORIZED_CODE,
} from '@/core/constants';
import { $t } from '@/core/locales';
import { getPreferences } from '@/core/preferences';
import { RequestClient, stringify } from '@/core/request';
import { resetAllStores, useAccessStore } from '@/core/stores';
import {
  AesEncryption,
  decodeBase64,
  encodeBase64,
  randomStr,
  RsaEncryption,
} from '@/core/utils';
import { message as staticMessage } from 'antd';

import { getMessageApi } from '../services/message';
import { appConfig } from './config';

/**
 * 非对称加密实现（前端持「请求加密公钥」+「响应解密私钥」）。
 * 可换成 Sm2Encryption，后端需同步替换密钥对。
 */
const asymmetricEncryption = new RsaEncryption({
  privateKey: appConfig.rsaPrivateKey,
  publicKey: appConfig.rsaPublicKey,
});

/** 对称加密实现（AES-ECB/PKCS7，密钥为每请求一次性生成的 32 位随机串） */
const symmetricEncryption = new AesEncryption();

export const requestClient = new RequestClient({
  baseURL: appConfig.apiURL,
  responseReturn: 'data',
  timeout: 10_000,
});

/** 业务失败时抛出，便于调用方区分网络错误与业务错误 */
export class BusinessException extends Error {
  constructor(
    public code: number,
    message: string,
  ) {
    super(message);
    this.name = 'BusinessException';
  }
}

function showMessage(type: 'error' | 'success' | 'warning', content: string) {
  if (!content) return;
  const api = getMessageApi();
  if (api) {
    api.message[type](content);
  } else {
    staticMessage[type](content);
  }
}

let isLoggingOut = false;

/**
 * 401 未授权处理：清空登录态并跳回登录页（对应 Vue 侧 handleUnauthorizedLogout）。
 * PoC 用 window.location 跳转，完整 router 联动在阶段 ④ 接入。
 */
function handleUnauthorized() {
  if (useAccessStore.getState().loginExpired) return;
  if (getPreferences().app.loginExpiredMode === 'modal' && useAccessStore.getState().isAccessChecked) {
    useAccessStore.getState().setLoginExpired(true);
    return;
  }
  if (isLoggingOut) return;
  isLoggingOut = true;
  resetAllStores();
  showMessage('error', $t('http.loginTimeout'));
  const redirect = encodeURIComponent(
    window.location.pathname + window.location.search,
  );
  window.location.assign(`${LOGIN_PATH}?redirect=${redirect}`);
}

// ---- 请求拦截器：token / 语言 / clientId / 加密 / 参数序列化 ----
requestClient.addRequestInterceptor({
  fulfilled: (config: InternalAxiosRequestConfig) => {
    const language = (getPreferences().app.locale ?? 'zh-CN').replace('-', '_');
    config.headers['Accept-Language'] = language;
    config.headers['Content-Language'] = language;
    config.headers.ClientID = appConfig.clientId;

    // token（withToken 默认开启）
    if (config.withToken !== false) {
      const token = useAccessStore.getState().accessToken;
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
    }

    // GET/DELETE 数组参数用 repeat 格式（后端 Map 参数需要）
    const method = (config.method ?? '').toUpperCase();
    if (['DELETE', 'GET'].includes(method) && config.params) {
      config.paramsSerializer = (params: any) =>
        stringify(params, { arrayFormat: 'repeat' });
    }

    const shouldEncrypt =
      appConfig.enableEncrypt && config.encrypt && ['POST', 'PUT'].includes(method);

    if (shouldEncrypt) {
      const key = randomStr(32);
      config.headers['encrypt-key'] = asymmetricEncryption.encrypt(
        encodeBase64(key),
      );
      // axios 默认会给字符串 body 加引号，RSA 能解但 SM2 不行，这里强制透传原始内容
      config.transformRequest = [(data: unknown) => data];
      config.data =
        typeof config.data === 'string'
          ? symmetricEncryption.encrypt(config.data, key)
          : symmetricEncryption.encrypt(JSON.stringify(config.data ?? ''), key);
      config.headers['Content-Type'] = 'application/json;charset=utf-8';
    }

    return config;
  },
});

// ---- 响应拦截器 1：解密（必须排在信封拆包之前）----
requestClient.addResponseInterceptor({
  fulfilled: (response: AxiosResponse) => {
    const encryptKey = response.headers?.['encrypt-key'];
    if (encryptKey) {
      try {
        const secret = decodeBase64(asymmetricEncryption.decrypt(encryptKey));
        response.data = JSON.parse(
          symmetricEncryption.decrypt(response.data as unknown as string, secret),
        );
      } catch (error) {
        // 密钥错配时 crypto-js 抛 Malformed UTF-8 data，转成可定位信息
        throw new BusinessException(
          -1,
          '响应解密失败：请检查前后端 RSA 密钥是否配对',
        );
      }
    }
    return response;
  },
});

// ---- 响应拦截器 2：信封拆包 + 业务码 + 消息 + 401 ----
requestClient.addResponseInterceptor({
  fulfilled: (response: AxiosResponse) => {
    const config = response.config as InternalAxiosRequestConfig;

    if (config.isReturnNativeResponse) {
      return response;
    }

    // blob / 非 JSON：直接返回，不做信封判断
    if (
      config.responseType === 'blob' ||
      config.isTransformResponse === false
    ) {
      return response.data;
    }

    const body = response.data;
    if (!body || typeof body !== 'object' || !('code' in body)) {
      return body;
    }

    const { code, data, msg, ...other } = body as {
      code: number;
      data?: any;
      msg?: string;
    };

    if (code === UNAUTHORIZED_CODE) {
      handleUnauthorized();
      throw new BusinessException(code, msg ?? $t('http.loginTimeout'));
    }

    if (code === BUSINESS_SUCCESS_CODE) {
      if (config.successMessageMode === 'message') {
        showMessage('success', msg || $t('http.operationSuccess'));
      }
      return data === undefined ? other : data;
    }

    if (config.errorMessageMode !== 'none') {
      showMessage('error', msg ?? $t('http.apiRequestFailed'));
    }
    throw new BusinessException(code, msg ?? '');
  },
  rejected: (error: any) => {
    const status = error?.response?.status;
    const msg: string = error?.response?.data?.msg ?? '';
    if (status === UNAUTHORIZED_CODE) {
      handleUnauthorized();
    } else if (status && error?.config?.errorMessageMode !== 'none') {
      showMessage('error', msg || error?.message || $t('http.apiRequestFailed'));
    }
    return Promise.reject(error);
  },
});

/**
 * 统一请求对象（对应 Vue 侧 utils/http 的 request）。
 * deleteWithMsg/putWithMsg/postWithMsg 会在成功时弹出后端返回的 msg。
 */
export const request = {
  get: <T = any>(url: string, config?: any) =>
    requestClient.get<T>(url, config),
  post: <T = any>(url: string, data?: any, config?: any) =>
    requestClient.post<T>(url, data, config),
  put: <T = any>(url: string, data?: any, config?: any) =>
    requestClient.put<T>(url, data, config),
  delete: <T = any>(url: string, data?: any, config?: any) =>
    requestClient.request<T>(url, { ...config, data, method: 'DELETE' }),
  getWithMsg: <T = any>(url: string, config?: any) =>
    requestClient.get<T>(url, { ...config, successMessageMode: 'message' }),
  postWithMsg: <T = any>(url: string, data?: any, config?: any) =>
    requestClient.post<T>(url, data, {
      ...config,
      successMessageMode: 'message',
    }),
  putWithMsg: <T = any>(url: string, data?: any, config?: any) =>
    requestClient.put<T>(url, data, {
      ...config,
      successMessageMode: 'message',
    }),
  deleteWithMsg: <T = any>(url: string, data?: any, config?: any) =>
    requestClient.request<T>(url, {
      ...config,
      data,
      method: 'DELETE',
      successMessageMode: 'message',
    }),
};
