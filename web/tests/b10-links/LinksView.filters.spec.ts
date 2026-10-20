/**
 * 短链列表筛选的服务端契约。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 用户在短链列表输入关键词 / 选择类型 / 选择状态
 *   When  筛选条件变化
 *   Then  条件必须作为请求参数下发，total 与分页以**筛选后的结果集**为准
 *
 * 为什么值得钉住：原先后端 handleListLinks 只认 page/pageSize，筛选是在当前页
 * 的内存副本上做的。于是「共 N 条」、分页器、翻页后的内容全部对不上：
 *   - 搜一个在第 3 页的短码 → 显示"未找到"，用户以为短链不存在；
 *   - 取消筛选取不到原来的记录，因为它根本不在这一页。
 * 这三点都属于「界面能渲染、数字却是错的」，只靠肉眼很难发现是筛选范围的问题。
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
import AppSelect from '@/components/app/AppSelect.vue';
import AppTabs from '@/components/app/AppTabs.vue';
import LinksView from '@/views/links/LinksView.vue';

/** 搜索防抖窗口(与视图里的 KEYWORD_DEBOUNCE_MS 对齐,留出调度余量) */
const DEBOUNCE_WAIT_MS = 350;

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function linkOf(code: string, id: number): Link {
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

function searchInput(wrapper: ReturnType<typeof mountView>) {
  return wrapper.get('input[aria-label="搜索短链"]');
}

/** 直接驱动 AppSelect 的 v-model：reka 的下拉在 happy-dom 里没有可靠的可点选项 */
async function chooseSelect(
  wrapper: ReturnType<typeof mountView>,
  index: number,
  value: string,
): Promise<void> {
  const selects = wrapper.findAllComponents(AppSelect);
  expect(selects.length, '工具栏应有两个筛选下拉(类型/状态)').toBeGreaterThan(index);
  selects[index]!.vm.$emit('update:modelValue', value);
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  domainsApi.listDomains.mockResolvedValue([]);
  linksApi.listDeletedLinks.mockResolvedValue({ items: [], total: 0 });
  linksApi.listLinks.mockResolvedValue({
    items: [linkOf('abc123', 1), linkOf('def456', 2)],
    total: 2,
  });
});

describe('LinksView 筛选下推', () => {
  it('Given 输入关键词, When 防抖窗口结束, Then 请求带 keyword 且输入框内容不被吞', async () => {
    const wrapper = mountView();
    await flushPromises();
    expect(linksApi.listLinks).toHaveBeenLastCalledWith({ page: 1, pageSize: 20 });

    await searchInput(wrapper).setValue('promo');
    await sleep(DEBOUNCE_WAIT_MS);
    await flushPromises();

    expect(linksApi.listLinks).toHaveBeenLastCalledWith({
      page: 1,
      pageSize: 20,
      keyword: 'promo',
    });
    // 防抖期间 v-model 不能把输入框回写成旧值,否则用户会看到字被吞掉
    expect((searchInput(wrapper).element as HTMLInputElement).value).toBe('promo');

    wrapper.unmount();
  });

  it('Given 连续输入, When 防抖窗口结束, Then 只发一次带最终关键词的请求', async () => {
    const wrapper = mountView();
    await flushPromises();
    const baseline = linksApi.listLinks.mock.calls.length;

    await searchInput(wrapper).setValue('p');
    await searchInput(wrapper).setValue('pr');
    await searchInput(wrapper).setValue('pro');
    await sleep(DEBOUNCE_WAIT_MS);
    await flushPromises();

    const added = linksApi.listLinks.mock.calls.slice(baseline);
    expect(added, '逐字符发请求会把列表刷新成"打字机"').toHaveLength(1);
    expect(added[0]![0]).toEqual({ page: 1, pageSize: 20, keyword: 'pro' });

    wrapper.unmount();
  });

  it('Given 选择类型筛选, Then 请求带 type', async () => {
    const wrapper = mountView();
    await flushPromises();

    await chooseSelect(wrapper, 0, 'landing');

    expect(linksApi.listLinks).toHaveBeenLastCalledWith({
      page: 1,
      pageSize: 20,
      type: 'landing',
    });

    wrapper.unmount();
  });

  it('Given 选择状态筛选, Then 请求带 status', async () => {
    const wrapper = mountView();
    await flushPromises();

    await chooseSelect(wrapper, 1, 'disabled');

    expect(linksApi.listLinks).toHaveBeenLastCalledWith({
      page: 1,
      pageSize: 20,
      status: 'disabled',
    });

    wrapper.unmount();
  });

  it('Given 服务端按筛选返回 total, Then 底栏与分页都用筛选后的结果集', async () => {
    linksApi.listLinks
      .mockResolvedValueOnce({ items: [linkOf('abc123', 1)], total: 40 }) // 初次:两页
      .mockResolvedValueOnce({ items: [linkOf('promo', 3)], total: 3 }); // 搜索后:一页

    const wrapper = mountView();
    await flushPromises();
    expect(wrapper.text()).toContain('1 / 2');

    await searchInput(wrapper).setValue('promo');
    await sleep(DEBOUNCE_WAIT_MS);
    await flushPromises();

    expect(wrapper.text()).toContain('共 3 条短链');
    expect(wrapper.text()).toContain('1 / 1');

    wrapper.unmount();
  });

  it('Given 回收站视图, When 输入关键词, Then 回收站请求同样带 keyword', async () => {
    const wrapper = mountView();
    await flushPromises();

    // reka 的页签在 happy-dom 里点不动,直接驱动 v-model 的入口
    wrapper.findComponent(AppTabs).vm.$emit('update:modelValue', 'deleted');
    await flushPromises();

    linksApi.listDeletedLinks.mockResolvedValue({ items: [], total: 0 });
    await wrapper.get('input[aria-label="搜索已删除的短链"]').setValue('old');
    await sleep(DEBOUNCE_WAIT_MS);
    await flushPromises();

    // 回收站只下发关键词:类型/状态对已删除记录只是历史快照
    expect(linksApi.listDeletedLinks).toHaveBeenLastCalledWith(1, 20, 'old');

    wrapper.unmount();
  });

  it('Given 输入关键词后又清空过滤, Then 旧关键词不会在防抖后把条件加回来', async () => {
    const wrapper = mountView();
    await flushPromises();

    await searchInput(wrapper).setValue('promo');
    // 不等防抖:立刻点「清空过滤」不适用(按钮此时还没出现),这里直接清空输入框,
    // 模拟用户在防抖窗口内反悔
    await searchInput(wrapper).setValue('');
    await sleep(DEBOUNCE_WAIT_MS);
    await flushPromises();

    // 最终条件必须是无筛选,而不是被那次已作废的输入重新加上 keyword
    expect(linksApi.listLinks).toHaveBeenLastCalledWith({ page: 1, pageSize: 20 });

    wrapper.unmount();
  });
});
