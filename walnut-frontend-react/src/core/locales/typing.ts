export type SupportedLanguagesType = 'en-US' | 'zh-CN';

/**
 * 词典消息树。
 * 键值是嵌套对象（按文件名分命名空间，如 `ui.formRules.required`），叶子才是字符串，
 * 所以不能写成扁平的 Record<string, string>。
 */
export interface LocaleMessages {
  [key: string]: LocaleMessages | string;
}

export type ImportLocaleFn = () => Promise<{ default: LocaleMessages }>;

export type LoadMessageFn = (
  lang: SupportedLanguagesType,
) => Promise<LocaleMessages | undefined>;

export interface LocaleSetupOptions {
  /**
   * Default language
   * @default zh-CN
   */
  defaultLocale?: SupportedLanguagesType;
  /**
   * Load message function
   * @param lang
   * @returns
   */
  loadMessages?: LoadMessageFn;
  /**
   * Whether to warn when the key is not found
   */
  missingWarn?: boolean;
}
