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

/** antd Modal.confirm 兼容:调用后立即返回,点击确认时执行 onOk */
export function confirm(options: ConfirmOptions): void {
  confirmState.current = { ...options, open: true, resolving: null };
}

/** Promise 形式:resolve(true)=确认,resolve(false)=取消 */
export function confirmAsync(options: Omit<ConfirmOptions, 'onOk'>): Promise<boolean> {
  return new Promise((resolve) => {
    confirmState.current = { ...options, open: true, resolving: resolve };
  });
}

/** 由 ConfirmHost 调用:关闭并结算 */
export function closeConfirm(result: boolean): void {
  const current = confirmState.current;
  if (current === null) return;
  current.open = false;
  if (current.resolving) current.resolving(result);
  confirmState.current = null;
}

export default confirm;
