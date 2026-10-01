<template>
  <div>
    <PageHeader
      title="调整租户等级"
      description="修改租户等级将立即调整其短链与自有域名的配额上限，请核对配额变动后再行保存"
    >
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回租户列表
        </AppButton>
      </template>
    </PageHeader>

    <AppSpin :spinning="loading">
      <AppCard :padding="false">
        <CardHeader>
          <CardTitle class="flex items-center gap-2">
            <Sliders :size="18" class="text-brand" />
            调整租户等级与配额
          </CardTitle>
          <CardDescription>
            等级决定该租户名下允许拥有的最大短链数与自有域名数
          </CardDescription>
        </CardHeader>

        <CardContent class="space-y-6">
          <!-- 目标租户信息卡片 -->
          <div class="rounded-xl border border-line bg-surface-muted/50 p-4 space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-ink-soft">目标租户账号</span>
              <AppTag color="info">
                {{ tenant?.tier?.name ? `当前等级: ${tenant.tier?.name}` : '未指定等级' }}
              </AppTag>
            </div>
            <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-1 text-xs">
              <span class="mono font-semibold text-ink text-sm">
                {{ tenant?.email ?? '—' }}
              </span>
              <span class="text-ink-faint">
                租户前缀标识: <code class="font-mono text-ink">{{ tenant?.slug ?? '-' }}</code>
              </span>
            </div>
          </div>

          <!-- 调整后等级表单项 -->
          <AppForm :model="form" :rules="rules" @finish="onSubmit">
            <AppFormItem
              name="tierId"
              label="调整为新等级"
              extra="等级变更立即生效。若租户现有资源已超过新等级上限，后续新增将被拒绝，已有资源不受影响。"
            >
              <AppSelect
                v-model="form.tierId"
                :options="tierOptions"
                placeholder="请选择目标等级"
                :loading="loading"
              />
            </AppFormItem>
          </AppForm>

          <!-- 动态配额变动对比卡片 -->
          <div v-if="selectedTier" class="space-y-3">
            <div class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-ink-soft">
              <TrendingUp :size="14" class="text-brand-600" />
              配额变动对比预览
            </div>

            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <!-- 短链配额对比卡 -->
              <div class="rounded-xl border border-line bg-surface-muted/40 p-4 space-y-2">
                <div class="flex items-center justify-between text-xs text-ink-soft">
                  <span class="font-medium">短链配额上限</span>
                  <AppTag :color="diffLinks > 0 ? 'ok' : diffLinks < 0 ? 'warn' : 'default'">
                    {{ diffLinks > 0 ? `+${diffLinks} 扩容` : diffLinks < 0 ? `${diffLinks} 缩减` : '无变化' }}
                  </AppTag>
                </div>
                <div class="flex items-baseline gap-2">
                  <span class="text-lg font-bold text-ink tabular-nums">
                    {{ currentTier?.maxLinks ?? 0 }}
                  </span>
                  <ArrowRight :size="14" class="text-ink-faint" />
                  <span class="text-xl font-bold text-brand tabular-nums">
                    {{ selectedTier.maxLinks }}
                  </span>
                  <span class="text-xs text-ink-faint">条短链</span>
                </div>
              </div>

              <!-- 域名配额对比卡 -->
              <div class="rounded-xl border border-line bg-surface-muted/40 p-4 space-y-2">
                <div class="flex items-center justify-between text-xs text-ink-soft">
                  <span class="font-medium">自有域名配额上限</span>
                  <AppTag :color="diffDomains > 0 ? 'ok' : diffDomains < 0 ? 'warn' : 'default'">
                    {{ diffDomains > 0 ? `+${diffDomains} 扩容` : diffDomains < 0 ? `${diffDomains} 缩减` : '无变化' }}
                  </AppTag>
                </div>
                <div class="flex items-baseline gap-2">
                  <span class="text-lg font-bold text-ink tabular-nums">
                    {{ currentTier?.maxDomains ?? 0 }}
                  </span>
                  <ArrowRight :size="14" class="text-ink-faint" />
                  <span class="text-xl font-bold text-brand tabular-nums">
                    {{ selectedTier.maxDomains }}
                  </span>
                  <span class="text-xs text-ink-faint">个域名</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 配额影响提示 Alert -->
          <AppAlert
            v-if="selectedTier"
            type="info"
            show-icon
            title="配额生效提示"
            :message="quotaMessage"
          />
        </CardContent>

        <!-- CardFooter 操作按钮 -->
        <CardFooter class="flex items-center justify-between border-t border-line bg-surface-muted/30 px-6 py-4">
          <span class="text-xs text-ink-faint">
            仅平台管理员具备调整权限
          </span>
          <div class="flex items-center gap-3">
            <AppButton @click="goBack">取消</AppButton>
            <AppButton
              type="primary"
              :loading="submitting"
              :disabled="loading || form.tierId === undefined"
              @click="onSubmit"
            >
              保存等级调整
            </AppButton>
          </div>
        </CardFooter>
      </AppCard>
    </AppSpin>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft, ArrowRight, Sliders, TrendingUp } from '@lucide/vue';

