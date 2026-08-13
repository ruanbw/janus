// 短链 API(后台,会话鉴权)
import { del, get, patch, post } from '@/utils/request';

import type { Link, LinkStatus, PageResult, RedirectStatus } from '@/types/api';

export interface LinkListQuery {
  page?: number;
  pageSize?: number;
}

/**
 * 归一化短链响应:契约定义 redirectStatus 为 "301"|"302" 字符串,当前后端以数字(301/302)
 * 序列化;domains 在后端极端情况下可能为 null。此处统一转为契约形状,页面无需再兜底。
 */
function normalizeLink(link: Link): Link {
  return {
    ...link,
    domains: Array.isArray(link.domains) ? link.domains : [],
    redirectStatus: String(link.redirectStatus) as RedirectStatus,
  };
}

/** 短链列表(分页,含 visits 访问数;默认不含逻辑删除项) */
export async function listLinks(query: LinkListQuery = {}): Promise<PageResult<Link>> {
  const result = await get<PageResult<Link>>('/links', { ...query });
  return { ...result, items: result.items.map(normalizeLink) };
}

/** 创建短链(code 省略则自动生成;403 配额超限;409 同域名同短码) */
export function createLink(data: {
  code?: string;
  targetUrl: string;
  domainIds: number[];
  redirectStatus?: RedirectStatus;
}): Promise<Link> {
  return post<Link>('/links', data).then(normalizeLink);
}

/** 短链详情 */
export function getLink(id: number): Promise<Link> {
  return get<Link>(`/links/${id}`).then(normalizeLink);
}

/** 编辑:目标 URL / 关联域名 / 重定向方式 / 状态 */
export function updateLink(
  id: number,
  data: {
    targetUrl?: string;
    domainIds?: number[];
    redirectStatus?: RedirectStatus;
    status?: LinkStatus;
  },
): Promise<Link> {
  return patch<Link>(`/links/${id}`, data).then(normalizeLink);
}

/** 逻辑删除(记录与访问信息保留) */
export function deleteLink(id: number): Promise<void> {
  return del<void>(`/links/${id}`);
}

/** 彻底删除(物理删除,含访问记录) */
export function purgeLink(id: number): Promise<void> {
  return post<void>(`/links/${id}/purge`);
}
