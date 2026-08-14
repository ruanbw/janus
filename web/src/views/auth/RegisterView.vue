<template>
  <AuthShell>
    <template #title>注册新租户</template>
    <template #subtitle>
      注册后将自动获得平台默认域名
      <span class="slug-chip mono">{{ slugHint }}</span>
    </template>

    <a-form :model="form" :rules="rules" layout="vertical" @finish="onSubmit">
      <a-form-item label="邮箱" name="email">
        <a-input
          v-model:value="form.email"
          placeholder="you@example.com"
          autocomplete="email"
          size="large"
        >
          <template #prefix><MailOutlined class="input-icon" /></template>
        </a-input>
      </a-form-item>
      <a-form-item label="密码" name="password">
        <a-input-password
          v-model:value="form.password"
          placeholder="至少 8 位"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><LockOutlined class="input-icon" /></template>
        </a-input-password>
      </a-form-item>
      <a-form-item label="确认密码" name="confirmPassword">
        <a-input-password
          v-model:value="form.confirmPassword"
          placeholder="再次输入密码"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><LockOutlined class="input-icon" /></template>
        </a-input-password>
      </a-form-item>
      <a-form-item
        label="前缀"
        name="slug"
        extra="小写字母或数字开头/结尾,可包含连字符;将用于生成你的平台默认域名"
      >
        <a-input
          v-model:value="form.slug"
          placeholder="例如 mybrand"
          :maxlength="SLUG_MAX_LENGTH"
          size="large"
        >
          <template #prefix><GlobalOutlined class="input-icon" /></template>
        </a-input>
      </a-form-item>
      <a-button type="primary" html-type="submit" block size="large" :loading="submitting">
        注册
      </a-button>
    </a-form>

    <div class="auth-footer-links">
      已有账号?
      <router-link to="/login">直接登录</router-link>
    </div>
  </AuthShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { GlobalOutlined, LockOutlined, MailOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { register } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import { SLUG_MAX_LENGTH, SLUG_PATTERN } from '@/constants/dict';
import { ApiError } from '@/types/api';

const router = useRouter();
const submitting = ref(false);

const PLATFORM_DOMAIN = import.meta.env.VITE_PLATFORM_DOMAIN || 'cloak.test';

const form = reactive({
  email: '',
  password: '',
  confirmPassword: '',
  slug: '',
});

const slugHint = computed(() => {
  const slug = form.slug.trim();
  return slug ? `${slug}.${PLATFORM_DOMAIN}` : `<前缀>.${PLATFORM_DOMAIN}`;
});

const rules: Record<string, Rule[]> = {
  email: [
    { required: true, message: '请输入邮箱' },
    { type: 'email', message: '邮箱格式不正确' },
  ],
  password: [
    { required: true, message: '请输入密码' },
    { min: 8, message: '密码至少 8 位' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入密码' },
    {
      validator: (_rule, value: string) => {
        if (!value || value === form.password) return Promise.resolve();
        return Promise.reject(new Error('两次输入的密码不一致'));
      },
    },
  ],
  slug: [
    { required: true, message: '请输入前缀' },
    { pattern: SLUG_PATTERN, message: '仅允许小写字母、数字与连字符,且须以字母或数字开头/结尾' },
  ],
};

async function onSubmit() {
  submitting.value = true;
  try {
    await register({
      email: form.email.trim(),
      password: form.password,
      slug: form.slug.trim(),
    });
    message.success('注册成功,请查收验证邮件');
    router.push({ path: '/verify-email', query: { sent: '1' } });
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 409) {
        message.error(error.message || '邮箱或前缀已被占用');
      } else if (error.status === 400) {
        message.error(error.message || '注册信息不合法,请检查后重试');
      } else {
        message.error(error.message);
      }
    } else {
      message.error('注册失败,请稍后重试');
    }
  } finally {
    submitting.value = false;
  }
}
</script>

<style scoped>
.input-icon {
  color: #8b98a5;
}

.slug-chip {
  display: inline-block;
  margin-left: 4px;
  padding: 1px 8px;
  border-radius: 5px;
  background: #f0f9fb;
  border: 1px solid #c9e4ec;
  color: #0e7490;
  font-size: 12.5px;
}

.auth-footer-links {
  margin-top: 24px;
  text-align: center;
  font-size: 13px;
  color: #8b98a5;
}
</style>
