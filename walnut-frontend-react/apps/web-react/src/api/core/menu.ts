import { request } from '@/lib/request';

export interface MenuMeta {
  activeMenu?: string;
  icon: string;
  link?: string;
  noCache: boolean;
  title: string;
}

export interface Menu {
  alwaysShow?: boolean;
  children: Menu[];
  component: string;
  hidden: boolean;
  meta: MenuMeta;
  name: string;
  path: string;
  query?: string;
  redirect?: string;
}

/** 获取用户所有菜单（动态路由数据源） */
export async function getAllMenusApi() {
  return request.get<Menu[]>('/system/menu/getRouters');
}
