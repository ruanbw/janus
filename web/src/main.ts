import { createApp } from 'vue';
import { createPinia } from 'pinia';

// Inter Variable:theme.css 的 --font-sans 第一顺位,CJK 仍走系统栈
import '@fontsource-variable/inter';

import '@/styles/main.css';

import App from './App.vue';
import router from './router';
import UI from '@/components/app';
import { useAuthStore } from '@/stores/auth';
import { useThemeStore } from '@/stores/theme';
import { setUnauthorizedHandler } from '@/utils/request';

const app = createApp(App);
const pinia = createPinia();

// 全局 Vue 渲染与运行时错误捕获，避免未处理异常导致渲染树崩溃静默白屏
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue Global Error]', err, info);
};

// 全局 Promise Rejection 兜底，捕获漏网的 Chunk 加载失败或未捕获的异步异常
window.addEventListener('unhandledrejection', (event) => {
  console.error('[Unhandled Promise Rejection]', event.reason);
  const msg = event.reason instanceof Error ? event.reason.message : String(event.reason || '');
  if (
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('Importing a module script failed') ||
    msg.includes('error loading dynamically imported module') ||
    msg.includes('Unable to preload CSS')
  ) {
    const reloadKey = `chunk_reload_${window.location.pathname}`;
    const reloadCount = parseInt(sessionStorage.getItem(reloadKey) || '0', 10);
    if (reloadCount < 1) {
      sessionStorage.setItem(reloadKey, String(reloadCount + 1));
      window.location.reload();
    }
  }
});

window.addEventListener('error', (event) => {
  console.error('[Global Error]', event.error || event.message);
});

app.use(pinia);
app.use(router);
app.use(UI);

// 首帧前应用主题,避免深浅色闪变
useThemeStore(pinia).apply();

// 401 → 清理登录态并回登录页(保留原路径,与路由守卫的 ?redirect= 衔接)。
// 路由未就绪时(首屏守卫内的会话探测)由守卫自行重定向,这里跳过避免重复导航。
let redirecting = false;
setUnauthorizedHandler((redirectTo) => {
  if (redirecting) return;
  redirecting = true;
  void router.isReady().then(() => {
    const auth = useAuthStore();
    auth.clear();
    if (router.currentRoute.value.path !== '/login') {
      const query: Record<string, string> = {};
      if (redirectTo && redirectTo !== '/') {
        query.redirect = redirectTo;
      }
      router.replace({ path: '/login', query }).catch(() => {
        // 导航被并发导航取消时忽略
      });
    }
    redirecting = false;
  });
});

app.mount('#app');
