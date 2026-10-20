<template>
  <AuthShell>
    <template #title>{{ t('auth.login.title') }}</template>
    <template #subtitle>{{ t('auth.login.subtitle') }}</template>

    <AppForm :model="form" :schema="schema" @finish="onSubmit">
      <AppFormItem ref="usernameItem" :label="t('auth.login.usernameLabel')" name="username">
        <AppInput
          v-model="form.username"
          :placeholder="t('auth.login.usernamePlaceholder')"
          autocomplete="username"
          :maxlength="USERNAME_MAX_LENGTH"
          size="large"
        >
          <template #prefix><User :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppFormItem ref="passwordItem" :label="t('auth.login.passwordLabel')" name="password">
        <AppInput
          v-model="form.password"
          type="password"
          :placeholder="t('auth.login.passwordPlaceholder')"
          autocomplete="current-password"
          size="large"
        >
          <template #prefix><Lock :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <div class="form-options mb-5 flex items-center">
        <AppCheckbox v-model="form.rememberMe">{{ t('auth.login.rememberMe') }}</AppCheckbox>
      </div>
      <AppButton type="primary" html-type="submit" block size="large" :loading="submitting">
        {{ t('auth.login.submit') }}
      </AppButton>
    </AppForm>

    <div class="auth-footer-links mt-6 text-center text-xs text-ink-faint">
      {{ t('auth.login.noAccount') }}
      <router-link to="/register" class="font-medium text-brand transition-colors hover:text-brand/75">
        {{ t('auth.login.registerLink') }}
      </router-link>
    </div>
  </AuthShell>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Lock, User } from '@lucide/vue';
import { z } from 'zod';
import type { FormSchema } from '@/components/app/form';

import AuthShell from '@/components/AuthShell.vue';
import { USERNAME_MAX_LENGTH, USERNAME_PATTERN } from '@/constants/dict';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import { message } from '@/utils/toast';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const submitting = ref(false);
const usernameItem = ref<{ setError: (m: string) => void } | null>(null);
const passwordItem = ref<{ setError: (m: string) => void } | null>(null);
const form = reactive({
  username: '',
  password: '',
  rememberMe: true,
});

// zod 的 message 是普通字符串，因此在这里（setup 内）按当前语言取一次：
// 校验文案随后由 AppFormItem 直接渲染，与模板里的 t() 用的是同一份字典。
const schema: FormSchema = {
  username: z
    .string()
    .min(1, t('auth.validation.usernameRequired'))
    .regex(USERNAME_PATTERN, t('auth.validation.usernamePattern')),
  password: z.string().min(1, t('auth.validation.passwordRequired')),
};

async function onSubmit() {
  submitting.value = true;
  try {
    await auth.login(form.username.trim(), form.password, form.rememberMe);
    message.success(t('auth.login.success'));
    // 仅允许站内单斜杠路径;拒绝 //evil.com 这类协议相对地址
    const rawRedirect = route.query.redirect;
    const redirect =
      typeof rawRedirect === 'string' && rawRedirect.startsWith('/') && !rawRedirect.startsWith('//')
        ? rawRedirect
        : '/domains';
    // 强制改密:后端会拒绝其余端点,先到账号设置页完成改密
    if (auth.mustChangePassword) {
      router.push('/account');
    } else {
      router.push(redirect);
    }
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        message.error(t('auth.login.errorBanned'));
      } else if (/账号或密码错误|用户名或密码错误/.test(error.message)) {
        // 服务端错误文案原样回填：接口返回什么就显示什么，不经过前端字典
        passwordItem.value?.setError(error.message);
      } else {
        message.error(error.message);
      }
    } else {
      message.error(t('auth.login.errorFallback'));
    }
  } finally {
    submitting.value = false;
  }
}
</script>
