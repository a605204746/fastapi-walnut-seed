import type { DeptTree } from '../user/model';
import type { Post } from './model';

import type { ID, IDS, PageQuery, PageResult } from '@/api/common';

import { commonExport } from '@/api/helper';
import { request } from '@/lib/request';

enum Api {
  postExport = '/system/post/export',
  postList = '/system/post/list',
  postSelect = '/system/post/optionselect',
  root = '/system/post',
}

/**
 * 获取岗位列表
 * @param params 参数
 * @returns Post[]
 */
export function postList(params?: PageQuery) {
  // ⚠️ 有意偏离 Vue 侧：Vue 标注为 `Post[]`，但后端 `/system/post/list` 返回的是
  // 分页信封 `{rows,total}`（与其它 list 接口一致），Vue 的类型是错的、只是没人消费。
  // React 的表格适配层 `proxyConfig.ajax.query` 要求 `{rows,total}`，照抄会编译不过。
  return request.get<PageResult<Post>>(Api.postList, { params });
}

/**
 * 导出岗位信息
 * @param data 请求参数
 * @returns blob
 */
export function postExport(data: Partial<Post>) {
  return commonExport(Api.postExport, data);
}

/**
 * 查询岗位信息
 * @param postId id
 * @returns 岗位信息
 */
export function postInfo(postId: ID) {
  return request.get<Post>(`${Api.root}/${postId}`);
}

/**
 * 岗位新增
 * @param data 参数
 * @returns void
 */
export function postAdd(data: Partial<Post>) {
  return request.postWithMsg<void>(Api.root, data);
}

/**
 * 岗位更新
 * @param data 参数
 * @returns void
 */
export function postUpdate(data: Partial<Post>) {
  return request.putWithMsg<void>(Api.root, data);
}

/**
 * 岗位删除
 * @param postIds ids
 * @returns void
 */
export function postRemove(postIds: IDS) {
  return request.deleteWithMsg<void>(`${Api.root}/${postIds}`);
}

/**
 * 根据部门id获取岗位下拉列表
 * @param deptId 部门id
 * @returns 岗位
 */
export function postOptionSelect(deptId: ID) {
  return request.get<Post[]>(Api.postSelect, { params: { deptId } });
}

/**
 * 岗位专用 - 获取部门树
 * @returns 部门树
 */
export function postDeptTreeSelect() {
  return request.get<DeptTree[]>('/system/post/deptTree');
}