import { getTenant, listTiers, updateTenant } from '@/api/admin';
import PageHeader from '@/components/PageHeader.vue';
import type { FormRule } from '@/components/app/types';
import { ApiError } from '@/types/api';
import type { Tenant, Tier } from '@/types/api';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();

const tenantId = Number(route.params.id);

const tenant = ref<Tenant | null>(null);
const tiers = ref<Tier[]>([]);
const loading = ref(true);
const submitting = ref(false);

const form = reactive<{ tierId: number | undefined }>({ tierId: undefined });

const rules: Record<string, FormRule[]> = {
  tierId: [{ required: true, message: '请选择调整后的等级' }],
};

/** 等级选项:直接使用平台全部等级,避免只显示“已有租户占用”的等级 */
const tierOptions = computed(() =>
  tiers.value.map((tier) => ({
    value: tier.id,
    label: tier.name + ' (短链 ' + tier.maxLinks + ' 条 / 域名 ' + tier.maxDomains + ' 个)',
  })),
);

const currentTier = computed(() => tenant.value?.tier);
const selectedTier = computed(() =>
  tiers.value.find((o) => o.id === form.tierId),
);

const diffLinks = computed(() => {
  if (!currentTier.value || !selectedTier.value) return 0;
  return (selectedTier.value.maxLinks ?? 0) - (currentTier.value.maxLinks ?? 0);
});

const diffDomains = computed(() => {
  if (!currentTier.value || !selectedTier.value) return 0;
  return (selectedTier.value.maxDomains ?? 0) - (currentTier.value.maxDomains ?? 0);
});

const quotaMessage = computed(() => {
  const tier = selectedTier.value;
  if (!tier) return '';
  return (
    '该等级配额为:短链 ' +
    tier.maxLinks +
    ' 条、自有域名 ' +
    tier.maxDomains +
    ' 个。调整后立即生效，若租户现有资源超出上限，后续新增将被拒绝，已存在的短链与域名正常工作不受影响。'
  );
});

function goBack() {
  router.push({ name: 'admin-tenants' });
}

async function load() {
  loading.value = true;
  try {
    const [found, availableTiers] = await Promise.all([
      getTenant(tenantId),
      listTiers(),
    ]);
    tenant.value = found;
    tiers.value = availableTiers;
    form.tierId = found.tier?.id;
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
  if (form.tierId === undefined) {
    message.warning('请选择调整后的等级');
    return;
  }
  submitting.value = true;
  try {
    await updateTenant(tenantId, { tierId: form.tierId });
    message.success('租户等级已成功调整');
    router.push({ name: 'admin-tenants' });
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('操作失败，请稍后重试');
  } finally {
    submitting.value = false;
  }
}
</script>
