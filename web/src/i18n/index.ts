import { ref } from 'vue';
import type { App } from 'vue';

import enUS from './locales/en-US';
import zhCN from './locales/zh-CN';
import type { Messages } from './types';

export type { Messages };

/**
 * 极简 i18n：一个响应式 locale + 纯函数 t()。
 *
 * 为什么不上 vue-i18n：本仓库只需要「查字典 + 插值 + 切语言」，
 * `$t` 挂在 globalProperties 后组件里用起来与 vue-i18n 没有区别，
 * 而多一个依赖就多一套（复数、日期、懒加载语言包）需要维护的语义。
 */
export const availableLocales = ['zh-CN', 'en-US'] as const;

export type Locale = (typeof availableLocales)[number];

export const defaultLocale: Locale = 'zh-CN';

export const messages: Record<Locale, Messages> = {
  'zh-CN': zhCN,
  'en-US': enUS,
};

/** 插值参数：number 也允许，界面上「共 3 条」这类数字不该逼调用方自己 String() */
export type TranslateParams = Record<string, string | number>;

/**
 * 当前语言。
 *
 * 做成模块级 ref 而不是 Pinia store：t() 在组件外（zod 校验、toast、请求层）
 * 也要能用，而 store 必须绑定 pinia 实例；ref 让模板里的 t() 自动建立依赖，
 * 切语言后已渲染的文案会跟着重渲染。
 */
const currentLocale = ref<Locale>(defaultLocale);

function isLocale(value: string): value is Locale {
  return (availableLocales as readonly string[]).includes(value);
}

/**
 * 切换语言，返回实际生效的语言。
 *
 * 不支持的 locale 一律回退到默认语言而不是「保留当前语言」：后者会让调用方
 * 以为已经切过去了（例如从设置项里读到 `fr-FR`），界面却仍是上一种语言，
 * 也没有任何报错可查。
 */
export function setLocale(locale: string): Locale {
  currentLocale.value = isLocale(locale) ? locale : defaultLocale;
  return currentLocale.value;
}

export function getLocale(): Locale {
  return currentLocale.value;
}

/**
 * 点号路径查字典。叶子必须是字符串：`common.save.deep` 这类半截路径返回 undefined，
 * 由 t() 回显原 key，而不是把某个子树塞进界面。
 */
function lookup(table: Messages, key: string): string | undefined {
  let node: string | Messages | undefined = table;

  for (const part of key.split('.')) {
    if (typeof node !== 'object') return undefined;
    node = node[part];
  }

  return typeof node === 'string' ? node : undefined;
}

/**
 * `{name}` 插值。
 *
 * 缺参数时保留占位符原样，不替换成空串、也不抛异常：替换成空串会让界面出现
 * 「共  条」这种看不出少了什么的空洞，抛异常则会让一个漏传参数直接白屏。
 */
function interpolate(template: string, params?: TranslateParams): string {
  if (params === undefined) return template;

  return template.replace(/\{(\w+)\}/g, (placeholder, name: string) => {
    const value = params[name];
    return value === undefined ? placeholder : String(value);
  });
}

/**
 * 取当前语言的文案；未知 key 原样回显。
 *
 * 回显 key 而不是返回空串：空串渲染成功且没有内容，比「页面上写着 common.save」
 * 更难定位漏了哪条翻译。
 */
export function t(key: string, params?: TranslateParams): string {
  const template = lookup(messages[currentLocale.value], key);
  return template === undefined ? key : interpolate(template, params);
}

/** 把 i18n 挂到 app 上：模板里可用 `$t('common.save')`，无需每个组件 import */
export function installI18n(app: App): void {
  app.config.globalProperties.$t = t;
}

declare module 'vue' {
  interface ComponentCustomProperties {
    /** 模板里的翻译入口，由 installI18n 注入 */
    $t: typeof t;
  }
}
