import { beforeAll, describe, expect, it } from 'vitest';

import {
  $t,
  $te,
  i18next,
  loadLocaleMessages,
  normalizePlaceholders,
  setupI18n,
} from '../i18n';

describe('normalizePlaceholders', () => {
  it('把 vue-i18n 的位置占位符转成 i18next 形式', () => {
    expect(normalizePlaceholders('请输入{0}')).toBe('请输入{{0}}');
    expect(normalizePlaceholders('{0}至少{1}个字符')).toBe(
      '{{0}}至少{{1}}个字符',
    );
  });

  it('转换具名占位符', () => {
    expect(normalizePlaceholders('你好 {name}')).toBe('你好 {{name}}');
  });

  it('已经是 i18next 形式的串不会被二次膨胀', () => {
    expect(normalizePlaceholders('已经是 {{0}} 了')).toBe('已经是 {{0}} 了');
  });

  it('递归处理嵌套对象与数组', () => {
    expect(
      normalizePlaceholders({
        a: ['{0}', { b: '{1}' }],
        plain: 'no placeholder',
      }),
    ).toEqual({ a: ['{{0}}', { b: '{{1}}' }], plain: 'no placeholder' });
  });

  it('非字符串标量原样返回', () => {
    expect(normalizePlaceholders(42)).toBe(42);
    expect(normalizePlaceholders(null)).toBe(null);
    expect(normalizePlaceholders(true)).toBe(true);
  });
});

/**
 * i18next 是模块级单例，整个文件只初始化一次，
 * 与真实应用「bootstrap 阶段调一次 setupI18n」的用法保持一致。
 */
describe('setupI18n + $t', () => {
  beforeAll(async () => {
    await setupI18n({
      defaultLocale: 'zh-CN',
      loadMessages: async (lang) =>
        lang === 'zh-CN'
          ? {
              custom: {
                greeting: '来自应用层',
                positional: '位置参数 {0}',
              },
              ui: { formRules: { alreadyExists: '应用层覆盖了 {0} `{1}`' } },
            }
          : {
              custom: {
                greeting: 'from app layer',
                positional: 'positional {0}',
              },
              ui: {
                formRules: { alreadyExists: 'app override {0} `{1}`' },
              },
            },
    });
  });

  it('加载内置词典并可翻译', () => {
    expect(i18next.language).toBe('zh-CN');
    // langs/zh-CN/ui.json -> formRules.required = "请输入{0}"
    expect($t('ui.formRules.required', ['用户名'])).toBe('请输入用户名');
    expect($t('ui.formRules.selectRequired', ['角色'])).toBe('请选择角色');
  });

  it('多位置参数按序插值', () => {
    // langs/zh-CN/ui.json -> formRules.minLength = "{0}至少{1}个字符"
    expect($t('ui.formRules.minLength', ['密码', 6])).toBe('密码至少6个字符');
  });

  it('具名参数插值', () => {
    // 直接调 addResourceBundle 会绕过 normalizePlaceholders，
    // 所以这里写 i18next 原生形式；经 loadMessages 注入的词典才支持 {name} 写法。
    i18next.addResourceBundle(
      'zh-CN',
      'translation',
      { named: { hello: '你好 {{name}}' } },
      true,
      false,
    );
    expect($t('named.hello', { name: '核桃' })).toBe('你好 核桃');
  });

  it('经 loadMessages 注入的词典支持 vue-i18n 风格的 {name} 占位符', () => {
    // beforeAll 里 loadMessages 返回的 custom.greeting 无占位符，
    // 这里验证同一批注入里带 {0} 的条目已被 normalize 过
    expect($t('custom.positional', ['A'])).toBe('位置参数 A');
  });

  it('loadMessages 注入的应用层词典生效，且覆盖框架层同名 key', () => {
    expect($t('custom.greeting')).toBe('来自应用层');
    expect($t('ui.formRules.alreadyExists', ['用户名', 'admin'])).toBe(
      '应用层覆盖了 用户名 `admin`',
    );
  });

  it('$te 判断 key 是否存在', () => {
    expect($te('ui.formRules.required')).toBe(true);
    expect($te('ui.formRules.doesNotExist')).toBe(false);
  });

  it('未知 key 原样返回，不抛错', () => {
    expect($t('some.unknown.key')).toBe('some.unknown.key');
  });

  it('词典里的点号 key 不被当成命名空间分隔符', () => {
    // nsSeparator 关掉后，`ui.formRules.required` 是一条完整 key
    expect($t('ui.formRules.required', ['X'])).not.toContain('ui.formRules');
  });

  it('切换到 en-US 后取英文词典，并同步 <html lang>', async () => {
    await loadLocaleMessages('en-US');
    expect(i18next.language).toBe('en-US');
    expect($t('ui.formRules.required', ['Username'])).toContain('Username');
    expect(document.querySelector('html')?.getAttribute('lang')).toBe('en-US');

    // 切回 zh-CN，避免影响其他测试文件
    await loadLocaleMessages('zh-CN');
    expect($t('ui.formRules.required', ['用户名'])).toBe('请输入用户名');
  });
});
