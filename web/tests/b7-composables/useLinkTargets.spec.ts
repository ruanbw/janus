/**
 * 多目标 URL 编辑（增删改、排序、校验）composable 契约。
 *
 * 背景：LinkFormView.vue 直接用 `string[]` 承载目标 URL，增删靠数组下标，
 * 校验塞在 zod schema 里。抽出来以后有三件事必须可验证：
 * 1. entry 有稳定 id —— 否则用户在第二行输入时删掉第一行，光标/内容会串行；
 * 2. 不允许删到 0 行 —— 短链没有目标 URL 就是死链；
 * 3. validate 的错误优先级固定（empty > invalidUrl > duplicate），
 *    否则同一份输入可能报出不同的一句话，用户按提示改完还是过不了。
 */
import { describe, expect, it } from 'vitest';

import { useLinkTargets } from '@/composables/useLinkTargets';
import type { TargetEntry } from '@/composables/useLinkTargets';

/** 由契约返回值推导，避免测试里另抄一份联合类型后悄悄漂移 */
type ValidateResult = ReturnType<ReturnType<typeof useLinkTargets>['validate']>;

function urlsOf(entries: TargetEntry[]): string[] {
  return entries.map((entry) => entry.url);
}

function idsOf(entries: TargetEntry[]): string[] {
  return entries.map((entry) => entry.id);
}

/** 按 URL 找 entry 的 id：断言「move 后 id 跟着 entry 走」时比下标可靠 */
function idOfUrl(entries: TargetEntry[], url: string): string {
  const found = entries.find((entry) => entry.url === url);
  if (!found) {
    throw new Error(`测试前置条件不成立：找不到 url=${url} 的目标行`);
  }
  return found.id;
}

describe('useLinkTargets 初始化', () => {
  it('Given 不传 initial, When 读取 targets/urls/canRemove, Then 只有一条空行且不可删除', () => {
    const { targets, urls, canRemove } = useLinkTargets();

    expect(targets.value).toHaveLength(1);
    expect(targets.value[0].url).toBe('');
    expect(idsOf(targets.value)[0]).toBeTruthy();
    expect(urls.value).toEqual([]);
    expect(canRemove.value).toBe(false);
  });

  it('Given initial 是空数组, When 读取 targets, Then 同样回落到一条空行', () => {
    const { targets, canRemove } = useLinkTargets([]);

    // 后端要求至少一个目标 URL：空数组不能变成「零行表单」。
    expect(targets.value).toHaveLength(1);
    expect(targets.value[0].url).toBe('');
    expect(canRemove.value).toBe(false);
  });

  it('Given initial 两个 URL, When 读取 targets 与 urls, Then 顺序一致且 id 互不相同', () => {
    const { targets, urls, canRemove } = useLinkTargets([
      'https://a.test/1',
      'https://b.test/2',
    ]);

    expect(urlsOf(targets.value)).toEqual(['https://a.test/1', 'https://b.test/2']);
    expect(urls.value).toEqual(['https://a.test/1', 'https://b.test/2']);
    expect(new Set(idsOf(targets.value)).size).toBe(2);
    expect(canRemove.value).toBe(true);
  });
});

describe('urls 计算属性', () => {
  it('Given 目标行含首尾空白与空白行, When 读取 urls, Then trim 后过滤空值并保持顺序', () => {
    const { urls } = useLinkTargets([' https://a.test ', '   ', 'https://b.test']);

    // 提交前按 urls 拼请求体；空白行若混进去就是一条 `targetUrls: ['']` 的 400。
    expect(urls.value).toEqual(['https://a.test', 'https://b.test']);
  });

  it('Given 两条目标, When replace 第二条为空白, Then urls 只剩第一条', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test']);

    ctx.replace(ctx.targets.value[1].id, '   ');

    // urls 必须是 computed：replace 之后立刻可见，不依赖任何「重新读取」。
    expect(ctx.urls.value).toEqual(['https://a.test']);
  });
});

