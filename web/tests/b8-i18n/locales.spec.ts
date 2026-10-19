/**
 * i18n 字典一致性契约（TDD Red 阶段：`web/src/i18n/**` 尚不存在）。
 *
 * 背景：仓库有 README.en.md 和英文产品页，但 UI 全是中文硬编码。本轮先冻结
 * 「中英两份字典必须结构等价」的基建契约，全量字符串抽取留到实现阶段。
 *
 * 为什么这四条断言值得单独钉住（它们对应的都是**运行时不报错、只在界面上错**的形态）：
 * - 漏 key：查找失败后回退成 key 原文或空白，页面「能渲染」所以没人发现；
 * - 空串：渲染成功但没有内容，比漏 key 更难定位；
 * - 占位符只在单侧出现：另一侧会把 `{n}` 原样显示给用户；
 * - 英文侧直接抄中文：看起来「翻译完了」，实际只是假翻译。
 */
import { describe, expect, it } from 'vitest';

import { availableLocales, defaultLocale, messages, type Locale } from '@/i18n';

import { extractPlaceholders, flattenLeaves, leafStrings } from './dictionary';

const zh = flattenLeaves(messages['zh-CN']);
const en = flattenLeaves(messages['en-US']);

/** 叶子值只允许 string 或 string[]（数组整体算叶子，不进字典递归） */
function collectInvalidLeaves(locale: Locale, leaves: Map<string, unknown>): string[] {
  const offenders: string[] = [];

  for (const [key, value] of leaves) {
    if (typeof value === 'string') continue;
    if (Array.isArray(value) && value.every((item) => typeof item === 'string')) continue;
    offenders.push(`${locale}:${key} → ${JSON.stringify(value)}`);
  }

  return offenders;
}

/** 空串与纯空白都算「界面静默空白」；数组叶子逐元素检查 */
function collectBlankLeaves(locale: Locale, leaves: Map<string, unknown>): string[] {
  return [...leaves]
    .filter(([, value]) => leafStrings(value).some((text) => text.trim() === ''))
    .map(([key]) => `${locale}:${key}`);
}

/** 该 key 上出现过的全部占位符（数组叶子取并集），排序后便于比对 */
function placeholdersOf(value: unknown): string[] {
  return [...new Set(leafStrings(value).flatMap((text) => [...extractPlaceholders(text)]))].sort();
}

const renderNames = (names: string[]): string => (names.length === 0 ? '(无)' : names.join(','));

/** 冻结的最小可用集：实现阶段必须先保证这 10 条有中文文案，否则任何页面都无法落地 */
const REQUIRED_ZH_KEYS = [
  'common.cancel',
  'common.save',
  'common.delete',
  'common.search',
  'auth.login.title',
  'auth.register.title',
  'links.title',
  'domains.title',
  'rules.title',
  'overview.title',
] as const;

describe('locale 常量', () => {
  it('Given 冻结契约，When 读取导出，Then 只有两种 locale、默认 zh-CN、且 messages 的键与之一致', () => {
    expect([...availableLocales]).toEqual(['zh-CN', 'en-US']);
    expect(defaultLocale).toBe('zh-CN');
    // messages 的键若与 availableLocales 不一致，说明有语言只接了一半：
    // 类型上仍然合法，但 setLocale 到那种语言会拿到 undefined 字典。
    expect(Object.keys(messages).sort()).toEqual(['en-US', 'zh-CN']);
  });
});

describe('字典结构', () => {
  it('Given 两份字典，When 递归展开成点号 key，Then 叶子 key 集合完全一致', () => {
    const zhKeys = [...zh.keys()];

    // 空字典会让下面两条差集断言「真空通过」，先排除这种假绿
    expect(zhKeys.length).toBeGreaterThan(0);

    const enKeys = [...en.keys()];

    // missing/extra 分开报：只说「不相等」的话，实现者还得自己找是少了还是多了
    expect(zhKeys.filter((key) => !en.has(key))).toEqual([]);
    expect(enKeys.filter((key) => !zh.has(key))).toEqual([]);
  });

  it('Given 两份字典，When 检查叶子类型，Then 只允许 string 或 string[]', () => {
    // 数字/布尔/null 叶子会被 t() 原样塞进界面（显示成 "null"），
    // 而 TypeScript 在 `Record<string, unknown>` 下不会拦下来。
    expect(collectInvalidLeaves('zh-CN', zh)).toEqual([]);
    expect(collectInvalidLeaves('en-US', en)).toEqual([]);
  });

  it('Given 两份字典，When 检查每个叶子值，Then 没有空串（空串比漏 key 更难发现）', () => {
    expect(collectBlankLeaves('zh-CN', zh)).toEqual([]);
    expect(collectBlankLeaves('en-US', en)).toEqual([]);
  });

  it('Given 两份字典，When 比对同名 key 的 {占位符}，Then 两侧集合相同', () => {
    const mismatches: string[] = [];

    for (const [key, zhValue] of zh) {
      // key 缺失已由上一条用例报出，这里跳过，避免同一个问题报两遍
      if (!en.has(key)) continue;

      const zhNames = placeholdersOf(zhValue);
      const enNames = placeholdersOf(en.get(key));
      if (renderNames(zhNames) !== renderNames(enNames)) {
        mismatches.push(`${key}: zh-CN={${renderNames(zhNames)}} en-US={${renderNames(enNames)}}`);
      }
    }

    // 单侧多一个占位符时，另一侧运行时会把它原样显示给用户（"删除 {n} 条"）
    expect(mismatches).toEqual([]);
  });

  it('Given en-US 的全部叶子值，When 扫描 CJK，Then 没有汉字（漏译最常见的形态是直接复制中文）', () => {
    const offenders: string[] = [];

    for (const [key, value] of en) {
      for (const text of leafStrings(value)) {
        if (/[\u4e00-\u9fff]/.test(text)) offenders.push(`en-US:${key} → ${text}`);
      }
    }

    expect(offenders).toEqual([]);
  });
});

describe('冻结的最小可用 key 集', () => {
  it.each(REQUIRED_ZH_KEYS)('Given key %s，When 读取 zh-CN 字典，Then 存在且是中文非空文案', (key) => {
    expect(zh.has(key), `zh-CN 缺少冻结 key: ${key}`).toBe(true);

    const value = zh.get(key);
    expect(typeof value, `${key} 必须是字符串叶子`).toBe('string');
    expect(value, `${key} 不能是空串`).not.toBe('');

    // 只断言「存在」不够：复制英文占位文案也能通过，界面会中英混排
    expect(value as string).toMatch(/[\u4e00-\u9fff]/);
  });
});
