<template>
  <div>
    <PageHeader
      title="平台管理"
      description="查看所有租户、封禁/解封账号、调整等级,并移除违规域名"
    >
      <template #actions>
        <a-button :loading="loading" @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
      </template>
    </PageHeader>

    <!-- 概览统计 -->
    <div class="stat-strip">
      <div class="stat-card">
        <span class="stat-label">租户总数</span>
        <span class="stat-value">{{ tenants.length }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">正常</span>
        <span class="stat-value stat-ok">{{ statusCount('active') }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">待验证</span>
        <span class="stat-value stat-pending">{{ statusCount('pending') }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">已封禁</span>
        <span class="stat-value stat-banned">{{ statusCount('banned') }}</span>
      </div>
    </div>

    <a-table
      :columns="columns"
      :data-source="tenants"
      :loading="loading"
      row-key="id"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'email'">
          <a-space :size="8">
            <span>{{ record.email }}</span>
            <a-tag v-if="record.isSuperAdmin" color="gold">平台管理员</a-tag>
          </a-space>
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="TENANT_STATUS[record.status as TenantStatus].color">
            {{ TENANT_STATUS[record.status as TenantStatus].label }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'tier'">
          <a-tag color="cyan">{{ record.tier?.name ?? '-' }}</a-tag>
        </template>
        <template v-else-if="column.key === 'usage'">
          <span class="usage-text">
            短链 {{ record.usage?.links ?? 0 }}/{{ record.usage?.maxLinks ?? '-' }} ·
            域名 {{ record.usage?.domains ?? 0 }}/{{ record.usage?.maxDomains ?? '-' }}
          </span>
        </template>
        <template v-else-if="column.key === 'createdAt'">
          {{ formatDateTime(record.createdAt) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space :size="4">
            <a-tooltip v-if="record.isSuperAdmin" title="平台管理员账号不可封禁">
              <a-button size="small" danger disabled>
                <template #icon><StopOutlined /></template>
                封禁
              </a-button>
            </a-tooltip>
            <a-button
              v-else-if="record.status === 'active'"
              size="small"
              danger
              @click="onToggleBan(record, 'banned')"
            >
              <template #icon><StopOutlined /></template>
              封禁
            </a-button>
            <a-button
              v-else-if="record.status === 'banned'"
              size="small"
              type="primary"
              ghost
              @click="onToggleBan(record, 'active')"
            >
              <template #icon><PlayCircleOutlined /></template>
              解封
            </a-button>
            <a-button size="small" @click="openTierModal(record)">
              <template #icon><SwapOutlined /></template>
              调整等级
            </a-button>
            <a-button size="small" type="text" danger @click="openRemoveDomain(record)">
              <template #icon><DeleteOutlined /></template>
              移除域名
            </a-button>
          </a-space>
        </template>
      </template>
    </a-table>

    <!-- 调整等级 -->
    <a-modal
      v-model:open="tierModalOpen"
      title="调整等级"
      :confirm-loading="tierSubmitting"
      ok-text="保存"
      cancel-text="取消"
      @ok="onSaveTier"
    >
      <a-form layout="vertical">
        <a-form-item label="租户">
          <span class="mono">{{ tierTarget?.email }}</span>
        </a-form-item>
        <a-form-item label="等级" required>
          <a-select v-model:value="selectedTierId" :options="tierOptions" />
        </a-form-item>
        <a-alert
          v-if="selectedTier"
          type="info"
          show-icon
          :message="`该等级配额:短链 ${selectedTier.maxLinks} 条 / 自有域名 ${selectedTier.maxDomains} 个。调整后立即生效,若现有用量超过新上限,后续新增将被拒绝。`"
        />
      </a-form>
    </a-modal>

    <!-- 移除域名 -->
    <a-modal
      v-model:open="removeDomainOpen"
      title="移除违规域名"
      :confirm-loading="removeSubmitting"
      ok-text="移除"
      :ok-button-props="{ danger: true }"
      cancel-text="取消"
      @ok="onRemoveDomain"
    >
      <a-form layout="vertical">
        <a-form-item label="租户">
          <span class="mono">{{ removeTarget?.email }}</span>
        </a-form-item>
        <a-form-item label="域名 ID" required extra="契约暂未提供超管域名列表端点,请填写域名 ID(可从数据库或后端日志获取)">
          <a-input-number v-model:value="removeDomainId" :min="1" style="width: 100%" />
        </a-form-item>
        <a-alert
          type="warning"
          show-icon
          message="平台强删将解除该域名上的短链关联,请确认该域名确为违规。"
        />
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Modal, message } from 'ant-design-vue';
import {
  DeleteOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  StopOutlined,
  SwapOutlined,
} from '@ant-design/icons-vue';
import type { TableColumnsType } from 'ant-design-vue';

import { listTenants, removeDomain, updateTenant } from '@/api/admin';
import PageHeader from '@/components/PageHeader.vue';
import { TENANT_STATUS } from '@/constants/dict';
import { ApiError } from '@/types/api';
import type { Tenant, TenantStatus, Tier } from '@/types/api';
import { formatDateTime } from '@/utils/format';

const tenants = ref<Tenant[]>([]);
const loading = ref(false);

const tierModalOpen = ref(false);
const tierSubmitting = ref(false);
const tierTarget = ref<Tenant | null>(null);
const selectedTierId = ref<number | undefined>(undefined);

const removeDomainOpen = ref(false);
const removeSubmitting = ref(false);
const removeTarget = ref<Tenant | null>(null);
const removeDomainId = ref<number | undefined>(undefined);

function statusCount(status: TenantStatus): number {
  return tenants.value.filter((t) => t.status === status).length;
}

/** 等级选项:从租户列表收集去重(契约无独立等级列表端点) */
const tierOptions = computed(() => {
  const map = new Map<number, Tier>();
  for (const t of tenants.value) {
    if (t.tier) map.set(t.tier.id, t.tier);
  }
  return [...map.values()].map((tier) => ({
    value: tier.id,
    label: `${tier.name}(短链 ${tier.maxLinks} / 域名 ${tier.maxDomains})`,
    maxLinks: tier.maxLinks,
    maxDomains: tier.maxDomains,
  }));
});

const selectedTier = computed(() =>
  tierOptions.value.find((o) => o.value === selectedTierId.value),
);

const columns: TableColumnsType = [
  { title: '邮箱', key: 'email', dataIndex: 'email' },
  { title: '前缀', key: 'slug', dataIndex: 'slug', width: 120 },
  { title: '状态', key: 'status', dataIndex: 'status', width: 100 },
  { title: '等级', key: 'tier', dataIndex: 'tier', width: 110 },
  { title: '用量(短链/域名)', key: 'usage', dataIndex: 'usage', width: 180 },
  { title: '注册时间', key: 'createdAt', dataIndex: 'createdAt', width: 170 },
  { title: '操作', key: 'action', width: 300 },
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
  Modal.confirm({
    title: `${label}租户 ${record.email}?`,
    content:
      status === 'banned'
        ? '封禁后该租户将无法登录,其域名下的短码将不再放行。'
        : '解封后该租户可恢复正常使用。',
    okText: label,
    okButtonProps: status === 'banned' ? { danger: true } : undefined,
    cancelText: '取消',
    onOk: async () => {
      try {
        await updateTenant(record.id, { status });
        message.success(`租户已${label}`);
        await load();
      } catch (error) {
        if (error instanceof ApiError) message.error(error.message);
        else message.error('操作失败,请稍后重试');
      }
    },
  });
}

function openTierModal(record: Tenant) {
  tierTarget.value = record;
  selectedTierId.value = record.tier?.id;
  tierModalOpen.value = true;
}

async function onSaveTier() {
  if (!tierTarget.value || selectedTierId.value === undefined) {
    message.warning('请选择等级');
    return;
  }
  tierSubmitting.value = true;
  try {
    await updateTenant(tierTarget.value.id, { tierId: selectedTierId.value });
    message.success('等级已调整');
    tierModalOpen.value = false;
    await load();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('操作失败,请稍后重试');
  } finally {
    tierSubmitting.value = false;
  }
}

function openRemoveDomain(record: Tenant) {
  removeTarget.value = record;
  removeDomainId.value = undefined;
  removeDomainOpen.value = true;
}

async function onRemoveDomain() {
  if (!removeDomainId.value) {
    message.warning('请输入域名 ID');
    return;
  }
  removeSubmitting.value = true;
  try {
    await removeDomain(removeDomainId.value);
    message.success('违规域名已移除');
    removeDomainOpen.value = false;
    await load();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('移除失败,请稍后重试');
  } finally {
    removeSubmitting.value = false;
  }
}
</script>

<style scoped>
.stat-strip {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  margin-bottom: 20px;
}

@media (min-width: 992px) {
  .stat-strip {
    grid-template-columns: repeat(4, 1fr);
  }
}

.stat-card {
  background: #f8fafc;
  border: 1px solid #e6ebf1;
  border-radius: 10px;
  padding: 14px 18px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.stat-label {
  font-size: 12px;
  color: #8b98a5;
}

.stat-value {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
}

.stat-ok {
  color: #16a34a;
}

.stat-pending {
  color: #d97706;
}

.stat-banned {
  color: #dc2626;
}

.usage-text {
  font-variant-numeric: tabular-nums;
  color: #334155;
}
</style>
