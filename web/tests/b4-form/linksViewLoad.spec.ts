/**
 * LinksView 数据加载的并发与失败语义（审查项 WEB5）。
 *
 * 1. 并发：翻页/刷新/切回收站可能在中途再触发一次加载，慢响应晚于新响应返回时
 *    会把新数据覆盖成旧数据。修复：请求序号 last-write-wins，过期响应整份丢弃。
 * 2. 失败：原先 catch 里清空 links/total，把「这次没拿到」谎报成「这里没有数据」。
 *    修复：失败保留旧数据，只弹错误提示（与 OverviewView 口径一致）。
 */
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';

import { ApiError } from '@/types/api';
import type { Link } from '@/types/api';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));
const domainsApi = vi.hoisted(() => ({ listDomains: vi.fn() }));
const linksApi = vi.hoisted(() => ({
  listLinks: vi.fn(),
  listDeletedLinks: vi.fn(),
  createLink: vi.fn(),
  deleteLink: vi.fn(),
  updateLink: vi.fn(),
  getLink: vi.fn(),
  batchDeleteLinks: vi.fn(),
  batchPurgeLinks: vi.fn(),
  purgeLink: vi.fn(),
  restoreLink: vi.fn(),
  uploadLanding: vi.fn(),
}));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/domains', () => domainsApi);
vi.mock('@/api/links', () => linksApi);
vi.mock('@/components/app/confirm', () => ({
  confirm: vi.fn(),
  confirmAsync: vi.fn(async () => true),
  closeConfirm: vi.fn(),
}));

const routerMock = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return { ...actual, useRouter: () => routerMock, useRoute: () => ({ params: {}, query: {} }) };
});

import LinksView from '@/views/links/LinksView.vue';
import UI from '@/components/app';

function makeLink(code: string): Link {
  return {
    id: code.charCodeAt(0),
    code,
    targetUrls: ['https://example.com/target'],
    domains: ['links.example.com'],
    redirectStatus: '302',
    linkType: 'redirect',
    status: 'enabled',
    visits: 0,
    clickVisits: 0,
    createdAt: '2026-01-01T00:00:00Z',
  } as unknown as Link;
}

interface LinksViewVm {
  page: number;
  links: Link[];
  total: number;
  loadData: () => Promise<void>;
}

async function mountView(): Promise<{ wrapper: ReturnType<typeof mount>; vm: LinksViewVm }> {
  const wrapper = mount(LinksView, {
    attachTo: document.body,
    global: {
      plugins: [createPinia(), UI],
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        AppTooltip: { template: '<div><slot /></div>' },
      },
    },
  });
  await flushPromises();
  return { wrapper, vm: wrapper.vm as unknown as LinksViewVm };
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  domainsApi.listDomains.mockResolvedValue([
    { id: 3, fqdn: 'links.example.com', status: 'active', origin: 'self' },
  ]);
  linksApi.listDeletedLinks.mockResolvedValue({ items: [], total: 0 });
});

describe('LinksView 加载并发防护', () => {
  it('慢响应(旧请求)不得覆盖后发出的新请求结果', async () => {
    let resolveSlow: ((value: { items: Link[]; total: number }) => void) | undefined;
    const slow = new Promise<{ items: Link[]; total: number }>((resolve) => {
      resolveSlow = resolve;
    });

    linksApi.listLinks.mockImplementation(({ page }: { page: number }) => {
      if (page === 2) return slow; // 第 2 页的请求挂住不返回
      return Promise.resolve({ items: [makeLink(`p${page}`)], total: 50 });
    });

    const { wrapper, vm } = await mountView();
    expect(vm.links.map((l) => l.code)).toEqual(['p1']);

    vm.page = 2;
    const stale = vm.loadData(); // 慢请求先发出

    vm.page = 3;
    const fresh = vm.loadData(); // 新请求后发出但先返回
    await fresh;
    expect(vm.links.map((l) => l.code)).toEqual(['p3']);

    // 慢响应此刻才回来:属于被废弃的那一次加载,必须整份丢弃
    resolveSlow?.({ items: [makeLink('p2-stale')], total: 50 });
    await stale;
    expect(vm.links.map((l) => l.code)).toEqual(['p3']);
    expect(vm.total).toBe(50);

    wrapper.unmount();
  });

  it('加载失败保留旧数据,不谎报空态', async () => {
    linksApi.listLinks.mockResolvedValue({ items: [makeLink('keep')], total: 1 });

    const { wrapper, vm } = await mountView();
    expect(vm.links.map((l) => l.code)).toEqual(['keep']);

    linksApi.listLinks.mockRejectedValue(new ApiError(500, 'E_UNKNOWN', '服务端异常'));
    await vm.loadData();

    expect(toastMock.error).toHaveBeenCalledWith('服务端异常');
    expect(vm.links.map((l) => l.code)).toEqual(['keep']);
    expect(vm.total).toBe(1);

    wrapper.unmount();
  });
});
