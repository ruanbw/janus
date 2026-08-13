<template>
  <div>
    <h2 class="page-title">账号设置</h2>

    <a-alert
      v-if="auth.tenant?.firstLoginSetup"
      class="setup-alert"
      type="warning"
      show-icon
      message="平台管理员账号尚未设置密码,请先设置初始密码后再使用后台。"
    />

    <a-row :gutter="16">
      <!-- 租户信息与配额 -->
      <a-col :xs="24" :md="12">
        <a-card title="租户信息" :bordered="false" size="small">
          <a-descriptions :column="1" size="small">
            <a-descriptions-item label="邮箱">{{ auth.tenant?.email }}</a-descriptions-item>
            <a-descriptions-item label="前缀">{{ auth.tenant?.slug }}</a-descriptions-item>
            <a-descriptions-item label="等级">
              <a-tag color="blue">{{ auth.tenant?.tier?.name ?? '-' }}</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="平台默认域名">
              <a-typography-text copyable>{{ auth.tenant?.defaultDomain }}</a-typography-text>
            </a-descriptions-item>
            <a-descriptions-item label="状态">
              <a-tag :color="TENANT_STATUS[auth.tenant?.status ?? 'pending'].color">
                {{ TENANT_STATUS[auth.tenant?.status ?? 'pending'].label }}
              </a-tag>
            </a-descriptions-item>
          </a-descriptions>

          <a-divider style="margin: 12px 0">配额用量</a-divider>
          <div class="quota-item">
            <div class="quota-label">
              短链:{{ usage?.links ?? 0 }} / {{ usage?.maxLinks ?? '-' }}
            </div>
            <a-progress
              :percent="linkPercent"
              :status="linkPercent >= 100 ? 'exception' : 'active'"
            />
          </div>
          <div class="quota-item">
            <div class="quota-label">
              自有域名:{{ usage?.domains ?? 0 }} / {{ usage?.maxDomains ?? '-' }}
              <span class="quota-note">(平台默认域名不计入)</span>
            </div>
            <a-progress
              :percent="domainPercent"
              :status="domainPercent >= 100 ? 'exception' : 'active'"
            />
          </div>

          <a-divider style="margin: 12px 0">自动生成短码长度</a-divider>
          <a-space>
            <a-input-number v-model:value="codeLength" :min="4" :max="32" />
            <a-button :loading="savingCodeLength" @click="onSaveCodeLength">保存</a-button>
          </a-space>
          <div class="quota-note">后端约束:长度须在 4-32 之间</div>
        </a-card>
      </a-col>

      <!-- 修改密码 -->
      <a-col :xs="24" :md="12">
        <a-card title="修改密码" :bordered="false" size="small">
          <a-form :model="form" :rules="rules" layout="vertical" @finish="onChangePassword">
            <a-form-item v-if="!auth.tenant?.firstLoginSetup" label="当前密码" name="oldPassword">
              <a-input-password
                v-model:value="form.oldPassword"
                placeholder="请输入当前密码"
                autocomplete="current-password"
              />
            </a-form-item>
            <a-form-item label="新密码" name="newPassword">
              <a-input-password
                v-model:value="form.newPassword"
                placeholder="至少 8 位"
                autocomplete="new-password"
              />
            </a-form-item>
            <a-form-item label="确认新密码" name="confirmPassword">
              <a-input-password
                v-model:value="form.confirmPassword"
                placeholder="再次输入新密码"
                autocomplete="new-password"
              />
            </a-form-item>
            <a-form-item>
              <a-button type="primary" html-type="submit" :loading="changingPassword">
                {{ auth.tenant?.firstLoginSetup ? '设置初始密码' : '修改密码' }}
              </a-button>
            </a-form-item>
          </a-form>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { changePassword } from '@/api/auth';
import { fetchMyTenant, updateMyTenant } from '@/api/me';
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

const rules: Record<string, Rule[]> = {
  oldPassword: [{ required: true, message: '请输入当前密码' }],
  newPassword: [
    { required: true, message: '请输入新密码' },
    { min: 8, message: '密码至少 8 位' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (_rule, value: string) => {
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

<style scoped>
.page-title {
  margin: 0 0 16px;
  font-size: 18px;
}

.setup-alert {
  margin-bottom: 16px;
}

.quota-item {
  margin-bottom: 8px;
}

.quota-label {
  margin-bottom: 4px;
  font-size: 13px;
  color: rgba(0, 0, 0, 0.65);
}

.quota-note {
  color: rgba(0, 0, 0, 0.45);
}
</style>
