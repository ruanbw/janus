/**
 * AppModal / AppDialog 的无障碍语义契约。
 *
 * 背景：DialogTitle 与 DialogDescription 原先只写在 `<slot name="header">` 的
 * fallback 分支里，调用方一旦传自定义 header（ErrorPagesCard / RuleSimulatorView /
 * RuleFormView 三处），reka-ui 必刷两条警告：
 *   - `DialogContent` requires a `DialogTitle`
 *   - Missing `Description` or `aria-describedby="undefined"`
 *
 * 契约：无论调用方是否提供自定义 header、是否传 title/description、是否关闭关闭按钮，
 * role=dialog 的元素都必须能通过 aria-labelledby / aria-describedby 找到**非空文本**的节点。
 */
import { mount } from '@vue/test-utils';

import AppModal from '@/components/app/AppModal.vue';
import AppDialog from '@/components/app/AppDialog.vue';

/** role=dialog 的内容节点（reka 通过 Portal 渲染到 body） */
function dialogElement(): HTMLElement {
  const el = document.querySelector<HTMLElement>('[role="dialog"]');
  if (!el) throw new Error('未找到 role=dialog 的节点');
  return el;
}

// reka 的 Dialog 通过 Portal 渲染到 body，unmount 后残留节点会污染下一个用例的
// document.querySelector('[role=dialog]')，每个用例前清一次。
beforeEach(() => {
  document.body.innerHTML = '';
});

/** 挂载期间收集 reka 的 console 警告 */
function captureWarnings<T>(run: () => T): { result: T; warnings: string[] } {
  const warnings: string[] = [];
  const spy = vi.spyOn(console, 'warn').mockImplementation((...args: unknown[]) => {
    warnings.push(args.map(String).join(' '));
  });
  try {
    const result = run();
    return { result, warnings };
  } finally {
    spy.mockRestore();
  }
}

/** reka 语义告警（DialogTitle 缺失 / Description 缺失） */
function semanticWarnings(warnings: string[]): string[] {
  return warnings.filter((w) => w.includes('DialogTitle') || w.includes('Missing'));
}

function mountModal(props: Record<string, unknown>, slots: Record<string, string>) {
  return mount(AppModal, {
    props: { open: true, ...props },
    slots,
    attachTo: document.body,
  });
}

/** 取 aria-labelledby / aria-describedby 指向的节点的纯文本 */
function accessibleTexts(wrapper: { vm: { $nextTick(): Promise<unknown> } }) {
  const dialog = dialogElement();
  const title = document.getElementById(dialog.getAttribute('aria-labelledby') ?? '');
  const desc = document.getElementById(dialog.getAttribute('aria-describedby') ?? '');
  return {
    labelledBy: dialog.getAttribute('aria-labelledby'),
    describedBy: dialog.getAttribute('aria-describedby'),
    titleEl: title,
    descEl: desc,
  };
}

