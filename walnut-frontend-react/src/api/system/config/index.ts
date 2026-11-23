import type { SysConfig } from './model';

import type { ID, IDS, PageQuery, PageResult } from '@/api/common';

import { commonExport } from '@/api/helper';
import { request } from '@/lib/request';

enum Api {
  configExport = '/system/config/export',
  configInfoByKey = '/system/config/configKey',
  configList = '/system/config/list',
  configRefreshCache = '/system/config/refreshCache',
  root = '/system/config',
}

/** 系统参数分页列表 */
export function configList(params?: PageQuery) {
  return request.get<PageResult<SysConfig>>(Api.configList, { params });
}

export function configInfo(configId: ID) {
  return request.get<SysConfig>(`${Api.root}/${configId}`);
}

/** 导出 */
export function configExport(data: Partial<SysConfig>) {
  return commonExport(Api.configExport, data);
}

/** 刷新缓存 */
export function configRefreshCache() {
  return request.deleteWithMsg<void>(Api.configRefreshCache);
}

/** 更新系统配置 */
export function configUpdate(data: Partial<SysConfig>) {
  return request.putWithMsg<void>(Api.root, data);
}

/** 新增系统配置 */
export function configAdd(data: Partial<SysConfig>) {
  return request.postWithMsg<void>(Api.root, data);
}

/** 删除配置 */
export function configRemove(configIds: IDS) {
  return request.deleteWithMsg<void>(`${Api.root}/${configIds}`);
}

/** 获取配置信息 */
export function configInfoByKey(configKey: string) {
  return request.get<string>(`${Api.configInfoByKey}/${configKey}`);
}
