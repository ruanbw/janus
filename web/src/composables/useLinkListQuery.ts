/**
 * 短链列表的筛选 + 分页状态。
 *
 * 三个筛选条件与页码之间有一组容易漏掉的隐含规则，页面里各写一遍必然漂移：
 * - 换筛选必须回第 1 页，否则用户在「第 3 页 + 刚换的筛选」看到一个空列表，
 *   会以为「没有数据」而不是「页码越界」；
 * - 页大小非法时旧页码同样失去意义，必须一起归 1；
 * - keyword 存 trim 后的值，纯空白即视为「没有筛选」。
 *
 * toParams 只输出真正生效的条件：把 `keyword: ''` 或 `type: 'all'` 发给后端，
 * 等于把「未筛选」伪装成一个筛选值，后端一旦实现服务端过滤就会返回空结果。
 */
import { computed, ref } from 'vue';
import type { ComputedRef, Ref } from 'vue';

export type LinkTypeFilter = 'all' | 'redirect' | 'landing';
export type LinkStatusFilter = 'all' | 'enabled' | 'disabled';

export interface LinkListQuery {
  keyword: string;
  typeFilter: LinkTypeFilter;
  statusFilter: LinkStatusFilter;
  page: number;
  pageSize: number;
}

/** 默认页大小：与后端 parsePageParams 的缺省值一致 */
const DEFAULT_PAGE_SIZE = 20;

/**
 * 默认查询。reset() 一律回到这里，而不是回到构造时传入的 initial ——
 * 「重置」的语义是恢复出厂状态，不是恢复「这次打开页面时的样子」。
 */
const DEFAULTS: LinkListQuery = {
  keyword: '',
  typeFilter: 'all',
  statusFilter: 'all',
  page: 1,
  pageSize: DEFAULT_PAGE_SIZE,
};

export function useLinkListQuery(initial?: Partial<LinkListQuery>): {
  query: Ref<LinkListQuery>;
  hasActiveFilter: ComputedRef<boolean>;
  setKeyword(v: string): void;
  setTypeFilter(v: LinkTypeFilter): void;
  setStatusFilter(v: LinkStatusFilter): void;
  setPage(v: number): void;
  setPageSize(v: number): void;
  reset(): void;
  toParams(): Record<string, string | number>;
} {
  const query = ref<LinkListQuery>({ ...DEFAULTS, ...initial });

  /** 筛选变更的统一收尾：条件变了，旧页码已经指不到同一批数据 */
  function applyFilterChange(patch: Partial<LinkListQuery>): void {
    query.value = { ...query.value, ...patch, page: 1 };
  }

  function setKeyword(v: string): void {
    applyFilterChange({ keyword: v.trim() });
  }

  function setTypeFilter(v: LinkTypeFilter): void {
    applyFilterChange({ typeFilter: v });
  }

  function setStatusFilter(v: LinkStatusFilter): void {
    applyFilterChange({ statusFilter: v });
  }

  function setPage(v: number): void {
    // 后端分页是 1-based：page=0 会让请求直接 400。
    query.value = { ...query.value, page: v < 1 ? 1 : v };
  }

  function setPageSize(v: number): void {
    if (v < 1) {
      // size<1 后端会拒绝；此时旧页码已无意义，必须一并回到第 1 页。
      query.value = { ...query.value, pageSize: 1, page: 1 };
      return;
    }
    query.value = { ...query.value, pageSize: v };
  }

  function reset(): void {
    query.value = { ...DEFAULTS };
  }

  const hasActiveFilter = computed(
    () =>
      query.value.keyword !== '' ||
      query.value.typeFilter !== 'all' ||
      query.value.statusFilter !== 'all',
  );

  /** 请求参数：筛选键只在真正生效时出现，避免把「未筛选」当成筛选值发给后端 */
  function toParams(): Record<string, string | number> {
    const { keyword, typeFilter, statusFilter, page, pageSize } = query.value;
    const params: Record<string, string | number> = { page, pageSize };
    if (keyword !== '') params.keyword = keyword;
    if (typeFilter !== 'all') params.type = typeFilter;
    if (statusFilter !== 'all') params.status = statusFilter;
    return params;
  }

  return {
    query,
    hasActiveFilter,
    setKeyword,
    setTypeFilter,
    setStatusFilter,
    setPage,
    setPageSize,
    reset,
    toParams,
  };
}
