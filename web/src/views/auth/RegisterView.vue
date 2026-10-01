<template>
  <AuthShell>
    <template #title>注册新租户</template>
    <template #subtitle>
      注册后将自动获得平台默认域名
      <span class="slug-chip mono ml-1 inline-block rounded-md border border-brand/30 bg-brand/10 px-1.5 py-px align-middle text-xs text-brand">{{ slugHint }}</span>
    </template>

    <AppForm :model="form" :rules="rules" @finish="onSubmit">
      <AppFormItem label="邮箱" name="email">
        <AppInput v-model="form.email" placeholder="you@example.com" autocomplete="email" size="large">
          <template #prefix><Mail :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppFormItem label="密码" name="password">
        <AppInput
          v-model="form.password"
          type="password"
          placeholder="至少 8 位"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><Lock :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppFormItem label="确认密码" name="confirmPassword">
        <AppInput
          v-model="form.confirmPassword"
          type="password"
          placeholder="再次输入密码"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><Lock :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppFormItem
        label="前缀"
        name="slug"
        extra="小写字母或数字开头/结尾,可包含连字符;将用于生成你的平台默认域名"
      >
        <AppInput
          v-model="form.slug"
          placeholder="例如 mybrand"
          :maxlength="SLUG_MAX_LENGTH"
          size="large"
        >
          <template #prefix><Globe :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppButton type="primary" html-type="submit" block size="large" :loading="submitting">
        注册
      </AppButton>
    </AppForm>

    <div class="auth-footer-links mt-6 text-center text-xs text-ink-faint">
      已有账号?
      <router-link to="/login" class="font-medium text-brand transition-colors hover:text-brand/75">
        直接登录
      </router-link>
    </div>
  </AuthShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Globe, Lock, Mail } from '@lucide/vue';
import type { FormRule } from '@/components/app/types';

import { register } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import { SLUG_MAX_LENGTH, SLUG_PATTERN } from '@/constants/dict';
import { ApiError } from '@/types/api';
import { message } from '@/utils/toast';

const router = useRouter();
const submitting = ref(false);

const PLATFORM_DOMAIN = import.meta.env.VITE_PLATFORM_DOMAIN || 'janus.test';

const form = reactive({
  email: '',
  password: '',
  confirmPassword: '',
  slug: '',
});

const slugHint = computed(() => {
  const slug = form.slug.trim();
  return slug ? slug + '.' + PLATFORM_DOMAIN : '<前缀>.' + PLATFORM_DOMAIN;
});

const rules: Record<string, FormRule[]> = {
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
      validator: (_rule, value) => {
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
