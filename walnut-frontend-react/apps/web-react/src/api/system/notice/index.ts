import type { Notice } from './model';

import type { ID, IDS, PageQuery, PageResult } from '@/api/common';

import { request } from '@/lib/request';

enum Api {
  noticeList = '/system/notice/list',
  root = '/system/notice',
}

/**
 * 通知公告分页
 * @param params 分页参数
 * @returns 分页结果
 */
export function noticeList(params?: PageQuery) {
  // 同 postList：Vue 侧标注 `Notice[]` 是错的，后端返回分页信封 `{rows,total}`
  return request.get<PageResult<Notice>>(Api.noticeList, { params });
}

/**
 * 通知公告详情
 * @param noticeId id
 * @returns 详情
 */
export function noticeInfo(noticeId: ID) {
  return request.get<Notice>(`${Api.root}/${noticeId}`);
}

/**
 * 通知公告新增
 * @param data 参数
 */
export function noticeAdd(data: Partial<Notice>) {
  return request.postWithMsg<void>(Api.root, data);
}

/**
 * 通知公告更新
 * @param data 参数
 */
export function noticeUpdate(data: any) {
  return request.putWithMsg<void>(Api.root, data);
}

/**
 * 通知公告删除
 * @param noticeIds ids
 */
export function noticeRemove(noticeIds: IDS) {
  return request.deleteWithMsg<void>(`${Api.root}/${noticeIds}`);
}
