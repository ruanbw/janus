<template>
  <div>
    <PageHeader title="统计" description="选择一条短链,查看访问记录与来源明细">
      <template #actions>
        <div class="w-80 max-w-full">
          <AppSelect
            v-model="selectedLinkId"
            placeholder="选择短链查看访问统计"
            :options="linkOptions"
            allow-clear
            show-search
            @change="onSelectLink"
          />
        </div>
      </template>
    </PageHeader>

    <AppEmpty
      v-if="!selectedLink"
      description="请选择一条短链查看访问统计"
      class="rounded-xl border border-line bg-surface py-14"
    />

    <template v-else>
      <!-- 汇总卡片:短码 / 访问数 / 点击数 / 目标 URL / 状态 -->
      <div class="mb-5 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-3 lg:grid-cols-5">
        <div class="flex min-w-0 flex-col gap-1.5 bg-surface p-4">
          <span class="text-xs text-ink-faint">短码</span>
          <CopyText :text="selectedLink.code" class="mono text-[15px] font-semibold text-ink">
            {{ selectedLink.code }}
          </CopyText>
        </div>

        <div class="flex min-w-0 flex-col gap-1.5 bg-surface p-4">
          <span class="text-xs text-ink-faint">访问数</span>
          <span class="flex items-baseline gap-1.5">
            <span class="text-[26px] font-bold leading-none text-brand-600 tabular-nums dark:text-brand-400">
              {{ selectedLink.visits }}
            </span>
            <span class="text-xs text-ink-faint">次</span>
          </span>
        </div>

        <div class="flex min-w-0 flex-col gap-1.5 bg-surface p-4">
          <span class="text-xs text-ink-faint">点击数</span>
          <span class="flex items-baseline gap-1.5">
            <span class="text-[26px] font-bold leading-none text-brand-600 tabular-nums dark:text-brand-400">
              {{ selectedLink.linkType === 'landing' ? selectedLink.clicks : '—' }}
            </span>
            <span v-if="selectedLink.linkType === 'landing'" class="text-xs text-ink-faint">次</span>
          </span>
        </div>

        <div class="flex min-w-0 flex-col gap-1.5 bg-surface p-4">
          <span class="text-xs text-ink-faint">目标 URL</span>
          <AppTooltip v-if="selectedLink.targetUrls.length > 0">
            <template #title>
              <div v-for="(url, i) in selectedLink.targetUrls" :key="i" class="break-all">{{ url }}</div>
            </template>
            <span class="flex min-w-0 items-center gap-1.5">
              <span class="truncate text-[13px] text-ink">
                {{ truncateText(selectedLink.targetUrls[0], 40) }}
              </span>
              <span
                v-if="selectedLink.targetUrls.length > 1"
                class="shrink-0 rounded-full bg-brand-50 px-1.5 py-0.5 text-[11px] font-medium text-brand-600 dark:bg-brand-500/15 dark:text-brand-300"
              >
                +{{ selectedLink.targetUrls.length - 1 }}
              </span>
            </span>
          </AppTooltip>
          <span v-else class="text-[13px] text-ink-faint">-</span>
        </div>

        <div class="flex min-w-0 flex-col gap-1.5 bg-surface p-4 col-span-2 sm:col-span-2 lg:col-span-1">
          <span class="text-xs text-ink-faint">状态</span>
          <AppTag :color="LINK_STATUS[selectedLink.status].color">
            {{ LINK_STATUS[selectedLink.status].label }}
          </AppTag>
        </div>
      </div>

      <!-- 访问明细 -->
      <AppTable
        :columns="columns"
        :data-source="visits"
        :loading="visitsLoading"
        row-key="id"
        :pagination="pagination"
        :scroll="{ x: 1030 }"
        @change="onTableChange"
      >
        <template #cell="{ column, record }">
          <template v-if="column.key === 'createdAt'">
            <span class="whitespace-nowrap">{{ formatDateTime(record.createdAt) }}</span>
          </template>
          <template v-else-if="column.key === 'ip'">
            <AppTooltip v-if="record.ip && record.ip.length > 15" :title="record.ip">
              <span class="mono block max-w-full truncate text-[13px] text-ink-faint">{{ record.ip }}</span>
            </AppTooltip>
            <span v-else class="mono whitespace-nowrap text-[13px] text-ink-faint">{{ record.ip || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'deviceKind'">
            <AppTag :color="parseDevice(record.userAgent).kindColor">
              {{ parseDevice(record.userAgent).kind }}
            </AppTag>
          </template>
          <template v-else-if="column.key === 'device'">
            <AppTooltip placement="top" :title="parseDevice(record.userAgent).device">
              <span class="block max-w-full truncate text-[13px] text-ink">
                {{ parseDevice(record.userAgent).device }}
              </span>
            </AppTooltip>
          </template>
          <template v-else-if="column.key === 'osBrowser'">
            <AppTooltip placement="top" :title="parseDevice(record.userAgent).osBrowser">
              <span class="block max-w-full truncate text-[13px] text-ink-faint">
                {{ parseDevice(record.userAgent).osBrowser }}
              </span>
            </AppTooltip>
          </template>
          <template v-else-if="column.key === 'referer'">
            <AppTooltip :title="record.referer || '直接访问'">
              <span class="block max-w-full truncate text-[13px] text-ink">
                {{ record.referer ? truncateText(record.referer, 50) : '直接访问' }}
              </span>
            </AppTooltip>
          </template>
        </template>
      </AppTable>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { message } from '@/utils/toast';
import { UAParser } from 'ua-parser-js';
import type { TableColumn, TablePaginationConfig } from '@/components/ui/types';

import { getLink, listLinks } from '@/api/links';
import { listVisits } from '@/api/visits';
import PageHeader from '@/components/PageHeader.vue';
import { LINK_STATUS } from '@/constants/dict';
import { ApiError } from '@/types/api';
import type { Link, Visit } from '@/types/api';
import { formatDateTime, truncateText } from '@/utils/format';

const route = useRoute();

const links = ref<Link[]>([]);
const selectedLinkId = ref<number | undefined>(
  route.query.linkId ? Number(route.query.linkId) : undefined,
);
const selectedLink = computed(() =>
  links.value.find((l) => l.id === selectedLinkId.value),
);

const visits = ref<Visit[]>([]);
const visitsLoading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(10);

const linkOptions = computed(() =>
  links.value.map((l) => ({
    value: l.id,
    label: l.code + '(' + l.visits + ' 次访问)',
  })),
);

const columns: TableColumn[] = [
  { title: '访问时间', key: 'createdAt', dataIndex: 'createdAt', width: 180, nowrap: true },
  { title: '访问IP', key: 'ip', dataIndex: 'ip', width: 150, nowrap: true },
  { title: '设备类型', key: 'deviceKind', width: 100, nowrap: true, align: 'center' },
  { title: '设备', key: 'device', width: 160, ellipsis: true },
  { title: '系统/浏览器', key: 'osBrowser', width: 220, ellipsis: true },
  { title: '来源', key: 'referer', dataIndex: 'referer', minWidth: 220, ellipsis: true },
];

interface DeviceInfo {
  kind: string;
  kindColor: string;
  device: string;
  osBrowser: string;
}

const deviceCache = new Map<string, DeviceInfo>();

/** 解析 User-Agent:设备类型(PC/移动端/平板)、品牌型号、系统与浏览器版本 */
function parseDevice(ua: string): DeviceInfo {
  const cached = deviceCache.get(ua);
  if (cached) return cached;
  const empty: DeviceInfo = { kind: '未知', kindColor: 'default', device: '-', osBrowser: '-' };
  if (!ua.trim()) return empty;
  const p = new UAParser(ua);
  const d = p.getDevice();
  const o = p.getOS();
  const b = p.getBrowser();
  let kind = 'PC';
  let kindColor = 'blue';
  if (d.type === 'mobile') {
    kind = '移动端';
    kindColor = 'green';
  } else if (d.type === 'tablet') {
    kind = '平板';
    kindColor = 'orange';
  } else if (d.type) {
    kind = '其他';
    kindColor = 'purple';
  }
  const device = [d.vendor, d.model].filter(Boolean).join(' ').trim() || o.name || '未知设备';
  const osPart = [o.name, o.version].filter(Boolean).join(' ').trim();
  const browserPart = [b.name, b.major || b.version].filter(Boolean).join(' ').trim();
  const osBrowser = [osPart, browserPart].filter(Boolean).join(' · ') || '未知';
  const info: DeviceInfo = { kind, kindColor, device, osBrowser };
  deviceCache.set(ua, info);
  return info;
}

const pagination = computed<TablePaginationConfig>(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showSizeChanger: true,
  showTotal: (t) => '共 ' + t + ' 条',
}));

