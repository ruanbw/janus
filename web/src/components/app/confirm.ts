// 确认对话框(替代 antd Modal.confirm):模块级单例,ConfirmHost.vue 负责渲染
import { reactive } from 'vue';

export interface ConfirmOptions {
  title: string;
  content?: string;
  okText?: string;
  cancelText?: string;
  /** 确认按钮是否危险色(删除/封禁等) */
  danger?: boolean;
  onOk?: () => void | Promise<void>;
}

interface ConfirmState extends ConfirmOptions {
  open: boolean;
  resolving: ((value: boolean) => void) | null;
}

export const confirmState = reactive<{ current: ConfirmState | null }>({ current: null });

/**
 * 待展示的确认请求队列（先进先出）。
 *
 * 此前 confirm() 与 confirmAsync() 共用 confirmState.current 这一个单例，谁后调用谁把
 * current 整个换掉。触发链：保存确认框（confirm，fire-and-forget + onOk）打开期间按后退键
 * → useDirtyGuard 的 confirmAsync 顶掉 current → 先前那次的 onOk 永不执行（保存静默丢失），
 * 且 confirmAsync 的 promise 也再无人结算。
 *
 * 排队后两次调用互不干扰：当前项结算后自动提升下一项，onOk 与 promise 都会执行。
 */
const queue: ConfirmState[] = [];

function enqueue(state: ConfirmState): void {
  queue.push(state);
  if (confirmState.current === null) {
    confirmState.current = state;
  }
}

/** antd Modal.confirm 兼容:调用后立即返回,点击确认时执行 onOk */
export function confirm(options: ConfirmOptions): void {
  enqueue({ ...options, open: true, resolving: null });
}

/** Promise 形式:resolve(true)=确认,resolve(false)=取消 */
export function confirmAsync(options: Omit<ConfirmOptions, 'onOk'>): Promise<boolean> {
  return new Promise((resolve) => {
    enqueue({ ...options, open: true, resolving: resolve });
  });
}

/** 由 ConfirmHost 调用:结算当前项,并把队列里的下一项提上来 */
export function closeConfirm(result: boolean): void {
  const current = confirmState.current;
  if (current === null) return;
  current.open = false;
  // 只能 shift 不能 indexOf:confirmState 是 reactive,读回的 current 是代理对象,
  // 与 queue 里存的原始对象引用不同，indexOf 永远返回 -1，队列排不出去。
  // closeConfirm 结算的永远是队首,shift 即是正解。
  queue.shift();
  if (current.resolving) current.resolving(result);
  confirmState.current = queue[0] ?? null;
}

export default confirm;
