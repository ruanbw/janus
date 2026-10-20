/**
 * 访问明细「只看失败」的服务端过滤契约。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 用户在访问明细页打开「只看失败」
 *   When  前端请求 /links/:id/visits
 *   Then  必须带 outcome=failed，并且总条数 / 分页以**过滤后的结果集**为准
 *
 * 为什么值得单独钉住：后端 handleListVisits 早就支持 action / outcome 过滤，
 * useVisitFilters 也一直在产出这两个参数，但这个页面只把 page/pageSize 下发，
 * 「只看失败」是在**当前页**里再筛一遍。于是：
 *   - 「共 N 条」与分页器显示的是全量，用户看到的失败条数只是第 1 页里的；
 *   - 打开过滤后翻页，第 2 页会把刚才被隐藏的成功记录又放出来；
 *   - 空态文案写着「仅在当前页数据内过滤」，用户只能自己意识到数字不可信。
 * 这三点都是「界面能渲染、数字却是错的」，最容易被当成"功能就这样"。
 */
import { flushPromises, mount } from '@vue/test-utils';

import type { Link, Visit } from '@/types/api';

const toastMock = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }));
const visitsApi = vi.hoisted(() => ({ listVisits: vi.fn() }));
const linksApi = vi.hoisted(() => ({ getLink: vi.fn() }));
const ruleTraceMock = vi.hoisted(() => ({
  loadAllRules: vi.fn(),
  buildDecisionTrace: vi.fn(),
  verdictOf: vi.fn(),
}));
const routerMock = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/visits', () => visitsApi);
vi.mock('@/api/links', () => linksApi);
// 回放链路与本组断言无关，且会去打真实接口；整体替换成空实现
vi.mock('@/views/rules/ruleTrace', () => ruleTraceMock);
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return {
    ...actual,
    useRouter: () => routerMock,
    useRoute: () => ({ params: { id: '5' } }),
  };
});

import UI from '@/components/app';
import LinkVisitsView from '@/views/links/LinkVisitsView.vue';

const LINK_ID = 5;

function linkOf(overrides: Partial<Link> = {}): Link {
  return {
    id: LINK_ID,
    code: 'abc123',
    targetUrls: ['https://example.com/landing'],
    redirectStatus: '302',
    linkType: 'redirect',
    landingSource: 'url',
    landingUrl: '',
    status: 'enabled',
    rulesEnabled: false,
    domains: ['alice.janus.test'],
    visits: 25,
    clicks: 0,
    clickVisits: 0,
    landingUploaded: false,
    ruleCount: 0,
    ruleNames: [],
    rules: [],
    createdAt: '2026-10-01T00:00:00Z',
    ...overrides,
  };
}

function visitOf(overrides: Partial<Visit> = {}): Visit {
  return {
    id: 1,
    linkId: LINK_ID,
    domain: 'alice.janus.test',
    ip: '203.0.113.9',
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)',
    referer: '',
    action: 'redirect',
    outcome: 'failed',
    reason: 'no_target',
    targetUrl: '',
    country: 'US',
    lang: 'en-US',
    ruleId: null,
    ruleAction: '',
    createdAt: '2026-10-20T10:00:00Z',
    ...overrides,
  };
}

function mountView() {
  return mount(LinkVisitsView, {
    global: {
      plugins: [UI],
      stubs: { Teleport: true },
    },
  });
}

/** 打开 / 关闭「只看失败」开关：走真实 DOM 事件，避免绕过 AppSwitch 的 v-model */
async function toggleOnlyFailed(wrapper: ReturnType<typeof mountView>, on: boolean): Promise<void> {
  const box = wrapper.get('[aria-label="只看失败"]');
  const checked = box.attributes('aria-checked') === 'true' || box.attributes('data-state') === 'checked';
  if (checked !== on) {
    await box.trigger('click');
    await flushPromises();
  }
}

beforeEach(() => {
  vi.clearAllMocks();
  ruleTraceMock.loadAllRules.mockResolvedValue([]);
  linksApi.getLink.mockResolvedValue(linkOf());
  visitsApi.listVisits.mockResolvedValue({ items: [visitOf()], total: 1 });
});

