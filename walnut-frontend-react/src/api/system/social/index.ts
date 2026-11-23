import type { SocialInfo } from './model';

import type { ID } from '@/api/common';

import { request } from '@/lib/request';

enum Api {
  root = '/system/social',
  socialList = '/system/social/list',
}

/**
 * 获取绑定的社交信息列表
 * @returns info
 */
export function socialList() {
  return request.get<SocialInfo[]>(Api.socialList);
}

/**
 * @deprecated 并没有用到这个方法
 */
export function socialInfo(id: ID) {
  return request.get(`${Api.root}/${id}`);
}
