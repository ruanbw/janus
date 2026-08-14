<template>
  <div>
    <PageHeader title="域名" description="管理自有域名与平台默认域名;DNS 指向本服务器后自动校验并签发证书">
      <template #actions>
        <a-button type="primary" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          添加自有域名
        </a-button>
      </template>
    </PageHeader>

    <QuotaBar :domains-used="usage?.domains" :domains-max="usage?.maxDomains" />

    <a-table
      :columns="columns"
      :data-source="domains"
      :loading="loading"
      row-key="id"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'fqdn'">
          <a-typography-text copyable class="fqdn mono">{{ record.fqdn }}</a-typography-text>
        </template>
        <template v-else-if="column.key === 'description'">
          {{ record.description || '-' }}
        </template>
        <template v-else-if="column.key === 'origin'">
          <a-tag :color="DOMAIN_ORIGIN[record.origin as DomainOrigin].color">
            {{ DOMAIN_ORIGIN[record.origin as DomainOrigin].label }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="DOMAIN_STATUS[record.status as DomainStatus].color">
            {{ DOMAIN_STATUS[record.status as DomainStatus].label }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'certStatus'">
          <a-tag :color="CERT_STATUS[record.certStatus as CertStatus].color">
            {{ CERT_STATUS[record.certStatus as CertStatus].label }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'activatedAt'">
          {{ formatDateTime(record.activatedAt) }}
        </template>
        <template v-else-if="column.key === 'createdAt'">
          {{ formatDateTime(record.createdAt) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space :size="4">
            <a-button size="small" type="text" @click="onRecheck(record)">
              <template #icon><SyncOutlined /></template>
              手动重检
            </a-button>
            <a-button
              v-if="record.status !== 'stopped'"
              size="small"
              type="text"
              danger
              @click="onToggleStatus(record, 'stopped')"
            >
              <template #icon><StopOutlined /></template>
              停用
            </a-button>
            <a-button v-else size="small" type="text" @click="onToggleStatus(record, 'active')">
              <template #icon><PlayCircleOutlined /></template>
              恢复
            </a-button>
            <a-button
              v-if="record.origin === 'self'"
              size="small"
              type="text"
              danger
              @click="onDelete(record)"
            >
              <template #icon><DeleteOutlined /></template>
              删除
            </a-button>
            <a-tooltip v-else title="平台默认域名不可删除,可停用">
              <a-button size="small" type="text" disabled>
                <template #icon><DeleteOutlined /></template>
                删除
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
      </template>
    </a-table>

    <!-- 添加自有域名 -->
    <a-modal
      v-model:open="createOpen"
      title="添加自有域名"
      :confirm-loading="creating"
      ok-text="添加"
      cancel-text="取消"
      @ok="onCreate"
    >
      <a-form ref="formRef" :model="formState" :rules="createRules" layout="vertical">
        <a-form-item
          name="fqdn"
          label="域名"
          :extra="'需先将该域名的 A/AAAA 记录指向本服务器' + (auth.config?.serverIp ? ' (IP: ' + auth.config.serverIp + ')' : '') + ',添加后系统会自动校验并签发证书。仅支持字母、数字与连字符,如 links.example.com'"
        >
          <a-input
            v-model:value="formState.fqdn"
            placeholder="例如 links.example.com"
            @press-enter="onCreate"
          />
        </a-form-item>
        <a-form-item
          name="description"
          label="描述"
          extra="可选,备注该域名的用途,便于在列表中区分"
        >
          <a-textarea
            v-model:value="formState.description"
            placeholder="例如:生产环境主站,用于产品文档"
            :maxlength="200"
            :rows="2"
            show-count
          />
        </a-form-item>
      </a-form>
      <a-alert
        type="warning"
        show-icon
        message="添加前请确认 DNS 已指向本服务器,否则域名将停留在「待激活」并在 72 小时后标记为「校验失败」。"
      />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { Modal, message } from 'ant-design-vue';
import {
  DeleteOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  StopOutlined,
  SyncOutlined,
} from '@ant-design/icons-vue';
import type { TableColumnsType } from 'ant-design-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { createDomain, deleteDomain, listDomains, recheckDomain, updateDomainStatus } from '@/api/domains';
import PageHeader from '@/components/PageHeader.vue';
import QuotaBar from '@/components/QuotaBar.vue';
import { CERT_STATUS, DOMAIN_ORIGIN, DOMAIN_STATUS } from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError, getQuotaUsage } from '@/types/api';
import type { CertStatus, Domain, DomainOrigin, DomainStatus } from '@/types/api';
import { formatDateTime } from '@/utils/format';

const auth = useAuthStore();

const domains = ref<Domain[]>([]);
const loading = ref(false);
const createOpen = ref(false);
const creating = ref(false);
const formRef = ref();
const formState = reactive({ fqdn: '', description: '' });

/** 域名格式校验(与后端 validFQDN 一致):点分标签,字母/数字/连字符,标签不以连字符开头结尾,总长 ≤253 */
function isValidFQDN(s: string): boolean {
  const value = s.endsWith('.') ? s.slice(0, -1) : s;
  if (!value || value.length > 253) return false;
  return value.split('.').every((label) => {
    if (!label || label.length > 63) return false;
    for (let i = 0; i < label.length; i++) {
      const c = label[i];
      const ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c === '-';
      if (!ok || (c === '-' && (i === 0 || i === label.length - 1))) return false;
    }
    return true;
  });
}

const createRules: Record<string, Rule[]> = {
  fqdn: [
    { required: true, message: '请输入域名' },
    {
      validator: (_rule, value: string) => {
        if (!value) return Promise.resolve();
        return isValidFQDN(value)
          ? Promise.resolve()
          : Promise.reject(new Error('域名格式非法:仅支持字母、数字、连字符,标签不能以连字符开头或结尾,如 links.example.com'));
      },
    },
  ],
  description: [{ max: 200, message: '描述最多 200 字' }],
};

const usage = computed(() => auth.config?.usage);

const columns: TableColumnsType = [
  { title: '域名', key: 'fqdn', dataIndex: 'fqdn' },
  { title: '描述', key: 'description', dataIndex: 'description', width: 200, ellipsis: true },
  { title: '来源', key: 'origin', dataIndex: 'origin', width: 140 },
  { title: '状态', key: 'status', dataIndex: 'status', width: 100 },
  { title: '证书', key: 'certStatus', dataIndex: 'certStatus', width: 100 },
  { title: '激活时间', key: 'activatedAt', dataIndex: 'activatedAt', width: 200 },
  { title: '创建时间', key: 'createdAt', dataIndex: 'createdAt', width: 200 },
  { title: '操作', key: 'action', width: 300 },
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
  formState.fqdn = '';
  formState.description = '';
  formRef.value?.clearValidate();
  createOpen.value = true;
}

async function onCreate() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }
  const fqdn = formState.fqdn.trim().toLowerCase();
  const description = formState.description.trim();
  creating.value = true;
  try {
    const domain = await createDomain({ fqdn, description });
    message.success(`域名 ${domain.fqdn} 已添加,正在等待 DNS 校验`);
    createOpen.value = false;
    await load();
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        const usage = getQuotaUsage(error.details);
        if (usage) {
          message.error(`域名配额超限:自有域名 ${usage.domains}/${usage.maxDomains} 已达上限`);
        } else {
          message.error(`域名配额超限:${error.message}`);
        }
      } else if (error.status === 409) {
        message.error(`域名已被占用:${error.message}`);
      } else if (error.status === 400) {
        message.error(`域名不合法:${error.message}`);
      } else {
        message.error(error.message);
      }
    } else {
      message.error('添加失败,请稍后重试');
    }
  } finally {
    creating.value = false;
  }
}

