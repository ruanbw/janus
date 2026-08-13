import { createApp } from 'vue';
import { createPinia } from 'pinia';
import Antd from 'ant-design-vue';

import 'ant-design-vue/dist/reset.css';

import App from './App.vue';
import router from './router';
import { useAuthStore } from '@/stores/auth';
import { setUnauthorizedHandler } from '@/utils/request';

const app = createApp(App);
const pinia = createPinia();

app.use(pinia);
app.use(router);
app.use(Antd);

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
