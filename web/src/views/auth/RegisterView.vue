<template>
  <AuthShell>
    <template #title>注册新租户</template>
    <template #subtitle>
      注册后将自动获得平台默认域名
      <span class="slug-chip mono ml-1 inline-block rounded-md border border-brand/30 bg-brand/10 px-1.5 py-px align-middle text-xs text-brand">{{ slugHint }}</span>
    </template>

    <AppForm :model="form" :schema="schema" @finish="onSubmit">
      <AppFormItem label="账号" name="username" extra="3-32 位小写字母、数字、下划线或连字符,用于登录后台">
        <AppInput
          v-model="form.username"
          placeholder="例如 mybrand"
          autocomplete="username"
          :maxlength="USERNAME_MAX_LENGTH"
          size="large"
        >
          <template #prefix><User :size="16" /></template>
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
        <PasswordStrength :password="form.password" />
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
        label="默认域名前缀"
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
import { Globe, Lock, User } from '@lucide/vue';
import { z } from 'zod';
import type { FormSchema } from '@/components/app/form';

import { register } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import PasswordStrength from '@/components/PasswordStrength.vue';
import {
  SLUG_MAX_LENGTH,
  SLUG_PATTERN,
  USERNAME_MAX_LENGTH,
  USERNAME_PATTERN,
} from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import { message } from '@/utils/toast';

const auth = useAuthStore();
const router = useRouter();
const submitting = ref(false);

const PLATFORM_DOMAIN = import.meta.env.VITE_PLATFORM_DOMAIN || 'janus.test';

const form = reactive({
  username: '',
  password: '',
  confirmPassword: '',
  slug: '',
});

const slugHint = computed(() => {
  const slug = form.slug.trim();
  return slug ? slug + '.' + PLATFORM_DOMAIN : '<前缀>.' + PLATFORM_DOMAIN;
});

const schema: FormSchema = {
  username: z
    .string()
    .min(1, '请输入账号')
    .regex(USERNAME_PATTERN, '账号需为 3-32 位小写字母、数字、下划线或连字符'),
  password: z
    .string()
    .min(1, '请输入密码')
    .min(8, '密码至少 8 位')
    .regex(/^(?=.*[A-Za-z])(?=.*\d)/, '密码需同时包含字母与数字'),
  confirmPassword: z
    .string()
    .min(1, '请再次输入密码')
    .refine((v) => !v || v === form.password, { message: '两次输入的密码不一致' }),
  slug: z
    .string()
    .min(1, '请输入前缀')
    .regex(SLUG_PATTERN, '仅允许小写字母、数字与连字符,且须以字母或数字开头/结尾'),
};

async function onSubmit() {
  submitting.value = true;
  try {
    // 注册即建立会话(后端同时下发 janus_session),因此直接把返回的租户写进 store,
    // 无需再走一次登录;mustChangePassword 由返回体决定首屏去向。
    const tenant = await register({
      username: form.username.trim(),
      password: form.password,
      slug: form.slug.trim(),
    });
    auth.tenant = tenant;
    message.success('注册成功');
    router.push(tenant.mustChangePassword ? '/account' : '/domains');
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 409) {
        message.error(error.message || '账号或前缀已被占用');
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
