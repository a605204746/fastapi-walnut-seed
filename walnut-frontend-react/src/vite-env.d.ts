/// <reference types="vite/client" />

/**
 * 扩展 axios 的请求配置。
 * 对应 Vue 侧 apps/web-antd/types/alova.d.ts 中的同名扩展。
 *
 * 注意：axios 1.21 起 AxiosRequestConfig 是 `<D = any, P = any>` 双泛型，
 * 接口合并要求类型参数逐字一致，否则 TS 报「所有声明必须有相同的类型参数」。
 */
declare module 'axios' {
  interface AxiosRequestConfig<D = any, P = any> {
    /**
     * 该请求是否走 RSA+AES 信封加密
     * （还需全局开关 VITE_GLOB_ENABLE_ENCRYPT 为 true，且方法为 POST/PUT）
     */
    encrypt?: boolean;
    /**
     * 是否在请求头带 Authorization: Bearer <token>
     */
    withToken?: boolean;
    /** 成功时是否弹出 message 提示（对应 Vue 侧 successMessageMode） */
    successMessageMode?: 'message' | 'none';
    /** 失败时是否弹出错误提示（对应 Vue 侧 errorMessageMode） */
    errorMessageMode?: 'message' | 'modal' | 'none';
    /** 是否返回原生响应（下载等场景） */
    isReturnNativeResponse?: boolean;
    /** 是否做信封拆包（false 时返回完整 body，供调用方自行判断 code） */
    isTransformResponse?: boolean;
  }
}

declare module '*.css';
declare module '*.scss';

export {};
