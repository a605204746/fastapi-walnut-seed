import type {
  ImportLocaleFn,
  LoadMessageFn,
  LocaleSetupOptions,
  SupportedLanguagesType,
} from './typing';

import i18next from 'i18next';

const modules = import.meta.glob('./langs/**/*.json');

const localesMap = loadLocalesMapFromDir(
  /\.\/langs\/([^/]+)\/(.*)\.json$/,
  modules,
);

let loadMessages: LoadMessageFn;

/**
 * vue-i18n 的位置/具名占位符 `{0}`、`{label}` 转成 i18next 的 `{{0}}`、`{{label}}`。
 * 这样 langs/*.json 可以与 Vue 版前端逐字节共用，无需维护两份词典。
 * 前后各加一个断言，避免把已经是 `{{x}}` 的串再次膨胀成 `{{{x}}}`。
 */
const PLACEHOLDER_RE = /(?<!\{)\{(\w+)\}(?!\})/g;

function normalizePlaceholders(value: unknown): unknown {
  if (typeof value === 'string') {
    return value.replace(PLACEHOLDER_RE, '{{$1}}');
  }
  if (Array.isArray(value)) {
    return value.map((item) => normalizePlaceholders(item));
  }
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(([key, item]) => [
        key,
        normalizePlaceholders(item),
      ]),
    );
  }
  return value;
}

/**
 * Load locale modules
 * @param modules
 */
function loadLocalesMap(modules: Record<string, () => Promise<unknown>>) {
  const localesMap: Record<string, ImportLocaleFn> = {};

  for (const [path, loadLocale] of Object.entries(modules)) {
    const key = path.match(/([\w-]*)\.(json)/)?.[1];
    if (key) {
      localesMap[key] = loadLocale as ImportLocaleFn;
    }
  }
  return localesMap;
}

/**
 * Load locale modules with directory structure
 * @param regexp - Regular expression to match language and file names
 * @param modules - The modules object containing paths and import functions
 * @returns A map of locales to their corresponding import functions
 */
function loadLocalesMapFromDir(
  regexp: RegExp,
  modules: Record<string, () => Promise<unknown>>,
): Record<string, ImportLocaleFn> {
  const localesRaw: Record<string, Record<string, () => Promise<unknown>>> = {};
  const localesMap: Record<string, ImportLocaleFn> = {};

  // Iterate over the modules to extract language and file names
  for (const path in modules) {
    const match = path.match(regexp);
    if (match) {
      const [_, locale, fileName] = match;
      if (locale && fileName) {
        if (!localesRaw[locale]) {
          localesRaw[locale] = {};
        }
        if (modules[path]) {
          localesRaw[locale]![fileName] = modules[path]!;
        }
      }
    }
  }

  // Convert raw locale data into async import functions
  for (const [locale, files] of Object.entries(localesRaw)) {
    localesMap[locale] = async () => {
      const messages: Record<string, any> = {};
      for (const [fileName, importFn] of Object.entries(files)) {
        messages[fileName] = ((await importFn()) as any)?.default;
      }
      return { default: messages };
    };
  }

  return localesMap;
}

/**
 * Set i18n language
 * @param locale
 */
async function setI18nLanguage(locale: SupportedLanguagesType) {
  // changeLanguage 是异步的，必须 await，否则调用方读到的 i18next.language 还是旧值
  await i18next.changeLanguage(locale);
  document?.querySelector('html')?.setAttribute('lang', locale);
  return locale;
}

async function setupI18n(options: LocaleSetupOptions = {}) {
  const { defaultLocale = 'zh-CN' } = options;
  // app 可以自行扩展一些第三方库和组件库的国际化
  loadMessages = options.loadMessages || (async () => ({}));

  if (!i18next.isInitialized) {
    await i18next.init({
      fallbackLng: defaultLocale,
      interpolation: {
        // React 已经在渲染时转义，这里不需要 i18next 再转一次
        escapeValue: false,
      },
      keySeparator: '.',
      lng: defaultLocale,
      // 词典的命名空间来自文件名（ui / common / ...），已合并进默认 ns，
      // 关掉 nsSeparator 才能让 `ui.formRules.required` 被当成一条完整 key。
      nsSeparator: false,
      resources: {},
      returnEmptyString: false,
      returnNull: false,
    });
  }

  await loadLocaleMessages(defaultLocale);

  if (options.missingWarn) {
    i18next.on('missingKey', (lngs, _ns, key) => {
      if (key.includes('.')) {
        console.warn(`[i18next] Not found '${key}' key in '${lngs}' locales.`);
      }
    });
  }
}

/**
 * Load locale messages
 * @param lang
 */
async function loadLocaleMessages(lang: SupportedLanguagesType) {
  if (i18next.language === lang && i18next.hasResourceBundle(lang, 'translation')) {
    return setI18nLanguage(lang);
  }

  const message = await localesMap[lang]?.();

  if (message?.default) {
    i18next.addResourceBundle(
      lang,
      'translation',
      normalizePlaceholders(message.default),
      true,
      true,
    );
  }

  const mergeMessage = await loadMessages(lang);
  if (mergeMessage) {
    i18next.addResourceBundle(
      lang,
      'translation',
      normalizePlaceholders(mergeMessage),
      true,
      // overwrite=true：应用层词典覆盖框架层同名 key
      true,
    );
  }

  return setI18nLanguage(lang);
}

/**
 * 翻译。兼容 vue-i18n 的三种调用形态，便于从 Vue 侧机械搬运调用点：
 * - `$t('key')`
 * - `$t('key', ['a', 'b'])`   位置插值，对应词典里的 `{0}` `{1}`
 * - `$t('key', { name: 'a' })` 具名插值
 * - `$t('key', 2)`            复数
 */
function $t(key: string, ...args: any[]): string {
  if (!i18next.isInitialized) {
    return key;
  }
  const [first] = args;
  if (Array.isArray(first)) {
    return i18next.t(key, {
      ...Object.fromEntries(first.map((value, index) => [index, value])),
    }) as string;
  }
  if (typeof first === 'number') {
    return i18next.t(key, { count: first }) as string;
  }
  if (first !== null && typeof first === 'object') {
    return i18next.t(key, first) as string;
  }
  return i18next.t(key) as string;
}

/**
 * 是否存在某个 key
 */
function $te(key: string): boolean {
  return i18next.exists(key);
}

export {
  $t,
  $te,
  i18next,
  loadLocaleMessages,
  loadLocalesMap,
  loadLocalesMapFromDir,
  normalizePlaceholders,
  setI18nLanguage,
  setupI18n,
};
