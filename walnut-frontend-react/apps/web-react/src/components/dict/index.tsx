import type { DictOption } from '@/store/dict';

import { Spin, Tag } from 'antd';

interface TagType {
  [key: string]: { color: string; label: string };
}

/** 字典 listClass → antd Tag color 映射（对应 Vue 侧 components/dict/src/data.tsx） */
export const tagTypes: TagType = {
  cyan: { color: 'cyan', label: 'cyan' },
  danger: { color: 'error', label: '危险(danger)' },
  default: { color: 'default', label: '默认(default)' },
  green: { color: 'green', label: 'green' },
  info: { color: 'default', label: '信息(info)' },
  orange: { color: 'orange', label: 'orange' },
  pink: { color: 'pink', label: 'pink' },
  primary: { color: 'processing', label: '主要(primary)' },
  purple: { color: 'purple', label: 'purple' },
  red: { color: 'red', label: 'red' },
  success: { color: 'success', label: '成功(success)' },
  warning: { color: 'warning', label: '警告(warning)' },
};

export interface DictTagProps {
  dicts?: DictOption[];
  fallback?: string;
  value: number | string;
}

/**
 * 字典标签（对应 Vue 侧 components/dict/src/index.vue 的 DictTag）。
 * 逻辑逐字复刻：loading → Spin；未匹配 → fallback；有 color → Tag；否则纯文本。
 */
export function DictTag({
  dicts = [],
  fallback = 'unknown',
  value,
}: DictTagProps) {
  // eslint-disable-next-line eqeqeq
  const current = dicts.find((item) => item.dictValue == value);
  const loading = dicts.length === 0;

  if (loading) {
    return (
      <div>
        <Spin size="small" spinning />
      </div>
    );
  }

  const label = current?.dictLabel ?? null;
  if (label === null) {
    return <div>{fallback}</div>;
  }

  const listClass = current?.listClass ?? '';
  const cssClass = current?.cssClass ?? '';
  const isDefault = Object.hasOwn(tagTypes, listClass);
  const color = isDefault ? tagTypes[listClass]!.color : listClass;

  if (color) {
    return (
      <div>
        <Tag className={cssClass} color={color}>
          {label}
        </Tag>
      </div>
    );
  }

  return (
    <div>
      <div className={cssClass}>{label}</div>
    </div>
  );
}
