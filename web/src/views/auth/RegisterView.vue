<template>
  <AuthShell>
    <template #title>{{ t('auth.register.title') }}</template>
    <template #subtitle>
      {{ t('auth.register.subtitle') }}
      <span class="slug-chip mono ml-1 inline-block rounded-md border border-brand/30 bg-brand/10 px-1.5 py-px align-middle text-xs text-brand">{{ slugHint }}</span>
    </template>

    <AppForm :model="form" :schema="schema" @finish="onSubmit">
      <AppFormItem
        :label="t('auth.register.usernameLabel')"
        name="username"
        :extra="t('auth.register.usernameExtra')"
      >
        <AppInput
          v-model="form.username"
          :placeholder="t('auth.register.usernamePlaceholder')"
          autocomplete="username"
          :maxlength="USERNAME_MAX_LENGTH"
          size="large"
        >
          <template #prefix><User :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppFormItem :label="t('auth.register.passwordLabel')" name="password">
        <AppInput
          v-model="form.password"
          type="password"
          :placeholder="t('auth.register.passwordPlaceholder')"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><Lock :size="16" /></template>
        </AppInput>
        <PasswordStrength :password="form.password" />
      </AppFormItem>
      <AppFormItem :label="t('auth.register.confirmPasswordLabel')" name="confirmPassword">
        <AppInput
          v-model="form.confirmPassword"
          type="password"
          :placeholder="t('auth.register.confirmPasswordPlaceholder')"
          autocomplete="new-password"
          size="large"
        >
          <template #prefix><Lock :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppFormItem
        :label="t('auth.register.slugLabel')"
        name="slug"
        :extra="t('auth.register.slugExtra')"
      >
        <AppInput
          v-model="form.slug"
          :placeholder="t('auth.register.slugPlaceholder')"
          :maxlength="SLUG_MAX_LENGTH"
          size="large"
        >
          <template #prefix><Globe :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppButton type="primary" html-type="submit" block size="large" :loading="submitting">
        {{ t('auth.register.submit') }}
      </AppButton>
    </AppForm>

    <div class="auth-footer-links mt-6 text-center text-xs text-ink-faint">
      {{ t('auth.register.hasAccount') }}
      <router-link to="/login" class="font-medium text-brand transition-colors hover:text-brand/75">
        {{ t('auth.register.loginLink') }}
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
import { t } from '@/i18n';
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
  // 未填前缀时的占位文案也进字典：英文环境下不能出现中文的「<前缀>」
  return slug ? `${slug}.${PLATFORM_DOMAIN}` : t('auth.register.slugHint', { domain: PLATFORM_DOMAIN });
});

// zod 的 message 是普通字符串，因此在这里（setup 内）按当前语言取一次；
// 语法规则本身不走字典（USERNAME_PATTERN / SLUG_PATTERN 是冻结契约）。
const schema: FormSchema = {
  username: z
    .string()
    .min(1, t('auth.validation.usernameRequired'))
    .regex(USERNAME_PATTERN, t('auth.validation.usernamePattern')),
  password: z
    .string()
    .min(1, t('auth.validation.passwordRequired'))
    .min(8, t('auth.validation.passwordMin', { n: 8 }))
    .regex(/^(?=.*[A-Za-z])(?=.*\d)/, t('auth.validation.passwordPattern')),
  confirmPassword: z
    .string()
    .min(1, t('auth.validation.confirmPasswordRequired'))
    .refine((v) => !v || v === form.password, {
      message: t('auth.validation.confirmPasswordMismatch'),
    }),
  slug: z
    .string()
    .min(1, t('auth.validation.slugRequired'))
    .regex(SLUG_PATTERN, t('auth.validation.slugPattern')),
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
    message.success(t('auth.register.success'));
    router.push(tenant.mustChangePassword ? '/account' : '/domains');
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 409) {
        message.error(error.message || t('auth.register.errorConflict'));
      } else if (error.status === 400) {
        message.error(error.message || t('auth.register.errorBadRequest'));
      } else {
        message.error(error.message);
      }
    } else {
      message.error(t('auth.register.errorFallback'));
    }
  } finally {
    submitting.value = false;
  }
}
</script>
