import type { MenuRecordRaw } from '@walnut-core/typings';
import type { MenuProps } from 'antd';

import { useMemo } from 'react';

import { usePreferences } from '@walnut/preferences';
import { cn } from '@walnut-core/shared/utils';

import { WalnutIcon } from '@walnut-core/shadcn-ui';
import { Menu } from 'antd';
import { $t } from '@/locales';

type AntdMenuItem = NonNullable<MenuProps['items']>[number];

/**
 * `MenuRecordRaw` 树 → antd `Menu` 的 items。
 *
 * `MenuRecordRaw.name` 存的是**已解析的标题**（`generateMenus` 取 `meta.title`，
 * 前端静态路由的 title 在模块加载时就过了一遍 `$t`），所以这里直接当文案用。
 * `path` 是绝对路径，直接当 key；外链菜单的 path 就是 http(s) URL 本身
 * （`generateMenus` 里 `link || path`），由调用方判断后开新窗口。
 */
function toAntdMenuItems(menus: MenuRecordRaw[], selected: string[] = [], autoActivate?: (menu: MenuRecordRaw) => void, horizontal = false): AntdMenuItem[] {
  return menus.map((menu) => {
    const iconName = selected.includes(menu.path) && menu.activeIcon ? menu.activeIcon : menu.icon || (menu.children?.length ? 'lucide:menu' : undefined);
    const icon = iconName ? <WalnutIcon icon={iconName} /> : undefined;
    // 水平菜单的顶层 li 是 inline-block：label 用块级 flex 会把图标与文字拆成两行而撑破顶栏，
    // 须用 inline-flex；嵌套子项渲染在垂直弹层里，仍用 flex 让徽标 ml-auto 生效
    const label = <span className={`${horizontal ? 'inline-flex' : 'flex'} items-center gap-2`}><span className="truncate">{$t(menu.name)}</span>{menu.badgeType === 'dot' ? <span className="ml-auto size-2 rounded-full bg-primary" /> : menu.badge ? <span className={`ml-auto rounded px-1 text-xs text-primary-foreground ${menu.badgeVariants === 'destructive' ? 'bg-destructive' : 'bg-primary'}`}>{menu.badge}</span> : null}</span>;
    const children = menu.children?.length
      ? toAntdMenuItems(menu.children, selected, autoActivate)
      : undefined;

    if (children && children.length > 0) {
      return { children, icon, key: menu.path, label, title: $t(menu.name), disabled: menu.disabled, onTitleClick: () => autoActivate?.(menu) };
    }
    return { icon, key: menu.path, label, title: $t(menu.name), disabled: menu.disabled };
  });
}

function buildMenuIndex(menus: MenuRecordRaw[]): Map<string, MenuRecordRaw> {
  const index = new Map<string, MenuRecordRaw>();
  const walk = (list: MenuRecordRaw[]) => {
    for (const menu of list) {
      index.set(menu.path, menu);
      if (menu.children) {
        walk(menu.children);
      }
    }
  };
  walk(menus);
  return index;
}

export interface WalnutMenuProps extends Omit<MenuProps, 'items' | 'onSelect'> {
  menus: MenuRecordRaw[];
  /**
   * 点击叶子菜单。antd 的 `onSelect` 只回传 key，这里换回完整的 `MenuRecordRaw`，
   * 调用方才能拿到 `query`（拼跳转参数）与 path（判断是否外链）。
   */
  onMenuSelect?: (menu: MenuRecordRaw) => void;
}

/**
 * 菜单适配层：支持各布局、节点禁用、手风琴、徽标、激活图标和目录自动跳转。
 * 视觉令牌与外壳共用，渲染时解析国际化标题。
 */
export function WalnutMenu({ className, menus, onMenuSelect, ...rest }: WalnutMenuProps) {
  const { preferences } = usePreferences();
  function activate(menu: MenuRecordRaw) { let target = menu; while (target.children?.length) target = target.children[0]!; onMenuSelect?.(target); }
  const items = toAntdMenuItems(menus, rest.selectedKeys, preferences.sidebar.autoActivateChild ? activate : undefined, rest.mode === 'horizontal');
  const index = useMemo(() => buildMenuIndex(menus), [menus]);

  return (
    <Menu
      {...rest}
      className={cn('walnut-menu', preferences.sidebar.collapsedShowTitle && 'walnut-menu-collapsed-title', preferences.navigation.styleType === 'rounded' && 'walnut-menu-rounded', className)}
      items={items}
      onSelect={({ key }) => {
        const menu = index.get(key);
        if (menu) {
          onMenuSelect?.(menu);
        }
      }}
    />
  );
}

export { toAntdMenuItems };
