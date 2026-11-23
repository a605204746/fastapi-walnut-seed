import type { DeptTree as DeptTreeNode } from '@/api/system/user/model';

import { useCallback, useEffect, useMemo, useState } from 'react';

import { cloneDeep, listToTree, treeToList } from '@/core/utils';
import { SyncOutlined } from '@ant-design/icons';
import { Button, Empty, Input, Skeleton, Space, Tree } from 'antd';

import { getDeptTree } from '@/api/system/user';
import { $t } from '@/locales';

export interface DeptTreeProps {
  /** 数据源接口，默认 `getDeptTree`（对应 Vue 侧 props.api） */
  api?: () => Promise<DeptTreeNode[]>;
  className?: string;
  /** 受控选中值（对应 Vue 侧 `v-model:selectDeptId`） */
  value: string[];
  onChange: (keys: string[]) => void;
  /** 点击刷新按钮（对应 Vue 侧 emit('reload')） */
  onReload?: () => void;
  /** 点击节点（对应 Vue 侧 emit('select')） */
  onSelect?: (keys: string[]) => void;
  /** 是否显示搜索框，默认 true */
  showSearch?: boolean;
}

/**
 * 部门树侧栏（对应 Vue 侧 `views/system/user/dept-tree.vue`，174 行）。
 *
 * 被 `system/post` 与 `system/user` 两个页面共用（Vue 侧同样如此）。
 *
 * 与 Vue 版的差异：
 * - `defineModel('selectDeptId')` 双向绑定 → React 受控 `value` + `onChange`。
 *   `loadTree()` 内部需要清空选中，所以 onChange 是必需的，不能只读。
 * - `defineModel('searchValue')` → 内部 state。Vue 侧虽暴露为 model，但两个调用方
 *   都没有绑定它，语义上等价于内部状态。
 * - `$attrs` 透传（Vue 会把 class 同时给外层 div 和 Tree）→ 收敛为 `className`，
 *   只作用于外层容器；Tree 的 `showLine`/`fieldNames`/`defaultExpandAll` 等原本
 *   就是模板里写死的，不是 attrs。
 */
export function DeptTree({
  api = getDeptTree,
  className,
  onChange,
  onReload,
  onSelect,
  showSearch = true,
  value,
}: DeptTreeProps) {
  const [deptTreeArray, setDeptTreeArray] = useState<DeptTreeNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchValue, setSearchValue] = useState('');

  const loadTree = useCallback(async () => {
    setLoading(true);
    setSearchValue('');
    onChange([]);
    const ret = await api();
    setDeptTreeArray(ret ?? []);
    setLoading(false);
  }, [api, onChange]);

  useEffect(() => {
    void loadTree();
    // 仅在挂载时加载（与 Vue 侧 onMounted(loadTree) 一致）；
    // api / onChange 变化不重新拉取，否则会陷入父组件重渲染 → 重拉的循环
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** 搜索过滤：树 → 平铺 → 过滤 → 重新组树（与 Vue 侧 deptTreeComputed 同构） */
  const filteredTree = useMemo(() => {
    if (!searchValue) return deptTreeArray;
    const flat = treeToList<DeptTreeNode[]>(cloneDeep(deptTreeArray), {
      id: 'id',
      pid: 'parentId',
    });
    flat.forEach((item) => {
      item.children = [];
    });
    const matched = flat.filter((item) =>
      item.label.toUpperCase().includes(searchValue.toUpperCase()),
    );
    return listToTree<DeptTreeNode>(matched, { id: 'id', pid: 'parentId' });
  }, [deptTreeArray, searchValue]);

  async function handleReload() {
    await loadTree();
    onReload?.();
  }

  /** 搜索命中时高亮关键字（对应 Vue 侧 #titleRender 插槽） */
  function titleRender(node: DeptTreeNode) {
    const label = node.label ?? '';
    if (!searchValue || !label.includes(searchValue)) {
      return <span>{label}</span>;
    }
    const index = label.indexOf(searchValue);
    return (
      <span>
        {label.slice(0, index)}
        <span className="text-primary">{searchValue}</span>
        {label.slice(index + searchValue.length)}
      </span>
    );
  }

  return (
    <div className={className}>
      <Skeleton active className="p-[8px]" loading={loading} paragraph={{ rows: 8 }}>
        <div className="bg-background flex h-full flex-col overflow-y-auto rounded-lg">
          {/* 固定在顶部，必须加 bg-background 否则会产生「穿透」效果 */}
          {showSearch ? (
            <div className="bg-background sticky left-0 top-0 z-100 p-[8px]">
              <Space.Compact className="w-full">
                <Input
                  allowClear
                  onChange={(e) => setSearchValue(e.target.value)}
                  placeholder={$t('pages.common.search')}
                  size="small"
                  value={searchValue}
                />
                <Button onClick={() => void handleReload()} size="small">
                  <SyncOutlined className="text-primary" />
                </Button>
              </Space.Compact>
            </div>
          ) : null}
          <div className="h-full overflow-x-hidden px-[8px]">
            {filteredTree.length > 0 ? (
              <Tree
                defaultExpandAll
                fieldNames={{ key: 'id', title: 'label' }}
                onSelect={(keys) => {
                  const next = keys as string[];
                  onChange(next);
                  onSelect?.(next);
                }}
                selectedKeys={value}
                showLine={{ showLeafIcon: false }}
                styles={
                  {
                    item: {
                      '--ant-tree-node-selected-bg':
                        'var(--ant-color-primary-bg-hover)',
                    },
                  } as any
                }
                titleRender={titleRender as any}
                treeData={filteredTree as any}
                virtual={false}
              />
            ) : (
              // 「仅本人」数据权限下可能没有部门数据
              <div className="mt-5">
                <Empty
                  description="无部门数据"
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                />
              </div>
            )}
          </div>
        </div>
      </Skeleton>
    </div>
  );
}

export default DeptTree;