describe('AppModal 无障碍语义', () => {
  it('调用方传自定义 header 时,DialogTitle / DialogDescription 仍然渲染', async () => {
    const { result: wrapper, warnings } = captureWarnings(() =>
      mountModal({}, { header: '<div class="custom">自定义头部</div>', default: '<p>正文</p>' }),
    );
    await wrapper.vm.$nextTick();

    const dialog = dialogElement();
    const labelledBy = dialog.getAttribute('aria-labelledby');
    const describedBy = dialog.getAttribute('aria-describedby');

    expect(labelledBy).toBeTruthy();
    expect(labelledBy && document.getElementById(labelledBy)).not.toBeNull();
    expect(describedBy).toBeTruthy();
    expect(describedBy && document.getElementById(describedBy)).not.toBeNull();
    expect(semanticWarnings(warnings)).toEqual([]);

    wrapper.unmount();
  });

  it('自定义 header + title/description props:a11y 名称取自 props,视觉仍由 header 插槽负责', async () => {
    const { result: wrapper, warnings } = captureWarnings(() =>
      mountModal(
        { title: '规则专属拦截页面沙箱预览', description: '已开启 sandbox 安全隔离' },
        { header: '<div class="custom">自定义头部</div>' },
      ),
    );
    await wrapper.vm.$nextTick();

    const { titleEl, descEl } = accessibleTexts(wrapper);

    expect(titleEl?.textContent).toBe('规则专属拦截页面沙箱预览');
    expect(descEl?.textContent).toBe('已开启 sandbox 安全隔离');
    // 语义层在自定义 header 下必须视觉隐藏,不能与自定义头部重复显示
    expect(titleEl?.className).toContain('sr-only');
    // header 插槽内容在 Portal 里,要从 document 查而不是 wrapper
    expect(document.querySelector('.custom')?.textContent).toBe('自定义头部');
    expect(warnings).toEqual([]);

    wrapper.unmount();
  });

  it('不传 description 时也渲染兜底 DialogDescription(不刷 Missing 警告)', async () => {
    const { result: wrapper, warnings } = captureWarnings(() =>
      mountModal({ title: '只有标题' }, { default: '<p>正文</p>' }),
    );
    await wrapper.vm.$nextTick();

    const dialog = dialogElement();
    const describedBy = dialog.getAttribute('aria-describedby');
    expect(describedBy && document.getElementById(describedBy)).not.toBeNull();
    expect(semanticWarnings(warnings)).toEqual([]);

    wrapper.unmount();
  });

  it('默认 header 路径:标题与描述可见,不是 sr-only', async () => {
    const { result: wrapper } = captureWarnings(() =>
      mountModal({ title: '可见标题', description: '可见描述' }, { default: '<p>正文</p>' }),
    );
    await wrapper.vm.$nextTick();

    const { titleEl, descEl } = accessibleTexts(wrapper);

    expect(titleEl?.textContent).toContain('可见标题');
    expect(titleEl?.className).not.toContain('sr-only');
    expect(descEl?.textContent).toContain('可见描述');
    expect(descEl?.className).not.toContain('sr-only');

    wrapper.unmount();
  });

  /**
   * 回归：`:closable="false"` 且不传 title / description 时，默认 header 的
   * v-if 为假 → 整段 header 不渲染 → DialogTitle 与 DialogDescription 双双消失，
   * reka 每次打开都刷两条警告。这条用例钉死「语义层无条件存在」。
   */
  it('closable=false 且无 title/description：语义层仍渲染，兜底文案非空', async () => {
    const { result: wrapper, warnings } = captureWarnings(() =>
      mountModal({ closable: false }, { default: '<p>正文</p>' }),
    );
    await wrapper.vm.$nextTick();

    const { titleEl, descEl } = accessibleTexts(wrapper);

    expect(titleEl, 'DialogTitle 必须存在').not.toBeNull();
    expect(titleEl?.textContent?.trim(), '可访问名称不能是空串').not.toBe('');
    expect(descEl, 'DialogDescription 必须存在').not.toBeNull();
    expect(descEl?.textContent?.trim(), '可访问描述不能是空串').not.toBe('');
    expect(semanticWarnings(warnings)).toEqual([]);

    wrapper.unmount();
  });

  it('默认 header 只因 closable 渲染时，标题退到 sr-only 兜底而不是空标题', async () => {
    const { result: wrapper } = captureWarnings(() =>
      mountModal({}, { default: '<p>正文</p>' }),
    );
    await wrapper.vm.$nextTick();

    const { titleEl } = accessibleTexts(wrapper);

    expect(titleEl).not.toBeNull();
    // 原实现：header 因 closable 渲染出 <DialogTitle>{{ title }}</DialogTitle>，
    // 节点在但文本为空 —— aria-labelledby 指向一个空标题，读屏念不出对话框叫什么
    expect(titleEl?.textContent?.trim()).not.toBe('');
    expect(titleEl?.className).toContain('sr-only');

    wrapper.unmount();
  });
});

describe('AppDialog 无障碍语义', () => {
  it('无头底座也必须渲染 DialogTitle / DialogDescription', async () => {
    const { result: wrapper, warnings } = captureWarnings(() =>
      mount(AppDialog, {
        props: { open: true, title: '编辑规则', description: '修改后立即生效' },
        slots: { default: '<p>表单</p>' },
        attachTo: document.body,
      }),
    );
    await wrapper.vm.$nextTick();

    const { titleEl, descEl } = accessibleTexts(wrapper);

    expect(titleEl?.textContent).toBe('编辑规则');
    expect(descEl?.textContent).toBe('修改后立即生效');
    expect(warnings).toEqual([]);

    wrapper.unmount();
  });

  it('调用方用 #title / #description 插槽提供语义层时，组件不再重复渲染兜底节点', async () => {
    const { result: wrapper, warnings } = captureWarnings(() =>
      mount(AppDialog, {
        props: { open: true },
        slots: {
          title: '<span class="from-slot">调用方标题</span>',
          description: '<span class="desc-slot">调用方描述</span>',
          default: '<p>表单</p>',
        },
        attachTo: document.body,
      }),
    );
    await wrapper.vm.$nextTick();

    const dialog = dialogElement();
    const titleEl = document.getElementById(dialog.getAttribute('aria-labelledby') ?? '');
    const descEl = document.getElementById(dialog.getAttribute('aria-describedby') ?? '');

    // 插槽内容被 AppDialog 自己的 DialogTitle / DialogDescription 包住，所以只有一个节点
    expect(titleEl?.querySelector('.from-slot')).not.toBeNull();
    expect(titleEl?.textContent).toBe('调用方标题');
    expect(descEl?.textContent).toBe('调用方描述');
    // 兜底文案不能再冒出来，否则 aria-labelledby 指向的是重复 id 里的第一个
    expect(titleEl?.textContent).not.toContain('对话框');
    expect(semanticWarnings(warnings)).toEqual([]);

    wrapper.unmount();
  });

  it('无头底座不传 title / description 时，兜底文案非空', async () => {
    const { result: wrapper, warnings } = captureWarnings(() =>
      mount(AppDialog, { props: { open: true }, slots: { default: '<p>表单</p>' }, attachTo: document.body }),
    );
    await wrapper.vm.$nextTick();

    const { titleEl, descEl } = accessibleTexts(wrapper);

    expect(titleEl?.textContent?.trim()).not.toBe('');
    expect(descEl?.textContent?.trim()).not.toBe('');
    expect(semanticWarnings(warnings)).toEqual([]);

    wrapper.unmount();
  });
});