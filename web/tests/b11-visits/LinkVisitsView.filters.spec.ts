/**
 * 访问明细「只看失败」与关键词搜索的服务端过滤契约。
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

/**
 * 搜索是防抖下发的（300ms）。这里用条件轮询而不是固定 sleep:
 * 并行跑整个测试套件时,一次 flushPromises 都可能被调度饿到几百毫秒外,
 * 固定 sleep 会让「只发一次请求」这类断言偶发失败(实测出现过)。
 */
async function waitFor(assertion: () => void, timeout = 8000): Promise<void> {
  await vi.waitFor(assertion, { timeout, interval: 25 });
}

function searchInput(wrapper: ReturnType<typeof mountView>) {
  return wrapper.get('input[aria-label="搜索访问明细"]');
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

  it('Given 打开只看失败后没有任何记录, Then 空态可一键清空全部过滤', async () => {
    visitsApi.listVisits.mockResolvedValue({ items: [], total: 0 });
    const wrapper = mountView();
    await flushPromises();

    await toggleOnlyFailed(wrapper, true);
    await flushPromises();

    expect(wrapper.text()).toContain('没有失败的访问记录');
    const clear = wrapper.findAll('button').find((btn) => btn.text() === '清空过滤条件');
    expect(clear, '空态里要有清除过滤的入口').toBeTruthy();

    await clear!.trigger('click');
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

describe('LinkVisitsView 关键词搜索', () => {
  it('Given 输入关键词, When 防抖结束, Then 请求带 keyword 且输入框内容不被吞', async () => {
    const wrapper = mountView();
    await flushPromises();
    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, { page: 1, pageSize: 20 });

    await searchInput(wrapper).setValue('203.0.113');
    await waitFor(() =>
      expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, {
        page: 1,
        pageSize: 20,
        keyword: '203.0.113',
      }),
    );
    // 防抖期间 v-model 不能把输入框回写成旧值,否则用户会看到字被吞掉
    expect((searchInput(wrapper).element as HTMLInputElement).value).toBe('203.0.113');

    wrapper.unmount();
  });

  it('Given 连续输入, When 防抖结束, Then 只发一次带最终关键词的请求', async () => {
    // 假时钟:这条断言依赖「三次输入落在同一防抖窗口内」,真时钟在并行跑套件时
    // 随时可能被切开(实测曾因此偶发失败)
    vi.useFakeTimers();
    try {
      const wrapper = mountView();
      await flushPromises();
      const baseline = visitsApi.listVisits.mock.calls.length;

      await searchInput(wrapper).setValue('2');
      await searchInput(wrapper).setValue('20');
      await searchInput(wrapper).setValue('203');
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(350);
      await flushPromises();

      const added = visitsApi.listVisits.mock.calls.slice(baseline);
      expect(added, '逐字符发请求会让明细列表抖成"打字机"').toHaveLength(1);
      expect(added[0]![1]).toEqual({ page: 1, pageSize: 20, keyword: '203' });

      wrapper.unmount();
    } finally {
      vi.useRealTimers();
    }
  });

  it('Given 服务端按关键词返回 total, Then 总数与分页都按筛选后的结果集', async () => {
    visitsApi.listVisits
      .mockResolvedValueOnce({ items: [visitOf()], total: 40 }) // 初次:两页
      .mockResolvedValue({ items: [visitOf({ ip: '203.0.113.9' })], total: 3 }); // 搜索后:一页

    const wrapper = mountView();
    await flushPromises();
    expect(wrapper.text()).toContain('1 / 2');

    await searchInput(wrapper).setValue('203.0.113.9');
    await waitFor(() => expect(wrapper.text()).toContain('共 3 条访问明细'));
    expect(wrapper.text()).toContain('1 / 1');

    wrapper.unmount();
  });

  it('Given 关键词搜索生效, Then 服务端返回的行不再被本地二次过滤', async () => {
    // 服务端说匹配就用它:前端再拿关键词去匹配 IP/UA 只会把服务端认定的结果丢掉
    visitsApi.listVisits
      .mockResolvedValueOnce({ items: [visitOf()], total: 1 })
      .mockResolvedValue({ items: [visitOf({ ip: '198.51.100.7' })], total: 1 });

    const wrapper = mountView();
    await flushPromises();

    await searchInput(wrapper).setValue('zzz-not-in-row');
    await waitFor(() => expect(wrapper.text()).toContain('198.51.100.7'));

    wrapper.unmount();
  });

  it('Given 关键词已生效, When 清空过滤, Then 请求回到不带 keyword 的条件', async () => {
    const wrapper = mountView();
    await flushPromises();

    await searchInput(wrapper).setValue('203.0.113');
    await waitFor(() =>
      expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, {
        page: 1,
        pageSize: 20,
        keyword: '203.0.113',
      }),
    );

    const clear = wrapper.findAll('button').find((btn) => btn.text() === '清空过滤');
    expect(clear, '筛选生效时应出现「清空过滤」').toBeTruthy();
    await clear!.trigger('click');
    await flushPromises();

    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, { page: 1, pageSize: 20 });
    expect((searchInput(wrapper).element as HTMLInputElement).value).toBe('');

    wrapper.unmount();
  });
});

