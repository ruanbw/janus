<template>
  <div>
    <PageHeader title="短链" description="创建与管理短链;同一短码可在不同域名下指向不同目标">
      <template #actions>
        <AppButton type="primary" @click="openCreate">
          <template #icon><Plus :size="15" /></template>
          创建短链
        </AppButton>
      </template>
    </PageHeader>

    <QuotaBar :links-used="usage?.links" :links-max="usage?.maxLinks" />

    <AppTable
      :columns="columns"
      :data-source="links"
      :loading="loading"
      row-key="id"
      :pagination="pagination"
      :scroll="{ x: 1600 }"
      @change="onTableChange"
    >
      <template #cell="{ column, record }">
        <template v-if="column.key === 'code'">
          <div class="flex min-w-0 flex-col items-start gap-0.5">
            <AppTooltip
              v-for="fqdn in record.domains"
              :key="fqdn"
              title="点击复制"
            >
              <span
                class="mono inline-block max-w-full cursor-pointer truncate text-xs text-brand-600 hover:underline dark:text-brand-400"
                @click="copyShortLink(record, fqdn)"
              >
                https://{{ fqdn }}/{{ record.code }}
              </span>
            </AppTooltip>
          </div>
        </template>
        <template v-else-if="column.key === 'linkType'">
          <AppTag :color="LINK_TYPE[record.linkType as LinkType].color">
            {{ LINK_TYPE[record.linkType as LinkType].label }}
          </AppTag>
        </template>
        <template v-else-if="column.key === 'targetUrls'">
          <AppTooltip v-if="record.targetUrls.length > 0" placement="top">
            <template #title>
              <div v-for="(url, i) in record.targetUrls" :key="i" class="max-w-xs break-all">{{ url }}</div>
            </template>
            <span class="inline-flex min-w-0 max-w-full items-center gap-1.5 text-[13px] text-ink">
              <Link2 :size="13" class="shrink-0 text-ink-faint" />
              <span class="truncate">{{ truncateText(record.targetUrls[0], 40) }}</span>
              <span
                v-if="record.targetUrls.length > 1"
                class="shrink-0 rounded-md bg-brand-50 px-1.5 py-0.5 text-xs font-medium text-brand-600 dark:bg-brand-500/15 dark:text-brand-300"
              >
                +{{ record.targetUrls.length - 1 }}
              </span>
            </span>
          </AppTooltip>
          <span v-else class="text-ink-faint">-</span>
        </template>
        <template v-else-if="column.key === 'landing'">
          <template v-if="record.linkType === 'landing'">
            <AppTooltip
              v-if="record.landingSource === 'url'"
              :title="record.landingUrl || ''"
            >
              <span class="block max-w-full truncate text-[13px] text-ink-soft">
                {{ record.landingUrl ? truncateText(record.landingUrl, 28) : '-' }}
              </span>
            </AppTooltip>
            <AppTag v-else :color="record.landingUploaded ? 'success' : 'default'">
              {{ record.landingUploaded ? '已上传' : '未上传' }}
            </AppTag>
          </template>
          <span v-else class="text-ink-faint">—</span>
        </template>
        <template v-else-if="column.key === 'redirectStatus'">
          <AppTag :color="REDIRECT_STATUS[record.redirectStatus as RedirectStatus].color">
            {{ REDIRECT_STATUS[record.redirectStatus as RedirectStatus].label }}
          </AppTag>
        </template>
        <template v-else-if="column.key === 'status'">
          <AppTag :color="LINK_STATUS[record.status as LinkStatus].color">
            {{ LINK_STATUS[record.status as LinkStatus].label }}
          </AppTag>
        </template>
        <template v-else-if="column.key === 'visits'">
          <span
            class="inline-flex cursor-pointer items-center gap-1.5 font-semibold text-brand-600 hover:underline dark:text-brand-400"
            @click="goStats(record)"
          >
            {{ record.visits }}
            <Eye :size="13" class="text-ink-faint" />
          </span>
        </template>
        <template v-else-if="column.key === 'clicks'">
          <span>{{ record.linkType === 'landing' ? record.clicks : '—' }}</span>
        </template>
        <template v-else-if="column.key === 'createdAt'">
          <span class="whitespace-nowrap">{{ formatDateTime(record.createdAt) }}</span>
        </template>
        <template v-else-if="column.key === 'action'">
          <div class="flex items-center gap-1 whitespace-nowrap">
            <AppButton size="small" type="text" @click="openEdit(record)">
              <template #icon><Pencil :size="13" /></template>
              编辑
            </AppButton>
            <AppButton
              v-if="record.status === 'enabled'"
              size="small"
              type="text"
              danger
              @click="onToggleStatus(record)"
            >
              <template #icon><CircleStop :size="13" /></template>
              停用
            </AppButton>
            <AppButton v-else size="small" type="text" @click="onToggleStatus(record)">
              <template #icon><CirclePlay :size="13" /></template>
              启用
            </AppButton>
            <AppButton size="small" type="text" danger @click="onDelete(record)">
              <template #icon><Trash2 :size="13" /></template>
              删除
            </AppButton>
            <AppPopconfirm
              title="彻底删除将物理删除该短链及其全部访问记录,且不可恢复。确定继续?"
              ok-text="彻底删除"
              cancel-text="取消"
              danger
              @confirm="onPurge(record)"
            >
              <AppButton size="small" type="text" danger>
                <template #icon><CircleAlert :size="13" /></template>
                彻底删除
              </AppButton>
            </AppPopconfirm>
          </div>
        </template>
      </template>
    </AppTable>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { CircleAlert, CirclePlay, CircleStop, Eye, Link2, Pencil, Plus, Trash2 } from '@lucide/vue';
