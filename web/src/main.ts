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

/** 部署更新 / 网络抖动导致懒加载 chunk 失效的特征错误 */
const CHUNK_ERROR_PATTERNS = [
  'Failed to fetch dynamically imported module',
  'Importing a module script failed',
  'error loading dynamically imported module',
  'Unable to preload CSS',
];

function isChunkLoadError(reason: unknown): boolean {
  const msg = reason instanceof Error ? reason.message : String(reason || '');
  return CHUNK_ERROR_PATTERNS.some((pattern) => msg.includes(pattern));
}

/**
 * 读 chunk 自动重载计数。sessionStorage 在受限环境(隐私模式 / 企业策略 / 扩展拦截)
 * 下会抛 SecurityError —— 这正是兜底代码最不该自己炸掉的地方,返回 null 让调用方
 * 跳过自动重载(拿不到计数就无法保证不重复重载,绝不无限重试)。
 */
function readChunkReloadCount(reloadKey: string): number | null {
  try {
    return parseInt(sessionStorage.getItem(reloadKey) || '0', 10);
  } catch {
    return null;
  }
}

/** 受限存储下也必须能执行的重载计数写入 */
function writeChunkReloadCount(reloadKey: string, count: number): void {
  try {
    sessionStorage.setItem(reloadKey, String(count));
  } catch {
    // 忽略 sessionStorage 访问限制异常
  }
}

// 全局 Vue 渲染与运行时错误捕获,避免未处理异常导致渲染树崩溃静默白屏
app.config.errorHandler = (err, instance, info) => {
  if (import.meta.env.DEV) console.error('[Vue Global Error]', err, info);
};

// 全局 Promise Rejection 兜底,捕获漏网的 Chunk 加载失败或未捕获的异步异常
window.addEventListener('unhandledrejection', (event) => {
  if (import.meta.env.DEV) console.error('[Unhandled Promise Rejection]', event.reason);
  if (!isChunkLoadError(event.reason)) return;
  // 我们接管了 chunk 失败:preventDefault 掉浏览器默认的
  // Uncaught (in promise),否则同一份错误会被打印两遍
  event.preventDefault();
  const reloadKey = `chunk_reload_${window.location.pathname}`;
  const reloadCount = readChunkReloadCount(reloadKey);
  if (reloadCount !== null && reloadCount < 1) {
    writeChunkReloadCount(reloadKey, reloadCount + 1);
    window.location.reload();
  }
});

window.addEventListener('error', (event) => {
  // 资源加载失败(img/script/link)只有 message 没有 error 对象,
  // 打出来只会是 "[Global Error] undefined" 的噪音,直接过滤
  if (!event.error) return;
  if (import.meta.env.DEV) console.error('[Global Error]', event.error);
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
  void router
    .isReady()
    .then(() => {
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
    })
    // 首次导航失败时 isReady() 会 reject(markAsReady(err));不接住就是
    // 真·Uncaught (in promise)。此处仍要清理登录态并复位 redirecting,
    // 否则会话已失效的用户会被永久卡在「处理中」。
    .catch(() => {
      useAuthStore().clear();
      redirecting = false;
    });
});

app.mount('#app');
