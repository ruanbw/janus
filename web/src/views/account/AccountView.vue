<template>
  <div>
    <PageHeader title="账号设置" description="租户信息、配额用量与安全设置" />

    <AppAlert
      v-if="auth.tenant?.firstLoginSetup"
      class="mb-4"
      type="warning"
      show-icon
      message="平台管理员账号尚未设置密码,请先设置初始密码后再使用后台。"
    />

    <div class="grid gap-4 md:grid-cols-2">
      <!-- 租户信息与配额 -->
      <AppCard title="租户信息" :bordered="false" size="small">
        <AppDescriptions :column="1">
          <AppDescriptionsItem label="邮箱">{{ auth.tenant?.email }}</AppDescriptionsItem>
          <AppDescriptionsItem label="前缀">{{ auth.tenant?.slug }}</AppDescriptionsItem>
          <AppDescriptionsItem label="等级">
            <AppTag color="cyan">{{ auth.tenant?.tier?.name ?? '-' }}</AppTag>
          </AppDescriptionsItem>
          <AppDescriptionsItem label="平台默认域名">
            <CopyText :text="auth.tenant?.defaultDomain ?? ''" class="mono text-[13px]">
              {{ auth.tenant?.defaultDomain ?? '-' }}
            </CopyText>
          </AppDescriptionsItem>
          <AppDescriptionsItem label="状态">
            <AppTag :color="TENANT_STATUS[auth.tenant?.status ?? 'pending'].color">
              {{ TENANT_STATUS[auth.tenant?.status ?? 'pending'].label }}
            </AppTag>
          </AppDescriptionsItem>
        </AppDescriptions>

        <AppDivider :style="{ margin: '16px 0 12px' }">配额用量</AppDivider>
        <div class="mb-3.5">
          <div class="mb-1.5 flex items-center justify-between text-[13px] text-ink-soft">
            <span>短链</span>
            <span class="font-semibold text-ink tabular-nums">
              {{ usage?.links ?? 0 }} / {{ usage?.maxLinks ?? '-' }}
            </span>
          </div>
          <AppProgress
            :percent="linkPercent"
            :status="linkPercent >= 100 ? 'exception' : 'active'"
            :stroke-width="8"
          />
        </div>
        <div class="mb-3.5">
          <div class="mb-1.5 flex items-center justify-between text-[13px] text-ink-soft">
            <span>自有域名</span>
            <span class="font-semibold text-ink tabular-nums">
              {{ usage?.domains ?? 0 }} / {{ usage?.maxDomains ?? '-' }}
            </span>
          </div>
          <AppProgress
            :percent="domainPercent"
            :status="domainPercent >= 100 ? 'exception' : 'active'"
            :stroke-width="8"
          />
        </div>
        <p class="mt-1 text-xs text-ink-faint">平台默认域名不计入域名配额</p>

        <AppDivider :style="{ margin: '16px 0 12px' }">自动生成短码长度</AppDivider>
        <div class="flex items-center gap-2">
          <div class="w-28">
            <AppInputNumber v-model="codeLength" :min="4" :max="32" />
          </div>
          <AppButton :loading="savingCodeLength" @click="onSaveCodeLength">保存</AppButton>
        </div>
        <p class="mt-1.5 text-xs text-ink-faint">后端约束:长度须在 4-32 之间</p>
      </AppCard>

      <!-- 修改密码 -->
      <AppCard title="修改密码" :bordered="false" size="small">
        <AppForm :model="form" :rules="rules" @finish="onChangePassword">
          <AppFormItem v-if="!auth.tenant?.firstLoginSetup" label="当前密码" name="oldPassword">
            <AppInput
              v-model="form.oldPassword"
              type="password"
              placeholder="请输入当前密码"
              autocomplete="current-password"
            />
          </AppFormItem>
          <AppFormItem label="新密码" name="newPassword">
            <AppInput
              v-model="form.newPassword"
              type="password"
              placeholder="至少 8 位"
              autocomplete="new-password"
            />
          </AppFormItem>
          <AppFormItem label="确认新密码" name="confirmPassword">
            <AppInput
              v-model="form.confirmPassword"
              type="password"
              placeholder="再次输入新密码"
              autocomplete="new-password"
            />
          </AppFormItem>
          <AppFormItem>
            <AppButton type="primary" html-type="submit" :loading="changingPassword">
              {{ auth.tenant?.firstLoginSetup ? '设置初始密码' : '修改密码' }}
            </AppButton>
          </AppFormItem>
        </AppForm>
      </AppCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from '@/utils/toast';
import type { FormRule } from '@/components/ui/types';

import { changePassword } from '@/api/auth';
import { fetchMyTenant, updateMyTenant } from '@/api/me';
import PageHeader from '@/components/PageHeader.vue';
import { TENANT_STATUS } from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type { QuotaUsage } from '@/types/api';

const auth = useAuthStore();

const usage = ref<QuotaUsage | undefined>(undefined);
const codeLength = ref<number | undefined>(undefined);
const savingCodeLength = ref(false);

const changingPassword = ref(false);
const form = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' });

const linkPercent = computed(() => {
  if (!usage.value || !usage.value.maxLinks) return 0;
  return Math.round((usage.value.links / usage.value.maxLinks) * 100);
});

const domainPercent = computed(() => {
  if (!usage.value || !usage.value.maxDomains) return 0;
  return Math.round((usage.value.domains / usage.value.maxDomains) * 100);
});

const rules: Record<string, FormRule[]> = {
  oldPassword: [{ required: true, message: '请输入当前密码' }],
  newPassword: [
    { required: true, message: '请输入新密码' },
    { min: 8, message: '密码至少 8 位' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (_rule, value) => {
        if (!value || value === form.newPassword) return Promise.resolve();
        return Promise.reject(new Error('两次输入的密码不一致'));
      },
    },
  ],
};

async function load() {
  try {
    const me = await fetchMyTenant();
    usage.value = me.usage;
    codeLength.value = me.codeLength;
    auth.tenant = me;
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  }
}

onMounted(load);

async function onSaveCodeLength() {
  const value = codeLength.value;
  if (!value || value < 4 || value > 32) {
    message.warning('自动生成短码长度须在 4-32 之间');
    return;
  }
  savingCodeLength.value = true;
  try {
    const me = await updateMyTenant({ codeLength: value });
    auth.tenant = me;
    message.success('自动生成短码长度已更新');
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('保存失败,请稍后重试');
  } finally {
    savingCodeLength.value = false;
  }
}

async function onChangePassword() {
  changingPassword.value = true;
  try {
    await changePassword({
      // 超管首次登录(尚无密码)省略 oldPassword
      oldPassword: auth.tenant?.firstLoginSetup ? undefined : form.oldPassword,
      newPassword: form.newPassword,
    });
    message.success(auth.tenant?.firstLoginSetup ? '初始密码已设置' : '密码已修改');
    form.oldPassword = '';
    form.newPassword = '';
    form.confirmPassword = '';
    if (auth.tenant?.firstLoginSetup) {
      // 设置完成后刷新租户信息,消除首次登录标记
      await load();
    }
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('修改失败,请稍后重试');
  } finally {
    changingPassword.value = false;
  }
}
</script>