import type { TableColumn, TablePaginationConfig } from '@/components/ui/types';

import { deleteLink, listLinks, purgeLink, updateLink } from '@/api/links';
import PageHeader from '@/components/PageHeader.vue';
import QuotaBar from '@/components/QuotaBar.vue';
import { confirm } from '@/components/ui/confirm';
import { LINK_STATUS, LINK_TYPE, REDIRECT_STATUS } from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type { Link, LinkStatus, LinkType, RedirectStatus } from '@/types/api';
import { formatDateTime, truncateText } from '@/utils/format';
import { message } from '@/utils/toast';

const auth = useAuthStore();
const router = useRouter();

const links = ref<Link[]>([]);
const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(10);

const usage = computed(() => auth.config?.usage);

const columns: TableColumn[] = [
  { title: '链接', key: 'code', dataIndex: 'code', width: 240, nowrap: true },
  { title: '类型', key: 'linkType', dataIndex: 'linkType', width: 90, nowrap: true },
  { title: '目标 URL', key: 'targetUrls', dataIndex: 'targetUrls', ellipsis: true },
  { title: '落地页', key: 'landing', width: 170, ellipsis: true },
  { title: '重定向', key: 'redirectStatus', dataIndex: 'redirectStatus', width: 130, nowrap: true },
  { title: '状态', key: 'status', dataIndex: 'status', width: 90, nowrap: true },
  { title: '访问数', key: 'visits', dataIndex: 'visits', width: 90, nowrap: true },
  { title: '点击', key: 'clicks', dataIndex: 'clicks', width: 80, nowrap: true },
  { title: '创建时间', key: 'createdAt', dataIndex: 'createdAt', width: 190, nowrap: true },
  { title: '操作', key: 'action', width: 330, nowrap: true },
];

const pagination = computed<TablePaginationConfig>(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showSizeChanger: true,
  showTotal: (t) => '共 ' + t + ' 条',
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

onMounted(() => {
  load();
});

function onTableChange(p: TablePaginationConfig) {
  page.value = p.current ?? 1;
  pageSize.value = p.pageSize ?? 10;
  load();
}

function openCreate() {
  router.push({ name: 'link-create' });
}

function openEdit(link: Link) {
  router.push({ name: 'link-edit', params: { id: String(link.id) } });
}

async function onToggleStatus(link: Link) {
  const next: 'enabled' | 'disabled' = link.status === 'enabled' ? 'disabled' : 'enabled';
  const label = next === 'disabled' ? '停用' : '启用';
  try {
    await updateLink(link.id, { status: next });
    message.success('短链「' + link.code + '」已' + label);
    await load();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('操作失败,请稍后重试');
  }
}

function onDelete(link: Link) {
  confirm({
    title: '删除短链「' + link.code + '」?',
    content: '删除为逻辑删除:记录、关联与访问信息保留,但「域名/短码」将不再命中。',
    okText: '删除',
    cancelText: '取消',
    danger: true,
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
  const url = 'https://' + fqdn + '/' + link.code;
  try {
    await navigator.clipboard.writeText(url);
    message.success('已复制:' + url);
  } catch {
    message.error('复制失败,请手动选择复制');
  }
}
</script>
