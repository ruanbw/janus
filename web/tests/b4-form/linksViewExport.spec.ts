/**
 * LinksView 导出 CSV 的 blob URL 生命周期契约（审查项 WEB7）。
 *
 * 背景：`a.click()` 之后同步调用 URL.revokeObjectURL 会在下载器真正接管 URL
 * 之前把它撤销，Firefox 等浏览器会因此取消下载（用户看到「点了导出没反应」）。
 * 修复：延迟释放。
 */
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';

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

const SAMPLE_LINK = {
  id: 1,
  code: 'Ab3xY9',
  targetUrls: ['https://example.com/target'],
  domains: ['links.example.com'],
  redirectStatus: '302',
  linkType: 'redirect',
  status: 'enabled',
  visits: 3,
  clickVisits: 0,
  createdAt: '2026-01-01T00:00:00Z',
};

let createObjectURL: ReturnType<typeof vi.fn>;
let revokeObjectURL: ReturnType<typeof vi.fn>;
let anchorClick: ReturnType<typeof vi.spyOn>;
let originalCreate: typeof URL.createObjectURL;
let originalRevoke: typeof URL.revokeObjectURL;

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  domainsApi.listDomains.mockResolvedValue([
    { id: 3, fqdn: 'links.example.com', status: 'active', origin: 'self' },
  ]);
  linksApi.listLinks.mockResolvedValue({ items: [SAMPLE_LINK], total: 1 });
  linksApi.listDeletedLinks.mockResolvedValue({ items: [], total: 0 });

  createObjectURL = vi.fn(() => 'blob:mock-url');
  revokeObjectURL = vi.fn();
  originalCreate = URL.createObjectURL;
  originalRevoke = URL.revokeObjectURL;
  URL.createObjectURL = createObjectURL as unknown as typeof URL.createObjectURL;
  URL.revokeObjectURL = revokeObjectURL as unknown as typeof URL.revokeObjectURL;
  // 不让 detached <a> 真的触发导航
  anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
});

afterEach(() => {
  URL.createObjectURL = originalCreate;
  URL.revokeObjectURL = originalRevoke;
  anchorClick.mockRestore();
});

async function mountWithLinks(): Promise<ReturnType<typeof mount>> {
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
  return wrapper;
}

describe('LinksView CSV 导出', () => {
  it('click 之后不立即释放 blob URL，延迟约 1s 再释放', async () => {
    const wrapper = await mountWithLinks();

    const exportBtn = wrapper.findAll('button').find((b) => /导出 CSV/.test(b.text()));
    if (!exportBtn) throw new Error('未找到「导出 CSV」按钮');
    await exportBtn.trigger('click');

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(anchorClick).toHaveBeenCalledTimes(1);
    // 同步释放会让下载器拿不到 URL（Firefox 直接取消下载）
    expect(revokeObjectURL).not.toHaveBeenCalled();

    await new Promise((resolve) => setTimeout(resolve, 1100));
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock-url');

    wrapper.unmount();
  });
});
