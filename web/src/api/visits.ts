// 访问统计 API(后台,会话鉴权)
import { get } from '@/utils/request';

import type {
  OverviewStats,
  PageResult,
  Visit,
  VisitAction,
  VisitOutcome,
} from '@/types/api';

export interface VisitListQuery {
  page?: number;
  pageSize?: number;
  /** 按动作过滤:redirect 跳转 / landing_view 落地页 / click 点击;省略=不过滤 */
  action?: VisitAction;
  /** 按结果过滤:success 成功 / failed 失败;省略=不过滤 */
  outcome?: VisitOutcome;
}

/** 某条短链的访问列表(时间、UA、来源、动作与结果) */
export function listVisits(
  linkId: number,
  query: VisitListQuery = {},
): Promise<PageResult<Visit>> {
  return get<PageResult<Visit>>(`/links/${linkId}/visits`, { ...query });
}

/**
 * 总览页的全量聚合(总访问数、CTR、各维度分布、热门短链排行)。
 *
 * 总览页不再用 listVisits 拉明细自己数:那会让分布图混入点击行与失败行,
 * 把"最近 50 行"当全量,并且只覆盖短链列表第一页的 100 条。口径与理由见
 * internal/httpapi/visits.go 的 handleVisitsOverview。
 */
export function fetchOverviewStats(): Promise<OverviewStats> {
  return get<OverviewStats>('/visits/overview');
}
