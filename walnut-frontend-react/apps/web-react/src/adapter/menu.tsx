import type { MenuRecordRaw } from '@vben-core/typings';
import type { MenuProps } from 'antd';

import { useMemo } from 'react';

import { VbenIcon } from '@vben-core/shadcn-ui';
import { Menu } from 'antd';

type AntdMenuItem = NonNullable<MenuProps['items']>[number];

/**
 * `MenuRecordRaw` 树 → antd `Menu` 的 items。
 *
 * `MenuRecordRaw.name` 存的是**已解析的标题**（`generateMenus` 取 `meta.title`，
 * 前端静态路由的 title 在模块加载时就过了一遍 `$t`），所以这里直接当文案用。
 * `path` 是绝对路径，直接当 key；外链菜单的 path 就是 http(s) URL 本身
 * （`generateMenus` 里 `link || path`），由调用方判断后开新窗口。
 */
function toAntdMenuItems(menus: MenuRecordRaw[]): AntdMenuItem[] {
  return menus.map((menu) => {
    const icon = menu.icon ? <VbenIcon icon={menu.icon} /> : undefined;
    const children = menu.children?.length
      ? toAntdMenuItems(menu.children)
      : undefined;

    if (children && children.length > 0) {
      return { children, icon, key: menu.path, label: menu.name };
    }
    return { icon, key: menu.path, label: menu.name };
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

export interface VbenMenuProps extends Omit<MenuProps, 'items' | 'onSelect'> {
  menus: MenuRecordRaw[];
  /**
   * 点击叶子菜单。antd 的 `onSelect` 只回传 key，这里换回完整的 `MenuRecordRaw`，
   * 调用方才能拿到 `query`（拼跳转参数）与 path（判断是否外链）。
   */
  onMenuSelect?: (menu: MenuRecordRaw) => void;
}

/**
 * 侧边/顶部菜单（对应 Vue 侧 `@vben-core/menu-ui`，清单 §5.5 的 2,243 行）。
 *
 * ⚠️ 与 Vue 版的刻意差异：Vue 侧是自绘菜单（含手风琴、动画、混合导航的三套形态），
 * React 侧按阶段 ⑥ 的决策**先用 antd `Menu`**，把 6 种布局模式与自绘样式留到 ⑥-b。
 * 因此本组件放在 app 的 `adapter/` 层而非 `@core/ui-kit/`——`@core` 在 vben 的
 * 分层里是 UI 库无关层，塞 antd 进去会破坏该约束（与 `adapter/grid.tsx` 同理）。
 *
 * 未移植：`badge` / `badgeType` / `badgeVariants` 徽标（当前后端菜单无此数据），
 * 以及 `activeIcon`（选中态换图标）。
 */
export function VbenMenu({ menus, onMenuSelect, ...rest }: VbenMenuProps) {
  const items = useMemo(() => toAntdMenuItems(menus), [menus]);
  const index = useMemo(() => buildMenuIndex(menus), [menus]);

  return (
    <Menu
      {...rest}
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
