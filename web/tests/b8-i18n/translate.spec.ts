/**
 * i18n 切换与插值语义契约（TDD Red 阶段：`web/src/i18n/**` 尚不存在）。
 *
 * 冻结的是「切换立即生效、未知 locale 回退、未知 key 回显、参数插值」四条语义，
 * 而不是文案内容本身 —— 文案在实现阶段还会大改，语义不能跟着改。
 *
 * 为什么这些边界重要：
 * - 未知 locale 若静默保留当前语言，用户以为自己切到了英文，看到仍是中文；
 * - 未知 locale 若落进空字典，界面整片空白，比报错更糟；
 * - 未知 key 返回空串会让页面出现无提示的空洞，回显 key 至少能定位漏了哪条；
 * - 缺参数时若抛异常，一个漏传参数就能让整个页面白屏。
 */
import { afterEach, describe, expect, it } from 'vitest';

import { getLocale, messages, setLocale, t } from '@/i18n';

import { extractPlaceholders, flattenLeaves, leafStrings } from './dictionary';

const CJK = /[\u4e00-\u9fff]/;

/**
 * 每个用例结束都还原默认语言。
 * locale 是模块级可变状态，用例之间会互相污染：例如「未知 locale 回退」若跑在
 * `setLocale('en-US')` 之后而不复位，断言就会变成「回退到了上一种语言」，
 * 那种实现同样能过，测试就失去鉴别力。
 */
afterEach(() => {
  setLocale('zh-CN');
});

type PlaceholderLeaf = { key: string; names: string[] };

/**
 * 从 zh-CN 真实文案里找出所有带占位符的叶子。
 * 契约没有冻结「哪条文案带占位符」，但 t() 的 params 参数是契约的一部分：
 * 若实现里一条可插值文案都没有，这条分支永远走不到，参数就是死代码。
 */
function placeholderLeaves(): PlaceholderLeaf[] {
  const found: PlaceholderLeaf[] = [];

  for (const [key, value] of flattenLeaves(messages['zh-CN'])) {
    const names = [...new Set(leafStrings(value).flatMap((text) => [...extractPlaceholders(text)]))];
    if (names.length > 0) found.push({ key, names: names.sort() });
  }

  return found;
}

const interpolated = placeholderLeaves();

describe('默认语言', () => {
  it('Given 未显式切换，When 读取 locale 与 common.save，Then 是 zh-CN 且返回中文', () => {
    expect(getLocale()).toBe('zh-CN');

    const save = t('common.save');
    // 原样回显 key 说明字典查找失败（例如只挂了 key 没挂文案）
    expect(save).not.toBe('common.save');
    expect(save).toMatch(CJK);
  });
});

describe('运行时切换语言', () => {
  it('Given 当前中文，When setLocale(en-US)，Then 文案立即变英文；切回 zh-CN 后恢复', () => {
    const zhSave = t('common.save');

    // 返回实际生效值是契约的一部分：调用方据此回写 UI，不能自己假设
    expect(setLocale('en-US')).toBe('en-US');
    expect(getLocale()).toBe('en-US');

    const enSave = t('common.save');
    expect(enSave).not.toBe(zhSave); // 两份字典指向同一文案 = 根本没切
    expect(enSave).not.toBe('common.save');
    expect(enSave).not.toMatch(CJK);

    expect(setLocale('zh-CN')).toBe('zh-CN');
    expect(getLocale()).toBe('zh-CN');
    expect(t('common.save')).toBe(zhSave);
  });

  it.each(['fr-FR', 'ja-JP'])(
    'Given 当前是英文，When setLocale(%s)（未支持），Then 回退 zh-CN 而不是保留英文',
    (unknown) => {
      setLocale('en-US');

      expect(setLocale(unknown)).toBe('zh-CN');
      expect(getLocale()).toBe('zh-CN');
      // 断言文案是中文而不只是 locale 值：回退必须落到真实字典，不能是「标记变了字典没变」
      const save = t('common.save');
      expect(save).toMatch(CJK);
      expect(save).not.toBe('common.save');
    },
  );
});

