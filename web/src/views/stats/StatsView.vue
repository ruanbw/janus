<template>
  <div>
    <div class="page-header">
      <h2 class="page-title">统计</h2>
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
    </div>

    <a-empty v-if="!selectedLink" description="请选择一条短链查看访问统计" />

    <template v-else>
      <a-card class="stat-card" :bordered="false" size="small">
        <a-descriptions :column="4" size="small">
          <a-descriptions-item label="短码">
            <a-typography-text strong copyable>{{ selectedLink.code }}</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="访问数">{{ selectedLink.visits }}</a-descriptions-item>
          <a-descriptions-item label="目标 URL">
            <a-tooltip :title="selectedLink.targetUrl">
              <span>{{ truncateText(selectedLink.targetUrl, 40) }}</span>
            </a-tooltip>
          </a-descriptions-item>
          <a-descriptions-item label="状态">
            <a-tag :color="LINK_STATUS[selectedLink.status].color">
              {{ LINK_STATUS[selectedLink.status].label }}
            </a-tag>
          </a-descriptions-item>
        </a-descriptions>
      </a-card>

      <a-table
        :columns="columns"
        :data-source="visits"
        :loading="visitsLoading"
        row-key="id"
        :pagination="pagination"
        @change="onTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'createdAt'">
            {{ formatDateTime(record.createdAt) }}
          </template>
          <template v-else-if="column.key === 'userAgent'">
            <a-tooltip :title="record.userAgent">
              <span>{{ truncateText(record.userAgent, 60) }}</span>
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'referer'">
            <a-tooltip :title="record.referer || '直接访问'">
              <span>{{ record.referer ? truncateText(record.referer, 40) : '直接访问' }}</span>
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'domain'">
            <a-tag color="blue">{{ record.domain }}</a-tag>
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
import type { TableColumnsType, TablePaginationConfig } from 'ant-design-vue';

import { getLink, listLinks } from '@/api/links';
import { listVisits } from '@/api/visits';
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
  { title: '域名', key: 'domain', dataIndex: 'domain', width: 180 },
  { title: 'User-Agent', key: 'userAgent', dataIndex: 'userAgent' },
  { title: '来源', key: 'referer', dataIndex: 'referer', width: 220 },
];

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
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.page-title {
  margin: 0;
  font-size: 18px;
}

.link-select {
  width: 320px;
}

.stat-card {
  margin-bottom: 16px;
  background: #fafafa;
}
</style>
