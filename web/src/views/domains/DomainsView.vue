<template>
  <div>
    <PageHeader title="域名" description="管理自有域名与平台默认域名;DNS 指向本服务器后自动校验并签发证书">
      <template #actions>
        <AppButton type="primary" @click="openCreate">
          <template #icon><Plus :size="15" /></template>
          添加自有域名
        </AppButton>
      </template>
    </PageHeader>

    <QuotaBar :domains-used="usage?.domains" :domains-max="usage?.maxDomains" />

    <AppTable
      :columns="columns"
      :data-source="tableData"
      :loading="loading"
      row-key="id"
      :pagination="false"
      :scroll="{ x: 1390 }"
    >
      <template #cell="{ column, record }">
        <template v-if="column.key === 'fqdn'">
          <CopyText :text="toDomain(record).fqdn">
            <span class="mono text-[13px]">{{ toDomain(record).fqdn }}</span>
          </CopyText>
        </template>
        <template v-else-if="column.key === 'description'">
          {{ toDomain(record).description || '-' }}
        </template>
        <template v-else-if="column.key === 'origin'">
          <AppTag :color="DOMAIN_ORIGIN[toDomain(record).origin].color">
            {{ DOMAIN_ORIGIN[toDomain(record).origin].label }}
          </AppTag>
        </template>
        <template v-else-if="column.key === 'status'">
          <AppTag :color="DOMAIN_STATUS[toDomain(record).status].color">
            {{ DOMAIN_STATUS[toDomain(record).status].label }}
          </AppTag>
        </template>
        <template v-else-if="column.key === 'certStatus'">
          <AppTag :color="CERT_STATUS[toDomain(record).certStatus].color">
            {{ CERT_STATUS[toDomain(record).certStatus].label }}
          </AppTag>
        </template>
        <template v-else-if="column.key === 'activatedAt'">
          <span class="whitespace-nowrap">{{ formatDateTime(toDomain(record).activatedAt) }}</span>
        </template>
        <template v-else-if="column.key === 'createdAt'">
          <span class="whitespace-nowrap">{{ formatDateTime(toDomain(record).createdAt) }}</span>
        </template>
        <template v-else-if="column.key === 'action'">
          <div class="flex items-center gap-1 whitespace-nowrap">
            <AppButton size="small" type="text" @click="onRecheck(toDomain(record))">
              <template #icon><RefreshCw :size="13" /></template>
              手动重检
            </AppButton>
            <AppButton
              v-if="toDomain(record).status !== 'stopped'"
              size="small"
              type="text"
              danger
              @click="onToggleStatus(toDomain(record), 'stopped')"
            >
              <template #icon><CircleStop :size="13" /></template>
              停用
            </AppButton>
            <AppButton v-else size="small" type="text" @click="onToggleStatus(toDomain(record), 'active')">
              <template #icon><CirclePlay :size="13" /></template>
              恢复
            </AppButton>
            <AppButton
              v-if="toDomain(record).origin === 'self'"
              size="small"
              type="text"
              danger
              @click="onDelete(toDomain(record))"
            >
              <template #icon><Trash2 :size="13" /></template>
              删除
            </AppButton>
            <AppTooltip v-else title="平台默认域名不可删除,可停用">
              <AppButton size="small" type="text" disabled>
                <template #icon><Trash2 :size="13" /></template>
                删除
              </AppButton>
            </AppTooltip>
          </div>
        </template>
      </template>
    </AppTable>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { CirclePlay, CircleStop, Plus, RefreshCw, Trash2 } from '@lucide/vue';

import { deleteDomain, listDomains, recheckDomain, updateDomainStatus } from '@/api/domains';
import PageHeader from '@/components/PageHeader.vue';
import QuotaBar from '@/components/QuotaBar.vue';
import { confirm } from '@/components/ui/confirm';
import type { TableColumn } from '@/components/ui/types';
import { message } from '@/utils/toast';
import { CERT_STATUS, DOMAIN_ORIGIN, DOMAIN_STATUS } from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type { Domain, DomainStatus } from '@/types/api';
import { formatDateTime } from '@/utils/format';

const router = useRouter();
const auth = useAuthStore();

const domains = ref<Domain[]>([]);
const loading = ref(false);

const usage = computed(() => auth.config?.usage);

/** AppTable 槽位 record 为 Record<string, unknown>,转换为领域类型以访问字段 */
function toDomain(r: Record<string, unknown>): Domain {
  return r as unknown as Domain;
}

const tableData = computed(() => domains.value as unknown as Record<string, unknown>[]);

const columns: TableColumn[] = [
  { title: '域名', key: 'fqdn', dataIndex: 'fqdn', width: 220, nowrap: true },
  { title: '描述', key: 'description', dataIndex: 'description', width: 200, ellipsis: true },
  { title: '来源', key: 'origin', dataIndex: 'origin', width: 130, nowrap: true },
  { title: '状态', key: 'status', dataIndex: 'status', width: 100, nowrap: true },
  { title: '证书', key: 'certStatus', dataIndex: 'certStatus', width: 100, nowrap: true },
  { title: '激活时间', key: 'activatedAt', dataIndex: 'activatedAt', width: 180, nowrap: true },
  { title: '创建时间', key: 'createdAt', dataIndex: 'createdAt', width: 180, nowrap: true },
  { title: '操作', key: 'action', width: 280, nowrap: true },
];

let timer: number | undefined;

async function load() {
  loading.value = true;
  try {
    domains.value = await listDomains();
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
  // 轮询刷新:观察 DNS 校验与证书签发状态变化
  timer = window.setInterval(load, 10_000);
});

onUnmounted(() => {
  if (timer) window.clearInterval(timer);
});

function openCreate() {
  router.push({ name: 'domain-create' });
}

async function onRecheck(domain: Domain) {
  try {
    await recheckDomain(domain.id);
    message.success('已提交 ' + domain.fqdn + ' 的重新校验,稍后自动刷新状态');
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('操作失败,请稍后重试');
  }
}

async function onToggleStatus(domain: Domain, status: DomainStatus) {
  const label = status === 'stopped' ? '停用' : '恢复';
  confirm({
    title: label + '域名 ' + domain.fqdn + '?',
    content:
      status === 'stopped'
        ? '停用后,该域名下的所有短码将立即未命中(404)。'
        : '恢复后,该域名下的短链将重新可访问(自有域名恢复前会重新校验 DNS)。',
    okText: label,
    danger: status === 'stopped',
    cancelText: '取消',
    onOk: async () => {
      try {
        await updateDomainStatus(domain.id, status);
        message.success('域名已' + label);
        await load();
      } catch (error) {
        if (error instanceof ApiError) message.error(error.message);
        else message.error('操作失败,请稍后重试');
      }
    },
  });
}

function onDelete(domain: Domain) {
  confirm({
    title: '删除域名 ' + domain.fqdn + '?',
    content: '删除为物理删除。若该域名下仍有关联的未删除短链,将被拒绝(409);请先清空关联。',
    okText: '删除',
    danger: true,
    cancelText: '取消',
    onOk: async () => {
      try {
        await deleteDomain(domain.id);
        message.success('域名已删除');
        await load();
      } catch (error) {
        if (error instanceof ApiError) {
          if (error.status === 409) {
            message.error('无法删除:' + error.message + '(请先移除该域名下关联的未删除短链)');
          } else {
            message.error(error.message);
          }
        } else {
          message.error('删除失败,请稍后重试');
        }
      }
    },
  });
}
</script>
