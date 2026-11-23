import { JsonPreview } from '@/components/json-preview';
import type { ReactNode } from 'react';

import type { DictOption } from '@/store/dict';

import { cn } from '@/core/shared/utils';
import { Tag } from 'antd';

import { WalnutIcon } from '@/core/shadcn-ui';

import { DictTag } from '@/components/dict';
import { getDictOptions } from '@/store/dict';

/**
 * 表格单元格渲染工具（对应 Vue 侧 `utils/render.tsx`，237 行）。
 *
 * 与 Vue 版的差异：
 * - `class={...}` → `className={...}`（JSX 属性名）。
 * - 图标：Vue 用 UnoCSS 的 `Iconify(logos--chrome)` 类名，React 侧统一走
 *   `WalnutIcon` + Iconify 名（`logos:chrome`）。两套 tailwind 配置都没有装
 *   iconify 的 class 插件，照抄类名会渲染成空 span。
 * - `renderJsonPreview`：Vue 依赖 `common-ui` 的 `JsonPreview`（内部是
 *   json-viewer，D 类 → `@microlink/react-json-view`）。React 侧该依赖尚未引入，
 *   暂用格式化 `<pre>` 承载，**视觉与 Vue 不等价**，已记入迁移计划的收尾项。
 */

export interface RenderDictOptions {
  fallback?: string;
}

/**
 * 显示字典标签（一般 table 使用），对应 Vue 侧 renderDict。
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

/** 渲染单个字典标签（dicts 由调用方显式给出） */
export function renderDictTag(
  value: number | string,
  dicts: DictOption[],
): ReactNode {
  return <DictTag dicts={dicts} value={value} />;
}

/**
 * 渲染多个字典标签
 * @param value key 数组
 * @param wrap 是否换行显示
 * @param gap 间隔 px
 */
export function renderDictTags(
  value: string[],
  dicts: DictOption[],
  wrap = true,
  gap = 1,
): ReactNode {
  if (!Array.isArray(value)) {
    return <div>{value as any}</div>;
  }
  return (
    <div
      className={cn('flex', wrap ? 'flex-col' : 'flex-row')}
      style={{ gap: `${gap}px` }}
    >
      {value.map((item, index) => (
        <div key={index}>{renderDictTag(item, dicts)}</div>
      ))}
    </div>
  );
}

/** 渲染普通标签 */
export function renderTag(text: string, color?: string): ReactNode {
  return <Tag color={color}>{text}</Tag>;
}

/** 渲染一组普通标签 */
export function renderTags(
  tags: string[],
  wrap = false,
  gap = 1,
): ReactNode {
  return (
    <div
      className={cn('flex', wrap ? 'flex-col' : 'flex-row')}
      style={{ gap: `${gap}px` }}
    >
      {tags.map((tag, index) => (
        <div key={index}>{renderTag(tag)}</div>
      ))}
    </div>
  );
}

/** httpMethod 标签（对应 Vue 侧 renderHttpMethodTag） */
export function renderHttpMethodTag(type: string): ReactNode {
  const method = (type ?? '').toUpperCase();
  const colors: Record<string, string> = {
    DELETE: 'red',
    GET: 'green',
    POST: 'blue',
    PUT: 'orange',
  };
  return <Tag color={colors[method] ?? 'default'}>{`${method}请求`}</Tag>;
}

/**
 * JSON 预览（对应 Vue 侧 renderJsonPreview）。
 *
 * ⚠️ 见文件头说明：暂用 `<pre>` 承载，未引入 react-json-view。
 */
export function renderJsonPreview(json: any): ReactNode {
  if (typeof json !== 'object' && typeof json !== 'string') {
    return <span>{json as any}</span>;
  }
  const toText = (value: any): null | string => {
    if (typeof value === 'object' && value !== null) {
      return JSON.stringify(value, null, 2);
    }
    if (typeof value === 'string') {
      try {
        const parsed = JSON.parse(value);
        return typeof parsed === 'object' && parsed !== null
          ? JSON.stringify(parsed, null, 2)
          : null;
      } catch {
        return null;
      }
    }
    return null;
  };
  const text = toText(json);
  if (text === null) {
    return <span>{String(json)}</span>;
  }
  return <JsonPreview value={JSON.parse(text)} />;
}

/** Iconify 图标（对应 Vue 侧 renderIcon） */
export function renderIcon(icon: string): ReactNode {
  return <WalnutIcon icon={icon} />;
}

const osOptions = [
  { icon: 'devicon:windows8', value: 'windows' },
  { icon: 'wpf:macos', value: 'osx' },
  { icon: 'devicon:linux', value: 'linux' },
  { icon: 'flat-color-icons:android-os', value: 'android' },
  { icon: 'majesticons:iphone-x-apps-line', value: 'ios' },
];

const DEFAULT_OS_ICON = 'ic:outline-computer';

/**
 * 浏览器图标，取值口径同 Vue 侧注释：`cn.hutool.http.useragent` → browsers。
 *
 * Vue 侧 `qq` 用的是本地离线 svg（`svg:qq`，type: 'offline'），React 侧没有那套
 * 离线 sprite，改用 Iconify 的 `mdi:qqchat`。
 */
const browserOptions = [
  { icon: 'logos:chrome', value: 'chrome' },
  { icon: 'logos:microsoft-edge', value: 'edge' },
  { icon: 'logos:firefox', value: 'firefox' },
  { icon: 'logos:opera', value: 'opera' },
  { icon: 'logos:safari', value: 'safari' },
  { icon: 'mdi:wechat', value: 'micromessenger' },
  { icon: 'mdi:wechat', value: 'windowswechat' },
  { icon: 'logos:quarkus-icon', value: 'quark' },
  { icon: 'mdi:wechat', value: 'wxwork' },
  { icon: 'mdi:qqchat', value: 'qq' },
  { icon: 'ri:dingding-line', value: 'dingtalk' },
  { icon: 'arcticons:uc-browser', value: 'uc' },
  { icon: 'ri:baidu-fill', value: 'baidu' },
];

const DEFAULT_BROWSER_ICON = 'ph:browser-duotone';

/** 操作系统图标 */
export function renderOsIcon(os: string, className?: string): ReactNode {
  if (!os) return null;
  let current = osOptions.find((item) =>
    os.toLocaleLowerCase().includes(item.value),
  );
  // windows 要特殊处理（"Windows 10 or Windows Server 2016" 这类）
  if (os.toLocaleLowerCase().includes('windows')) {
    current = osOptions[0];
  }
  return (
    <WalnutIcon
      className={cn('inline-block size-4', className)}
      icon={current ? current.icon : DEFAULT_OS_ICON}
    />
  );
}

/** 浏览器图标 */
export function renderBrowserIcon(
  browser: string,
  className?: string,
): ReactNode {
  if (!browser) return null;
  const current = browserOptions.find((item) =>
    browser.toLocaleLowerCase().includes(item.value),
  );
  return (
    <WalnutIcon
      className={cn('inline-block size-4', className)}
      icon={current ? current.icon : DEFAULT_BROWSER_ICON}
    />
  );
}
