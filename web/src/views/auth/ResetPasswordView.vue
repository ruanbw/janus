<template>
  <AuthShell :show-heading="!done">
    <template #title>重置密码</template>
    <template #subtitle>设置你的新密码</template>

    <a-result v-if="done" status="success" title="密码已重置" sub-title="请使用新密码登录">
      <template #extra>
        <a-button type="primary" @click="router.push('/login')">前往登录</a-button>
      </template>
    </a-result>

    <a-form v-else :model="form" :rules="rules" layout="vertical" @finish="onSubmit">
      <a-form-item label="新密码" name="newPassword">
        <a-input-password
          v-model:value="form.newPassword"
          placeholder="至少 8 位"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><LockOutlined class="input-icon" /></template>
        </a-input-password>
      </a-form-item>
      <a-form-item label="确认新密码" name="confirmPassword">
        <a-input-password
          v-model:value="form.confirmPassword"
          placeholder="再次输入新密码"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><LockOutlined class="input-icon" /></template>
        </a-input-password>
      </a-form-item>
      <a-button type="primary" html-type="submit" block size="large" :loading="submitting">
        重置密码
      </a-button>
    </a-form>
  </AuthShell>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { LockOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { resetPassword } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import { ApiError } from '@/types/api';

const route = useRoute();
const router = useRouter();

const submitting = ref(false);
const done = ref(false);
const form = reactive({ newPassword: '', confirmPassword: '' });

const rules: Record<string, Rule[]> = {
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

async function onSubmit() {
  const token = (route.query.token as string) || '';
  if (!token) {
    message.error('缺少重置令牌,请从邮件中的链接进入');
    return;
  }
  submitting.value = true;
  try {
    await resetPassword({ token, newPassword: form.newPassword });
    done.value = true;
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('重置失败,请稍后重试');
  } finally {
    submitting.value = false;
  }
}
</script>

<style scoped>
.input-icon {
  color: #8b98a5;
}
</style>
