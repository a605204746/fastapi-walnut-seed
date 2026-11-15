import type { ReactNode } from 'react';

import { DictTag } from '@/components/dict';
import { getDictOptions } from '@/store/dict';

export interface RenderDictOptions {
  fallback?: string;
}

/**
 * 显示字典标签（一般 table 使用），对应 Vue 侧 utils/render.tsx 的 renderDict。
 * @param value 值
 * @param dictName 字典类型
 */
export function renderDict(
  value: number | string,
  dictName: string,
  options?: RenderDictOptions,
): ReactNode {
  const dictInfo = getDictOptions(dictName);
  return (
    <DictTag dicts={dictInfo} fallback={options?.fallback} value={value} />
  );
}
