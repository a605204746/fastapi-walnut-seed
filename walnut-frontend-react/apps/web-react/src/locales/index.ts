import type { SupportedLanguagesType } from '@walnut/locales';

import { loadLocalesMapFromDir } from '@walnut/locales';

/**
 * 应用级语言包加载（对应 Vue 侧 apps/web-antd/src/locales/index.ts）。
 *
 * 词典 JSON 与 Vue 侧逐字节共用（从 apps/web-antd/src/locales/langs 复制），
 * 通过 setupI18n({ loadMessages }) 合并进 i18next 的 translation 命名空间，
 * 使 `pages.common.*` / `common.*` / `http.*` 等 app 级 key 可被 $t 解析。
 */
const modules = import.meta.glob('./langs/**/*.json');

const localesMap = loadLocalesMapFromDir(
  /\.\/langs\/([^/]+)\/(.*)\.json$/,
  modules,
);

export async function loadAppMessages(lang: SupportedLanguagesType) {
  const appLocaleMessages = await localesMap[lang]?.();
  return appLocaleMessages?.default ?? {};
}

export { $t } from '@walnut/locales';
