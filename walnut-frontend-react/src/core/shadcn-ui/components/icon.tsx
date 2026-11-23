import type {
  ComponentType,
  ImgHTMLAttributes,
  SVGAttributes,
} from 'react';

import type { IconProps } from '@iconify/react';

import {
  isFunction,
  isHttpUrl,
  isObject,
  isString,
} from '@/core/shared/utils';
import { Icon as IconifyIcon } from '@iconify/react';
import { Menu as IconDefault } from 'lucide-react';

interface WalnutIconProps extends Partial<Omit<IconProps, 'icon'>> {
  /** icon 为空时是否显示默认图标 */
  fallback?: boolean;
  icon?: ComponentType<any> | string;
}

/**
 * 图标渲染（对应 Vue 侧 `shadcn-ui/src/components/icon/icon.vue`）。
 *
 * 四种入参形态，判断顺序与 Vue 版一致：
 * 1. 组件 / 函数 → 直接渲染
 * 2. http(s) 字符串 → `<img>`
 * 3. 其他字符串 → Iconify 图标名（后端 `sys_menu.icon` 下发的就是这种，如 `eos-icons:system-group`）
 * 4. 空值 + `fallback` → lucide 的 `Menu`（对应 Vue 的 `IconDefault`）
 *
 * ⚠️ Iconify 图标数据默认在运行时按需从 api.iconify.design 拉取，与 Vue 侧
 * `@iconify/vue` 行为一致。若将来要离线，用 `addCollection()` 预注册图标集，
 * 对应 Vue 侧的 `packages/icons/src/iconify-offline/`。
 */
export function WalnutIcon({ fallback, icon, ...rest }: WalnutIconProps) {
  if (!isString(icon) && (isObject(icon) || isFunction(icon))) {
    const Component = icon as ComponentType<any>;
    return <Component {...rest} />;
  }

  if (isString(icon) && isHttpUrl(icon)) {
    return (
      <img
        alt=""
        src={icon}
        {...(rest as ImgHTMLAttributes<HTMLImageElement>)}
      />
    );
  }

  if (isString(icon) && icon) {
    return <IconifyIcon {...rest} icon={icon} />;
  }

  if (fallback) {
    return <IconDefault {...(rest as SVGAttributes<SVGSVGElement>)} />;
  }

  return null;
}
