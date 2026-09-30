// 访问统计 API(后台,会话鉴权)
import { get } from '@/utils/request';

import type { PageResult, Visit, VisitAction } from '@/types/api';

export interface VisitListQuery {
  page?: number;
  pageSize?: number;
  /** 按动作过滤:redirect 跳转 / landing_view 落地页 / click 点击;省略=不过滤 */
  action?: VisitAction;
}

/** 某条短链的访问列表(时间、UA、来源、动作与结果) */
export function listVisits(
  linkId: number,
  query: VisitListQuery = {},
): Promise<PageResult<Visit>> {
  return get<PageResult<Visit>>(`/links/${linkId}/visits`, { ...query });
}
