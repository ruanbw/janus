/**
 * 访问明细筛选状态：四组筛选 + 时间窗口 + 分页 + 「是否在过滤」的唯一来源。
 *
 * 抽出来的动机见 tests/b9-composables/useVisitFilters.spec.ts 的文件头。三条语义
 * 在这里各只实现一次，视图层不再自己拼参数：
 *
 * 1. 任何筛选变化都把页码归 1（`set` 内统一做，调用点不用记得）。
 * 2. 空字符串字段不下发：后端把「参数缺失」当不过滤，显式空串没有约定语义。
 * 3. keyword / country 入库前 trim：前后空格会让匹配静默空结果，也会让
 *    「重置筛选」在结果集未变化时亮着。
 */
import { computed, ref } from 'vue';

import type { OverviewRange, VisitAction, VisitOutcome } from '@/types/api';

/** 空串表示「全部」，对应后端不传该参数 */
export type VisitActionFilter = '' | VisitAction;
export type VisitOutcomeFilter = '' | VisitOutcome;
/** 时间窗口；空串 = 不限（默认）。取值与后端 overviewRangeDays 的键一致 */
export type VisitRangeFilter = '' | OverviewRange;

export interface VisitFilterState {
  action: VisitActionFilter;
  outcome: VisitOutcomeFilter;
  country: string;
  keyword: string;
  range: VisitRangeFilter;
}

/** 每页条数的默认值；分页控件默认选中项也取它 */
export const DEFAULT_VISIT_PAGE_SIZE = 20;

/** 每次都要新建对象：默认值若共享，两个表单实例会互相污染 */
function defaultFilters(): VisitFilterState {
  return { action: '', outcome: '', country: '', keyword: '', range: '' };
}

/** 文本筛选统一 trim；空串与纯空格都归一成「不过滤」 */
function normalize(state: VisitFilterState): VisitFilterState {
  return {
    action: state.action ?? '',
    outcome: state.outcome ?? '',
    country: (state.country ?? '').trim(),
    keyword: (state.keyword ?? '').trim(),
    range: state.range ?? '',
  };
}

export function useVisitFilters(initial?: Partial<VisitFilterState>) {
  const filters = ref<VisitFilterState>(normalize({ ...defaultFilters(), ...initial }));
  const page = ref(1);
  const pageSize = ref(DEFAULT_VISIT_PAGE_SIZE);

  // 必须是 computed：模板里的「重置筛选」入口要随 set/reset 立即显隐，
  // 快照成普通布尔会让按钮状态停在创建那一刻。
  const hasActive = computed(() => {
    const { action, outcome, country, keyword, range } = filters.value;
    return action !== '' || outcome !== '' || country !== '' || keyword !== '' || range !== '';
  });

  /**
   * 局部更新筛选项。**任何**筛选变化都归 page=1 —— 用户停在第 5 页改筛选却只
   * 看到第 5 页的空结果，是本页最常见的「筛不出东西」投诉。
   */
  function set(patch: Partial<VisitFilterState>): void {
    filters.value = normalize({ ...filters.value, ...patch });
    page.value = 1;
  }

  /**
   * 调用方只知道「当前页」与它想跳的页，没有总数可判上界，所以这里只钳下界，
   * 上界交给视图按 totalPages 判断。
   */
  function setPage(value: number): void {
    page.value = Number.isFinite(value) ? Math.max(1, Math.trunc(value)) : 1;
  }

  /**
   * 清空筛选并回到第 1 页。pageSize 是用户的浏览偏好（不是筛选条件），
   * 重置筛选不该顺手把它改回默认值。
   */
  function reset(): void {
    filters.value = defaultFilters();
    page.value = 1;
  }

  /**
   * 请求参数：分页恒在，其余字段为空时**整体缺席**而不是下发空串。
   * 后端拿「参数缺失」当不过滤，`action=` 这种显式空值没有约定语义。
   */
  function toParams(): Record<string, string | number> {
    const params: Record<string, string | number> = {
      page: page.value,
      pageSize: pageSize.value,
    };
    const { action, outcome, country, keyword, range } = filters.value;
    if (action) params.action = action;
    if (outcome) params.outcome = outcome;
    if (country) params.country = country;
    if (keyword) params.keyword = keyword;
    if (range) params.range = range;
    return params;
  }

  return { filters, page, pageSize, hasActive, set, setPage, reset, toParams };
}
