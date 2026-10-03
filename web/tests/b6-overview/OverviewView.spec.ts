/**
 * 总览页的数据加载契约。
 *
 * 这一组对着三个真实故障：
 *  1. `Promise.all` 把两个互不依赖的请求绑成同生共死 —— 域名列表挂掉就把
 *     **已经拿到的**统计一起丢掉，页面谎报「暂无流量访问数据」。
 *  2. 统计请求失败时的兜底只认 ApiError，非 ApiError（程序错误、chunk 404）
 *     既不提示也不记录，生产环境完全不可见。
 *  3. 可选链只保护到 `facets`，后端少返一个 coverage 字段整页就抛错。
 */
import { flushPromises, mount } from '@vue/test-utils';

import { ApiError } from '@/types/api';
import type { Domain, OverviewStats } from '@/types/api';

const toastMock = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }));
const apiMock = vi.hoisted(() => ({
  fetchOverviewStats: vi.fn(),
  listDomains: vi.fn(),
}));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/visits', () => ({ fetchOverviewStats: apiMock.fetchOverviewStats }));
vi.mock('@/api/domains', () => ({ listDomains: apiMock.listDomains }));
// 地图组件自己去动态 import 100KB 的国界数据，与本组断言无关，直接换成占位组件
vi.mock('@/views/overview/WorldMapPanel.vue', () => ({
  default: { name: 'WorldMapPanel', template: '<div data-test="world-map" />' },
}));

import OverviewView from '@/views/overview/OverviewView.vue';

function statsOf(overrides: Partial<OverviewStats> = {}): OverviewStats {
  return {
    totals: {
      links: 3,
      activeLinks: 2,
      landingLinks: 1,
      visits: 120,
      redirectVisits: 80,
      landingVisits: 40,
      clicks: 8,
    },
    topLinks: [{ id: 1, code: 'promo', linkType: 'redirect', visits: 120 }],
    facets: {
      userAgents: [
        { value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36', count: 120 },
      ],
      sources: [{ name: '直接访问', count: 120 }],
      countries: [{ value: 'US', count: 100 }],
      userAgentCoverage: { total: 120, returned: 1, truncated: false },
      countryCoverage: { total: 120, returned: 100, truncated: false },
    },
    ...overrides,
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

describe('OverviewView 加载', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('域名接口失败时，统计仍然显示（不回退成「暂无流量访问数据」）', async () => {
    apiMock.fetchOverviewStats.mockResolvedValue(statsOf());
    apiMock.listDomains.mockRejectedValue(new ApiError(500, 'E_INTERNAL', 'boom'));

    const wrapper = mountOverview();
    await flushPromises();

    // 统计已到：总访问数出现在 KPI 上，而不是空态卡片
    expect(wrapper.text()).toContain('总访问数');
    expect(wrapper.text()).toContain('120');
    expect(wrapper.text()).not.toContain('暂无流量访问数据');
    // 失败必须让用户看见，不能只闷在控制台
    expect(toastMock.error).toHaveBeenCalledWith('boom');
  });

  it('统计接口失败时，域名统计仍然生效（两个失败互不影响）', async () => {
    apiMock.fetchOverviewStats.mockRejectedValue(new ApiError(503, 'E_UNAVAILABLE', '统计服务不可用'));
    apiMock.listDomains.mockResolvedValue([
      { id: 1, fqdn: 'a.example.com', status: 'active' } as Domain,
      { id: 2, fqdn: 'b.example.com', status: 'pending' } as Domain,
    ]);

    const wrapper = mountOverview();
    await flushPromises();

    // 域名列表到了，KPI 的「承载域名」必须跟着更新，而不是被统计失败一并清成 0
    expect(apiMock.listDomains).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain('已激活 · 共 2 个域名');
    expect(toastMock.error).toHaveBeenCalledWith('统计服务不可用');
  });

  it('两个请求并发发出，不因为其中一个失败而取消另一个', async () => {
    apiMock.fetchOverviewStats.mockResolvedValue(statsOf());
    apiMock.listDomains.mockRejectedValue(new ApiError(500, 'E_INTERNAL', 'boom'));

    mountOverview();
    await flushPromises();

    expect(apiMock.fetchOverviewStats).toHaveBeenCalledTimes(1);
    expect(apiMock.listDomains).toHaveBeenCalledTimes(1);
  });

  it('非 ApiError 的异常被记录到控制台（DEV），不再被静默吞掉', async () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    apiMock.fetchOverviewStats.mockRejectedValue(new TypeError('Failed to fetch dynamically imported module'));
    apiMock.listDomains.mockResolvedValue([]);

    mountOverview();
    await flushPromises();

    expect(consoleSpy).toHaveBeenCalledTimes(1);
    expect(String(consoleSpy.mock.calls[0]?.[0])).toContain('overview');
    consoleSpy.mockRestore();
  });

  it('401 不弹错误提示（会话失效由全局拦截器处理）', async () => {
    apiMock.fetchOverviewStats.mockRejectedValue(new ApiError(401, 'E_UNAUTHORIZED', '未登录'));
    apiMock.listDomains.mockResolvedValue([]);

    mountOverview();
    await flushPromises();

    expect(toastMock.error).not.toHaveBeenCalled();
  });

  it('后端少返 userAgentCoverage / countryCoverage 时不抛错，页面照常渲染', async () => {
    const stats = statsOf();
    // 只给 totals/topLinks，facets 全缺：可选链必须一路兜到叶子
    apiMock.fetchOverviewStats.mockResolvedValue({
      totals: stats.totals,
      topLinks: stats.topLinks,
    } as unknown as OverviewStats);
    apiMock.listDomains.mockResolvedValue([]);

    const wrapper = mountOverview();
    await flushPromises();

    expect(wrapper.text()).toContain('总访问数');
    expect(wrapper.text()).toContain('120');
    expect(toastMock.error).not.toHaveBeenCalled();
  });

  it('facets 在但 coverage 字段缺失时不抛错', async () => {
    const stats = statsOf();
    apiMock.fetchOverviewStats.mockResolvedValue({
      totals: stats.totals,
      topLinks: stats.topLinks,
      facets: { userAgents: stats.facets.userAgents, sources: stats.facets.sources, countries: [] },
    } as unknown as OverviewStats);
    apiMock.listDomains.mockResolvedValue([]);

    const wrapper = mountOverview();
    await flushPromises();

    expect(wrapper.text()).toContain('设备类型分布');
  });
});