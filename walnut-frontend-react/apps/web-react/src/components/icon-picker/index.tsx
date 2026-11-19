import { useState } from 'react';
import { Button, Input, Popover, Tabs } from 'antd';
import { VbenIcon } from '@vben-core/shadcn-ui';
const groups: Record<string, string[]> = {
  lucide: [
    'home',
    'settings',
    'users',
    'user',
    'shield',
    'bell',
    'menu',
    'folder',
    'file',
    'file-text',
    'database',
    'monitor',
    'chart-no-axes-combined',
    'layout-dashboard',
    'briefcase-business',
    'key',
    'lock',
    'globe',
    'mail',
    'calendar',
    'clock',
    'search',
    'download',
    'upload',
    'heart',
    'star',
    'circle-help',
    'check',
    'plus',
    'pencil',
    'trash-2',
  ],
  'ant-design': [
    'home-outlined',
    'setting-outlined',
    'user-outlined',
    'team-outlined',
    'safety-outlined',
    'bell-outlined',
    'menu-outlined',
    'folder-outlined',
    'file-outlined',
    'database-outlined',
    'dashboard-outlined',
    'appstore-outlined',
  ],
};
export function IconPicker({
  value,
  onChange,
  disabled,
}: {
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
}) {
  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  return (
    <Popover
      trigger="click"
      open={open && !disabled}
      onOpenChange={setOpen}
      content={
        <div className="w-80">
          <Input.Search
            placeholder="搜索图标"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <Tabs
            items={Object.entries(groups).map(([prefix, icons]) => ({
              key: prefix,
              label: prefix,
              children: (
                <div className="grid max-h-64 grid-cols-6 gap-1 overflow-y-auto">
                  {icons
                    .filter((icon) => icon.includes(search))
                    .map((icon) => (
                      <Button
                        key={icon}
                        aria-label={`${prefix}:${icon}`}
                        type={value === `${prefix}:${icon}` ? 'primary' : 'text'}
                        icon={<VbenIcon icon={`${prefix}:${icon}`} />}
                        onClick={() => {
                          onChange?.(`${prefix}:${icon}`);
                          setOpen(false);
                        }}
                      />
                    ))}
                </div>
              ),
            }))}
          />
        </div>
      }
    >
      <Input
        disabled={disabled}
        value={value}
        onChange={(e) => onChange?.(e.target.value)}
        allowClear
        placeholder="请选择图标或输入 Iconify 名称"
        prefix={value ? <VbenIcon icon={value} /> : undefined}
      />
    </Popover>
  );
}
