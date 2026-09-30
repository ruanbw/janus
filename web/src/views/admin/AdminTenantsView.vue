<template>
  <div>
    <PageHeader
      title="平台管理"
      description="查看所有租户、封禁/解封账号、调整等级,并移除违规域名"
    >
      <template #actions>
        <AppButton :loading="loading" @click="load">
          <template #icon><RefreshCw :size="15" /></template>
          刷新
        </AppButton>
      </template>
    </PageHeader>

    <AppTable
      :columns="columns"
      :data-source="tenants"
      :loading="loading"
      row-key="id"
      :pagination="false"
      :scroll="{ x: 1220 }"
    >
      <template #cell="{ column, record }">
        <template v-if="column.key === 'email'">
          <div class="flex min-w-0 items-center gap-2">
            <AppTooltip :title="String(record.email)">
              <span class="truncate font-medium text-ink">{{ record.email }}</span>
            </AppTooltip>
            <AppTag v-if="record.isSuperAdmin" color="gold" class="shrink-0">平台管理员</AppTag>
          </div>
        </template>
        <template v-else-if="column.key === 'status'">
          <AppTag :color="TENANT_STATUS[record.status as TenantStatus]?.color || 'default'">
            {{ TENANT_STATUS[record.status as TenantStatus]?.label || record.status || '-' }}
          </AppTag>
        </template>
        <template v-else-if="column.key === 'tier'">
          <AppTag color="cyan">{{ record.tier?.name ?? '-' }}</AppTag>
        </template>
        <template v-else-if="column.key === 'usage'">
          <span class="tabular-nums text-ink-soft">
            短链 {{ record.usage?.links ?? 0 }}/{{ record.usage?.maxLinks ?? '-' }} ·
            域名 {{ record.usage?.domains ?? 0 }}/{{ record.usage?.maxDomains ?? '-' }}
          </span>
        </template>
        <template v-else-if="column.key === 'createdAt'">
          <span class="whitespace-nowrap">{{ formatDateTime(record.createdAt as string) }}</span>
        </template>
        <template v-else-if="column.key === 'action'">
          <div class="flex items-center gap-1 whitespace-nowrap">
            <AppTooltip v-if="record.isSuperAdmin" title="平台管理员账号不可封禁">
              <AppButton size="small" danger disabled>
                <template #icon><CircleStop :size="13" /></template>
                封禁
              </AppButton>
            </AppTooltip>
            <AppButton
              v-else-if="record.status === 'active'"
              size="small"
              danger
              @click="onToggleBan(record as Tenant, 'banned')"
            >
              <template #icon><CircleStop :size="13" /></template>
              封禁
            </AppButton>
            <AppButton
              v-else-if="record.status === 'banned'"
              size="small"
              type="ghost"
              @click="onToggleBan(record as Tenant, 'active')"
            >
              <template #icon><CirclePlay :size="13" /></template>
              解封
            </AppButton>
            <AppButton size="small" @click="openTierModal(record as Tenant)">
              <template #icon><ArrowRightLeft :size="13" /></template>
              调整等级
            </AppButton>
            <AppButton size="small" type="text" danger @click="openRemoveDomain(record as Tenant)">
              <template #icon><Trash2 :size="13" /></template>
              移除域名
            </AppButton>
          </div>
        </template>
      </template>
    </AppTable>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ArrowRightLeft, CirclePlay, CircleStop, RefreshCw, Trash2 } from '@lucide/vue';

import { listTenants, updateTenant } from '@/api/admin';
import PageHeader from '@/components/PageHeader.vue';
import { confirm } from '@/components/ui/confirm';
import type { TableColumn } from '@/components/ui/types';
import { TENANT_STATUS } from '@/constants/dict';
import { ApiError } from '@/types/api';
import type { Tenant, TenantStatus } from '@/types/api';
import { formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';

const router = useRouter();

const tenants = ref<Tenant[]>([]);
const loading = ref(false);

const columns: TableColumn[] = [
  { title: '邮箱', key: 'email', dataIndex: 'email', minWidth: 200, ellipsis: true },
  { title: '前缀', key: 'slug', dataIndex: 'slug', width: 120, nowrap: true },
  { title: '状态', key: 'status', dataIndex: 'status', width: 100, nowrap: true },
  { title: '等级', key: 'tier', dataIndex: 'tier', width: 110, nowrap: true },
  { title: '用量(短链/域名)', key: 'usage', dataIndex: 'usage', width: 200, nowrap: true },
  { title: '注册时间', key: 'createdAt', dataIndex: 'createdAt', width: 180, nowrap: true },
  { title: '操作', key: 'action', width: 310, nowrap: true },
];

async function load() {
  loading.value = true;
  try {
    tenants.value = await listTenants();
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        message.error('仅平台管理员可访问该页面');
      } else if (error.status !== 401) {
        message.error(error.message);
      }
    }
  } finally {
    loading.value = false;
  }
}

onMounted(load);

function onToggleBan(record: Tenant, status: 'banned' | 'active') {
  const label = status === 'banned' ? '封禁' : '解封';
  confirm({
    title: label + '租户 ' + record.email + '?',
    content:
      status === 'banned'
        ? '封禁后该租户将无法登录,其域名下的短码将不再放行。'
        : '解封后该租户可恢复正常使用。',
    okText: label,
    danger: status === 'banned',
    cancelText: '取消',
    onOk: async () => {
      try {
        await updateTenant(record.id, { status });
        message.success('租户已' + label);
        await load();
      } catch (error) {
        if (error instanceof ApiError) message.error(error.message);
        else message.error('操作失败,请稍后重试');
      }
    },
  });
}

function openTierModal(record: Tenant) {
  router.push({ name: 'admin-tenant-tier', params: { id: String(record.id) } });
}

function openRemoveDomain(record: Tenant) {
  router.push({ name: 'admin-tenant-remove-domain', params: { id: String(record.id) } });
}
</script>
