import type { ComponentType, ReactNode } from 'react';

/**
 * 路由元信息（对应 Vue 侧 typings/src/vue-router.d.ts 的 RouteMeta）。
 *
 * 字段与 Vue 侧逐字一致，使 `views/**\/data.ts(x)`、widgets、tabbar 的判断逻辑
 * 可以从 Vue 侧机械搬运，不需要改字段名。
 */
interface RouteMeta {
  /**
   * 激活图标（菜单/tab）
   */
  activeIcon?: string;
  /**
   * 当前激活的菜单，有时候不想激活现有菜单，需要激活父级菜单时使用
   */
  activePath?: string;
  /**
   * 是否固定标签页
   * @default false
   */
  affixTab?: boolean;
  /**
   * 固定标签页的顺序
   * @default 0
   */
  affixTabOrder?: number;
  /**
   * 需要特定的角色标识才可以访问
   * @default []
   */
  authority?: string[];
  /**
   * 徽标
   */
  badge?: string;
  /**
   * 徽标类型
   */
  badgeType?: 'dot' | 'normal';
  /**
   * 徽标颜色
   */
  badgeVariants?:
    | 'default'
    | 'destructive'
    | 'primary'
    | 'success'
    | 'warning'
    | string;
  /**
   * 路由对应 dom 是否缓存起来
   */
  domCached?: boolean;
  /**
   * 路由的完整路径作为 key（默认 true）
   */
  fullPathKey?: boolean;
  /**
   * 当前路由的子级在菜单中不展现
   * @default false
   */
  hideChildrenInMenu?: boolean;
  /**
   * 当前路由在面包屑中不展现
   * @default false
   */
  hideInBreadcrumb?: boolean;
  /**
   * 当前路由在菜单中不展现
   * @default false
   */
  hideInMenu?: boolean;
  /**
   * 当前路由在标签页不展现
   * @default false
   */
  hideInTab?: boolean;
  /**
   * 图标（菜单/tab）
   */
  icon?: ComponentType<any> | string;
  /**
   * iframe 地址
   */
  iframeSrc?: string;
  /**
   * 忽略权限，直接可以访问
   * @default false
   */
  ignoreAccess?: boolean;
  /**
   * 开启缓存（React 侧由 `<Activity>` 承载，对应 Vue 的 KeepAlive）
   */
  keepAlive?: boolean;
  /**
   * 外链-跳转路径
   */
  link?: string;
  /**
   * 路由是否已经加载过
   */
  loaded?: boolean;
  /**
   * 标签页最大打开数量
   * @default -1
   */
  maxNumOfOpenTab?: number;
  /**
   * 菜单可以看到，但是访问会被重定向到 403
   */
  menuVisibleWithForbidden?: boolean;
  /**
   * 不使用基础布局（仅在顶级生效）
   */
  noBasicLayout?: boolean;
  /**
   * 在新窗口打开
   */
  openInNewWindow?: boolean;
  /**
   * 用于路由 -> 菜单排序
   */
  order?: number;
  /**
   * 菜单所携带的参数
   */
  query?: Record<string, any>;
  /**
   * 管理员切换租户 该页面是否需要重定向到首页
   * 用于区分带路由参数的页面 比如 /oss/:id 这种路由是需要回到首页的
   * @default false
   */
  requireHomeRedirect?: boolean;
  /**
   * 标题名称
   */
  title: string;
}

/**
 * React Router 7 的 `RouteObject.handle` 载荷。
 *
 * RR7 没有 `name` / `meta` 字段，且 `RouteObject` 是联合类型别名（无法声明合并），
 * 因此元信息统一挂到 `handle` 上：`patchRoutes` 后仍可通过 `useMatches()` 读回。
 */
interface RouteHandle {
  meta?: RouteMeta;
  name?: string;
}

/**
 * 应用路由对象（Vue 侧 `RouteRecordRaw` 的对应物）。
 *
 * 结构上覆盖 RR7 `RouteObject` 的全部字段，另外附带 Vue 侧沿用下来的
 * `name` / `meta` / `redirect`。装配完成后由 app 层的 `toRouteObjects()`
 * 转成 RR7 `RouteObject[]`（`meta`/`name` 折进 `handle`，`redirect` 转 index loader）。
 */
interface AppRouteObject {
  caseSensitive?: boolean;
  children?: AppRouteObject[];
  Component?: ComponentType<any> | null;
  element?: ReactNode | null;
  ErrorBoundary?: ComponentType<any> | null;
  errorElement?: ReactNode | null;
  handle?: RouteHandle;
  id?: string;
  /**
   * 索引路由。与 Vue 侧一致地由 `redirect` 自动派生，业务代码不直接写。
   */
  index?: boolean;
  loader?: ((args: any) => any) | boolean;
  /**
   * 路由元信息，转 RR7 时折进 `handle.meta`。
   */
  meta?: RouteMeta;
  /**
   * 路由名（后端菜单的 `name`，如 `Config106`）。RR7 无此概念，转 RR7 时
   * 同时写入 `id` 与 `handle.name`，供 tabbar 去重与 `patchRoutes` 幂等判断使用。
   */
  name?: string;
  /**
   * 路由路径。生成的路由树中一律为**绝对路径**（由 `backMenuToRouteObjects`
   * 逐级拼接），转 RR7 时再剥掉父级前缀还原为相对路径。
   */
  path?: string;
  /**
   * 重定向目标。RR7 无此字段，转换时生成一条 `index` 子路由 + loader。
   */
  redirect?: string;
  shouldRevalidate?: (args: any) => boolean;
}

/**
 * 带父子链信息的路由对象（对应 Vue 侧 `ExRouteRecordRaw`），`generateMenus` 使用。
 */
type ExAppRouteObject = AppRouteObject & {
  parent?: string;
  parents?: string[];
};

/**
 * 组件映射表。值统一为 React 组件；异步页面组件在 app 层用 `React.lazy` 包好再放入，
 * 避免运行时区分「组件」与「返回 Promise 的 loader」这两种同为 function 的形态。
 */
type ComponentRecordType = Record<string, ComponentType<any>>;

/**
 * 后端菜单转换的中间形态：`component` 仍是字符串（对应 Vue 侧 `RouteRecordStringComponent`）。
 */
type RouteRecordStringComponent<T = string> = {
  children?: RouteRecordStringComponent<T>[];
  component: T;
  meta?: RouteMeta;
  name?: string;
  path: string;
  redirect?: string;
};

/**
 * 生成菜单与路由的入参（对应 Vue 侧 `GenerateMenuAndRoutesOptions`）。
 *
 * 差异：Vue 侧需要 `router` 实例（`generateMenus` 要靠 `router.getRoutes()` 解析绝对路径），
 * React 侧的路由树在生成阶段就已经是绝对路径，故不需要 router。
 */
interface GenerateMenuAndRoutesOptions {
  fetchMenuListAsync?: () => Promise<RouteRecordStringComponent[]>;
  forbiddenComponent?: ComponentType<any>;
  layoutMap?: ComponentRecordType;
  pageMap?: ComponentRecordType;
  roles?: string[];
  routes: AppRouteObject[];
}

export type {
  AppRouteObject,
  ComponentRecordType,
  ExAppRouteObject,
  GenerateMenuAndRoutesOptions,
  RouteHandle,
  RouteMeta,
  RouteRecordStringComponent,
};
