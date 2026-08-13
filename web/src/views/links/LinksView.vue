<template>
  <div>
    <div class="page-header">
      <h2 class="page-title">短链</h2>
      <a-button type="primary" @click="openCreate">
        <template #icon><PlusOutlined /></template>
        创建短链
      </a-button>
    </div>

    <a-alert
      v-if="quotaInfo"
      class="quota-alert"
      type="info"
      show-icon
      :message="quotaInfo"
    />

    <a-table
      :columns="columns"
      :data-source="links"
      :loading="loading"
      row-key="id"
      :pagination="pagination"
      @change="onTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'code'">
          <a-typography-text strong copyable>{{ record.code }}</a-typography-text>
        </template>
        <template v-else-if="column.key === 'targetUrl'">
          <a-tooltip :title="record.targetUrl">
            <span>{{ truncateText(record.targetUrl, 40) }}</span>
          </a-tooltip>
        </template>
        <template v-else-if="column.key === 'domains'">
          <a-space :size="4" wrap>
            <a-tag v-for="fqdn in record.domains" :key="fqdn" color="blue">{{ fqdn }}</a-tag>
          </a-space>
        </template>
        <template v-else-if="column.key === 'redirectStatus'">
          <a-tag :color="REDIRECT_STATUS[record.redirectStatus as RedirectStatus].color">
            {{ REDIRECT_STATUS[record.redirectStatus as RedirectStatus].label }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="LINK_STATUS[record.status as LinkStatus].color">
            {{ LINK_STATUS[record.status as LinkStatus].label }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'visits'">
          <a class="visits-link" @click="goStats(record)">{{ record.visits }}</a>
        </template>
        <template v-else-if="column.key === 'createdAt'">
          {{ formatDateTime(record.createdAt) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-button size="small" @click="openEdit(record)">编辑</a-button>
            <a-button
              v-if="record.status === 'enabled'"
              size="small"
              danger
              @click="onToggleStatus(record)"
            >
              停用
            </a-button>
            <a-button v-else size="small" type="primary" ghost @click="onToggleStatus(record)">
              启用
            </a-button>
            <a-button size="small" type="text" danger @click="onDelete(record)">删除</a-button>
            <a-popconfirm
              title="彻底删除将物理删除该短链及其全部访问记录,且不可恢复。确定继续?"
              ok-text="彻底删除"
              :ok-button-props="{ danger: true }"
              cancel-text="取消"
              @confirm="onPurge(record)"
            >
              <a-button size="small" type="text" danger>彻底删除</a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <LinkFormModal
      v-model:open="modalOpen"
      :link="editingLink"
      :domains="domains"
      @saved="load"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Modal, message } from 'ant-design-vue';
import { PlusOutlined } from '@ant-design/icons-vue';
import type { TableColumnsType, TablePaginationConfig } from 'ant-design-vue';

import { listDomains } from '@/api/domains';
import { deleteLink, listLinks, purgeLink, updateLink } from '@/api/links';
import { LINK_STATUS, REDIRECT_STATUS } from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type { Domain, Link, LinkStatus, RedirectStatus } from '@/types/api';
import { formatDateTime, truncateText } from '@/utils/format';

import LinkFormModal from './LinkFormModal.vue';

const auth = useAuthStore();
const router = useRouter();

const links = ref<Link[]>([]);
const domains = ref<Domain[]>([]);
const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(10);

const modalOpen = ref(false);
const editingLink = ref<Link | null>(null);

const quotaInfo = computed(() => {
  const usage = auth.tenant?.usage;
  if (!usage) return '';
  return `当前配额:短链 ${usage.links}/${usage.maxLinks} 条,自有域名 ${usage.domains}/${usage.maxDomains} 条。`;
});

const columns: TableColumnsType = [
  { title: '短码', key: 'code', dataIndex: 'code', width: 140 },
  { title: '目标 URL', key: 'targetUrl', dataIndex: 'targetUrl' },
  { title: '关联域名', key: 'domains', dataIndex: 'domains' },
  { title: '重定向', key: 'redirectStatus', dataIndex: 'redirectStatus', width: 130 },
  { title: '状态', key: 'status', dataIndex: 'status', width: 90 },
  { title: '访问数', key: 'visits', dataIndex: 'visits', width: 90 },
  { title: '创建时间', key: 'createdAt', dataIndex: 'createdAt', width: 170 },
  { title: '操作', key: 'action', width: 320 },
];

const pagination = computed<TablePaginationConfig>(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showSizeChanger: true,
  showTotal: (t) => `共 ${t} 条`,
}));

async function load() {
  loading.value = true;
  try {
    let result = await listLinks({ page: page.value, pageSize: pageSize.value });
    // 删除/彻底删除后当前页可能已空:自动回退到最后一页
    if (result.items.length === 0 && result.total > 0 && page.value > 1) {
      page.value = Math.max(1, Math.ceil(result.total / pageSize.value));
      result = await listLinks({ page: page.value, pageSize: pageSize.value });
    }
    links.value = result.items;
    total.value = result.total;
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  } finally {
    loading.value = false;
  }
}

async function loadDomains() {
  try {
    domains.value = await listDomains();
  } catch {
    // 域名列表加载失败不阻塞短链页
  }
}

onMounted(() => {
  load();
  loadDomains();
});

function onTableChange(p: TablePaginationConfig) {
  page.value = p.current ?? 1;
  pageSize.value = p.pageSize ?? 10;
  load();
}

function openCreate() {
  editingLink.value = null;
  modalOpen.value = true;
}

function openEdit(link: Link) {
  editingLink.value = link;
  modalOpen.value = true;
}

async function onToggleStatus(link: Link) {
  const next: 'enabled' | 'disabled' = link.status === 'enabled' ? 'disabled' : 'enabled';
  const label = next === 'disabled' ? '停用' : '启用';
  try {
    await updateLink(link.id, { status: next });
    message.success(`短链「${link.code}」已${label}`);
    await load();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('操作失败,请稍后重试');
  }
}

function onDelete(link: Link) {
  Modal.confirm({
    title: `删除短链「${link.code}」?`,
    content: '删除为逻辑删除:记录、关联与访问信息保留,但「域名/短码」将不再命中。',
    okText: '删除',
    cancelText: '取消',
    onOk: async () => {
      try {
        await deleteLink(link.id);
        message.success('短链已删除(逻辑删除)');
        await load();
      } catch (error) {
        if (error instanceof ApiError) message.error(error.message);
        else message.error('删除失败,请稍后重试');
      }
    },
  });
}

async function onPurge(link: Link) {
  try {
    await purgeLink(link.id);
    message.success('短链已彻底删除');
    await load();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('彻底删除失败,请稍后重试');
  }
}

function goStats(link: Link) {
  router.push({ path: '/stats', query: { linkId: String(link.id) } });
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

.quota-alert {
  margin-bottom: 16px;
}

.visits-link {
  cursor: pointer;
  font-weight: 600;
}
</style>
