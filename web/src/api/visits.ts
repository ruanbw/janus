// 访问统计 API(后台,会话鉴权)
import { get } from '@/utils/request';

import type { PageResult, Visit } from '@/types/api';

export interface VisitListQuery {
  page?: number;
  pageSize?: number;
}

/** 某条短链的访问列表(时间、UA、来源) */
export function listVisits(
  linkId: number,
  query: VisitListQuery = {},
): Promise<PageResult<Visit>> {
  return get<PageResult<Visit>>(`/links/${linkId}/visits`, { ...query });
}

/** 某条短链的访问数与点击数 */
export function getLinkStats(linkId: number): Promise<{ visits: number; clicks: number }> {
  return get<{ visits: number; clicks: number }>(`/links/${linkId}/stats`);
}
