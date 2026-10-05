/**
 * 总览时间范围的 URL 契约。
 *
 * 时间范围是"可刷新、可分享"的状态:选中今日后刷新页面应该还在今日,
 * 把链接发给同事看到的也必须是同一个窗口。地址栏与页面不允许各说各话 ——
 * URL 里的非法值按默认窗口加载后,要把它规范回合法值。
 *
 * 与服务端窗口语义的契约见 internal/httpapi/visits_overview_range_test.go。
 */
import { flushPromises, mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';

import type { OverviewStats } from '@/types/api';

const apiMock = vi.hoisted(() => ({
  fetchOverviewStats: vi.fn(),
  listDomains: vi.fn(),
}));

vi.mock('@/utils/toast', () => ({
  message: { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() },
}));
vi.mock('@/api/visits', () => ({ fetchOverviewStats: apiMock.fetchOverviewStats }));
vi.mock('@/api/domains', () => ({ listDomains: apiMock.listDomains }));
vi.mock('@/views/overview/WorldMapPanel.vue', () => ({
  default: { name: 'WorldMapPanel', template: '<div data-test="world-map" />' },
}));

import OverviewView from '@/views/overview/OverviewView.vue';

function statsOf(): OverviewStats {
  return {
    totals: {
      links: 1,
      activeLinks: 1,
      landingLinks: 0,
      visits: 10,
      redirectVisits: 10,
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

/** 在指定 query 下挂载总览页(真实 router:memory history,不 mock useRoute)。 */
async function mountAt(query: Record<string, string> = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/overview', component: { template: '<div />' } },
      { path: '/links', component: { template: '<div />' } },
      { path: '/links/:id/visits', component: { template: '<div />' } },
    ],
  });
  await router.push({ path: '/overview', query });
  await router.isReady();

  const wrapper = mount(OverviewView, {
    global: {
      plugins: [router],
      stubs: { AppTooltip: { template: '<div><slot /></div>' } },
    },
  });
  await flushPromises();
  return { wrapper, router };
}

/** 快捷选择里的按钮(与访问明细页的筛选同一交互模式)。 */
function rangeButtons(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('[role="group"][aria-label*="时间范围"] button');
}

describe('OverviewView 时间范围 URL 同步', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.fetchOverviewStats.mockResolvedValue(statsOf());
    apiMock.listDomains.mockResolvedValue([]);
  });

  it('带 ?range=today 打开时按今日加载并选中今日', async () => {
    const { wrapper } = await mountAt({ range: 'today' });

    expect(apiMock.fetchOverviewStats).toHaveBeenCalledWith('today');
    const pressed = rangeButtons(wrapper).filter((b) => b.attributes('aria-pressed') === 'true');
    expect(pressed).toHaveLength(1);
    expect(pressed[0]?.text()).toBe('今日');
  });

  it('切换范围后 URL 带上新值(可刷新、可分享)', async () => {
    const { wrapper, router } = await mountAt();
    const button = rangeButtons(wrapper).find((b) => b.text() === '近一个月');
    if (!button) throw new Error('近一个月按钮未渲染');

    await button.trigger('click');
    await flushPromises();

    expect(router.currentRoute.value.query.range).toBe('30d');
    expect(apiMock.fetchOverviewStats).toHaveBeenLastCalledWith('30d');
  });

  it('URL 里是非法范围时按默认 7d 加载,并把地址栏规范回 7d', async () => {
    const { router } = await mountAt({ range: 'week' });

    expect(apiMock.fetchOverviewStats).toHaveBeenCalledWith('7d');
    expect(router.currentRoute.value.query.range).toBe('7d');
  });
});
