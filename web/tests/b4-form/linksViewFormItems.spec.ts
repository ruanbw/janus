/**
 * 简报 #2 的核实：LinksView「批量导入」弹窗里的两个 AppFormItem 没有 AppForm 祖先。
 *
 * 简报称 AppFormItem.vue:47 的 `inject(formContextKey)` 没有默认值，会刷
 * `[Vue warn]: injection "Symbol(janus-form)" not found.` 两次。
 * 但实际代码是 `inject(formContextKey, undefined)` —— **有**默认值，Vue 不会告警。
 *
 * 本组把这个结论钉成可执行断言（而不是靠读代码下结论）：挂载 LinksView、打开批量导入弹窗、
 * 捕获 console.warn。同时断言这两个表单项在无 form 上下文时仍能正常渲染并完成自身校验，
 * 避免「不告警」变成「根本没渲染」这种假绿。
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

async function mountAndOpenBatchModal() {
  domainsApi.listDomains.mockResolvedValue([{ id: 3, fqdn: 'links.example.com', status: 'active' }]);
  linksApi.listLinks.mockResolvedValue({ items: [], total: 0 });
  linksApi.listDeletedLinks.mockResolvedValue({ items: [], total: 0 });

  const wrapper = mount(LinksView, {
    attachTo: document.body,
    global: {
      plugins: [createPinia(), UI],
      stubs: { RouterLink: { template: '<a><slot /></a>' }, AppTooltip: { template: '<div><slot /></div>' } },
    },
  });
  await flushPromises();

  const openBtn = wrapper.findAll('button').find((b) => /批量导入/.test(b.text()));
  if (!openBtn) throw new Error('未找到「批量导入」按钮');
  await openBtn.trigger('click');
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
});

describe('LinksView 批量导入弹窗的 AppFormItem', () => {
  it('打开弹窗时不产生 Vue 的 injection 告警（inject 有默认值，form 缺失不告警）', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});

    const wrapper = await mountAndOpenBatchModal();

    const injectionWarnings = warn.mock.calls.filter((c) => String(c[0]).includes('not found'));
    expect(injectionWarnings).toEqual([]);
    expect(error.mock.calls.filter((c) => String(c[0]).includes('injection'))).toEqual([]);

    warn.mockRestore();
    error.mockRestore();
    wrapper.unmount();
  });

  it('两个表单项确实渲染出来了（保证上一条不是「因为没渲染才没告警」的假绿）', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    const wrapper = await mountAndOpenBatchModal();
    const labels = Array.from(document.querySelectorAll('label')).map((l) => l.textContent?.trim());

    expect(labels).toContain('指定承载域名');
    expect(labels).toContain('短链行列表');

    warn.mockRestore();
    wrapper.unmount();
  });

  it('无 form 上下文时字段仍可用：输入的行列表能被读到并触发导入', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const wrapper = await mountAndOpenBatchModal();

    const textarea = document.querySelector('textarea');
    if (!textarea) throw new Error('未找到批量导入的 textarea');
    textarea.value = 'deal-a https://example.com/target-a';
    textarea.dispatchEvent(new Event('input'));
    await flushPromises();

    // 弹窗经 DialogPortal 传送到了 body，按钮要从 document 上找而不是 wrapper 内
    const startBtn = Array.from(document.querySelectorAll('button')).find((b) =>
      /开始导入/.test(b.textContent ?? ''),
    );
    if (!startBtn) throw new Error('未找到「开始导入」按钮');
    startBtn.click();
    await flushPromises();

    expect(linksApi.createLink).toHaveBeenCalledTimes(1);

    warn.mockRestore();
    wrapper.unmount();
  });
});