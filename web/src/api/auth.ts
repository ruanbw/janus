// 认证 API(后台,会话鉴权)
import { get, post } from '@/utils/request';

import type { Tenant } from '@/types/api';

/** 注册:创建租户(pending)+ 平台默认域名 */
export function register(data: { email: string; password: string; slug: string }): Promise<Tenant> {
  return post<Tenant>('/auth/register', data);
}

/** 重发验证邮件:恒 202,不泄露邮箱是否存在 */
export function resendVerification(data: { email: string }): Promise<void> {
  return post<void>('/auth/resend-verification', data);
}

/** 邮箱验证 */
export function verifyEmail(data: { token: string }): Promise<void> {
  return post<void>('/auth/verify-email', data);
}

/** 登录:200 tenant + Set-Cookie(janus_session / janus_csrf) */
export function login(data: {
  email: string;
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

/** 修改密码(超管首次登录可省略 oldPassword) */
export function changePassword(data: {
  oldPassword?: string;
  newPassword: string;
}): Promise<void> {
  return post<void>('/auth/change-password', data);
}

/** 忘记密码:始终 202,不泄露邮箱是否存在 */
export function forgotPassword(data: { email: string }): Promise<void> {
  return post<void>('/auth/forgot-password', data);
}

/** 重置密码 */
export function resetPassword(data: { token: string; newPassword: string }): Promise<void> {
  return post<void>('/auth/reset-password', data);
}
