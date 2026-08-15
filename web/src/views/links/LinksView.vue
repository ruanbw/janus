<template>
  <div>
    <PageHeader title="短链" description="创建与管理短链;同一短码可在不同域名下指向不同目标">
      <template #actions>
        <a-button type="primary" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          创建短链
        </a-button>
      </template>
    </PageHeader>

    <QuotaBar :links-used="usage?.links" :links-max="usage?.maxLinks" />

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
          <div class="code-cell">
            <div class="domain-links">
              <a-tooltip
                v-for="fqdn in record.domains"
                :key="fqdn"
                title="点击复制"
              >
                <span class="domain-link mono" @click="copyShortLink(record, fqdn)">
                  https://{{ fqdn }}/{{ record.code }}
                </span>
              </a-tooltip>
            </div>
          </div>
        </template>
        <template v-else-if="column.key === 'linkType'">
          <a-tag :color="LINK_TYPE[record.linkType as LinkType].color">
            {{ LINK_TYPE[record.linkType as LinkType].label }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'targetUrls'">
          <a-tooltip v-if="record.targetUrls.length > 0">
            <template #title>
              <div v-for="(url, i) in record.targetUrls" :key="i">{{ url }}</div>
            </template>
            <span class="target-cell">
              <LinkOutlined class="target-icon" />
              {{ truncateText(record.targetUrls[0], 40) }}
              <span v-if="record.targetUrls.length > 1" class="target-more">
                +{{ record.targetUrls.length - 1 }}
              </span>
            </span>
          </a-tooltip>
          <span v-else class="target-cell">-</span>
        </template>
        <template v-else-if="column.key === 'landing'">
          <template v-if="record.linkType === 'landing'">
            <a-tooltip
              v-if="record.landingSource === 'url'"
              :title="record.landingUrl || ''"
            >
              <span class="landing-cell">
                {{ record.landingUrl ? truncateText(record.landingUrl, 28) : '-' }}
              </span>
            </a-tooltip>
            <a-tag v-else :color="record.landingUploaded ? 'success' : 'default'">
              {{ record.landingUploaded ? '已上传' : '未上传' }}
            </a-tag>
          </template>
          <span v-else class="landing-cell">—</span>
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
          <a class="visits-link" @click="goStats(record)">
            {{ record.visits }}
            <EyeOutlined class="visits-icon" />
          </a>
        </template>
        <template v-else-if="column.key === 'clicks'">
          <span>{{ record.linkType === 'landing' ? record.clicks : '—' }}</span>
        </template>
        <template v-else-if="column.key === 'createdAt'">
          {{ formatDateTime(record.createdAt) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space :size="4">
            <a-button size="small" type="text" @click="openEdit(record)">
              <template #icon><EditOutlined /></template>
              编辑
            </a-button>
            <a-button
              v-if="record.status === 'enabled'"
              size="small"
              type="text"
              danger
              @click="onToggleStatus(record)"
            >
              <template #icon><StopOutlined /></template>
              停用
            </a-button>
            <a-button v-else size="small" type="text" @click="onToggleStatus(record)">
              <template #icon><PlayCircleOutlined /></template>
              启用
            </a-button>
            <a-button size="small" type="text" danger @click="onDelete(record)">
              <template #icon><DeleteOutlined /></template>
              删除
            </a-button>
            <a-popconfirm
              title="彻底删除将物理删除该短链及其全部访问记录,且不可恢复。确定继续?"
              ok-text="彻底删除"
              :ok-button-props="{ danger: true }"
              cancel-text="取消"
              @confirm="onPurge(record)"
            >
              <a-button size="small" type="text" danger>
                <template #icon><ExclamationCircleOutlined /></template>
                彻底删除
              </a-button>
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
import {
  DeleteOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  EyeOutlined,
  LinkOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  StopOutlined,
} from '@ant-design/icons-vue';
import type { TableColumnsType, TablePaginationConfig } from 'ant-design-vue';

import { listDomains } from '@/api/domains';
import { deleteLink, listLinks, purgeLink, updateLink } from '@/api/links';
import PageHeader from '@/components/PageHeader.vue';
import QuotaBar from '@/components/QuotaBar.vue';
import { LINK_STATUS, LINK_TYPE, REDIRECT_STATUS } from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type { Domain, Link, LinkStatus, LinkType, RedirectStatus } from '@/types/api';
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

const usage = computed(() => auth.config?.usage);

const columns: TableColumnsType = [
  { title: '链接', key: 'code', dataIndex: 'code', width: 240 },
  { title: '类型', key: 'linkType', dataIndex: 'linkType', width: 80 },
  { title: '目标 URL', key: 'targetUrls', dataIndex: 'targetUrls' },
  { title: '落地页', key: 'landing', width: 170 },
  { title: '重定向', key: 'redirectStatus', dataIndex: 'redirectStatus', width: 130 },
  { title: '状态', key: 'status', dataIndex: 'status', width: 90 },
  { title: '访问数', key: 'visits', dataIndex: 'visits', width: 90 },
  { title: '点击', key: 'clicks', dataIndex: 'clicks', width: 80 },
  { title: '创建时间', key: 'createdAt', dataIndex: 'createdAt', width: 200 },
  { title: '操作', key: 'action', width: 330 },
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

/** 复制"域名/短码"的完整短链(https://<域名>/<短码>)到剪贴板 */
async function copyShortLink(link: Link, fqdn: string) {
  const url = `https://${fqdn}/${link.code}`;
  try {
    await navigator.clipboard.writeText(url);
    message.success(`已复制:${url}`);
  } catch {
    message.error('复制失败,请手动选择复制');
  }
}
</script>

<style scoped>
.code-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}

.domain-links {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
}

.domain-link {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: #1677ff;
  cursor: pointer;
}

.domain-link:hover {
  text-decoration: underline;
}

.target-cell {
  color: #334155;
}

.target-icon {
  margin-right: 4px;
  color: #8b98a5;
  font-size: 12px;
}

.landing-cell {
  color: #334155;
  word-break: break-all;
}

.target-more {
  margin-left: 6px;
  padding: 0 6px;
  font-size: 12px;
  line-height: 18px;
  color: #1677ff;
  background: rgba(22, 119, 255, 0.1);
  border-radius: 9px;
  white-space: nowrap;
}

.visits-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  font-weight: 600;
}

.visits-icon {
  font-size: 12px;
  color: #8b98a5;
}
</style>
