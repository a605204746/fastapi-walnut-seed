import { useMemo, useState } from 'react';
import { Alert, Button, Checkbox, Input, Space, Table, Tag } from 'antd';
import { $t } from '@/core/locales';
import { usePreferences } from '@/core/preferences';
import type { MenuOption } from '@/api/system/menu/model';
import { allKeys, togglePermission, type PermissionKey } from './permission-keys';

/** Menu/button permissions retain their API IDs; button permissions stay in a separate column. */
export function MenuSelectTable({
  menus,
  value = [],
  onChange,
  association = true,
  onAssociationChange,
}: {
  menus: MenuOption[];
  value?: PermissionKey[];
  onChange?: (keys: PermissionKey[]) => void;
  association?: boolean;
  onAssociationChange?: (associated: boolean) => void;
}) {
  // 订阅偏好：locale 变化时重译展开/折叠文案
  usePreferences();
  const [search, setSearch] = useState('');
  const [expanded, setExpanded] = useState<PermissionKey[] | null>(null);
  const keys = useMemo(() => allKeys(menus), [menus]);
  const chosen = new Set(value);
  const rows = useMemo(() => {
    function convert(nodes: MenuOption[]): any[] {
      return nodes.flatMap((node) => {
        const children = convert(
          (node.children ?? []).filter((n) => n.menuType !== 'F' || node.menuType === 'M'),
        );
        const permissions = (node.children ?? []).filter(
          (n) => n.menuType === 'F' && node.menuType !== 'M',
        );
        if (
          search &&
          !$t(node.label).toLowerCase().includes(search.toLowerCase()) &&
          !children.length &&
          !permissions.some((n) => $t(n.label).includes(search))
        )
          return [];
        return [
          {
            ...node,
            children: children.length ? children : undefined,
            permissions,
            original: node,
          },
        ];
      });
    }
    return convert(menus);
  }, [menus, search]);
  function checkbox(node: MenuOption, buttons = false) {
    const descendants = allKeys([node]);
    const count = descendants.filter((k) => chosen.has(k)).length;
    const checked = association ? count === descendants.length : chosen.has(node.id);
    return (
      <Checkbox
        checked={checked}
        indeterminate={association && count > 0 && !checked}
        onChange={(e) =>
          onChange?.(
            buttons
              ? e.target.checked
                ? [...new Set([...value, node.id])]
                : value.filter((k) => k !== node.id)
              : togglePermission(menus, value, node, e.target.checked, association),
          )
        }
      >
        {$t(node.label)}
      </Checkbox>
    );
  }
  return (
    <div className="permission-panel">
      <Alert showIcon type="info" title={`已选择 ${value.length} 项权限`} />
      <Space wrap className="my-3">
        <Input.Search
          aria-label="搜索菜单权限"
          placeholder="搜索菜单权限"
          allowClear
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Checkbox checked={association} onChange={(e) => onAssociationChange?.(e.target.checked)}>
          节点关联
        </Checkbox>
        <Checkbox
          checked={value.length === keys.length && keys.length > 0}
          indeterminate={value.length > 0 && value.length < keys.length}
          onChange={(e) => onChange?.(e.target.checked ? keys : [])}
        >
          全选
        </Checkbox>
        <Button size="small" onClick={() => setExpanded((expanded ?? keys).length ? [] : keys)}>
          {$t('pages.common.expand')}/{$t('pages.common.collapse')}
        </Button>
      </Space>
      <Table
        size="small"
        pagination={false}
        rowKey="id"
        dataSource={rows}
        scroll={{ y: 450, x: 540 }}
        expandable={{
          expandedRowKeys: search ? keys : (expanded ?? keys),
          onExpandedRowsChange: (k) =>
            setExpanded(k.map((key) => (typeof key === 'bigint' ? String(key) : key))),
        }}
        columns={[
          { title: '菜单名称', width: 260, render: (_, row) => checkbox(row.original) },
          {
            title: '类型',
            width: 70,
            render: (_, row) => (
              <Tag>{row.menuType === 'M' ? '目录' : row.menuType === 'F' ? '按钮' : '菜单'}</Tag>
            ),
          },
          {
            title: '操作权限',
            render: (_, row) => (
              <Space wrap>
                {row.permissions.map((n: MenuOption) => (
                  <span key={n.id}>{checkbox(n, true)}</span>
                ))}
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}
