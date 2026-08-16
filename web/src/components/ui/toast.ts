// 轻量 toast(替代 antd message):模块级单例,Toaster.vue 负责渲染
import { reactive } from 'vue';

export type ToastType = 'success' | 'error' | 'warning' | 'info';

export interface ToastItem {
  id: number;
  type: ToastType;
  content: string;
}

export const toasts = reactive<ToastItem[]>([]);

let seq = 0;

function push(type: ToastType, content: string, duration: number): void {
  const id = ++seq;
  toasts.push({ id, type, content });
  if (duration > 0) {
    window.setTimeout(() => dismiss(id), duration);
  }
}

export function dismiss(id: number): void {
  const index = toasts.findIndex((t) => t.id === id);
  if (index >= 0) toasts.splice(index, 1);
}

/** antd message 兼容 API:message.success/error/warning/info */
export const message = {
  success(content: string): void {
    push('success', content, 3000);
  },
  error(content: string): void {
    push('error', content, 5000);
  },
  warning(content: string): void {
    push('warning', content, 4000);
  },
  info(content: string): void {
    push('info', content, 3000);
  },
};

export default message;
