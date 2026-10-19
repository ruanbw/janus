/**
 * 短链列表查询状态（筛选 + 分页）composable 契约。
 *
 * 背景：LinksView.vue 把 keyword / typeFilter / statusFilter / page / pageSize
 * 五个 ref 直接铺在页面里，模板事件各自负责「换筛选要回到第 1 页」「拼请求参数
 * 时只带真正生效的筛选项」。拆分后这些规则必须逐条钉死：
 * 少一条的表现就是「停在第 3 页 + 刚换的筛选」这种看起来空掉的列表，
 * 或者把 all / 空关键字也发给后端当作筛选条件。
 */
import { describe, expect, it } from 'vitest';

import { useLinkListQuery } from '@/composables/useLinkListQuery';
import type { LinkListQuery } from '@/composables/useLinkListQuery';

const DEFAULTS: LinkListQuery = {
  keyword: '',
  typeFilter: 'all',
  statusFilter: 'all',
  page: 1,
  pageSize: 20,
};

type LinkListQueryCtx = ReturnType<typeof useLinkListQuery>;

describe('useLinkListQuery 初始化', () => {
  it('Given 不传 initial, When 读取 query, Then 得到默认筛选与分页', () => {
    const { query } = useLinkListQuery();

    expect(query.value).toEqual(DEFAULTS);
  });

  it('Given initial 只给部分字段, When 读取 query, Then 其余字段回落到默认值', () => {
    const { query } = useLinkListQuery({ typeFilter: 'landing', pageSize: 50 });

    expect(query.value).toEqual({
      ...DEFAULTS,
      typeFilter: 'landing',
      pageSize: 50,
    });
  });
});

const filterChanges: Array<{
  label: string;
  apply: (ctx: LinkListQueryCtx) => void;
  assertFilter: (query: LinkListQuery) => void;
}> = [
  {
    label: 'setKeyword',
    apply: (ctx) => ctx.setKeyword('促销'),
    assertFilter: (query) => expect(query.keyword).toBe('促销'),
  },
  {
    label: 'setTypeFilter',
    apply: (ctx) => ctx.setTypeFilter('redirect'),
    assertFilter: (query) => expect(query.typeFilter).toBe('redirect'),
  },
  {
    label: 'setStatusFilter',
    apply: (ctx) => ctx.setStatusFilter('disabled'),
    assertFilter: (query) => expect(query.statusFilter).toBe('disabled'),
  },
];

describe('筛选变更的分页语义', () => {
  it.each(filterChanges)(
    'Given 已翻到第 3 页, When $label, Then 筛选生效且 page 回到 1',
    ({ apply, assertFilter }) => {
      const ctx = useLinkListQuery();
      ctx.setPage(3);
      expect(ctx.query.value.page).toBe(3);

      apply(ctx);

      assertFilter(ctx.query.value);
      // 换了筛选条件还停留在第 3 页 = 用户看到空列表却以为「没有数据」。
      expect(ctx.query.value.page).toBe(1);
    },
  );

  it('Given 三个筛选都已设置, When setPage(4), Then 页码变化不改动任何筛选', () => {
    const ctx = useLinkListQuery();
    ctx.setKeyword('促销');
    ctx.setTypeFilter('landing');
    ctx.setStatusFilter('enabled');

    ctx.setPage(4);

    expect(ctx.query.value).toEqual({
      keyword: '促销',
      typeFilter: 'landing',
      statusFilter: 'enabled',
      page: 4,
      pageSize: 20,
    });
  });
});

const keywordCases = [
  { input: '  abc  ', expected: 'abc', label: '首尾空格' },
  { input: '\t\n abc \n', expected: 'abc', label: '制表符与换行' },
  { input: '   ', expected: '', label: '纯空白' },
  { input: 'a b', expected: 'a b', label: '内部空格' },
];

describe('setKeyword 归一化', () => {
  it.each(keywordCases)('Given keyword 为 $label, When setKeyword, Then query 存 $expected', ({ input, expected }) => {
    const ctx = useLinkListQuery();

    ctx.setKeyword(input);

    // 存未 trim 的值会让 hasActiveFilter 与 toParams 对「用户其实没输入」误判。
    expect(ctx.query.value.keyword).toBe(expected);
  });
});

const pageCases = [
  { input: 0, expected: 1, label: '0' },
  { input: -3, expected: 1, label: '负数' },
  { input: 2, expected: 2, label: '合法页码' },
];

describe('setPage 边界', () => {
  it.each(pageCases)('Given setPage($label), When 读取 page, Then 为 $expected', ({ input, expected }) => {
    const ctx = useLinkListQuery();

    ctx.setPage(input);

    // 后端分页是 1-based，page=0 会让请求直接 400。
    expect(ctx.query.value.page).toBe(expected);
  });
});

