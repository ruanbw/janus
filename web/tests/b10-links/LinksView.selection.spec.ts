/**
 * 短链列表批量选择的可见性不变式。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 用户勾选了若干条短链
 *   When  任何会让这些行不再可见的操作发生（翻页、刷新、换筛选）
 *   Then  选择随之清空，批量操作永远只作用于「用户此刻看得见」的行
 *
 * 为什么值得钉住：批量删除/彻底删除的确认文案里列的是 `selectedCodesSummary()`,
 * 而它只遍历当前可见行（filteredLinks）。可 `ids` 取的是完整 selectedIds ——
 * 于是「已选 5 条」的确认框里只列出 3 个短码，另外 2 条会被静默删掉。
 *
 * 真实触发路径（修复前）：停在第 1 页 → 勾选 2 条 → 在搜索框里输入关键字把其中
 * 一条筛掉。筛选是本地 computed，不会重载数据，所以 loadData 里那句 clearSelection
 * 根本不会执行；批量条上还显示「已选 2 / 1 条(当前页筛选结果)」，数字自己都不自洽。
 */
import { flushPromises, mount } from '@vue/test-utils';

import type { Link } from '@/types/api';

const toastMock = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }));
const linksApi = vi.hoisted(() => ({
  listLinks: vi.fn(),
  listDeletedLinks: vi.fn(),
  batchDeleteLinks: vi.fn(),
  batchPurgeLinks: vi.fn(),
  createLink: vi.fn(),
  deleteLink: vi.fn(),
  getLink: vi.fn(),
  purgeLink: vi.fn(),
  restoreLink: vi.fn(),
  updateLink: vi.fn(),
  uploadLanding: vi.fn(),
}));
const domainsApi = vi.hoisted(() => ({ listDomains: vi.fn() }));
const confirmMock = vi.hoisted(() => ({ confirm: vi.fn() }));
const routerMock = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/links', () => linksApi);
vi.mock('@/api/domains', () => domainsApi);
vi.mock('@/components/app/confirm', () => confirmMock);
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return {
    ...actual,
    useRouter: () => routerMock,
    useRoute: () => ({ query: {}, params: {} }),
  };
});

import UI from '@/components/app';
import LinksView from '@/views/links/LinksView.vue';

function linkOf(code: string, id: number, overrides: Partial<Link> = {}): Link {
  return {
    id,
    code,
    targetUrls: [`https://example.com/${code}`],
    redirectStatus: '302',
    linkType: 'redirect',
    landingSource: 'url',
    landingUrl: '',
    status: 'enabled',
    rulesEnabled: false,
    domains: ['alice.janus.test'],
    visits: 0,
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

function mountView() {
  return mount(LinksView, {
    global: {
      plugins: [UI],
      stubs: { Teleport: true },
    },
  });
}

/** 行内复选框：aria-label 就是「选中短链 <code>」 */
async function selectRow(wrapper: ReturnType<typeof mountView>, code: string): Promise<void> {
  const box = wrapper.get(`[role="checkbox"][aria-label="选中短链 ${code}"]`);
  await box.trigger('click');
  await flushPromises();
}

function batchBar(wrapper: ReturnType<typeof mountView>) {
  return wrapper.find('[data-od-id="link-batch-bar"]');
}

/** 搜索是防抖下发的：用条件轮询代替固定 sleep(并行跑套件时 sleep 会被调度饿到很晚) */
async function waitFor(assertion: () => void, timeout = 8000): Promise<void> {
  await vi.waitFor(assertion, { timeout, interval: 25 });
}

beforeEach(() => {
  vi.clearAllMocks();
  domainsApi.listDomains.mockResolvedValue([]);
  linksApi.listDeletedLinks.mockResolvedValue({ items: [], total: 0 });
  linksApi.listLinks.mockResolvedValue({
    items: [linkOf('abc123', 1), linkOf('def456', 2), linkOf('ghi789', 3)],
    total: 3,
  });
});

describe('LinksView 批量选择的可见性', () => {
  it('Given 勾选了一条短链, When 搜索关键字把它筛掉, Then 批量条消失(选择已清空)', async () => {
    const wrapper = mountView();
    await flushPromises();

    await selectRow(wrapper, 'abc123');
    expect(batchBar(wrapper).exists(), '勾选后应出现批量操作条').toBe(true);
    expect(batchBar(wrapper).text()).toContain('已选');

    // 关键字只匹配另外两条：abc123 被筛掉后就不该还留在选择里
    await wrapper.get('input[aria-label="搜索短链"]').setValue('def456');
    await waitFor(() =>
      expect(batchBar(wrapper).exists(), '被筛掉的行不能再被批量删除').toBe(false),
    );
    expect(linksApi.batchDeleteLinks).not.toHaveBeenCalled();

    wrapper.unmount();
  }, 15000);

  it('Given 勾选了一条短链, When 刷新列表, Then 选择清空(不会用陈旧 id 批量删除)', async () => {
    const wrapper = mountView();
    await flushPromises();

    await selectRow(wrapper, 'abc123');
    expect(batchBar(wrapper).exists()).toBe(true);

    const refresh = wrapper.findAll('button').find((btn) => btn.text() === '刷新');
    expect(refresh, '页头应有刷新按钮').toBeTruthy();
    await refresh!.trigger('click');
    await flushPromises();

    expect(batchBar(wrapper).exists()).toBe(false);

    wrapper.unmount();
  });

  it('Given 勾选了一条短链, When 翻到下一页, Then 选择清空', async () => {
    linksApi.listLinks.mockResolvedValue({
      items: [linkOf('abc123', 1), linkOf('def456', 2)],
      total: 40, // 2 页
    });
    const wrapper = mountView();
    await flushPromises();

    await selectRow(wrapper, 'abc123');
    expect(batchBar(wrapper).exists()).toBe(true);

    const next = wrapper.findAll('button').find((btn) => btn.text() === '下一页');
    expect(next).toBeTruthy();
    await next!.trigger('click');
    await flushPromises();

    expect(batchBar(wrapper).exists()).toBe(false);

    wrapper.unmount();
  });
});
