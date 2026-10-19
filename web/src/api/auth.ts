// 认证 API(后台,会话鉴权)
import { get, post } from '@/utils/request';

import type { Tenant } from '@/types/api';

/** 注册:创建租户 + 平台默认域名,注册即生效(不再有验证邮件) */
export function register(data: { username: string; password: string; slug: string }): Promise<Tenant> {
  return post<Tenant>('/auth/register', data);
}

/** 登录:200 tenant + Set-Cookie(janus_session / janus_csrf) */
export function login(data: {
  username: string;
  password: string;
  rememberMe?: boolean;
}): Promise<Tenant> {
  return post<Tenant>('/auth/login', data);
}

/** 登出(需 X-CSRF-Token;契约与后端路由均为 POST) */
export function logout(): Promise<void> {
  return post<void>('/auth/logout');
}

/** 当前租户信息 */
export function fetchMe(): Promise<Tenant> {
  return get<Tenant>('/auth/me');
}

/** 修改密码(强制改密的首登可省略 oldPassword) */
export function changePassword(data: {
  oldPassword?: string;
  newPassword: string;
}): Promise<void> {
  return post<void>('/auth/change-password', data);
}
