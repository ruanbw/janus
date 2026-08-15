<template>
  <div>
    <PageHeader
      title="调整等级"
      description="修改租户等级将立即调整其短链与自有域名的配额上限,请确认后再保存"
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
      class="tier-form"
      @finish="onSubmit"
    >
      <a-form-item
        label="租户邮箱"
        extra="本次要调整等级的租户,仅用于确认操作对象,不可修改"
      >
        <span class="mono">{{ tenant?.email ?? '—' }}</span>
      </a-form-item>

      <a-form-item
        label="当前等级"
        extra="该租户当前生效的等级,保存后将被下方所选等级替换"
      >
        <a-tag v-if="tenant" color="cyan">{{ tenant.tier?.name ?? '-' }}</a-tag>
        <span v-else>—</span>
      </a-form-item>

      <a-form-item
        name="tierId"
        label="调整后等级"
        extra="等级决定租户的配额上限(短链与自有域名数量),调整后立即生效。若租户现有用量已超过新等级上限,后续新增短链/域名将被拒绝,已有资源不受影响。"
      >
        <a-select
          v-model:value="form.tierId"
          :options="tierOptions"
          placeholder="请选择等级"
          :loading="loading"
        />
      </a-form-item>

      <a-alert
        v-if="selectedTier"
        type="info"
        show-icon
        :message="`该等级配额:短链 ${selectedTier.maxLinks} 条 / 自有域名 ${selectedTier.maxDomains} 个。调整后立即生效,若现有用量超过新上限,后续新增将被拒绝。`"
      />

      <a-form-item>
        <a-space>
          <a-button type="primary" html-type="submit" :loading="submitting" :disabled="loading">
            保存
          </a-button>
          <a-button @click="goBack">取消</a-button>
        </a-space>
      </a-form-item>
    </a-form>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { ArrowLeftOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { getTenant, listTiers, updateTenant } from '@/api/admin';
import PageHeader from '@/components/PageHeader.vue';
import { ApiError } from '@/types/api';
import type { Tenant, Tier } from '@/types/api';

const route = useRoute();
const router = useRouter();

const tenantId = Number(route.params.id);

const tenant = ref<Tenant | null>(null);
const tiers = ref<Tier[]>([]);
const loading = ref(true);
const submitting = ref(false);

const form = reactive<{ tierId: number | undefined }>({ tierId: undefined });

const rules: Record<string, Rule[]> = {
  tierId: [{ required: true, message: '请选择调整后的等级' }],
};

/** 等级选项:直接使用平台全部等级,避免只显示“已有租户占用”的等级 */
const tierOptions = computed(() =>
  tiers.value.map((tier) => ({
    value: tier.id,
    label: `${tier.name}(短链 ${tier.maxLinks} / 域名 ${tier.maxDomains})`,
    maxLinks: tier.maxLinks,
    maxDomains: tier.maxDomains,
  })),
);

const selectedTier = computed(() =>
  tierOptions.value.find((o) => o.value === form.tierId),
);

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
    message.success('等级已调整');
    router.push({ name: 'admin-tenants' });
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('操作失败,请稍后重试');
  } finally {
    submitting.value = false;
  }
}
</script>

<style scoped>
.tier-form {
  max-width: 640px;
}
</style>
