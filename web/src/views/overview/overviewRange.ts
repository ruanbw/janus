/**
 * 总览页时间范围快捷选择:值、文案与跨度。
 *
 * 与服务端 internal/httpapi/visits.go 的 overviewRangeDays 是同一份契约的两端:
 * value 必须逐一对应(改一处必须同步另一处),label 是纯展示文案。
 * 窗口边界不在这里推算 —— 服务端响应里的 window.from/to 才是唯一事实来源,
 * 浏览器时区与服务器时区不一致时,前端自己算会算出第二个"今日"。
 */
import dayjs from 'dayjs';

import type { OverviewRange } from '@/types/api';

export const OVERVIEW_RANGES = [
  { value: 'today', label: '今日' },
  { value: '3d', label: '近 3 天' },
  { value: '7d', label: '近 7 天' },
  { value: '30d', label: '近一个月' },
  { value: '1y', label: '近一年' },
] as const satisfies ReadonlyArray<{ value: OverviewRange; label: string }>;

/** 默认窗口:与服务端的 defaultOverviewRange 一致。 */
export const DEFAULT_OVERVIEW_RANGE: OverviewRange = '7d';

/** 各快捷范围的自然日跨度(含今日),与后端 overviewRangeDays 同值。 */
export const OVERVIEW_RANGE_DAYS: Record<OverviewRange, number> = {
  today: 1,
  '3d': 3,
  '7d': 7,
  '30d': 30,
  '1y': 365,
};

/** 快捷范围的显示文案(KPI 副文案、口径说明共用,避免两处各写一份)。 */
export function overviewRangeLabel(range: OverviewRange): string {
  return OVERVIEW_RANGES.find((item) => item.value === range)?.label ?? range;
}

/**
 * 判断一个来自 URL 的值是不是合法范围。
 *
 * 用 OVERVIEW_RANGES 而不是 `value in OVERVIEW_RANGE_DAYS` 收窄:后者会把
 * `?range=toString` 这类原型链上的键也算合法,请求带着一个后端不认的值出去。
 */
export function isOverviewRange(value: unknown): value is OverviewRange {
  return OVERVIEW_RANGES.some((item) => item.value === value);
}

/** 把服务端回显的窗口边界格式化成 YYYY-MM-DD(跨年窗口也不歧义)。 */
export function formatWindowDay(value?: string | null): string {
  if (!value) return '—';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD') : '—';
}
