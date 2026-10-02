import { onMounted, onUnmounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { confirmAsync } from '@/components/app/confirm';

export interface UseDirtyGuardOptions {
  isDirty: () => boolean;
  message?: string;
  title?: string;
}

/**
 * useDirtyGuard
 * 表单脏状态拦截 Composable：
 * 1. 监听 vue-router 路由跳转，如果表单已被用户修改且未提交，弹窗确认
 * 2. 监听 window beforeunload 事件，防止误触刷新或关闭浏览器窗口
 */
export function useDirtyGuard(options: UseDirtyGuardOptions) {
  const {
    isDirty,
    title = '离开确认',
    message = '当前表单有未保存的修改，离开将丢失已填写的内容。确定要离开吗？',
  } = options;

  const isBypassed = ref(false);

  // 路由离开前拦截
  onBeforeRouteLeave((_to, _from, next) => {
    if (isBypassed.value || !isDirty()) {
      next();
      return;
    }

    void confirmAsync({
      title,
      content: message,
      okText: '确认离开',
      cancelText: '继续编辑',
      danger: true,
    }).then((ok) => {
      if (ok) {
        next();
      } else {
        next(false);
      }
    });
  });

  // 浏览器原生关闭/刷新前拦截
  const handleBeforeUnload = (e: BeforeUnloadEvent) => {
    if (isBypassed.value || !isDirty()) {
      return;
    }
    e.preventDefault();
    e.returnValue = '';
  };

  onMounted(() => {
    window.addEventListener('beforeunload', handleBeforeUnload);
  });

  onUnmounted(() => {
    window.removeEventListener('beforeunload', handleBeforeUnload);
  });

  /**
   * 提交表单或放弃时手动解除保护，允许顺畅跳转
   */
  function markClean() {
    isBypassed.value = true;
  }

  return {
    markClean,
  };
}