describe('LinkVisitsView 时间窗口', () => {
  /** 工具栏里的时间窗口按钮（按文案定位，避免依赖 DOM 顺序） */
  function rangeButton(wrapper: ReturnType<typeof mountView>, label: string) {
    const btn = wrapper.findAll('button').find((b) => b.text() === label);
    expect(btn, `工具栏应有「${label}」窗口按钮`).toBeTruthy();
    return btn!;
  }

  it('Given 选择「近 7 天」, Then 请求带 range=7d', async () => {
    const wrapper = mountView();
    await flushPromises();

    await rangeButton(wrapper, '近 7 天').trigger('click');
    await flushPromises();

    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, {
      page: 1,
      pageSize: 20,
      range: '7d',
    });

    wrapper.unmount();
  });

  it('Given 时间窗口生效, Then 总数与分页按窗口内的结果集计算', async () => {
    visitsApi.listVisits
      .mockResolvedValueOnce({ items: [visitOf()], total: 40 }) // 不限时间:两页
      .mockResolvedValue({ items: [visitOf()], total: 3 }); // 窗口内:一页

    const wrapper = mountView();
    await flushPromises();
    expect(wrapper.text()).toContain('1 / 2');

    await rangeButton(wrapper, '今日').trigger('click');
    await flushPromises();

    expect(wrapper.text()).toContain('共 3 条访问明细');
    expect(wrapper.text()).toContain('1 / 1');

    wrapper.unmount();
  });

  it('Given 只选了时间窗口, When 清空过滤, Then 请求回到不限时间', async () => {
    const wrapper = mountView();
    await flushPromises();

    await rangeButton(wrapper, '近 30 天').trigger('click');
    await flushPromises();
    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, {
      page: 1,
      pageSize: 20,
      range: '30d',
    });

    const clear = wrapper.findAll('button').find((btn) => btn.text() === '清空过滤');
    expect(clear, '只选时间窗口时也应出现「清空过滤」').toBeTruthy();
    await clear!.trigger('click');
    await flushPromises();

    expect(visitsApi.listVisits).toHaveBeenLastCalledWith(LINK_ID, { page: 1, pageSize: 20 });

    wrapper.unmount();
  });
});

describe('LinkVisitsView 导出', () => {
  let createObjectURL: ReturnType<typeof vi.fn>;
  let anchorClick: ReturnType<typeof vi.spyOn>;
  let originalCreate: typeof URL.createObjectURL;
  let originalBlob: typeof Blob;
  let blobParts: BlobPart[] = [];

  beforeEach(() => {
    blobParts = [];
    createObjectURL = vi.fn(() => 'blob:mock-url');
    originalCreate = URL.createObjectURL;
    URL.createObjectURL = createObjectURL as unknown as typeof URL.createObjectURL;
    anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    const RealBlob = globalThis.Blob;
    originalBlob = globalThis.Blob;
    globalThis.Blob = class extends RealBlob {
      constructor(parts: BlobPart[] = [], options?: BlobPropertyBag) {
        super(parts, options);
        blobParts = parts;
      }
    } as typeof Blob;
  });

  afterEach(() => {
    URL.createObjectURL = originalCreate;
    globalThis.Blob = originalBlob;
    anchorClick.mockRestore();
  });

  function csvText(): string {
    return blobParts.map((part) => String(part)).join('');
  }

  it('Given 有明细, When 点导出, Then 文件名含短码且表头是访问明细的列', async () => {
    const wrapper = mountView();
    await flushPromises();

    const exportBtn = wrapper.findAll('button').find((b) => /导出本页 CSV/.test(b.text()));
    expect(exportBtn, '页头应有导出按钮').toBeTruthy();
    await exportBtn!.trigger('click');

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(anchorClick).toHaveBeenCalledTimes(1);

    const text = csvText();
    expect(text.startsWith('\uFEFF'), '缺 BOM 时 Excel 打开中文是乱码').toBe(true);
    for (const col of ['访问时间', '访客 IP', '失败原因', '完整 User-Agent']) {
      expect(text).toContain(col);
    }
    // 数据行:失败原因用界面文案而不是原始 reason 码
    expect(text).toContain('无可用目标');

    wrapper.unmount();
  });

  it('Given 当前页没有明细, Then 导出按钮禁用（不产出空文件）', async () => {
    visitsApi.listVisits.mockResolvedValue({ items: [], total: 0 });
    const wrapper = mountView();
    await flushPromises();

    const exportBtn = wrapper.findAll('button').find((b) => /导出本页 CSV/.test(b.text()));
    expect(exportBtn?.attributes('disabled'), '空列表时不该允许导出').toBeDefined();
    expect(createObjectURL).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it('Given 设备类型在本页被过滤, Then 导出的是过滤后的行（与屏幕一致）', async () => {
    visitsApi.listVisits.mockResolvedValue({
      items: [
        visitOf({ id: 1, ip: '198.51.100.1', userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)' }),
        visitOf({ id: 2, ip: '198.51.100.2', userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)' }),
      ],
      total: 2,
    });
    const wrapper = mountView();
    await flushPromises();

    const mobileBtn = wrapper.findAll('button').find((b) => b.text() === '移动端');
    expect(mobileBtn, '工具栏应有设备筛选项').toBeTruthy();
    await mobileBtn!.trigger('click');
    await flushPromises();

    const exportBtn = wrapper.findAll('button').find((b) => /导出本页 CSV/.test(b.text()));
    await exportBtn!.trigger('click');

    const text = csvText();
    expect(text).toContain('198.51.100.1');
    expect(text).not.toContain('198.51.100.2');

    wrapper.unmount();
  });
});
