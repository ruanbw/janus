<template>
  <div>
    <div class="relative overflow-hidden rounded-xl border border-line bg-surface">
      <div class="overflow-x-auto">
        <table class="app-table w-full min-w-full border-collapse" :style="tableMinWidth">
          <colgroup>
            <col v-for="col in columns" :key="col.key" :style="columnStyle(col)" />
          </colgroup>
          <thead>
            <tr>
              <th
                v-for="(col, colIndex) in columns"
                :key="col.key"
                class="whitespace-nowrap border-b border-line bg-muted px-4 py-2.5 font-semibold text-muted-foreground"
                :class="[
                  col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left',
                  pinnedClasses(col, true),
                ]"
                :style="columnStyle(col)"
              >
                <slot name="header" :column="col" :index="colIndex">{{ col.title ?? '' }}</slot>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(record, rowIndex) in dataSource"
              :key="String(record[rowKey] ?? rowIndex)"
              class="group transition-colors hover:bg-muted"
              :class="[rowClickable ? 'cursor-pointer' : '', rowClass?.(record, rowIndex)]"
              v-bind="rowProps?.(record, rowIndex) ?? {}"
              :tabindex="rowClickable ? 0 : undefined"
              @click="onRowClick(record, $event)"
              @keydown="onRowKeydown(record, $event)"
            >
              <td
                v-for="col in columns"
                :key="col.key"
                class="border-b border-line px-4 py-2.5 align-middle text-ink"
                :class="[
                  col.ellipsis ? 'max-w-0 truncate' : '',
                  col.nowrap ? 'whitespace-nowrap' : '',
                  col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left',
                  pinnedClasses(col, false),
                ]"
                :style="columnStyle(col)"
              >
                <slot name="cell" :column="col" :record="record" :index="rowIndex">
                  {{ cellText(record, col) }}
                </slot>
              </td>
            </tr>
            <tr v-if="dataSource.length === 0 && !loading">
              <td :colspan="columns.length" class="px-4 py-8">
                <slot name="empty" :columns="columns" :colspan="columns.length">
                  <AppEmpty description="暂无数据" />
                </slot>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 加载遮罩 -->
      <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-surface/55 backdrop-blur-[1px]">
        <Loader2 :size="26" class="animate-spin text-primary" />
      </div>
    </div>

    <!-- 分页 -->
    <div v-if="pagination !== false && pagination" class="mt-4 flex flex-wrap items-center justify-between gap-3">
      <span v-if="pagination.showTotal" class="text-xs text-ink-faint">
        {{ pagination.showTotal(pagination.total ?? 0) }}
      </span>
      <span v-else />
      <div class="flex items-center gap-1.5">
        <select
          :value="pageSize"
          class="h-7 rounded-md border border-input bg-background px-1.5 text-xs text-muted-foreground outline-none focus:border-ring"
          @change="onPageSizeChange"
        >
          <option v-for="size in pageSizeOptions" :key="size" :value="size">{{ size }} 条/页</option>
        </select>
        <button
          type="button"
          class="flex h-7 w-7 items-center justify-center rounded-md border border-input text-muted-foreground transition-colors hover:border-primary hover:text-primary disabled:opacity-40 disabled:hover:border-input disabled:hover:text-muted-foreground"
          :disabled="currentPage <= 1"
          aria-label="上一页"
          :title="'上一页'"
          @click="goPage(currentPage - 1)"
        >
          <ChevronLeft :size="14" />
        </button>
        <template v-for="p in pageNumbers" :key="p">
          <span v-if="p < 0" class="px-1 text-xs text-ink-faint">…</span>
          <button
            v-else
            type="button"
            class="flex h-7 min-w-7 items-center justify-center rounded-md border px-1.5 text-xs transition-colors"
            :class="p === currentPage ? 'border-primary bg-primary text-primary-foreground' : 'border-input text-muted-foreground hover:border-primary hover:text-primary'"
            @click="goPage(p)"
          >
            {{ p }}
          </button>
        </template>
        <button
          type="button"
          class="flex h-7 w-7 items-center justify-center rounded-md border border-input text-muted-foreground transition-colors hover:border-primary hover:text-primary disabled:opacity-40 disabled:hover:border-input disabled:hover:text-muted-foreground"
          :disabled="currentPage >= totalPages"
          aria-label="下一页"
          :title="'下一页'"
          @click="goPage(currentPage + 1)"
        >
          <ChevronRight :size="14" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance, ref, watch } from 'vue';
import { ChevronLeft, ChevronRight, Loader2 } from '@lucide/vue';

import AppEmpty from './AppEmpty.vue';
import type { TableColumn, TablePaginationConfig } from './types';

const props = withDefaults(defineProps<{
    columns: TableColumn[];
    dataSource: Record<string, unknown>[];
    loading?: boolean;
    rowKey?: string;
    pagination?: false | TablePaginationConfig;
    scroll?: { x?: number | string };
    /** 行级 class,如按 outcome 给行加失败底色 */
    rowClass?: (record: Record<string, unknown>, index: number) => string | undefined;
    /** 行级透传属性,如 data-* 供 scoped 样式使用 */
    rowProps?: (record: Record<string, unknown>, index: number) => Record<string, unknown> | undefined;
    /**
     * 行是否可点击。默认自动检测:挂了 @row-click 就启用(tabindex + 键盘可达),
     * 显式传 false 可只保留样式不要点击行为。
     */
    rowClickable?: boolean;
  }>(), { loading: false, rowKey: 'id', pagination: false });

