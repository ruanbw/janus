/**
 * 批量导入的行解析契约。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 用户粘贴一段「短码 目标URL」或「仅目标URL」的文本
 *   When  解析成待导入条目
 *   Then  每行要么是合法条目，要么带着**能指导用户改**的原因被拒绝；
 *         绝不能在没有任何提示的情况下被悄悄丢掉
 *
 * 为什么值得钉住：原实现（视图内联）有几处静默失败 ——
 *   - 一行里有 3 段时只取前两段，第三个词直接消失（用户以为导入了带备注的行）；
 *   - 只有短码没有 URL 的行会被拼成 `https://`，报错信息里只剩一个空 URL；
 *   - 逐行串行插入，没有总数上限提示，粘 2000 行时前端会卡住几分钟。
 * 这些都是「看起来导入成功了，其实少了几条」。
 */
import { describe, expect, it } from 'vitest';

import { parseBatchLines } from '@/views/links/batchImport';

describe('parseBatchLines', () => {
  it('Given 「短码 URL」行, Then 拆成 code + url', () => {
    const [item] = parseBatchLines('deal-a https://example.com/a');

    expect(item).toMatchObject({ ok: true, code: 'deal-a', url: 'https://example.com/a' });
  });

  it('Given 只有 URL 的行, Then code 为 undefined（由后端自动生成）', () => {
    const [item] = parseBatchLines('https://example.com/a');

    expect(item).toMatchObject({ ok: true, url: 'https://example.com/a' });
    expect(item?.ok === true ? item.code : 'unexpected').toBeUndefined();
  });

  it('Given 省略协议的裸域名, Then 补成 https://', () => {
    const [item] = parseBatchLines('example.com/a');

    expect(item).toMatchObject({ ok: true, url: 'https://example.com/a' });
  });

  it('Given 一行超过两段, Then 明确报错而不是静默丢弃多余内容', () => {
    const [item] = parseBatchLines('deal-a https://example.com/a 备注');

    expect(item?.ok).toBe(false);
    if (item && !item.ok) {
      expect(item.reason).toContain('两段');
      expect(item.line).toBe('deal-a https://example.com/a 备注');
    }
  });

  it('Given 只有短码没有 URL, Then 报错并回显原文（不是拼出 https:// 再说非法）', () => {
    const [item] = parseBatchLines('deal-a');

    expect(item?.ok).toBe(false);
    if (item && !item.ok) {
      expect(item.reason).toContain('缺少目标 URL');
      expect(item.source).toBe('deal-a');
    }
  });

  it('Given 非 http/https 协议, Then 拒绝（与后端 validTargetURL 同一白名单）', () => {
    const [item] = parseBatchLines('deal-a javascript:alert(1)');

    expect(item?.ok).toBe(false);
    if (item && !item.ok) expect(item.reason).toContain('http');
  });

  it('Given 空行与纯空白行, Then 不产生条目（也不算失败）', () => {
    const items = parseBatchLines('\n\n  \ndeal-a https://example.com/a\n\n');

    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({ ok: true });
  });

  it('Given 逗号/制表符分隔, Then 与空格同等对待', () => {
    const items = parseBatchLines('deal-a,https://example.com/a\ndeal-b\thttps://example.com/b');

    expect(items.map((item) => (item.ok ? item.code : null))).toEqual(['deal-a', 'deal-b']);
  });

  it('Given 同一批里重复短码, Then 只保留第一条, 后续标为重复并指出与哪一行冲突', () => {
    const items = parseBatchLines(
      ['deal-a https://example.com/a', 'deal-b https://example.com/b', 'deal-a https://example.com/c'].join('\n'),
    );

    expect(items[0]?.ok).toBe(true);
    expect(items[1]?.ok).toBe(true);
    const third = items[2];
    expect(third?.ok).toBe(false);
    if (third && !third.ok) {
      expect(third.reason).toContain('重复');
      expect(third.reason).toContain('第 1 行');
    }
  });

  it('Given 行号, Then 以原始文本的物理行号为准（跳过空行也不重排）', () => {
    const items = parseBatchLines('\n\ndeal-a https://example.com/a');

    expect(items[0]?.ok).toBe(true);
    expect(items[0]?.ok === true ? items[0].lineNo : 0).toBe(3);
  });
});
