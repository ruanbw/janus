<template>
  <div>
    <PageHeader
      title="添加自有域名"
      description="将自有域名的 DNS 指向本服务器后,系统会自动进行 DNS 校验并签发证书"
    >
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回
        </AppButton>
      </template>
    </PageHeader>

    <div class="max-w-[640px]">
      <AppForm
        ref="formRef"
        :model="formState as unknown as Record<string, unknown>"
        :rules="createRules"
      >
        <AppFormItem name="fqdn" label="域名" :extra="fqdnExtra">
          <AppInput
            v-model="formState.fqdn"
            placeholder="例如 links.example.com"
            @press-enter="onSubmit"
          />
        </AppFormItem>
        <AppFormItem
          name="description"
          label="描述"
          extra="可选,用于备注该域名的用途,便于在列表中区分;最长 200 字"
        >
          <AppTextarea
            v-model="formState.description"
            placeholder="例如:生产环境主站,用于产品文档"
            :maxlength="200"
            :rows="2"
            show-count
          />
        </AppFormItem>
        <AppAlert
          class="mb-5"
          type="warning"
          show-icon
          message="添加前请确认 DNS 已指向本服务器,否则域名将停留在「待激活」并在 72 小时后标记为「校验失败」。"
        />
        <AppButton type="primary" :loading="submitting" @click="onSubmit">添加</AppButton>
      </AppForm>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ArrowLeft } from '@lucide/vue';

import { createDomain } from '@/api/domains';
import PageHeader from '@/components/PageHeader.vue';
import type { FormRule } from '@/components/ui/types';
import { message } from '@/utils/toast';
import { useAuthStore } from '@/stores/auth';
import { ApiError, getQuotaUsage } from '@/types/api';

const router = useRouter();
const auth = useAuthStore();

const formRef = ref();
const submitting = ref(false);
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

const createRules: Record<string, FormRule[]> = {
  fqdn: [
    { required: true, message: '请输入域名' },
    {
      validator: (_rule, value: unknown) => {
        if (!value) return Promise.resolve();
        return isValidFQDN(value as string)
          ? Promise.resolve()
          : Promise.reject(new Error('域名格式非法:仅支持字母、数字、连字符,标签不能以连字符开头或结尾,如 links.example.com'));
      },
    },
  ],
  description: [{ max: 200, message: '描述最多 200 字' }],
};

/** 域名项配置说明:指向地址、格式规则、示例、自动校验与证书流程 */
const fqdnExtra = computed(() => {
  const ip = auth.config?.serverIp;
  return [
    '需先将该域名的 A/AAAA 记录指向本服务器' + (ip ? '（IP：' + ip + '）' : '') + '。',
    '格式规则：仅支持字母、数字与连字符；以点分多个标签，每个标签不能以连字符开头或结尾；总长度不超过 253 字符。',
    '示例：links.example.com',
    '添加后系统会自动进行 DNS 校验并签发证书；72 小时内未通过校验将标记为「校验失败」。',
  ].join('\n');
});

function goBack() {
  router.push({ name: 'domains' });
}

async function onSubmit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }
  const fqdn = formState.fqdn.trim().toLowerCase();
  const description = formState.description.trim();
  submitting.value = true;
  try {
    const domain = await createDomain({ fqdn, description });
    message.success('域名 ' + domain.fqdn + ' 已添加,正在等待 DNS 校验');
    router.push({ name: 'domains' });
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        const usage = getQuotaUsage(error.details);
        if (usage) {
          message.error('域名配额超限:自有域名 ' + usage.domains + '/' + usage.maxDomains + ' 已达上限');
        } else {
          message.error('域名配额超限:' + error.message);
        }
      } else if (error.status === 409) {
        message.error('域名已被占用:' + error.message);
      } else if (error.status === 400) {
        message.error('域名不合法:' + error.message);
      } else {
        message.error(error.message);
      }
    } else {
      message.error('添加失败,请稍后重试');
    }
  } finally {
    submitting.value = false;
  }
}
</script>

