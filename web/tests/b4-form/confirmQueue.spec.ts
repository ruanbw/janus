/**
 * confirm() 与 confirmAsync() 曾共用 `confirmState.current` 这一个模块级单例。
 *
 * 触发链：保存确认框（confirm，fire-and-forget + onOk）打开期间按后退键 →
 * useDirtyGuard 的 confirmAsync 把 current 整个换掉 → 先前那次的 onOk 永不执行
 * （保存静默丢失），且 confirmAsync 的 promise 也再无人结算。
 *
 * 本组钉住修复后的契约：**先进先出排队**，两次并发调用都能结算。
 */
import { closeConfirm, confirm, confirmAsync, confirmState } from '@/components/app/confirm';

/** 忠实复刻 ConfirmHost 的两个按钮：onOk 先执行，再 closeConfirm */
async function clickOk(): Promise<void> {
  const current = confirmState.current;
  if (current === null) throw new Error('没有待确认的对话框');
  if (current.onOk) await current.onOk();
  closeConfirm(true);
}

function clickCancel(): void {
  closeConfirm(false);
}

beforeEach(() => {
  // 模块级单例跨用例残留，排空队列保证每个用例从空态开始
  for (let i = 0; i < 20 && confirmState.current !== null; i++) closeConfirm(false);
});

describe('confirm 队列：confirm 与 confirmAsync 互不干扰', () => {
  it('先 confirm() 后 confirmAsync()：保存的 onOk 不丢，离开确认的 promise 也结算', async () => {
    const onOk = vi.fn();
    confirm({ title: '该规则未关联任何短链，确认保存？', onOk });
    const pending = confirmAsync({ title: '离开确认' });

    // 先弹出的那个仍然是当前项，没有被后来的调用顶掉
    expect(confirmState.current?.title).toBe('该规则未关联任何短链，确认保存？');

    await clickOk();
    expect(onOk).toHaveBeenCalledTimes(1);

    // 接着轮到离开确认框，它没有被吞掉
    expect(confirmState.current?.title).toBe('离开确认');
    await clickOk();
    await expect(pending).resolves.toBe(true);
    expect(confirmState.current).toBeNull();
  });

  it('先 confirmAsync() 后 confirm()：promise 先结算，随后的 onOk 照常执行', async () => {
    const onOk = vi.fn();
    const pending = confirmAsync({ title: '离开确认' });
    confirm({ title: '删除确认', onOk });

    expect(confirmState.current?.title).toBe('离开确认');

    await clickOk();
    await expect(pending).resolves.toBe(true);

    expect(confirmState.current?.title).toBe('删除确认');
    await clickOk();
    expect(onOk).toHaveBeenCalledTimes(1);
    expect(confirmState.current).toBeNull();
  });

  it('两个 confirmAsync 连续调用：各自按顺序结算，不会互相顶掉', async () => {
    const first = confirmAsync({ title: 'A' });
    const second = confirmAsync({ title: 'B' });

    expect(confirmState.current?.title).toBe('A');
    await clickOk();
    await expect(first).resolves.toBe(true);

    expect(confirmState.current?.title).toBe('B');
    clickCancel();
    await expect(second).resolves.toBe(false);

    expect(confirmState.current).toBeNull();
  });

  it('取消当前项同样会提升下一项，而不是把队列清空', async () => {
    const onOk = vi.fn();
    confirm({ title: 'X', onOk });
    const pending = confirmAsync({ title: 'Y' });

    clickCancel();
    expect(onOk).not.toHaveBeenCalled();
    expect(confirmState.current?.title).toBe('Y');

    await clickOk();
    await expect(pending).resolves.toBe(true);
    expect(confirmState.current).toBeNull();
  });

  it('队列排空后不残留，再来的请求直接开新框', () => {
    confirm({ title: 'X' });
    closeConfirm(false);
    confirm({ title: 'Y' });
    expect(confirmState.current?.title).toBe('Y');
    closeConfirm(false);
    expect(confirmState.current).toBeNull();
  });
});