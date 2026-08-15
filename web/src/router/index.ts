import { createRouter, createWebHistory } from 'vue-router';

import AdminLayout from '@/layouts/AdminLayout.vue';
import { useAuthStore } from '@/stores/auth';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/auth/LoginView.vue'),
      meta: { public: true, title: '登录' },
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('@/views/auth/RegisterView.vue'),
      meta: { public: true, title: '注册' },
    },
    {
      path: '/verify-email',
      name: 'verify-email',
      component: () => import('@/views/auth/VerifyEmailView.vue'),
      meta: { public: true, title: '邮箱验证' },
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      component: () => import('@/views/auth/ForgotPasswordView.vue'),
      meta: { public: true, title: '忘记密码' },
    },
    {
      path: '/reset-password',
      name: 'reset-password',
      component: () => import('@/views/auth/ResetPasswordView.vue'),
      meta: { public: true, title: '重置密码' },
    },
    {
      path: '/',
      component: AdminLayout,
      redirect: '/domains',
      children: [
        {
          path: 'domains',
          name: 'domains',
          component: () => import('@/views/domains/DomainsView.vue'),
          meta: { title: '域名' },
        },
        {
          path: 'links',
          name: 'links',
          component: () => import('@/views/links/LinksView.vue'),
          meta: { title: '短链' },
        },
        {
          path: 'stats',
          name: 'stats',
          component: () => import('@/views/stats/StatsView.vue'),
          meta: { title: '统计' },
        },
        {
          path: 'account',
          name: 'account',
          component: () => import('@/views/account/AccountView.vue'),
          meta: { title: '账号设置' },
        },
        {
          path: 'admin/tenants',
          name: 'admin-tenants',
          component: () => import('@/views/admin/AdminTenantsView.vue'),
          meta: { title: '平台管理', superAdmin: true },
        },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
});

// 全局守卫:会话探测 + 登录拦截 + 超管路由拦截
router.beforeEach(async (to) => {
  const auth = useAuthStore();

  if (!auth.initialized) {
    await auth.fetchMe();
    auth.initialized = true;
  }

  if (to.meta.public) {
    // 已登录访问登录/注册页 → 回后台
    if (auth.isAuthenticated && (to.path === '/login' || to.path === '/register')) {
      return { path: '/domains' };
    }
    return true;
  }

  if (!auth.isAuthenticated) {
    return { path: '/login', query: to.fullPath === '/' ? {} : { redirect: to.fullPath } };
  }

  if (to.meta.superAdmin && !auth.isSuperAdmin) {
    return { path: '/domains' };
  }

  return true;
});

router.afterEach((to) => {
  const title = to.meta.title as string | undefined;
  document.title = title ? `${title} · CLOAK 后台` : 'CLOAK 后台';
});

export default router;
