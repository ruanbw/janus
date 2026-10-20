/**
 * 访问明细筛选状态（useVisitFilters）的冻结契约。
 *
 * 背景：LinkVisitsView.vue 把四组筛选、分页和「是否激活过滤」的判断散在模板里，
 * 每加一个筛选项就要同步改三处（v-model、请求参数拼装、重置按钮显隐），
 * 抽 composable 之前先把边界钉死。下面三条是最容易在重构中回退的语义：
 *
 *  1. **任何筛选变化都必须把页码归 1**。用户停在第 5 页改筛选、却只看得到
 *     第 5 页的空结果，是这个页面最常见的「筛不出东西」投诉来源。
 *  2. **空字符串字段不下发给后端**。后端拿「参数缺失」当不过滤，显式空串
 *     没有约定语义，别把解析歧义压到后端。
 *  3. **keyword / country 一律 trim**。前后空格会让等值/模糊匹配静默空结果，
 *     而且会让「重置筛选」按钮在结果集并未变化时亮着。
 */
import { describe, expect, it } from 'vitest';

import { useVisitFilters, type VisitFilterState } from '@/composables/useVisitFilters';

const EMPTY: VisitFilterState = { action: '', outcome: '', country: '', keyword: '', range: '' };

describe('useVisitFilters 初始状态', () => {
  it('Given 不传 initial，When 创建，Then 筛选全空、第 1 页、每页 20 条', () => {
    const f = useVisitFilters();

    expect(f.filters.value).toEqual(EMPTY);
    expect(f.page.value).toBe(1);
    expect(f.pageSize.value).toBe(20);
    // 四个字段都空时不应显示「重置筛选」之类的入口
    expect(f.hasActive.value).toBe(false);
  });

  it('Given 只传 initial 的部分字段，When 创建，Then 只覆盖对应字段，分页仍是默认值', () => {
    const f = useVisitFilters({ outcome: 'failed', keyword: 'abc' });

    expect(f.filters.value).toEqual({ ...EMPTY, outcome: 'failed', keyword: 'abc' });
    expect(f.page.value).toBe(1);
    expect(f.pageSize.value).toBe(20);
  });

  it('Given 默认状态，When 取 toParams，Then 只剩分页两个参数', () => {
    // 空筛选字段必须整体缺席，而不是以 `action=` 的形式下发
    expect(useVisitFilters().toParams()).toEqual({ page: 1, pageSize: 20 });
  });
});

describe('useVisitFilters.set：任何筛选变化都回到第 1 页', () => {
  it('Given 停在第 4 页且已有其它筛选，When set 只改 action，Then page 归 1 且其它筛选保留', () => {
    const f = useVisitFilters({ keyword: 'abc' });
    f.setPage(4);

    f.set({ action: 'click' });

    expect(f.filters.value.action).toBe('click');
    // 局部 patch 不能把没提到的字段洗掉，否则表单里其它筛选会莫名清空
    expect(f.filters.value.keyword).toBe('abc');
    expect(f.page.value).toBe(1);
  });

  it('Given 停在第 3 页，When set 一个与分页无关的国家过滤，Then 仍然归 1', () => {
    const f = useVisitFilters();
    f.setPage(3);

    f.set({ country: 'CN' });

    expect(f.page.value).toBe(1);
  });

  it('Given 已有动作过滤，When set 传空串清除它，Then 动作回归「全部」且其它字段不受影响', () => {
    const f = useVisitFilters({ action: 'redirect', country: 'JP' });

    f.set({ action: '' });

    expect(f.filters.value).toEqual({ ...EMPTY, country: 'JP' });
  });

  it.each([
    ['keyword', '  abc  ', 'abc'],
    ['country', '  CN ', 'CN'],
  ] as const)('Given %s 前后带空格，When set，Then trim 后再存储', (field, raw, trimmed) => {
    const f = useVisitFilters();

    f.set({ [field]: raw });

    expect(f.filters.value[field]).toBe(trimmed);
  });
});

describe('useVisitFilters.hasActive', () => {
  it.each([
    ['动作', { action: 'redirect' }],
    ['结果', { outcome: 'failed' }],
    ['国家', { country: 'CN' }],
    ['关键字', { keyword: 'chrome' }],
  ] as const)('Given 只设置了 %s 筛选，When 读 hasActive，Then 为 true', (_label, patch) => {
    expect(useVisitFilters(patch as Partial<VisitFilterState>).hasActive.value).toBe(true);
  });

  it('Given 默认状态，When set 一个关键字，Then hasActive 立刻变 true（必须是响应式 computed）', () => {
    const f = useVisitFilters();
    expect(f.hasActive.value).toBe(false);

    f.set({ keyword: 'chrome' });

    expect(f.hasActive.value).toBe(true);
  });

  it('Given 只有空格的 keyword，When set，Then trim 成空串且不算激活筛选', () => {
    // 否则「重置筛选」按钮会亮着，但查询结果与不过滤完全一样
    const f = useVisitFilters();

    f.set({ keyword: '   ' });

    expect(f.filters.value.keyword).toBe('');
    expect(f.hasActive.value).toBe(false);
  });
});

