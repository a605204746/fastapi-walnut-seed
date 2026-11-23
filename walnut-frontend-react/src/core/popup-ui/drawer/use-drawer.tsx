import type { ComponentType } from 'react';

import type { DrawerRendererProps } from './drawer';
import type { DrawerApi, DrawerApiOptions } from './drawer-api';

import { useCallback, useRef } from 'react';

import { DrawerRenderer } from './drawer';
import { DrawerApi as DrawerApiClass } from './drawer-api';

/**
 * 对应 Vue 侧 `use-drawer.ts`：保留 `useWalnutDrawer(options) → [Drawer, drawerApi]` 签名，
 * 使 8 个使用 `useWalnutDrawer` 的业务页（dept / client / menu / post / role /
 * role-assign / user / operlog）可以机械搬运。
 *
 * 与同目录 `use-modal.tsx` 同构。Vue 侧的 `connectedComponent` 父子挂载
 * （provide/inject 传递 api + `reCreateDrawer` 重建）在 React 下改由
 * 「抽屉组件自持 useWalnutDrawer + useWalnutForm，父组件通过 ref 命令式打开」实现，
 * 因此 `connectedComponent` 选项被接受但忽略。
 */
/** 对外暴露的 Drawer 组件 props（api 由 hook 内部注入） */
export type WalnutDrawerComponentProps = Omit<DrawerRendererProps, 'api'>;

/** 全局默认配置（对应 Vue 侧 `DEFAULT_DRAWER_PROPS`） */
const DEFAULT_DRAWER_PROPS: Partial<DrawerApiOptions> = {};

/**
 * 设置全局默认抽屉配置（对应 Vue 侧同名函数）。
 *
 * ⚠️ 必须在创建任何抽屉**之前**调用（即 bootstrap 阶段）：默认值在 `DrawerApi`
 * 构造时并入 store，之后再改不会影响已创建的实例。Vue 侧同样如此。
 */
export function setDefaultDrawerProps(props: Partial<DrawerApiOptions>) {
  Object.assign(DEFAULT_DRAWER_PROPS, props);
}

export function useWalnutDrawer(
  options: DrawerApiOptions = {},
): [ComponentType<WalnutDrawerComponentProps>, DrawerApi] {
  const mergedOptions = { ...DEFAULT_DRAWER_PROPS, ...options };

  const apiRef = useRef<DrawerApi | undefined>(undefined);
  if (!apiRef.current) {
    apiRef.current = new DrawerApiClass(mergedOptions);
  }
  // 每次渲染同步最新回调，避免闭包过期
  apiRef.current.updateCallbacks(mergedOptions);
  const api = apiRef.current;

  const Drawer = useCallback(
    (props: WalnutDrawerComponentProps) => (
      <DrawerRenderer {...props} api={api} />
    ),
    [api],
  );

  return [Drawer, api];
}
