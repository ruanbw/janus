// 认证状态:当前租户、登录/登出
import { computed, ref } from 'vue';
import { defineStore } from 'pinia';

import * as authApi from '@/api/auth';
import * as configApi from '@/api/config';
import * as meApi from '@/api/me';

import { ApiError } from '@/types/api';
import type { AppConfig, Tenant } from '@/types/api';

export const useAuthStore = defineStore('auth', () => {
  const tenant = ref<Tenant | null>(null);
  /** 启动配置(GET /api/config):服务器 IP/平台域名/当前租户配额,登录后默认加载 */
  const config = ref<AppConfig | null>(null);
  /** 是否已完成启动时的会话探测 */
  const initialized = ref(false);
  /**
   * 会话探测是否失败(网络抖动/超时/5xx)。
   *
   * 探测失败时登录态是「未知」而不是「未登录」:tenant 保持原值不清空,
   * 路由守卫据此不跳登录页 —— 否则一次网络抖动就能把持有效会话的用户踢出去。
   */
  const sessionProbeFailed = ref(false);

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

  /**
   * 探测当前会话(未登录时返回 null,由路由守卫决定去向)。
   *
   * 只有 401 才代表会话真的失效(服务端明确否认),此时清空 tenant;
   * 超时/断网/5xx 属于「探测失败」,必须保留现有登录态并置 sessionProbeFailed,
   * 否则一次网络抖动就会把持有效会话的用户踢到登录页。
   */
  async function fetchMe(): Promise<Tenant | null> {
    try {
      tenant.value = await authApi.fetchMe();
      sessionProbeFailed.value = false;
      await enrichUsage();
      await loadConfig();
      return tenant.value;
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        tenant.value = null;
        sessionProbeFailed.value = false;
        return null;
      }
      sessionProbeFailed.value = true;
      return tenant.value;
    }
  }

  /** 登录:成功后后端 Set-Cookie(janus_session + janus_csrf),请求层自动携带 CSRF
   *
   * setupToken 只在超管首次登录(尚无密码)时传:后端要求它才能建立会话,
   * 缺了就是一个普通 401。其余账号传 undefined(JSON 序列化时直接丢掉)。 */
  async function login(
    email: string,
    password: string,
    rememberMe?: boolean,
    setupToken?: string,
  ): Promise<Tenant> {
    const me = await authApi.login({ email, password, rememberMe, setupToken });
    tenant.value = me;
    sessionProbeFailed.value = false;
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
    sessionProbeFailed.value = false;
  }

  function clear(): void {
    tenant.value = null;
    config.value = null;
    sessionProbeFailed.value = false;
  }

  return {
    tenant,
    config,
    initialized,
    sessionProbeFailed,
    isAuthenticated,
    isSuperAdmin,
    fetchMe,
    login,
    logout,
    clear,
  };
});
