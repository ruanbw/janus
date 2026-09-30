/**
 * 国家清单：ISO 3166-1 alpha-2 码 + 中文名，数据来自 i18n-iso-countries 的 zh 语言包。
 *
 * 为什么不自己写一张表：国家码是会变动的清单，手写必然漏码或写错，
 * 而这里要的是**与后端 ip2region 返回值完全一致的码集合**——多一个不存在的码，
 * 就是一个用户能选、但永远不命中的死选项（和当初 `asn` 那种假能力一个性质）。
 * 交给维护良好的库，以后新增/更名的国家跟着上游走。
 */
import { getNames, registerLocale } from 'i18n-iso-countries';
import zhLocale from 'i18n-iso-countries/langs/zh.json';

registerLocale(zhLocale);

const NAMES: Record<string, string> = getNames('zh');

export interface CountryOption {
  /** ISO 3166-1 alpha-2 码，如 US。后端 visits.country 存的就是它 */
  value: string;
  /** 中文名，如 美国 */
  name: string;
  /** 下拉展示：`美国 US`。带上码是为了能按码搜（运营习惯直接敲 US / cn） */
  label: string;
}

// 按中文名排序（zh-CN collation 走浏览器内置 ICU，不引拼音库）
export const COUNTRY_OPTIONS: CountryOption[] = Object.entries(NAMES)
  .map(([value, name]) => ({ value, name, label: `${name} ${value}` }))
  .sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'));

/** 码 → 中文名。查不到返回空串：宁可空着，也不显示猜出来的名字。 */
export function countryName(code?: string | null): string {
  const c = (code || '').trim().toUpperCase();
  if (!c) return '';
  return NAMES[c] ?? '';
}