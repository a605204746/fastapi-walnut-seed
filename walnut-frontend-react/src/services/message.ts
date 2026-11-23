/**
 * antd `App.useApp()` 的全局桥接（对应 Vue 侧 utils/popup.ts 的 window.message/modal）。
 *
 * 命令式代码（请求拦截器、store）无法直接使用 hook，因此在 <AntdApp> 内的组件里
 * 调用 `initMessageApi(App.useApp())` 注册一次，之后全局通过本模块使用。
 * 未初始化时回退到 antd 静态方法（拿不到 ConfigProvider 主题，但不会崩）。
 */
export interface MessageApiLike {
  message: {
    /** 关闭全部提示（对应 Vue 侧 window.message.destroy()） */
    destroy?: () => void;
    error: (content: any, duration?: number) => void;
    /** 加载态提示，参数形态与 antd 一致，可传配置对象 */
    loading?: (config: any) => void;
    success: (content: any, duration?: number) => void;
    warning: (content: any, duration?: number) => void;
  };
  modal: {
    confirm: (config: any) => void;
    error: (config: any) => void;
  };
  notification: {
    error: (config: any) => void;
    success: (config: any) => void;
  };
}

let api: MessageApiLike | undefined;

export function initMessageApi(instance: MessageApiLike) {
  api = instance;
}

export function getMessageApi(): MessageApiLike | undefined {
  return api;
}