describe('add', () => {
  it('Given 一条目标, When add(), Then 追加一条空行且旧 entry 的 id 不变', () => {
    const ctx = useLinkTargets(['https://a.test']);
    const firstId = ctx.targets.value[0].id;

    ctx.add();

    expect(ctx.targets.value).toHaveLength(2);
    expect(ctx.targets.value[1].url).toBe('');
    // 旧行 id 不能变：模板用 id 作 v-for key，变了就会重建 DOM、丢输入焦点。
    expect(ctx.targets.value[0].id).toBe(firstId);
    expect(new Set(idsOf(ctx.targets.value)).size).toBe(2);
    expect(ctx.urls.value).toEqual(['https://a.test']);
    expect(ctx.canRemove.value).toBe(true);
  });

  it('Given 默认一条空行, When add(url), Then 追加带该 URL 的行并出现在 urls 里', () => {
    const ctx = useLinkTargets();

    ctx.add('  https://b.test  ');

    expect(ctx.targets.value).toHaveLength(2);
    // 存储值 trim 与否不是契约，统一按 urls 的口径断言最终提交值。
    expect(ctx.targets.value[1].url.trim()).toBe('https://b.test');
    expect(ctx.urls.value).toEqual(['https://b.test']);
  });
});

describe('replace', () => {
  it('Given 两条目标, When replace 第一条, Then 只改该条且两条 id 都不变', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test']);
    const [firstId, secondId] = idsOf(ctx.targets.value);

    ctx.replace(firstId, 'https://a2.test');

    expect(urlsOf(ctx.targets.value)).toEqual(['https://a2.test', 'https://b.test']);
    expect(idsOf(ctx.targets.value)).toEqual([firstId, secondId]);
  });
});

describe('remove', () => {
  it('Given 三条目标, When remove 中间项, Then 返回 true 且其余顺序与 id 保持', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test', 'https://c.test']);
    const [firstId, secondId, thirdId] = idsOf(ctx.targets.value);

    const removed = ctx.remove(secondId);

    expect(removed).toBe(true);
    expect(urlsOf(ctx.targets.value)).toEqual(['https://a.test', 'https://c.test']);
    // 剩下两行的 id 不受影响：删除中间项不能让后面行「继承」被删行的 key。
    expect(idsOf(ctx.targets.value)).toEqual([firstId, thirdId]);
    expect(ctx.urls.value).toEqual(['https://a.test', 'https://c.test']);
  });

  it('Given 只剩一条, When remove, Then 返回 false 且 targets 完全不变', () => {
    const ctx = useLinkTargets(['https://a.test']);
    const before = ctx.targets.value.map((entry) => ({ ...entry }));

    const removed = ctx.remove(ctx.targets.value[0].id);

    // 归零后表单没有任何目标 URL 可提交；按钮 disabled 之外还要有逻辑兜底。
    expect(removed).toBe(false);
    expect(ctx.targets.value).toEqual(before);
    expect(ctx.canRemove.value).toBe(false);
  });

  it('Given 两条目标, When remove 一条, Then canRemove 变为 false', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test']);

    ctx.remove(ctx.targets.value[0].id);

    expect(ctx.canRemove.value).toBe(false);
  });
});

