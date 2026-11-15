import type { DictData } from './dict-data-model';

import { request } from '@/lib/request';

enum Api {
  root = '/system/dict/data',
}

/**
 * 主要是 DictTag 组件使用
 * @param dictType 字典类型
 * @returns 字典数据
 */
export function dictDataInfo(dictType: string) {
  return request.get<DictData[]>(`${Api.root}/type/${dictType}`);
}
