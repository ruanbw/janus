// 短链 API(后台,会话鉴权)
import { del, get, patch, post, upload } from '@/utils/request';

import type {
  LandingSource,
  Link,
  LinkStatus,
  LinkType,
  PageResult,
  RedirectStatus,
} from '@/types/api';

export interface LinkListQuery {
  page?: number;
  pageSize?: number;
}

/**
 * 归一化短链响应:契约定义 redirectStatus 为 "301"|"302" 字符串,当前后端以数字(301/302)
 * 序列化;domains 在后端极端情况下可能为 null。此处统一转为契约形状,页面无需再兜底。
 * 落地页相关新字段在后端尚未回传时给出契约默认值。
 * 规则相关字段(ruleCount / ruleNames)同理:后端未回传时按“无关联规则”处理。
 */
function normalizeLink(link: Link): Link {
  return {
    ...link,
    domains: Array.isArray(link.domains) ? link.domains : [],
    targetUrls: Array.isArray(link.targetUrls) ? link.targetUrls : [],
    redirectStatus: String(link.redirectStatus) as RedirectStatus,
    linkType: link.linkType || 'redirect',
    landingSource: link.landingSource || 'url',
    landingUrl: link.landingUrl || '',
    clicks: typeof link.clicks === 'number' ? link.clicks : 0,
    landingUploaded: link.landingUploaded === true,
    ruleCount: typeof link.ruleCount === 'number' ? link.ruleCount : 0,
    ruleNames: Array.isArray(link.ruleNames) ? link.ruleNames : [],
    rules: Array.isArray(link.rules) ? link.rules : [],
  };
}

/** 短链列表(分页,含 visits/clicks 计数;默认不含逻辑删除项) */
export function listLinks(page: number, pageSize?: number): Promise<PageResult<Link>>;
export function listLinks(query?: LinkListQuery): Promise<PageResult<Link>>;
export async function listLinks(
  queryOrPage: LinkListQuery | number = {},
  pageSize?: number,
): Promise<PageResult<Link>> {
  const query: LinkListQuery =
    typeof queryOrPage === 'number'
      ? { page: queryOrPage, pageSize }
      : queryOrPage;
  const result = await get<PageResult<Link>>('/links', { ...query });
  return { ...result, items: result.items.map(normalizeLink) };
}

/** 创建短链(code 省略则自动生成;403 配额超限;409 同域名同短码) */
export function createLink(data: {
  code?: string;
  targetUrls: string[];
  domainIds: number[];
  redirectStatus?: RedirectStatus;
  linkType?: LinkType;
  landingSource?: LandingSource;
  landingUrl?: string;
}): Promise<Link> {
  return post<Link>('/links', data).then(normalizeLink);
}

/** 短链详情 */
export function getLink(id: number): Promise<Link> {
  return get<Link>('/links/' + id).then(normalizeLink);
}

/** 编辑:目标 URL / 关联域名 / 重定向方式 / 类型 / 落地页来源与地址 / 状态 */
export function updateLink(
  id: number,
  data: {
    targetUrls?: string[];
    domainIds?: number[];
    redirectStatus?: RedirectStatus;
    status?: LinkStatus;
    linkType?: LinkType;
    landingSource?: LandingSource;
    landingUrl?: string;
  },
): Promise<Link> {
  return patch<Link>('/links/' + id, data).then(normalizeLink);
}

/** 上传落地页 zip(multipart,字段 file,替换式;成功后 landingSource=upload、landingUploaded=true) */
export function uploadLanding(id: number, file: File): Promise<Link> {
  const formData = new FormData();
  formData.append('file', file);
  return upload<Link>('/links/' + id + '/landing', formData).then(normalizeLink);
}

/** 逻辑删除(记录与访问信息保留) */
export function deleteLink(id: number): Promise<void> {
  return del<void>('/links/' + id);
}

/** 批量逻辑删除:ids 非空且 ≤200;跨租户/已删除/不存在的 id 静默跳过,返回实际删除条数 */
export function batchDeleteLinks(ids: number[]): Promise<{ deleted: number }> {
  return post<{ deleted: number }>('/links/batch-delete', { ids });
}

/** 批量彻底删除(物理删除,含访问记录与落地页文件);语义同 batchDeleteLinks */
export function batchPurgeLinks(ids: number[]): Promise<{ deleted: number }> {
  return post<{ deleted: number }>('/links/batch-purge', { ids });
}

/** 彻底删除(物理删除,含访问记录) */
export function purgeLink(id: number): Promise<void> {
  return post<void>('/links/' + id + '/purge');
}