const emit = defineEmits<{
  change: [payload: { current: number; pageSize: number }];
  rowClick: [record: Record<string, unknown>, event: MouseEvent | KeyboardEvent];
}>();

/** @row-click 存在与否决定行是否可聚焦;vnode.props 是这里唯一可靠的检测点 */
const hasRowClickListener = !!getCurrentInstance()?.vnode.props?.['onRowClick'];
const rowClickable = computed(() => props.rowClickable ?? hasRowClickListener);

function onRowClick(record: Record<string, unknown>, event: MouseEvent): void {
  if (!rowClickable.value) return;
  emit('rowClick', record, event);
}

function onRowKeydown(record: Record<string, unknown>, event: KeyboardEvent): void {
  if (!rowClickable.value) return;
  if (event.key !== 'Enter' && event.key !== ' ') return;
  event.preventDefault();
  emit('rowClick', record, event);
}

const pageSizeOptions = [10, 20, 50];

const currentPage = ref(props.pagination && props.pagination.current ? props.pagination.current : 1);
const pageSize = ref(props.pagination && props.pagination.pageSize ? props.pagination.pageSize : 10);

watch(
  () => props.pagination,
  (p) => {
    if (p === false || p === undefined) return;
    if (p.current !== undefined && p.current !== currentPage.value) currentPage.value = p.current;
    if (p.pageSize !== undefined && p.pageSize !== pageSize.value) pageSize.value = p.pageSize;
  },
  { deep: true },
);

const total = computed(() => (props.pagination && props.pagination.total) || 0);
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

function columnStyle(col: TableColumn): Record<string, string> | undefined {
  const style: Record<string, string> = {};
  if (col.width !== undefined) {
    const w = typeof col.width === 'number' ? `${col.width}px` : col.width;
    style.width = w;
    style.minWidth = col.minWidth ? (typeof col.minWidth === 'number' ? `${col.minWidth}px` : col.minWidth) : w;
  } else if (col.minWidth !== undefined) {
    style.minWidth = typeof col.minWidth === 'number' ? `${col.minWidth}px` : col.minWidth;
  }
  return Object.keys(style).length > 0 ? style : undefined;
}

/**
 * 固定列（当前仅 right）的样式。
 *
 * sticky 单元格必须自带不透明底色，否则右侧被横向滚走的内容会从它下面透出来；
 * 表头沿用已有的 bg-muted，数据格补 bg-surface 并跟随行的 hover 变 bg-muted，
 * 否则鼠标划过时行底色到了固定列就断了，出现一块颜色不一样的"补丁"。
 * 左侧再加一条 border-line 作为与滚动区的分界，滚到哪都看得清操作列在哪。
 */
function pinnedClasses(col: TableColumn, isHeader: boolean): string {
  if (col.fixed !== 'right') return '';
  return isHeader
    ? 'sticky right-0 z-20 border-l border-line'
    : 'sticky right-0 z-10 border-l border-line bg-surface group-hover:bg-muted';
}

const tableMinWidth = computed(() => {
  if (props.scroll && typeof props.scroll.x === 'number') {
    return { minWidth: props.scroll.x + 'px' };
  }
  if (props.scroll && typeof props.scroll.x === 'string') {
    return { minWidth: props.scroll.x };
  }
  let totalExplicit = 0;
  let hasExplicit = false;
  for (const col of props.columns) {
    if (typeof col.width === 'number') {
      totalExplicit += col.width;
      hasExplicit = true;
    } else if (typeof col.minWidth === 'number') {
      totalExplicit += col.minWidth;
      hasExplicit = true;
    }
  }
  if (hasExplicit && totalExplicit > 0) {
    return { minWidth: totalExplicit + 'px' };
  }
  return undefined;
});

/** 页码序列:当前页前后各 2 页,首尾页 + 省略 */
const pageNumbers = computed<number[]>(() => {
  const total = totalPages.value;
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }
  const current = currentPage.value;
  const set = new Set<number>([1, total, current - 2, current - 1, current, current + 1, current + 2]);
  const list = Array.from(set).filter((p) => p >= 1 && p <= total).sort((a, b) => a - b);
  const result: number[] = [];
  let prev = 0;
  for (const p of list) {
    if (prev !== 0 && p - prev > 1) result.push(-prev);
    result.push(p);
    prev = p;
  }
  return result;
});

function cellText(record: Record<string, unknown>, col: TableColumn): string {
  const key = col.dataIndex ?? col.key;
  const value = record[key];
  if (value === undefined || value === null) return '-';
  return String(value);
}

function goPage(page: number): void {
  if (page < 1 || page > totalPages.value || page === currentPage.value) return;
  currentPage.value = page;
  emit('change', { current: page, pageSize: pageSize.value });
}

function onPageSizeChange(event: Event): void {
  const next = Number((event.target as HTMLSelectElement).value);
  pageSize.value = next;
  currentPage.value = 1;
  emit('change', { current: 1, pageSize: next });
}
</script>