describe('LinkVisitsView「只看失败」', () => {
  it('Given 初始进入页面, When 未打开只看失败, Then 请求不带 outcome', async () => {
    const wrapper = mountView();
    await flushPromises();

    expect(visitsApi.listVisits).toHaveBeenCalledWith(LINK_ID, { page: 1, pageSize: 20 });

    wrapper.unmount();
  });

  it('Given 已停在第 2 页, When 打开只看失败, Then 请求带 outcome=failed 且页码归 1', async () => {
    visitsApi.listVisits.mockResolvedValue({ items: [visitOf()], total: 60 });
    const wrapper = mountView();
    await flushPromises();

    // 先翻到第 2 页：过滤条件变化必须把页码归 1，否则用户会停在一个空页
    const next = wrapper.findAll('button').find((btn) => btn.text() === '下一页');
    expect(next, '分页器里应有「下一页」').toBeTruthy();
    await next!.trigger('click');
    await flushPromises();
    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, { page: 2, pageSize: 20 });

    await toggleOnlyFailed(wrapper, true);

    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, {
      page: 1,
      pageSize: 20,
      outcome: 'failed',
    });

    wrapper.unmount();
  });

  it('Given 服务端只回失败的记录, Then 总条数与总页数按过滤后的结果集计算', async () => {
    // 25 条失败记录：分页器应是 1 / 2，而不是拿全量 60 条算出来
    visitsApi.listVisits.mockResolvedValue({
      items: [visitOf({ id: 1 }), visitOf({ id: 2 })],
      total: 25,
    });
    const wrapper = mountView();
    await flushPromises();

    await toggleOnlyFailed(wrapper, true);

    expect(wrapper.text()).toContain('25');
    expect(wrapper.text()).toContain('1 / 2');

    wrapper.unmount();
  });

  it('Given 只看失败已打开, When 再关闭, Then 请求恢复为不带 outcome 且不再有失败限定', async () => {
    const wrapper = mountView();
    await flushPromises();

    await toggleOnlyFailed(wrapper, true);
    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, {
      page: 1,
      pageSize: 20,
      outcome: 'failed',
    });

    await toggleOnlyFailed(wrapper, false);
    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, { page: 1, pageSize: 20 });

    wrapper.unmount();
  });

  it('Given 打开只看失败后没有任何记录, Then 空态提供「显示全部访问」并能一键清除过滤', async () => {
    visitsApi.listVisits.mockResolvedValue({ items: [], total: 0 });
    const wrapper = mountView();
    await flushPromises();

    await toggleOnlyFailed(wrapper, true);
    await flushPromises();

    expect(wrapper.text()).toContain('没有失败的访问记录');
    const showAll = wrapper.findAll('button').find((btn) => btn.text() === '显示全部访问');
    expect(showAll, '空态里要有清除过滤的入口').toBeTruthy();

    await showAll!.trigger('click');
    await flushPromises();
    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, { page: 1, pageSize: 20 });

    wrapper.unmount();
  });

  it('Given 快速连续切换只看失败, Then 过期响应不覆盖最后一次请求的结果', async () => {
    // 第一次(慢)与第二次(快)都返回,但只有后发的那次能写进表格
    let resolveSlow: ((value: unknown) => void) | null = null;
    const slow = new Promise((resolve) => {
      resolveSlow = resolve;
    });
    visitsApi.listVisits
      .mockResolvedValueOnce({ items: [visitOf({ ip: '198.51.100.1' })], total: 1 }) // 初次加载
      .mockReturnValueOnce(slow) // 打开过滤：慢请求
      .mockResolvedValueOnce({ items: [visitOf({ ip: '198.51.100.2' })], total: 1 }); // 关闭过滤：快请求

    const wrapper = mountView();
    await flushPromises();

    await toggleOnlyFailed(wrapper, true); // 慢请求在途
    await toggleOnlyFailed(wrapper, false); // 快请求先回
    await flushPromises();
    expect(wrapper.text()).toContain('198.51.100.2');

    // 慢请求这时才回来：它携带的是「只看失败」的旧结果，不能覆盖当前视图
    resolveSlow!({ items: [visitOf({ ip: '198.51.100.9' })], total: 1 });
    await flushPromises();
    expect(wrapper.text()).not.toContain('198.51.100.9');
    expect(wrapper.text()).toContain('198.51.100.2');

    wrapper.unmount();
  });
});
