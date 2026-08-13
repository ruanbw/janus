// 认证状态:当前租户、登录/登出
import { computed, ref } from 'vue';
import { defineStore } from 'pinia';

import * as authApi from '@/api/auth';

import type { Tenant } from '@/types/api';

export const useAuthStore = defineStore('auth', () => {
  const tenant = ref<Tenant | null>(null);
  /** 是否已完成启动时的会话探测 */
  const initialized = ref(false);

  const isAuthenticated = computed(() => tenant.value !== null);
  const isSuperAdmin = computed(() => tenant.value?.isSuperAdmin === true);

  /** 探测当前会话(未登录时返回 null,由路由守卫决定去向) */
  async function fetchMe(): Promise<Tenant | null> {
    try {
      tenant.value = await authApi.fetchMe();
    } catch {
      tenant.value = null;
    }
    return tenant.value;
  }

  /** 登录:成功后后端 Set-Cookie(cloak_session + cloak_csrf),请求层自动携带 CSRF */
  async function login(email: string, password: string, rememberMe?: boolean): Promise<Tenant> {
    const me = await authApi.login({ email, password, rememberMe });
    tenant.value = me;
    return me;
  }

  /** 登出 */
  async function logout(): Promise<void> {
    try {
      await authApi.logout();
    } catch {
      // 会话已失效等情况下忽略,本地状态照常清理
    }
    tenant.value = null;
  }

  function clear(): void {
    tenant.value = null;
  }

  return { tenant, initialized, isAuthenticated, isSuperAdmin, fetchMe, login, logout, clear };
});
