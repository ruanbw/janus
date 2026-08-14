// 认证状态:当前租户、登录/登出
import { computed, ref } from 'vue';
import { defineStore } from 'pinia';

import * as authApi from '@/api/auth';
import * as configApi from '@/api/config';
import * as meApi from '@/api/me';

import type { AppConfig, Tenant } from '@/types/api';

export const useAuthStore = defineStore('auth', () => {
  const tenant = ref<Tenant | null>(null);
  /** 启动配置(GET /api/config):服务器 IP/平台域名/当前租户配额,登录后默认加载 */
  const config = ref<AppConfig | null>(null);
  /** 是否已完成启动时的会话探测 */
  const initialized = ref(false);

  const isAuthenticated = computed(() => tenant.value !== null);
  const isSuperAdmin = computed(() => tenant.value?.isSuperAdmin === true);

  /**
   * 用量增强:GET /api/me 返回 tenant + usage(配额用量)。
   * 登录/会话探测成功后补充,失败不阻塞(usage 为可选展示信息)。
   */
  async function enrichUsage(): Promise<void> {
    try {
      const me = await meApi.fetchMyTenant();
      tenant.value = me;
    } catch {
      // 忽略:/api/me 失败不影响会话探测与登录
    }
  }

  /** 加载启动配置(GET /api/config),失败不阻塞(配置为可选展示信息) */
  async function loadConfig(): Promise<void> {
    try {
      config.value = await configApi.fetchConfig();
    } catch {
      config.value = null;
    }
  }

  /** 探测当前会话(未登录时返回 null,由路由守卫决定去向) */
  async function fetchMe(): Promise<Tenant | null> {
    try {
      tenant.value = await authApi.fetchMe();
      await enrichUsage();
      await loadConfig();
      return tenant.value;
    } catch {
      tenant.value = null;
      return null;
    }
  }

  /** 登录:成功后后端 Set-Cookie(cloak_session + cloak_csrf),请求层自动携带 CSRF */
  async function login(email: string, password: string, rememberMe?: boolean): Promise<Tenant> {
    const me = await authApi.login({ email, password, rememberMe });
    tenant.value = me;
    await enrichUsage();
    await loadConfig();
    return tenant.value;
  }

  /** 登出 */
  async function logout(): Promise<void> {
    try {
      await authApi.logout();
    } catch {
      // 会话已失效等情况下忽略,本地状态照常清理
    }
    tenant.value = null;
    config.value = null;
  }

  function clear(): void {
    tenant.value = null;
    config.value = null;
  }

  return { tenant, config, initialized, isAuthenticated, isSuperAdmin, fetchMe, login, logout, clear };
});
