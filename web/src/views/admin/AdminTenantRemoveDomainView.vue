<template>
  <div>
    <PageHeader
      title="移除违规域名"
      description="从平台侧强制移除租户的违规域名,并解除其上的短链关联"
    >
      <template #actions>
        <a-button @click="goBack">
          <template #icon><ArrowLeftOutlined /></template>
          返回
        </a-button>
      </template>
    </PageHeader>

    <a-form
      :model="form"
      :rules="rules"
      layout="vertical"
      class="remove-form"
      @finish="onSubmit"
    >
      <a-form-item
        label="租户邮箱"
        extra="本次要移除违规域名的租户,仅用于确认操作对象,不可修改"
      >
        <span class="mono">{{ tenant?.email ?? '—' }}</span>
      </a-form-item>

      <a-form-item
        name="domainId"
        label="域名 ID"
        extra="契约暂未提供超管域名列表端点,请从数据库或后端日志获取域名 ID。平台强删将解除该域名上的短链关联,请确认该域名确为违规后再提交。"
      >
        <a-input-number
          v-model:value="form.domainId"
          :min="1"
          :precision="0"
          style="width: 100%"
          placeholder="请输入域名 ID"
        />
      </a-form-item>

      <a-alert
        type="warning"
        show-icon
        message="平台强删将解除该域名上的短链关联,请确认该域名确为违规。"
      />

      <a-form-item>
        <a-space>
          <a-button type="primary" danger html-type="submit" :loading="submitting" :disabled="loading">
            移除
          </a-button>
          <a-button @click="goBack">取消</a-button>
        </a-space>
      </a-form-item>
    </a-form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { ArrowLeftOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { getTenant, removeDomain } from '@/api/admin';
import PageHeader from '@/components/PageHeader.vue';
import { ApiError } from '@/types/api';
import type { Tenant } from '@/types/api';

const route = useRoute();
const router = useRouter();

const tenantId = Number(route.params.id);

const tenant = ref<Tenant | null>(null);
const loading = ref(true);
const submitting = ref(false);

const form = reactive<{ domainId: number | undefined }>({ domainId: undefined });

const rules: Record<string, Rule[]> = {
  domainId: [
    { required: true, message: '请输入域名 ID' },
    {
      validator: (_rule, value: number | undefined) => {
        if (value === undefined || value === null) return Promise.resolve();
        return Number.isInteger(value) && value >= 1
          ? Promise.resolve()
          : Promise.reject(new Error('域名 ID 须为不小于 1 的整数'));
      },
    },
  ],
};

function goBack() {
  router.push({ name: 'admin-tenants' });
}

async function load() {
  loading.value = true;
  try {
    tenant.value = await getTenant(tenantId);
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        message.error('仅平台管理员可访问该页面');
      } else if (error.status === 404) {
        message.error('租户不存在');
        goBack();
      } else if (error.status !== 401) {
        message.error(error.message);
      }
    }
  } finally {
    loading.value = false;
  }
}

onMounted(load);

async function onSubmit() {
  if (form.domainId === undefined) {
    message.warning('请输入域名 ID');
    return;
  }
  submitting.value = true;
  try {
    await removeDomain(form.domainId);
    message.success('违规域名已移除');
    router.push({ name: 'admin-tenants' });
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('移除失败,请稍后重试');
  } finally {
    submitting.value = false;
  }
}
</script>

<style scoped>
.remove-form {
  max-width: 640px;
}
</style>
