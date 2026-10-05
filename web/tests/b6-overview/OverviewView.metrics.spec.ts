/**
 * 总览指标重构契约。
 *
 * 1. 去重:「落地页点击数」KPI 与「短链类型与流量结构」卡里的数字与其它卡片完全
 *    重复(点击/CTR/访问量各出现两遍以上)。移除这两处,但**跳转/落地页占比**
 *    是结构卡唯一独有的信息,必须保留 —— 折进「总访问数」KPI 卡片。
 * 2. 失败与风险拦截:规则拦截(404/429)与短链失效都已在 visits 留痕,总览此前
 *    一个数都不显示。按原因列出,并给出失败率。
 * 3. 语言分布:lang 是规则引擎 13 个访客字段之一,却没有对应图表。
 * 4. 环比:只看绝对值分不清"有没有量"和"在涨还是在跌",访问量给出上一周期对比。
 *
 * 服务端口径(失败窗口/语言分桶/上一周期边界)见
 * internal/httpapi/visits_overview_metrics_test.go。
 */
import { flushPromises, mount } from '@vue/test-utils';

import type { OverviewStats } from '@/types/api';

const toastMock = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }));
const apiMock = vi.hoisted(() => ({
  fetchOverviewStats: vi.fn(),
  listDomains: vi.fn(),
}));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/visits', () => ({ fetchOverviewStats: apiMock.fetchOverviewStats }));
vi.mock('@/api/domains', () => ({ listDomains: apiMock.listDomains }));
vi.mock('@/views/overview/WorldMapPanel.vue', () => ({
  default: { name: 'WorldMapPanel', template: '<div data-test="world-map" />' },
}));

import OverviewView from '@/views/overview/OverviewView.vue';

interface StatsPatch {
  totals?: Record<string, unknown>;
  previousTotals?: Record<string, unknown> | null;
  facets?: Record<string, unknown>;
}

/**
 * 造一份总览响应。
 *
 * 用宽松类型 + 末尾断言:语言/失败/环比字段是本次新增的契约,
 * 在实现之前 TS 类型里还没有它们,这里不能因此编译不过。
 */
function statsOf(patch: StatsPatch = {}): OverviewStats {
  const stats = {
    totals: {
      links: 3,
      activeLinks: 2,
      landingLinks: 1,
      visits: 120,
      redirectVisits: 80,
      landingVisits: 40,
      clicks: 8,
      ...(patch.totals ?? {}),
    },
    previousTotals:
      patch.previousTotals === undefined
        ? { visits: 100, landingVisits: 32, clicks: 6 }
        : patch.previousTotals,
    topLinks: [],
    facets: {
      userAgents: [],
      sources: [],
      countries: [],
      userAgentCoverage: { total: 0, returned: 0, truncated: false },
      countryCoverage: { total: 0, returned: 0, truncated: false },
      ...(patch.facets ?? {}),
    },
  };
  return stats as unknown as OverviewStats;
}

async function mountWith(stats: OverviewStats) {
  apiMock.fetchOverviewStats.mockResolvedValue(stats);
  apiMock.listDomains.mockResolvedValue([]);
  const wrapper = mount(OverviewView, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        AppTooltip: { template: '<div><slot /></div>' },
      },
    },
  });
  await flushPromises();
  return wrapper;
}

describe('OverviewView 指标重构', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('重复卡片被移除,跳转/落地页占比折进访问量卡片', async () => {
    const wrapper = await mountWith(statsOf());

    // 与其它卡片完全重复的两处不再存在
    expect(wrapper.text()).not.toContain('落地页点击数');
    expect(wrapper.text()).not.toContain('短链类型与流量结构');

    // 结构卡唯一独有的信息(跳转 vs 落地页占比)保留在访问量 KPI 里
    const visitsCard = wrapper.find('[data-od-id="kpi-visits"]');
    expect(visitsCard.exists()).toBe(true);
    const split = visitsCard.find('[role="img"]');
    expect(split.exists()).toBe(true);
    expect(split.attributes('aria-label')).toContain('跳转型 80 次');
    expect(split.attributes('aria-label')).toContain('落地页型 40 次');
    expect(visitsCard.text()).toContain('跳转 80 / 落地页 40');

    // 点击数不再有独立卡片,但仍能从 CTR 卡片读到(不是被删了)
    expect(wrapper.text()).toContain('点击 8 / 落地页访问 40');
  });

  it('失败与风险拦截:按原因列出并给出失败率', async () => {
    const wrapper = await mountWith(
      statsOf({
        facets: {
          failureReasons: [
            { value: 'rule_blocked', count: 7 },
            { value: 'rule_throttled', count: 2 },
            { value: 'link_disabled', count: 1 },
          ],
        },
      }),
    );

    expect(wrapper.text()).toContain('失败与风险拦截');
    expect(wrapper.text()).toContain('规则拦截（404）');
    expect(wrapper.text()).toContain('7 次 · 70%');
    expect(wrapper.text()).toContain('规则限流（429）');
    expect(wrapper.text()).toContain('2 次 · 20%');
    expect(wrapper.text()).toContain('短链已停用');
    expect(wrapper.text()).toContain('1 次 · 10%');
    // 失败率 = 失败 / (成功访问 + 失败) = 10 / 130
    expect(wrapper.text()).toContain('失败率 7.7%');
  });

  it('窗口内没有失败时只给空态,不编造原因行', async () => {
    const wrapper = await mountWith(statsOf({ facets: { failureReasons: [] } }));

    expect(wrapper.text()).toContain('失败与风险拦截');
    expect(wrapper.text()).toContain('窗口内没有失败访问');
    expect(wrapper.text()).not.toContain('规则拦截（404）');
  });

  it('语言分布:按 Accept-Language 首标签分桶,「未携带」单列', async () => {
    const wrapper = await mountWith(
      statsOf({
        facets: {
          languages: [
            { value: 'zh-CN', count: 60 },
            { value: 'en', count: 30 },
            { value: '未携带', count: 10 },
          ],
          languageCoverage: { total: 100, returned: 100, truncated: false },
        },
      }),
    );

    expect(wrapper.text()).toContain('语言分布');
    expect(wrapper.text()).toContain('zh-CN');
    expect(wrapper.text()).toContain('60 次 · 60%');
    expect(wrapper.text()).toContain('未携带');
    expect(wrapper.text()).toContain('10 次 · 10%');
  });

  it.each([
    [100, '较上一周期 +20.0%'],
    [0, '较上一周期 新增'],
    [120, '较上一周期 持平'],
    [240, '较上一周期 -50.0%'],
  ])('访问量环比:上一周期 %i 次 → %s', async (previous, expected) => {
    const wrapper = await mountWith(statsOf({ previousTotals: { visits: previous, landingVisits: 0, clicks: 0 } }));

    expect(wrapper.text()).toContain(expected);
  });

  it('响应缺 previousTotals 时不显示环比(降级不撒谎)', async () => {
    const wrapper = await mountWith(statsOf({ previousTotals: null }));

    expect(wrapper.text()).not.toContain('较上一周期');
  });
});
