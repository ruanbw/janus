/**
 * LinkFormView 创建 landing+upload 短链时的「脏状态」契约。
 *
 * 背景（审查项 WEB2）：创建顺序是「先建跳转型短链 → 上传压缩包 → 再 PATCH 定型」。
 * 上传失败时会退回列表页，但原先跳转前没有 markClean —— 表单确实已提交，
 * dirtyGuard 却仍把它当成未保存离开，多弹一个误导性的「放弃未保存修改？」确认框。
 *
 * 背景（审查项 WEB13）：创建分支内与分支外各调了一次 markClean（冗余），
 * 保留下来的应是外层那一次；这里用调用次数把「只调一次」钉住。
 */
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';

import { ApiError } from '@/types/api';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));
const linksApi = vi.hoisted(() => ({
  createLink: vi.fn(),
  updateLink: vi.fn(),
  getLink: vi.fn(),
  listLinks: vi.fn(),
  uploadLanding: vi.fn(),
}));
const domainsApi = vi.hoisted(() => ({ listDomains: vi.fn() }));
const rulesApi = vi.hoisted(() => ({
  ruleOptions: vi.fn(async () => []),
  listLinkRules: vi.fn(async () => []),
  setLinkRules: vi.fn(async () => undefined),
}));
const dirtyGuard = vi.hoisted(() => ({ markClean: vi.fn(), rearm: vi.fn() }));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/components/app/confirm', () => ({
  confirm: vi.fn(),
  confirmAsync: vi.fn(async () => true),
  closeConfirm: vi.fn(),
}));
vi.mock('@/api/links', () => linksApi);
vi.mock('@/api/domains', () => domainsApi);
vi.mock('@/api/rules', () => rulesApi);
// 只关心视图「在跳转前调了 markClean」这一调用顺序契约，替换掉 composable 以便断言
vi.mock('@/composables/useDirtyGuard', () => ({
  useDirtyGuard: () => dirtyGuard,
}));

const routerMock = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
}));
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return {
    ...actual,
    useRouter: () => routerMock,
    useRoute: () => ({ name: 'link-create', params: {}, query: {} }),
  };
});

import LinkFormView from '@/views/links/LinkFormView.vue';
import UI from '@/components/app';

const ACTIVE_DOMAIN = {
  id: 1,
  fqdn: 'links.example.com',
  description: '',
  origin: 'self',
  status: 'active',
  certStatus: 'issued',
  createdAt: '2026-01-01T00:00:00Z',
};

async function mountCreateForm(): Promise<ReturnType<typeof mount>> {
  const wrapper = mount(LinkFormView, {
    global: {
      plugins: [createPinia(), UI],
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        AppTooltip: { template: '<div><slot /></div>' },
      },
    },
  });
  await flushPromises();

  // 直接置状态而不是走下拉/上传控件交互：本用例针对的是提交分支的调用顺序
  const vm = wrapper.vm as unknown as {
    form: { linkType: string; landingSource: string; targetUrls: string[] };
    landingFile: File | null;
  };
  vm.form.linkType = 'landing';
  vm.form.landingSource = 'upload';
  vm.form.targetUrls = ['https://example.com/target'];
  vm.landingFile = new File(['zip-bytes'], 'landing.zip', { type: 'application/zip' });
  return wrapper;
}

async function clickCreate(wrapper: ReturnType<typeof mount>): Promise<void> {
  const btn = wrapper.findAll('button').find((b) => /立即创建短链/.test(b.text()));
  if (!btn) throw new Error('未找到「立即创建短链」按钮');
  await btn.trigger('click');
  await flushPromises();
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  domainsApi.listDomains.mockResolvedValue([ACTIVE_DOMAIN]);
  linksApi.createLink.mockResolvedValue({ id: 9, code: 'Ab3xY9', linkType: 'redirect' });
  linksApi.updateLink.mockResolvedValue({ id: 9, code: 'Ab3xY9', linkType: 'landing' });
});

describe('LinkFormView 创建 landing+upload', () => {
  it('上传失败：跳转回列表前必须 markClean，且顺序在 router.push 之前', async () => {
    linksApi.uploadLanding.mockRejectedValue(new ApiError(500, 'E_UNKNOWN', '落盘失败'));

    const wrapper = await mountCreateForm();
    await clickCreate(wrapper);

    expect(linksApi.createLink).toHaveBeenCalledTimes(1);
    expect(linksApi.uploadLanding).toHaveBeenCalledTimes(1);
    expect(dirtyGuard.markClean).toHaveBeenCalledTimes(1);
    expect(routerMock.push).toHaveBeenCalledWith({ name: 'links' });

    const cleanOrder = dirtyGuard.markClean.mock.invocationCallOrder[0];
    const pushOrder = routerMock.push.mock.invocationCallOrder[0];
    expect(cleanOrder).toBeLessThan(pushOrder);

    // 失败留痕：短链已建成，只是落地页没传上去
    expect(toastMock.warning).toHaveBeenCalledWith(expect.stringContaining('已创建'));

    wrapper.unmount();
  });

  it('上传成功：创建路径只调一次 markClean（分支内那次冗余已删）', async () => {
    linksApi.uploadLanding.mockResolvedValue({ id: 9, landingUploaded: true });

    const wrapper = await mountCreateForm();
    await clickCreate(wrapper);

    expect(linksApi.uploadLanding).toHaveBeenCalledTimes(1);
    expect(linksApi.updateLink).toHaveBeenCalledTimes(1);
    expect(dirtyGuard.markClean).toHaveBeenCalledTimes(1);
    expect(toastMock.success).toHaveBeenCalledWith('短链已创建');
    expect(routerMock.push).toHaveBeenCalledWith({ name: 'links' });

    wrapper.unmount();
  });
});