async function onRecheck(domain: Domain) {
  try {
    await recheckDomain(domain.id);
    message.success(`已提交 ${domain.fqdn} 的重新校验,稍后自动刷新状态`);
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('操作失败,请稍后重试');
  }
}

async function onToggleStatus(domain: Domain, status: DomainStatus) {
  const label = status === 'stopped' ? '停用' : '恢复';
  Modal.confirm({
    title: `${label}域名 ${domain.fqdn}?`,
    content:
      status === 'stopped'
        ? '停用后,该域名下的所有短码将立即未命中(404)。'
        : '恢复后,该域名下的短链将重新可访问(自有域名恢复前会重新校验 DNS)。',
    okText: label,
    okButtonProps: status === 'stopped' ? { danger: true } : undefined,
    cancelText: '取消',
    onOk: async () => {
      try {
        await updateDomainStatus(domain.id, status);
        message.success(`域名已${label}`);
        await load();
      } catch (error) {
        if (error instanceof ApiError) message.error(error.message);
        else message.error('操作失败,请稍后重试');
      }
    },
  });
}

function onDelete(domain: Domain) {
  Modal.confirm({
    title: `删除域名 ${domain.fqdn}?`,
    content: '删除为物理删除。若该域名下仍有关联的未删除短链,将被拒绝(409);请先清空关联。',
    okText: '删除',
    okButtonProps: { danger: true },
    cancelText: '取消',
    onOk: async () => {
      try {
        await deleteDomain(domain.id);
        message.success('域名已删除');
        await load();
      } catch (error) {
        if (error instanceof ApiError) {
          if (error.status === 409) {
            message.error(`无法删除:${error.message}(请先移除该域名下关联的未删除短链)`);
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

<style scoped>
.fqdn {
  font-size: 13px;
}
</style>
