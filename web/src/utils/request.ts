// 请求封装:
// - 会话 cookie(cloak_session,HTTP-only)由浏览器自动携带(axios withCredentials)
// - CSRF 双提交 token:从 cookie cloak_csrf 读取,放入 X-CSRF-Token 请求头
// - 统一错误处理:解析 { code, message, details? } 结构并抛出 ApiError
import axios, { AxiosError } from 'axios';

import type { ApiErrorBody } from '@/types/api';
import { ApiError } from '@/types/api';

/** API 前缀,开发环境经 Vite proxy 转发到 Go 后端 */
const API_PREFIX = import.meta.env.VITE_API_PREFIX || '/api';

/** CSRF cookie 名(后端约定) */
const CSRF_COOKIE = 'cloak_csrf';

/** 无需 CSRF 的安全方法 */
const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);

/** 公开页面:401 时不强制跳登录(如邮箱验证、重置密码链接页)。用精确匹配,避免 /login-xxx 误命中 */
const PUBLIC_PATHS = new Set(['/login', '/register', '/verify-email', '/forgot-password', '/reset-password']);

/** 401 处理器(由 main.ts 注册,与 Vue Router 集成);未注册时回退整页跳转 */
let unauthorizedHandler: ((redirectTo?: string) => void) | null = null;

/** 注册 401 处理(会话失效时清理登录态并跳登录页,保留原路径) */
export function setUnauthorizedHandler(handler: (redirectTo?: string) => void): void {
  unauthorizedHandler = handler;
}

/** 读取 cookie 值(双提交 token 从 cookie 取) */
export function getCookie(name: string): string | undefined {
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`));
  if (!match) return undefined;
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return match[1];
  }
}

export function getCsrfToken(): string | undefined {
  return getCookie(CSRF_COOKIE);
}

/** 回退方案:整页跳登录并携带 redirect(与路由守卫的 ?redirect= 衔接) */
function redirectToLogin(redirectTo?: string): void {
  const query = redirectTo && redirectTo !== '/' ? `?redirect=${encodeURIComponent(redirectTo)}` : '';
  window.location.href = `/login${query}`;
}

const http = axios.create({
  baseURL: API_PREFIX,
  withCredentials: true, // 携带/接收会话 cookie
  timeout: 15000,
});

// 请求拦截:为写方法附加 X-CSRF-Token(双提交 token)
http.interceptors.request.use((config) => {
  if (!SAFE_METHODS.has((config.method || 'get').toUpperCase())) {
    const token = getCsrfToken();
    if (token) {
      config.headers.set('X-CSRF-Token', token);
    }
  }
  return config;
});

/** 从错误对象中提取统一错误结构 */
function extractApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error;
  if (axios.isAxiosError(error)) {
    const axiosError = error as AxiosError<ApiErrorBody>;
    const status = axiosError.response?.status ?? 0;
    const body = axiosError.response?.data;
    if (body && typeof body === 'object' && typeof body.message === 'string') {
      return new ApiError(status, body.code || 'E_UNKNOWN', body.message, body.details);
    }
    if (axiosError.code === 'ECONNABORTED') {
      return new ApiError(status, 'E_TIMEOUT', '请求超时,请稍后重试');
    }
    if (!axiosError.response) {
      return new ApiError(status, 'E_NETWORK', '无法连接后台服务,请检查网络或稍后重试');
    }
    return new ApiError(status, 'E_UNKNOWN', `请求失败(HTTP ${status})`);
  }
  if (error instanceof Error) {
    return new ApiError(0, 'E_UNKNOWN', error.message);
  }
  return new ApiError(0, 'E_UNKNOWN', '未知错误');
}

// 响应拦截:统一错误处理
http.interceptors.response.use(
  (response) => response,
  (error: unknown) => {
    const apiError = extractApiError(error);
    // 未认证:清除本地登录态并回到登录页(公开页面除外)
    if (apiError.status === 401 && !PUBLIC_PATHS.has(window.location.pathname)) {
      // 保留原路径与查询串,登录后由 LoginView 依据 ?redirect= 跳回
      const redirectTo = window.location.pathname + window.location.search;
      if (unauthorizedHandler) {
        unauthorizedHandler(redirectTo);
      } else {
        redirectToLogin(redirectTo);
      }
    }
    return Promise.reject(apiError);
  },
);

/**
 * 通用请求:成功时返回响应体数据;失败时抛出 ApiError
 * (错误已统一结构化,页面层用 message.error(apiError.message) 展示)
 */
export async function request<T>(config: Parameters<typeof http.request>[0]): Promise<T> {
  const response = await http.request<T>(config);
  return response.data;
}

/** GET */
export function get<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  return request<T>({ method: 'GET', url, params });
}

/** POST */
export function post<T>(url: string, data?: unknown): Promise<T> {
  return request<T>({ method: 'POST', url, data });
}

/** PATCH */
export function patch<T>(url: string, data?: unknown): Promise<T> {
  return request<T>({ method: 'PATCH', url, data });
}

/** DELETE(204 无响应体,返回 void) */
export function del<T = void>(url: string): Promise<T> {
  return request<T>({ method: 'DELETE', url });
}

export default http;
