import type { DictData } from '@/api/system/dict/dict-data-model';

import { dictDataInfo } from '@/api/system/dict/dict-data';
import { create } from 'zustand';

/**
 * antd select/radio 通用选项（对 DictData 的拓展）。
 * 对应 Vue 侧 store/dict.ts 的 DictOption。
 */
export interface DictOption extends Omit<DictData, 'id'> {
  disabled?: boolean;
  label: string;
  value: number | string;
}

/** 将字典数据转为 Options（剔除数字主键 id，避免透传给组件 id prop） */
export function dictToOptions(
  data: DictData[],
  formatNumber = false,
): DictOption[] {
  return (data ?? []).map(({ id: _id, ...item }) => ({
    ...item,
    label: item.dictLabel,
    value: formatNumber ? Number(item.dictValue) : item.dictValue,
  }));
}

interface DictState {
  map: Record<string, DictOption[]>;
  resetCache(): void;
  setDictInfo(
    dictName: string,
    data: DictData[],
    formatNumber?: boolean,
  ): void;
}

/**
 * 字典缓存 store（对应 Vue 侧 store/dict.ts）。
 * PoC 用 Zustand 替代 Pinia，天然去重 + 缓存（阶段 ③ 可升级为 TanStack Query）。
 */
export const useDictStore = create<DictState>()((set) => ({
  map: {},
  resetCache: () => {
    requestCache.clear();
    set({ map: {} });
  },
  setDictInfo: (dictName, data, formatNumber = false) =>
    set((state) => ({
      map: {
        ...state.map,
        [dictName]: dictToOptions(data, formatNumber),
      },
    })),
}));

/** 进行中的请求缓存，避免同一字典被重复请求 */
const requestCache = new Map<string, Promise<unknown>>();

/**
 * 获取字典 options（对应 Vue 侧 utils/dict.ts 的 getDictOptions）。
 *
 * 同步返回当前缓存（可能为空数组），首次调用触发异步请求写入 store；
 * 调用方组件需订阅 useDictStore 以在字典到达后重渲染。
 */
export function getDictOptions(
  dictName: string,
  formatNumber = false,
): DictOption[] {
  const cached = useDictStore.getState().map[dictName];
  if (cached && cached.length > 0) {
    return cached;
  }
  if (dictName && !requestCache.has(dictName)) {
    requestCache.set(
      dictName,
      dictDataInfo(dictName)
        .then((resp) => {
          useDictStore.getState().setDictInfo(dictName, resp, formatNumber);
        })
        .catch(() => {
          // 忽略：401 等异常由 http 层统一处理
        })
        .finally(() => {
          requestCache.delete(dictName);
        }),
    );
  }
  return cached ?? [];
}
