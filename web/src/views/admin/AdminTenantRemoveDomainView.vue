<template>
  <div>
    <PageHeader
      title="移除违规域名"
      description="从平台侧强制移除租户的违规域名,并解除其上的短链关联"
    >
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回
        </AppButton>
      </template>
    </PageHeader>

    <AppForm
      :model="form"
      :rules="rules"
      class="max-w-[640px]"
      @finish="onSubmit"
    >
      <AppFormItem
        label="租户邮箱"
        extra="本次要移除违规域名的租户,仅用于确认操作对象,不可修改"
      >
        <span class="mono">{{ tenant?.email ?? '—' }}</span>
      </AppFormItem>

      <AppFormItem
        name="domainId"
        label="域名 ID"
        extra="契约暂未提供超管域名列表端点,请从数据库或后端日志获取域名 ID。平台强删将解除该域名上的短链关联,请确认该域名确为违规后再提交。"
      >
        <AppInputNumber
          v-model="form.domainId"
          :min="1"
          :precision="0"
          placeholder="请输入域名 ID"
        />
      </AppFormItem>

      <AppAlert
        type="warning"
        show-icon
        message="平台强删将解除该域名上的短链关联,请确认该域名确为违规。"
      />

      <AppFormItem>
        <AppSpace>
          <AppButton type="primary" danger html-type="submit" :loading="submitting" :disabled="loading">
            移除
          </AppButton>
          <AppButton @click="goBack">取消</AppButton>
        </AppSpace>
      </AppFormItem>
    </AppForm>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft } from '@lucide/vue';

import { getTenant, removeDomain } from '@/api/admin';
import PageHeader from '@/components/PageHeader.vue';
import type { FormRule } from '@/components/ui/types';
import { ApiError } from '@/types/api';
import type { Tenant } from '@/types/api';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();

const tenantId = Number(route.params.id);

const tenant = ref<Tenant | null>(null);
const loading = ref(true);
const submitting = ref(false);

const form = reactive<{ domainId: number | undefined }>({ domainId: undefined });

const rules: Record<string, FormRule[]> = {
  domainId: [
    { required: true, message: '请输入域名 ID' },
    {
      validator: (_rule, value) => {
        if (value === undefined || value === null) return Promise.resolve();
        const v = value as number;
        return Number.isInteger(v) && v >= 1
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
