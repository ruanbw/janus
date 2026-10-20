/**
 * 批量导入的执行契约（视图层）。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 用户粘贴若干行（其中有的行有问题）
 *   When  点「开始导入」
 *   Then  只对解析通过的行发请求、逐条推进进度，最后如实报告成功/失败数
 *
 * 为什么值得钉住：原实现把解析、校验、发请求混在一个循环里 ——
 * 有问题的行要等发出去才失败，用户在几十秒里只能看到最后一条 toast；
 * 而一行有 3 段时第三个词会被静默丢掉（用户以为导入了带备注的行）。
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

/**
 * 模态框内容经 DialogPortal 传送到 body，不在组件根节点里 ——
 * 所以这里用 document 查询而不是 wrapper.findAll。
 */
function mountView() {
  return mount(LinksView, {
    attachTo: document.body,
    global: { plugins: [UI] },
  });
}

/** 按文本找 body 里的按钮（reka 的传送门内容只能在 document 上找） */
function docButton(text: string): HTMLButtonElement | undefined {
  return [...document.querySelectorAll('button')].find((b) => (b.textContent ?? '').includes(text));
}

async function click(el: Element | undefined | null) {
  expect(el, '目标按钮应存在').toBeTruthy();
  el!.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  await flushPromises();
}

async function openBatchModal() {
  await click(docButton('批量导入'));
}

function textarea(): HTMLTextAreaElement {
  const el = document.querySelector('textarea');
  expect(el, '模态框里应有行列表输入框').toBeTruthy();
  return el as HTMLTextAreaElement;
}

/** 往受控 textarea 里写值并派发 input 事件（v-model 监听的是 input） */
async function fillTextarea(value: string) {
  const el = textarea();
  el.value = value;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  domainsApi.listDomains.mockResolvedValue([
    { id: 7, fqdn: 'alice.janus.test', status: 'active', origin: 'platform' },
  ]);
  linksApi.listLinks.mockResolvedValue({ items: [linkOf('abc123', 1)], total: 1 });
  linksApi.listDeletedLinks.mockResolvedValue({ items: [], total: 0 });
  linksApi.createLink.mockResolvedValue(linkOf('created', 2));
});

describe('LinksView 批量导入', () => {
  it('Given 有问题的行, Then 导入前就列出错误并在提交时跳过它们', async () => {
    const wrapper = mountView();
    await flushPromises();
    await openBatchModal();

    await fillTextarea(
      ['deal-a https://example.com/a', 'deal-b https://example.com/b 备注', 'deal-a https://example.com/c'].join('\n'),
    );

    // 预览:合法 1 条,有误 2 条（第三条是重复短码）
    const preview = document.querySelector('[data-od-id="batch-parse-preview"]');
    expect(preview, '导入前应有解析预览').toBeTruthy();
    const previewText = preview!.textContent ?? '';
    expect(previewText).toContain('可导入 1 条');
    expect(previewText).toContain('有误 2 条');
    expect(previewText).toContain('第 2 行');
    expect(previewText).toContain('第 3 行');

    await click(docButton('开始导入'));

    // 只为合法那一行发请求
    expect(linksApi.createLink).toHaveBeenCalledTimes(1);
    expect(linksApi.createLink).toHaveBeenCalledWith({
      code: 'deal-a',
      targetUrls: ['https://example.com/a'],
      domainIds: [7],
      redirectStatus: '302',
      linkType: 'redirect',
    });

    wrapper.unmount();
  });

  it('Given 全部行都合法, Then 逐条创建并报告成功数', async () => {
    const wrapper = mountView();
    await flushPromises();
    await openBatchModal();

    await fillTextarea(['a1 https://example.com/1', 'a2 https://example.com/2'].join('\n'));

    await click(docButton('开始导入'));

    expect(linksApi.createLink).toHaveBeenCalledTimes(2);
    expect(toastMock.success).toHaveBeenCalledWith('成功导入 2 条短链');

    wrapper.unmount();
  });

  it('Given 某一行被服务端拒绝, Then 失败数被统计且不影响后续行', async () => {
    linksApi.createLink
      .mockResolvedValueOnce(linkOf('a1', 2))
      .mockRejectedValueOnce(new Error('409 conflict'));

    const wrapper = mountView();
    await flushPromises();
    await openBatchModal();

    await fillTextarea(
      ['a1 https://example.com/1', 'a2 https://example.com/2', 'a3 https://example.com/3'].join('\n'),
    );

    await click(docButton('开始导入'));

    expect(linksApi.createLink).toHaveBeenCalledTimes(3);
    expect(toastMock.success).toHaveBeenCalledWith('成功导入 2 条短链，失败 1 条');

    wrapper.unmount();
  });

  it('Given 没有任何合法行, Then 开始导入按钮禁用（不产出请求）', async () => {
    const wrapper = mountView();
    await flushPromises();
    await openBatchModal();

    await fillTextarea('只有短码没有URL');

    const start = docButton('开始导入');
    expect(start?.disabled, '没有合法行时不该允许点').toBe(true);

    await click(start);
    expect(linksApi.createLink).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it('Given 超过单次上限, Then 明确提示分批导入且按钮禁用', async () => {
    const wrapper = mountView();
    await flushPromises();
    await openBatchModal();

    const lines = Array.from({ length: 501 }, (_, i) => `c${i} https://example.com/${i}`);
    await fillTextarea(lines.join('\n'));

    expect(document.body.textContent).toContain('超过单次上限 500 行');
    expect(docButton('开始导入')?.disabled).toBe(true);

    wrapper.unmount();
  });
});
