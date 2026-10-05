/**
 * 总览页时间范围筛选契约(GET /api/visits/overview?range=...)。
 *
 * 背景:总览此前只有"保留期内累计"一个口径 —— 看不出今天有没有量、这周比上周
 * 是涨是跌。这一组把筛选的交互契约钉死:
 *  1. 快捷范围固定为 今日 / 近 3 天 / 近 7 天 / 近一个月 / 近一年,默认近 7 天;
 *  2. 切换范围必须按新 range 重新请求聚合端点,而不是在前端裁剪旧数据;
 *  3. 新窗口的数据到达前,旧窗口的数字必须先从页面消失 —— 把 A 窗口的数字
 *     挂在 B 窗口的标签下,比看到加载态更糟;
 *  4. 加载期间筛选控件保持可见:正在切范围时控件消失,用户连改回都做不到。
 *
 * 窗口的边界语义(本地自然日、左闭、服务器时区)在服务端黑盒测试里:
 * internal/httpapi/visits_overview_range_test.go。
 */
import { flushPromises, mount } from '@vue/test-utils';
import type { VueWrapper } from '@vue/test-utils';

import type { OverviewStats } from '@/types/api';

const toastMock = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }));
const apiMock = vi.hoisted(() => ({
  fetchOverviewStats: vi.fn(),
  listDomains: vi.fn(),
}));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/visits', () => ({ fetchOverviewStats: apiMock.fetchOverviewStats }));
vi.mock('@/api/domains', () => ({ listDomains: apiMock.listDomains }));
// 地图组件自己动态 import 100KB 的国界数据,与本组断言无关,换成占位组件
vi.mock('@/views/overview/WorldMapPanel.vue', () => ({
  default: { name: 'WorldMapPanel', template: '<div data-test="world-map" />' },
}));

import OverviewView from '@/views/overview/OverviewView.vue';

/** 快捷范围契约:顺序即界面顺序,label 是给人看的那一份规格。 */
const RANGE_OPTIONS = [
  { value: 'today', label: '今日' },
  { value: '3d', label: '近 3 天' },
  { value: '7d', label: '近 7 天' },
  { value: '30d', label: '近一个月' },
  { value: '1y', label: '近一年' },
] as const;

/** 只给足页面渲染所需字段;数字取短值,便于在文本里唯一定位。 */
function statsOf(visits: number): OverviewStats {
  return {
    totals: {
      links: 1,
      activeLinks: 1,
      landingLinks: 0,
      visits,
      redirectVisits: visits,
      landingVisits: 0,
      clicks: 0,
    },
    topLinks: [],
    facets: {
      userAgents: [],
      sources: [],
      countries: [],
      userAgentCoverage: { total: 0, returned: 0, truncated: false },
      countryCoverage: { total: 0, returned: 0, truncated: false },
    },
  };
}

function mountOverview() {
  return mount(OverviewView, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        AppTooltip: { template: '<div><slot /></div>' },
      },
    },
  });
}

/** 快捷选择容器(与访问明细页的设备筛选同一交互模式:role=group + aria-pressed)。 */
function rangeGroup(wrapper: VueWrapper) {
  const group = wrapper.find('[role="group"][aria-label*="时间范围"]');
  if (!group.exists()) throw new Error('时间范围快捷选择未渲染');
  return group;
}

function rangeButtons(wrapper: VueWrapper) {
  return rangeGroup(wrapper).findAll('button');
}

function buttonOf(wrapper: VueWrapper, label: string) {
  const button = rangeButtons(wrapper).find((b) => b.text() === label);
  if (!button) throw new Error(`快捷范围按钮未渲染: ${label}`);
  return button;
}

describe('OverviewView 时间范围筛选', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.fetchOverviewStats.mockResolvedValue(statsOf(120));
    apiMock.listDomains.mockResolvedValue([]);
  });

  it('默认按近 7 天请求,并选中「近 7 天」', async () => {
    const wrapper = mountOverview();
    await flushPromises();

    expect(apiMock.fetchOverviewStats).toHaveBeenCalledWith('7d');

    const pressed = rangeButtons(wrapper).filter((b) => b.attributes('aria-pressed') === 'true');
    expect(pressed).toHaveLength(1);
    expect(pressed[0]?.text()).toBe('近 7 天');
  });

  it('五个快捷范围按固定顺序渲染', async () => {
    const wrapper = mountOverview();
    await flushPromises();

    expect(rangeButtons(wrapper).map((b) => b.text())).toEqual(RANGE_OPTIONS.map((o) => o.label));
  });

  it.each(RANGE_OPTIONS.filter((o) => o.value !== '7d'))(
    '点击「$label」按 $value 重新请求',
    async ({ value, label }) => {
      const wrapper = mountOverview();
      await flushPromises();
      apiMock.fetchOverviewStats.mockClear();

      await buttonOf(wrapper, label).trigger('click');
      await flushPromises();

      expect(apiMock.fetchOverviewStats).toHaveBeenCalledTimes(1);
      expect(apiMock.fetchOverviewStats).toHaveBeenCalledWith(value);
    },
  );

  it('切换范围时旧窗口的数字先消失,新数据到达后再显示,期间控件保持可见', async () => {
    apiMock.fetchOverviewStats.mockResolvedValue(statsOf(111));
    const wrapper = mountOverview();
    await flushPromises();
    expect(wrapper.text()).toContain('111');

    // 第二次请求挂起:模拟切到「今日」但数据还没回来。
    let resolveNext!: (value: OverviewStats) => void;
    apiMock.fetchOverviewStats.mockReturnValueOnce(
      new Promise<OverviewStats>((resolve) => {
        resolveNext = resolve;
      }),
    );
    await buttonOf(wrapper, '今日').trigger('click');
    await flushPromises();

    expect(wrapper.text()).not.toContain('111');
    expect(rangeGroup(wrapper).exists()).toBe(true);

    resolveNext(statsOf(222));
    await flushPromises();
    expect(wrapper.text()).toContain('222');
  });
});
