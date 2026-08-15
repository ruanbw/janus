<template>
  <div>
    <PageHeader title="统计" description="选择一条短链,查看访问记录与来源明细">
      <template #actions>
        <a-select
          v-model:value="selectedLinkId"
          class="link-select"
          placeholder="选择短链查看访问统计"
          :options="linkOptions"
          allow-clear
          show-search
          option-filter-prop="label"
          @change="onSelectLink"
        />
      </template>
    </PageHeader>

    <a-empty v-if="!selectedLink" description="请选择一条短链查看访问统计" class="stats-empty" />

    <template v-else>
      <div class="summary">
        <div class="summary-item">
          <span class="summary-label">短码</span>
          <a-typography-text strong copyable class="summary-code mono">
            {{ selectedLink.code }}
          </a-typography-text>
        </div>
        <div class="summary-item">
          <span class="summary-label">访问数</span>
          <span class="summary-value">{{ selectedLink.visits }}</span>
          <span class="summary-unit">次</span>
        </div>
        <div class="summary-item">
          <span class="summary-label">点击数</span>
          <span class="summary-value">
            {{ selectedLink.linkType === 'landing' ? selectedLink.clicks : '—' }}
          </span>
          <span v-if="selectedLink.linkType === 'landing'" class="summary-unit">次</span>
        </div>
        <div class="summary-item">
          <span class="summary-label">目标 URL</span>
          <a-tooltip v-if="selectedLink.targetUrls.length > 0">
            <template #title>
              <div v-for="(url, i) in selectedLink.targetUrls" :key="i">{{ url }}</div>
            </template>
            <span class="summary-target">
              {{ truncateText(selectedLink.targetUrls[0], 40) }}
              <span v-if="selectedLink.targetUrls.length > 1" class="summary-target-more">
                +{{ selectedLink.targetUrls.length - 1 }}
              </span>
            </span>
          </a-tooltip>
          <span v-else class="summary-target">-</span>
        </div>
        <div class="summary-item">
          <span class="summary-label">状态</span>
          <a-tag :color="LINK_STATUS[selectedLink.status].color">
            {{ LINK_STATUS[selectedLink.status].label }}
          </a-tag>
        </div>
      </div>

      <a-table
        :columns="columns"
        :data-source="visits"
        :loading="visitsLoading"
        row-key="id"
        :pagination="pagination"
        :scroll="{ x: 940 }"
        @change="onTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'createdAt'">
            {{ formatDateTime(record.createdAt) }}
          </template>
          <template v-else-if="column.key === 'ip'">
            <span class="ua-muted mono">{{ record.ip || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'deviceKind'">
            <a-tag :color="parseDevice(record.userAgent).kindColor">
              {{ parseDevice(record.userAgent).kind }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'device'">
            <a-tooltip placement="top" :title="record.userAgent || '未知'">
              <span class="ua-cell">{{ parseDevice(record.userAgent).device }}</span>
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'osBrowser'">
            <span class="ua-muted ua-cell">{{ parseDevice(record.userAgent).osBrowser }}</span>
          </template>
          <template v-else-if="column.key === 'referer'">
            <a-tooltip :title="record.referer || '直接访问'">
              <span>{{ record.referer ? truncateText(record.referer, 40) : '直接访问' }}</span>
            </a-tooltip>
          </template>

        </template>
      </a-table>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { message } from 'ant-design-vue';
import { UAParser } from 'ua-parser-js';
import type { TableColumnsType, TablePaginationConfig } from 'ant-design-vue';

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
    label: `${l.code}(${l.visits} 次访问)`,
  })),
);

const columns: TableColumnsType = [
  { title: '访问时间', key: 'createdAt', dataIndex: 'createdAt', width: 180 },
  { title: '访问IP', key: 'ip', dataIndex: 'ip', width: 130 },
  { title: '设备类型', key: 'deviceKind', width: 100 },
  { title: '设备', key: 'device', width: 140 },
  { title: '系统/浏览器', key: 'osBrowser', width: 190 },
  { title: '来源', key: 'referer', dataIndex: 'referer', width: 200 },
];

interface DeviceInfo {
  kind: string;
  kindColor: string;
  device: string;
  osBrowser: string;
}

/** 解析 User-Agent:设备类型(PC/移动端/平板)、品牌型号、系统与浏览器版本 */
function parseDevice(ua: string): DeviceInfo {
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
  return { kind, kindColor, device, osBrowser };
}

const pagination = computed<TablePaginationConfig>(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showSizeChanger: true,
  showTotal: (t) => `共 ${t} 条`,
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

<style scoped>
.link-select {
  width: 320px;
  max-width: 100%;
}

.stats-empty {
  padding: 48px 0;
}

.summary {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1px;
  background: #e6ebf1;
  border: 1px solid #e6ebf1;
  border-radius: 10px;
  overflow: hidden;
  margin-bottom: 20px;
}

@media (min-width: 992px) {
  .summary {
    grid-template-columns: repeat(4, 1fr);
  }
}

.summary-item {
  background: #f8fafc;
  padding: 14px 18px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.summary-label {
  font-size: 12px;
  color: #8b98a5;
}

.summary-code {
  font-size: 15px;
}

.summary-value {
  font-size: 24px;
  font-weight: 700;
  color: #0e7490;
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
}

.summary-unit {
  font-size: 12px;
  color: #8b98a5;
}

.summary-target {
  font-size: 13px;
  color: #334155;
  word-break: break-all;
}

.summary-target-more {
  margin-left: 6px;
  padding: 0 6px;
  font-size: 12px;
  line-height: 18px;
  color: #1677ff;
  background: rgba(22, 119, 255, 0.1);
  border-radius: 9px;
  white-space: nowrap;
}

.ua-muted {
  color: #5b6b7c;
  font-size: 13px;
}

.ua-cell {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}
</style>