describe('useVisitFilters.setPage', () => {
  it('Given 正常页码，When setPage，Then 原样采用（调用方只知道当前页码，没有上界可钳）', () => {
    const f = useVisitFilters();

    f.setPage(99);

    expect(f.page.value).toBe(99);
  });

  it.each([0, -3])('Given 非法页码 %i，When setPage，Then 钳到 1', (bad) => {
    const f = useVisitFilters();
    f.setPage(5);

    f.setPage(bad);

    expect(f.page.value).toBe(1);
  });
});

describe('useVisitFilters.toParams', () => {
  it('Given 四组筛选都有值，When toParams，Then 分页数字与 trim 后的字符串字段并存', () => {
    const f = useVisitFilters();
    f.set({ action: 'landing_view', outcome: 'success', country: ' US ', keyword: ' chrome ' });
    f.setPage(3);

    expect(f.toParams()).toEqual({
      page: 3,
      pageSize: 20,
      action: 'landing_view',
      outcome: 'success',
      country: 'US',
      keyword: 'chrome',
    });
  });

  it('Given 只设置了部分筛选，When toParams，Then 未设置字段整体不出现', () => {
    const f = useVisitFilters();
    f.set({ outcome: 'failed' });

    const params = f.toParams();

    expect(Object.keys(params).sort()).toEqual(['outcome', 'page', 'pageSize']);
  });

  it('Given 页码与每页条数被改动，When toParams，Then 下发当前值而不是初始值', () => {
    const f = useVisitFilters();
    f.setPage(2);
    f.pageSize.value = 50;

    expect(f.toParams()).toMatchObject({ page: 2, pageSize: 50 });
  });

  it('Given 筛选被逐个清空，When toParams，Then 又只剩分页参数', () => {
    const f = useVisitFilters();
    f.set({ action: 'click', keyword: 'x' });

    f.set({ action: '', keyword: '' });

    expect(f.toParams()).toEqual({ page: 1, pageSize: 20 });
  });
});

describe('useVisitFilters.reset', () => {
  it('Given 任意筛选加翻页，When reset，Then 四字段清空、page 回到 1、请求参数同步收敛', () => {
    const f = useVisitFilters({
      action: 'click',
      outcome: 'success',
      country: 'CN',
      keyword: 'x',
    });
    f.setPage(6);

    f.reset();

    expect(f.filters.value).toEqual(EMPTY);
    expect(f.page.value).toBe(1);
    expect(f.hasActive.value).toBe(false);
    expect(f.toParams()).toEqual({ page: 1, pageSize: 20 });
  });

  it('Given 已 reset 过一次，When 再 reset，Then 状态保持默认（幂等）', () => {
    const f = useVisitFilters({ keyword: 'x' });

    f.reset();
    f.reset();

    expect(f.filters.value).toEqual(EMPTY);
    expect(f.page.value).toBe(1);
  });
});

describe('useVisitFilters 时间窗口', () => {
  it('Given 设置 range=7d, When toParams, Then 下发 range 且页码归 1', () => {
    const f = useVisitFilters();
    f.setPage(3);

    f.set({ range: '7d' });

    expect(f.page.value, '换窗口后旧页码已指不到同一批数据').toBe(1);
    expect(f.toParams()).toEqual({ page: 1, pageSize: 20, range: '7d' });
    expect(f.hasActive.value, '只有时间窗口也属于"正在过滤"').toBe(true);
  });

  it('Given range 生效, When reset, Then 回到不限时间', () => {
    const f = useVisitFilters({ range: '30d' });

    f.reset();

    expect(f.filters.value.range).toBe('');
    expect(f.toParams()).toEqual({ page: 1, pageSize: 20 });
  });

  it('Given range=all 之外的非法值, When 创建, Then 原样保留(校验在服务端)', () => {
    // composable 不做值域校验:非法值应由后端 400,而不是前端悄悄改成"全部"
    const f = useVisitFilters({ range: 'bogus' as never });
    expect(f.filters.value.range).toBe('bogus');
    expect(f.toParams().range).toBe('bogus');
  });
});
