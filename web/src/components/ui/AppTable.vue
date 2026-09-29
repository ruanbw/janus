<template>
  <div>
    <div class="relative overflow-hidden rounded-xl border border-line bg-surface">
      <div class="overflow-x-auto">
        <table class="app-table w-full min-w-full border-collapse text-[13px]" :style="tableMinWidth">
          <colgroup>
            <col v-for="col in columns" :key="col.key" :style="columnStyle(col)" />
          </colgroup>
          <thead>
            <tr>
              <th
                v-for="col in columns"
                :key="col.key"
                class="whitespace-nowrap border-b border-line bg-surface-muted px-4 py-2.5 font-semibold text-ink-soft"
                :class="[
                  col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left',
                ]"
                :style="columnStyle(col)"
              >
                {{ col.title ?? '' }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(record, rowIndex) in dataSource" :key="String(record[rowKey] ?? rowIndex)" class="group transition-colors hover:bg-surface-muted">
              <td
                v-for="col in columns"
                :key="col.key"
                class="border-b border-line px-4 py-2.5 align-middle text-ink"
                :class="[
                  col.ellipsis ? 'max-w-0 truncate' : '',
                  col.nowrap ? 'whitespace-nowrap' : '',
                  col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left',
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
                <AppEmpty description="暂无数据" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 加载遮罩 -->
      <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-surface/55 backdrop-blur-[1px]">
        <Loader2 :size="26" class="animate-spin text-brand-600 dark:text-brand-400" />
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
          class="h-7 rounded-md border border-line bg-surface px-1.5 text-xs text-ink-soft focus:border-brand-500 focus:outline-none"
          @change="onPageSizeChange"
        >
          <option v-for="size in pageSizeOptions" :key="size" :value="size">{{ size }} 条/页</option>
        </select>
        <button
          type="button"
          class="flex h-7 w-7 items-center justify-center rounded-md border border-line text-ink-soft transition-colors hover:border-brand-400 hover:text-brand-600 disabled:opacity-40 disabled:hover:border-line disabled:hover:text-ink-soft"
          :disabled="currentPage <= 1"
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
            :class="p === currentPage ? 'border-brand-600 bg-brand-600 text-white' : 'border-line text-ink-soft hover:border-brand-400 hover:text-brand-600'"
            @click="goPage(p)"
          >
            {{ p }}
          </button>
        </template>
        <button
          type="button"
          class="flex h-7 w-7 items-center justify-center rounded-md border border-line text-ink-soft transition-colors hover:border-brand-400 hover:text-brand-600 disabled:opacity-40 disabled:hover:border-line disabled:hover:text-ink-soft"
          :disabled="currentPage >= totalPages"
          @click="goPage(currentPage + 1)"
        >
          <ChevronRight :size="14" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { ChevronLeft, ChevronRight, Loader2 } from '@lucide/vue';

import AppEmpty from './AppEmpty.vue';
import type { TableColumn, TablePaginationConfig } from './types';

const props = withDefaults(
  defineProps<{
    columns: TableColumn[];
    dataSource: Record<string, unknown>[];
    loading?: boolean;
    rowKey?: string;
    pagination?: false | TablePaginationConfig;
    scroll?: { x?: number | string };
  }>(),
  { loading: false, rowKey: 'id', pagination: false },
);

const emit = defineEmits<{
  change: [payload: { current: number; pageSize: number }];
}>();

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
