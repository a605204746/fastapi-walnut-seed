import type { Dept } from './model';

import type { ID } from '@/api/common';

import { request } from '@/lib/request';

enum Api {
  deptList = '/system/dept/list',
  deptNodeInfo = '/system/dept/list/exclude',
  root = '/system/dept',
}

/**
 * 部门列表
 * @returns list
 */
export function deptList(params?: { deptName?: string; status?: string }) {
  return request.get<Dept[]>(Api.deptList, { params });
}

/**
 * 查询部门列表（排除节点）
 * @param deptId 部门ID
 * @returns void
 */
export function deptNodeList(deptId: ID) {
  return request.get<Dept[]>(`${Api.deptNodeInfo}/${deptId}`);
}

/**
 * 部门详情
 * @param deptId 部门id
 * @returns 部门信息
 */
export function deptInfo(deptId: ID) {
  return request.get<Dept>(`${Api.root}/${deptId}`);
}

/**
 * 部门新增
 * @param data 参数
 */
export function deptAdd(data: Partial<Dept>) {
  return request.postWithMsg<void>(Api.root, data);
}

/**
 * 部门更新
 * @param data 参数
 */
export function deptUpdate(data: Partial<Dept>) {
  return request.putWithMsg<void>(Api.root, data);
}

/**
 * 注意这里只允许单删除
 * @param deptId ID
 * @returns void
 */
export function deptRemove(deptId: ID) {
  return request.deleteWithMsg<void>(`${Api.root}/${deptId}`);
}