describe('move', () => {
  it('Given 三条目标, When 中间项 direction=-1, Then 与前一条交换', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test', 'https://c.test']);
    const bId = idOfUrl(ctx.targets.value, 'https://b.test');

    ctx.move(bId, -1);

    expect(urlsOf(ctx.targets.value)).toEqual([
      'https://b.test',
      'https://a.test',
      'https://c.test',
    ]);
    // 位置变了但 id 没变：同一个 entry 只是换了顺序。
    expect(ctx.targets.value[0].id).toBe(bId);
  });

  it('Given 三条目标, When 中间项 direction=1, Then 与后一条交换', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test', 'https://c.test']);
    const bId = idOfUrl(ctx.targets.value, 'https://b.test');

    ctx.move(bId, 1);

    expect(urlsOf(ctx.targets.value)).toEqual([
      'https://a.test',
      'https://c.test',
      'https://b.test',
    ]);
    expect(ctx.targets.value[2].id).toBe(bId);
  });

  it('Given 首项, When direction=-1, Then 顺序不变', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test', 'https://c.test']);
    const before = ctx.targets.value.map((entry) => ({ ...entry }));

    ctx.move(ctx.targets.value[0].id, -1);

    expect(ctx.targets.value).toEqual(before);
  });

  it('Given 末项, When direction=1, Then 顺序不变', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test', 'https://c.test']);
    const before = ctx.targets.value.map((entry) => ({ ...entry }));

    ctx.move(ctx.targets.value[2].id, 1);

    expect(ctx.targets.value).toEqual(before);
  });

  it('Given 不存在的 id, When move, Then 顺序不变', () => {
    const ctx = useLinkTargets(['https://a.test', 'https://b.test', 'https://c.test']);
    const before = ctx.targets.value.map((entry) => ({ ...entry }));

    ctx.move('missing-entry-id', 1);

    expect(ctx.targets.value).toEqual(before);
  });
});

const validateCases: Array<{ label: string; input: string[]; expected: ValidateResult }> = [
  {
    label: 'http 与 https 绝对地址',
    input: ['https://a.test', 'http://b.test'],
    expected: { ok: true },
  },
  { label: '仅一条合法地址', input: ['https://a.test'], expected: { ok: true } },
  { label: '唯一空行', input: [''], expected: { ok: false, reason: 'empty', index: 0 } },
  { label: '纯空白行', input: ['   '], expected: { ok: false, reason: 'empty', index: 0 } },
  {
    label: '两个空行',
    input: ['https://a.test', '', '   '],
    expected: { ok: false, reason: 'empty', index: 1 },
  },
  {
    label: '相对路径',
    input: ['/landing'],
    expected: { ok: false, reason: 'invalidUrl', index: 0 },
  },
  {
    label: '缺少协议',
    input: ['https://a.test', 'example.com'],
    expected: { ok: false, reason: 'invalidUrl', index: 1 },
  },
  {
    label: '非 http(s) 协议',
    input: ['ftp://a.test'],
    expected: { ok: false, reason: 'invalidUrl', index: 0 },
  },
  {
    label: '完全重复',
    input: ['https://a.test', 'https://a.test'],
    expected: { ok: false, reason: 'duplicate', index: 1 },
  },
  {
    label: 'trim 后重复',
    input: ['https://a.test', '  https://a.test  '],
    expected: { ok: false, reason: 'duplicate', index: 1 },
  },
  {
    label: '空行与非法地址并存',
    input: ['https://a.test', 'not a url', ''],
    expected: { ok: false, reason: 'empty', index: 2 },
  },
  {
    label: '非法地址与重复并存',
    input: ['https://a.test', 'https://a.test', '/relative'],
    expected: { ok: false, reason: 'invalidUrl', index: 2 },
  },
  {
    label: '空行与重复并存',
    input: ['', 'https://a.test', 'https://a.test'],
    expected: { ok: false, reason: 'empty', index: 0 },
  },
];

describe('validate', () => {
  it.each(validateCases)(
    'Given 目标 URL 为 $label, When validate, Then 返回契约规定的判定',
    ({ input, expected }) => {
      const ctx = useLinkTargets(input);

      expect(ctx.validate()).toEqual(expected);
    },
  );

  it('Given 默认空行, When replace 成 https 地址, Then validate 由 empty 变为通过', () => {
    const ctx = useLinkTargets();
    expect(ctx.validate()).toEqual({ ok: false, reason: 'empty', index: 0 });

    ctx.replace(ctx.targets.value[0].id, 'https://a.test');

    // validate 读的是当前 targets，而不是构造时的 initial 快照。
    expect(ctx.validate()).toEqual({ ok: true });
  });
});