async function loadLinks() {
  try {
    const result = await listLinks({ page: 1, pageSize: 100 });
    links.value = result.items;
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  }
}

async function loadVisits() {
  if (!selectedLinkId.value) return;
  visitsLoading.value = true;
  try {
    const result = await listVisits(selectedLinkId.value, {
      page: page.value,
      pageSize: pageSize.value,
    });
    visits.value = result.items;
    total.value = result.total;
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  } finally {
    visitsLoading.value = false;
  }
}

onMounted(async () => {
  await loadLinks();
  if (selectedLinkId.value) {
    // 保持所选短链数据为最新
    const fresh = links.value.find((l) => l.id === selectedLinkId.value);
    if (!fresh) {
      // 列表前 100 条内未找到:直接按 id 拉取详情(短链页直达统计时可能落在更早的分页)
      try {
        const detail = await getLink(selectedLinkId.value);
        links.value = [detail, ...links.value];
      } catch (error) {
        if (error instanceof ApiError && error.status !== 401) {
          message.error(error.message);
        }
        // 不存在或无权访问:重置选择
        selectedLinkId.value = undefined;
      }
    }
    await loadVisits();
  }
});

function onSelectLink() {
  page.value = 1;
  loadVisits();
}

function onTableChange(p: TablePaginationConfig) {
  page.value = p.current ?? 1;
  pageSize.value = p.pageSize ?? 10;
  loadVisits();
}
</script>
