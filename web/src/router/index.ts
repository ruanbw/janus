
import { createRouter, createWebHistory } from 'vue-router';

import AdminLayout from '@/layouts/AdminLayout.vue';
import { useAuthStore } from '@/stores/auth';
import { message } from '@/utils/toast';

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
      redirect: '/overview',
      children: [
        {
          path: 'overview',
          name: 'overview',
          component: () => import('@/views/overview/OverviewView.vue'),
          meta: { title: '系统总览' },
        },
        {
          path: 'domains',
          name: 'domains',
          component: () => import('@/views/domains/DomainsView.vue'),
          meta: { title: '域名池' },
        },
        {
          path: 'domains/new',
          name: 'domain-create',
          component: () => import('@/views/domains/DomainCreateView.vue'),
          meta: { title: '添加自有域名' },
        },
        {
          path: 'links',
          name: 'links',
          component: () => import('@/views/links/LinksView.vue'),
          meta: { title: '短链与目标' },
        },
        {
          path: 'links/new',
          name: 'link-create',
          component: () => import('@/views/links/LinkFormView.vue'),
          meta: { title: '创建短链' },
        },
        {
          path: 'links/:id/edit',
          name: 'link-edit',
          component: () => import('@/views/links/LinkFormView.vue'),
          meta: { title: '编辑短链' },
        },
        {
          path: 'links/:id/visits',
          name: 'link-visits',
          component: () => import('@/views/links/LinkVisitsView.vue'),
          meta: { title: '访问明细' },
        },
        {
          path: 'rules',
          name: 'rules',
          component: () => import('@/views/rules/RulesListView.vue'),
          meta: { title: '规则引擎' },
        },
        {
          path: 'rules/new',
          name: 'rule-create',
          component: () => import('@/views/rules/RuleFormView.vue'),
          meta: { title: '新建规则' },
        },
        {
          path: 'rules/:id/edit',
          name: 'rule-edit',
          component: () => import('@/views/rules/RuleFormView.vue'),
          meta: { title: '编辑规则' },
        },
        {
          path: 'rules/simulator',
          name: 'rule-simulator',
          component: () => import('@/views/rules/RuleSimulatorView.vue'),
          meta: { title: '规则模拟器' },
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
        {
          path: 'admin/tenants/:id/tier',
          name: 'admin-tenant-tier',
          component: () => import('@/views/admin/AdminTenantTierView.vue'),
          meta: { title: '调整等级', superAdmin: true },
        },
        {
          path: 'admin/tenants/:id/remove-domain',
          name: 'admin-tenant-remove-domain',
          component: () => import('@/views/admin/AdminTenantRemoveDomainView.vue'),
          meta: { title: '移除违规域名', superAdmin: true },
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
  document.title = title ? `${title} · Janus 后台` : 'Janus 后台';
  try {
    sessionStorage.removeItem(`chunk_reload_${to.fullPath}`);
  } catch {
    // 忽略 sessionStorage 访问限制异常
  }
});

/**
 * 读 chunk 自动重载计数。受限存储环境下 sessionStorage 会抛 SecurityError,
 * 此时返回 null —— 调用方据此**跳过自动重载**(拿不到计数就无法保证不重复重载),
 * 而不是让兜底机制自己先炸掉。
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

/** 受限存储下也必须能执行的重载计数清除 */
function clearChunkReloadCount(reloadKey: string): void {
  try {
    sessionStorage.removeItem(reloadKey);
  } catch {
    // 忽略 sessionStorage 访问限制异常
  }
}

// 路由错误捕获：防止前端部署更新或偶发网络抖动导致 Chunk 加载失败卡在白屏
router.onError((error, to) => {
  if (import.meta.env.DEV) console.error('[Router Error]', error, to);
  const msg = error instanceof Error ? error.message : String(error);
  const isChunkLoadFailed =
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('Importing a module script failed') ||
    msg.includes('error loading dynamically imported module') ||
    msg.includes('Unable to preload CSS') ||
    (error as { name?: string })?.name === 'ChunkLoadError';

  if (isChunkLoadFailed) {
    const targetPath = to?.fullPath || window.location.href;
    const reloadKey = `chunk_reload_${targetPath}`;
    const reloadCount = readChunkReloadCount(reloadKey);
    if (reloadCount === null) {
      // sessionStorage 不可用:无法判断是否已重载过,直接提示手动重试,绝不自动重载
      message.error('页面资源加载失败，请检查网络连接后刷新重试');
      return;
    }
    if (reloadCount < 1) {
      writeChunkReloadCount(reloadKey, reloadCount + 1);
      window.location.assign(targetPath);
      return;
    }
    // 已尝试自动刷新但仍未成功，提示用户手动重试，避免死循环重载
    clearChunkReloadCount(reloadKey);
    message.error('页面资源加载失败，请检查网络连接后刷新重试');
  }
});

export default router;
