/**
 * ConfirmHost 的 AlertDialog 语义契约。
 *
 * 背景：AlertDialogDescription 原先被 v-if="state?.content" 门控，而
 * ConfirmOptions.content 是可选的。调用方不传 content 时描述节点消失，
 * reka 的 useWarning 查不到 descriptionId 对应的元素 → 刷
 * `Missing Description or aria-describedby="undefined" for AlertDialogContent`。
 */
import { mount } from '@vue/test-utils';

import ConfirmHost from '@/components/app/ConfirmHost.vue';
import { closeConfirm, confirm, confirmState } from '@/components/app/confirm';

beforeEach(() => {
  document.body.innerHTML = '';
});

function contentElement(): HTMLElement {
  const el = document.querySelector<HTMLElement>('[role="alertdialog"]');
  if (!el) throw new Error('未找到 role=alertdialog 的节点');
  return el;
}

async function flush() {
  await Promise.resolve();
  await new Promise((r) => setTimeout(r, 0));
}

describe('ConfirmHost AlertDialog 语义', () => {
  it('不传 content 时仍渲染 AlertDialogDescription(不刷 Missing 警告)', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    try {
      confirm({ title: '删除短链' });
      const wrapper = mount(ConfirmHost, { attachTo: document.body });
      await flush();

      const dialog = contentElement();
      const describedBy = dialog.getAttribute('aria-describedby');
      expect(describedBy).toBeTruthy();
      expect(describedBy && document.getElementById(describedBy)).not.toBeNull();
      expect(warn.mock.calls.flat().map(String).join('\n')).not.toContain('Missing');

      wrapper.unmount();
    } finally {
      warn.mockRestore();
      closeConfirm(false);
    }
  });

  it('传了 content 时描述用 content 原文，而不是兜底文案', async () => {
    confirm({ title: '删除短链', content: '删除后不可恢复' });
    const wrapper = mount(ConfirmHost, { attachTo: document.body });
    await flush();

    const dialog = contentElement();
    const descEl = document.getElementById(dialog.getAttribute('aria-describedby') ?? '');
    expect(descEl?.textContent).toContain('删除后不可恢复');
    expect(confirmState.current?.content).toBe('删除后不可恢复');

    wrapper.unmount();
    closeConfirm(false);
  });
});