describe('未知 key', () => {
  it.each(['no.such.key', 'common.noSuchAction', 'auth.login.title.deep'])(
    'Given 字典里没有 %s，When t()，Then 原样返回 key 且不抛异常',
    (key) => {
      expect(() => t(key)).not.toThrow();
      // 回显 key 而不是空串：界面上还能看出漏了哪条文案，便于定位
      expect(t(key)).toBe(key);
    },
  );
});

describe('插值', () => {
  it('Given 冻结的 t(key, params)，When 收集可插值文案，Then 字典里至少有一条', () => {
    // 没有可插值文案时，下面三条用例都无从验证；与其静默 skip，不如直接判失败
    expect(interpolated.length).toBeGreaterThan(0);
  });

  it('Given 含占位符的文案，When 传入全部参数，Then 每个 {name} 都被替换', () => {
    const failures: string[] = [];

    for (const { key, names } of interpolated) {
      const params: Record<string, string | number> = {};
      // 参数值刻意带上 `#` 前缀：避免与文案里本来就有的数字/词撞车造成假绿
      names.forEach((name, index) => {
        params[name] = `#v${index}#`;
      });

      const out = t(key, params);
      for (const [index, name] of names.entries()) {
        if (out.includes(`{${name}}`)) failures.push(`${key}: {${name}} 未被替换 → ${out}`);
        if (!out.includes(`#v${index}#`)) failures.push(`${key}: 输出缺少参数值 #v${index}# → ${out}`);
      }
    }

    expect(failures).toEqual([]);
  });

  it('Given 含占位符的文案，When 参数是数字，Then 数字被转成字符串插入', () => {
    const first = interpolated[0];
    expect(first, '字典里没有含占位符的文案，无法验证数字参数').toBeDefined();
    if (!first) return;

    const name = first.names[0]!;
    const out = t(first.key, { [name]: 3 });

    // number 是 params 的合法类型；实现只接受字符串、或把它当数组下标时在这里露馅
    expect(out).not.toContain(`{${name}}`);
    expect(out).toContain('3');
  });

  it('Given 含占位符的文案，When 完全不传参数或只传无关参数，Then 占位符原样保留且不抛异常', () => {
    const failures: string[] = [];

    for (const { key, names } of interpolated) {
      // 两条路径都要走：`t(key)` 可能命中「没参数就直接返回原文」的短路分支，
      // 只测它的话「参数表里取不到值 → 替换成空串」这个 bug 能整片漏过。
      const calls: Array<[string, () => string]> = [
        ['无 params', () => t(key)],
        ['params 不含占位符名', () => t(key, { __unrelated__: 'x' })],
      ];

      for (const [label, call] of calls) {
        try {
          const out = call();
          for (const name of names) {
            // 缺参数时若把占位符替换成空串，界面上就是「删除  条」，用户看不出少了什么；
            // 抛异常则更糟：一个漏传参数就能让整页白屏。
            if (!out.includes(`{${name}}`))
              failures.push(`${key}（${label}）: {${name}} 被吞掉 → ${out}`);
          }
        } catch (error) {
          failures.push(`${key}（${label}）: 抛异常 → ${String(error)}`);
        }
      }
    }

    expect(failures).toEqual([]);
  });

  it('Given 文案，When 传入多余参数，Then 多余参数被忽略', () => {
    const failures: string[] = [];

    for (const { key, names } of interpolated) {
      const exact: Record<string, string | number> = {};
      for (const name of names) exact[name] = `#${name}#`;

      const out = t(key, { ...exact, __unused__: '#extra#' });
      if (out !== t(key, exact)) failures.push(`${key}: 多余参数改变了输出 → ${out}`);
      if (out.includes('#extra#')) failures.push(`${key}: 多余参数泄漏到输出 → ${out}`);
    }

    // 无占位符的 key 传参也不能把参数拼进输出
    expect(t('common.save', { n: 3 })).toBe(t('common.save'));
    expect(failures).toEqual([]);
  });
});
