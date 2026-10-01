<template>
  <div>
    <PageHeader
      title="移除违规域名"
      description="从平台侧强制注销并解绑指定违规域名，立即物理删除记录并解除所有短链绑定"
    >
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回租户列表
        </AppButton>
      </template>
    </PageHeader>

    <AppSpin :spinning="loading">
      <!-- 带有警告/危险语义的 AppCard -->
      <AppCard :padding="false" class="border-err/40 shadow-xs">
        <CardHeader class="border-b border-err/10 bg-err/10 pb-4">
          <CardTitle class="flex items-center gap-2 text-err">
            <ShieldAlert :size="20" />
            强制移除违规域名
          </CardTitle>
          <CardDescription class="text-ink-soft">
            此操作仅供平台管理员处置恶意、侵权或违规域名，执行后不可撤销
          </CardDescription>
        </CardHeader>

        <CardContent class="space-y-6 p-6">
          <!-- 目标租户信息栏 -->
          <div class="rounded-xl border border-line bg-surface-muted/50 p-4 space-y-2.5">
            <div class="flex items-center justify-between text-xs">
              <span class="font-medium text-ink-soft">目标租户邮箱</span>
              <span class="mono font-semibold text-ink">{{ tenant?.email ?? '—' }}</span>
            </div>
            <div class="flex items-center justify-between text-xs">
              <span class="font-medium text-ink-soft">租户前缀 / 等级</span>
              <span class="text-ink">
                <code class="font-mono">{{ tenant?.slug ?? '—' }}</code>
                <span class="text-ink-faint mx-1.5">·</span>
                <AppTag color="info">{{ tenant?.tier?.name ?? '—' }}</AppTag>
              </span>
            </div>
          </div>

          <!-- 域名 ID 输入表单 -->
          <AppForm ref="formRef" :model="form" :rules="rules" @finish="onFormSubmit">
            <AppFormItem
              name="domainId"
              label="违规域名 ID"
              extra="请输入要强删的域名数字 ID（可从数据库或后端运行日志中获取，须为 ≥ 1 的正整数）。"
            >
              <AppInputNumber
                v-model="form.domainId"
                :min="1"
                :precision="0"
                placeholder="请输入要移除的域名 ID"
                class="w-full"
              />
            </AppFormItem>
          </AppForm>

          <!-- 强力破坏性警示 Alert -->
          <AppAlert
            type="error"
            variant="destructive"
            show-icon
            title="不可撤销的破坏性操作警示"
            message="平台强删为物理硬删除：将立即永久删除该域名记录，并即刻解除其上所有关联短链的域名绑定。原有以此域名访问的短链将即刻失效！请再次确认域名 ID 与租户对应关系。"
          />
        </CardContent>

        <!-- CardFooter 操作按钮 -->
        <CardFooter class="flex items-center justify-between border-t border-line bg-surface-muted/30 px-6 py-4">
          <span class="text-xs text-ink-faint">
            需要二次安全确认
          </span>
          <div class="flex items-center gap-3">
            <AppButton @click="goBack">取消</AppButton>
            <AppButton
              type="primary"
              danger
              :loading="submitting"
              :disabled="loading || form.domainId === undefined"
              @click="onRemoveClick"
            >
              <template #icon><Trash2 :size="15" /></template>
              强制移除违规域名
            </AppButton>
          </div>
        </CardFooter>
      </AppCard>
    </AppSpin>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft, ShieldAlert, Trash2 } from '@lucide/vue';

import { getTenant, removeDomain } from '@/api/admin';
import PageHeader from '@/components/PageHeader.vue';
import { confirmAsync } from '@/components/app/confirm';
import type { FormRule } from '@/components/app/types';
import { ApiError } from '@/types/api';
import type { Tenant } from '@/types/api';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();

const tenantId = Number(route.params.id);

const tenant = ref<Tenant | null>(null);
const loading = ref(true);
const submitting = ref(false);
const formRef = ref();

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

/** 点击强制移除按钮:先校验表单，然后弹出破坏性二次确认框 */
async function onRemoveClick() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  if (form.domainId === undefined) {
    message.warning('请输入域名 ID');
    return;
  }

  const ok = await confirmAsync({
    title: `确认强制移除域名 ID #${form.domainId}？`,
    content: `该操作不可撤销！将物理删除该域名，并同时解除租户「${tenant.value?.email ?? '该租户'}」所有绑定在此域名上的短链关联。`,
    okText: '确认强制删除',
    cancelText: '取消',
    danger: true,
  });

  if (!ok) return;

  await executeRemove();
}

async function onFormSubmit() {
  await onRemoveClick();
}

async function executeRemove() {
  if (form.domainId === undefined) return;
  submitting.value = true;
  try {
    await removeDomain(form.domainId);
    message.success(`域名 #${form.domainId} 已被强制移除并解绑`);
    router.push({ name: 'admin-tenants' });
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('移除失败，请稍后重试');
  } finally {
    submitting.value = false;
  }
}
</script>