describe('setPageSize', () => {
  it('Given 合法页大小, When setPageSize(50), Then pageSize 变为 50', () => {
    const ctx = useLinkListQuery();

    ctx.setPageSize(50);

    expect(ctx.query.value.pageSize).toBe(50);
  });

  it.each([
    { input: 0, label: '0' },
    { input: -10, label: '负数' },
  ])('Given setPageSize($label), When 读取 query, Then pageSize 钳到 1 且 page 回到 1', ({ input }) => {
    const ctx = useLinkListQuery();
    ctx.setPage(3);

    ctx.setPageSize(input);

    // size<1 后端会拒绝；此时旧页码已无意义，必须一并回到第 1 页。
    expect(ctx.query.value.pageSize).toBe(1);
    expect(ctx.query.value.page).toBe(1);
  });
});

describe('hasActiveFilter', () => {
  const cases: Array<{
    label: string;
    setup: (ctx: LinkListQueryCtx) => void;
    expected: boolean;
  }> = [
    { label: '默认状态', setup: () => {}, expected: false },
    { label: 'keyword 非空', setup: (ctx) => ctx.setKeyword('abc'), expected: true },
    { label: 'keyword 只有空白', setup: (ctx) => ctx.setKeyword('   '), expected: false },
    { label: 'typeFilter=redirect', setup: (ctx) => ctx.setTypeFilter('redirect'), expected: true },
    { label: 'statusFilter=disabled', setup: (ctx) => ctx.setStatusFilter('disabled'), expected: true },
  ];

  it.each(cases)('Given $label, When 读取 hasActiveFilter, Then 为 $expected', ({ setup, expected }) => {
    const ctx = useLinkListQuery();

    setup(ctx);

    expect(ctx.hasActiveFilter.value).toBe(expected);
  });

  it('Given 无筛选但翻到第 5 页, When 读取 hasActiveFilter, Then 仍为 false', () => {
    const ctx = useLinkListQuery();

    ctx.setPage(5);

    // 「有筛选」决定是否显示「清除筛选」按钮，页码不属于筛选条件，
    // 否则用户会在第 2 页看到一个点下去毫无作用的清除按钮。
    expect(ctx.hasActiveFilter.value).toBe(false);
  });

  it('Given 有筛选且在第 5 页, When 读取 hasActiveFilter, Then 为 true', () => {
    const ctx = useLinkListQuery();
    ctx.setKeyword('abc');
    ctx.setPage(5);

    expect(ctx.hasActiveFilter.value).toBe(true);
  });
});

describe('toParams', () => {
  it('Given 默认查询, When toParams, Then 只含 page 与 pageSize', () => {
    const { toParams } = useLinkListQuery();

    const params = toParams();

    expect(params).toEqual({ page: 1, pageSize: 20 });
    // 用 toEqual 已隐含，但这里显式断键：把 keyword:'' 或 type:'all' 发给后端
    // 等于把「未筛选」伪装成一个筛选值。
    expect(Object.keys(params).sort()).toEqual(['page', 'pageSize']);
  });

  it('Given 所有筛选项都生效, When toParams, Then 参数名映射为 keyword/type/status 且带当前页码', () => {
    const ctx = useLinkListQuery();
    ctx.setPageSize(50);
    ctx.setKeyword('  促销  ');
    ctx.setTypeFilter('landing');
    ctx.setStatusFilter('enabled');
    ctx.setPage(2);

    const params = ctx.toParams();

    expect(params).toEqual({
      keyword: '促销',
      type: 'landing',
      status: 'enabled',
      page: 2,
      pageSize: 50,
    });
    expect(typeof params.page).toBe('number');
    expect(typeof params.pageSize).toBe('number');
    expect(typeof params.keyword).toBe('string');
  });

  it('Given keyword 只有空白, When toParams, Then 不含 keyword 键', () => {
    const ctx = useLinkListQuery();

    ctx.setKeyword('   ');

    const params = ctx.toParams();
    expect(params).toEqual({ page: 1, pageSize: 20 });
    expect('keyword' in params).toBe(false);
  });

  it('Given 类型筛选改回 all, When toParams, Then 不含 type 键', () => {
    const ctx = useLinkListQuery();
    ctx.setTypeFilter('redirect');

    ctx.setTypeFilter('all');

    const params = ctx.toParams();
    expect(params).toEqual({ page: 1, pageSize: 20 });
    expect('type' in params).toBe(false);
  });

  it('Given 状态筛选改回 all, When toParams, Then 不含 status 键', () => {
    const ctx = useLinkListQuery();
    ctx.setStatusFilter('enabled');

    ctx.setStatusFilter('all');

    const params = ctx.toParams();
    expect(params).toEqual({ page: 1, pageSize: 20 });
    expect('status' in params).toBe(false);
  });
});

describe('reset', () => {
  it('Given 筛选项、页码与页大小都被改过, When reset, Then 全部回到默认值', () => {
    const ctx = useLinkListQuery({ pageSize: 50 });
    ctx.setKeyword('促销');
    ctx.setTypeFilter('landing');
    ctx.setStatusFilter('disabled');
    ctx.setPage(4);

    ctx.reset();

    expect(ctx.query.value).toEqual(DEFAULTS);
    expect(ctx.hasActiveFilter.value).toBe(false);
    // pageSize 也要回默认 20：只清筛选不清 pageSize 会让「重置」后请求仍按 50 条拉。
    expect(ctx.toParams()).toEqual({ page: 1, pageSize: 20 });
  });
});
