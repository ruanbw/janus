<template>
  <AuthShell>
    <template #title>登录后台</template>
    <template #subtitle>管理你的域名、短链与访问统计</template>

    <AppForm :model="form" :schema="schema" @finish="onSubmit">
      <AppFormItem ref="usernameItem" label="账号" name="username">
        <AppInput
          v-model="form.username"
          placeholder="请输入账号"
          autocomplete="username"
          :maxlength="USERNAME_MAX_LENGTH"
          size="large"
        >
          <template #prefix><User :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppFormItem ref="passwordItem" label="密码" name="password">
        <AppInput
          v-model="form.password"
          type="password"
          placeholder="请输入密码"
          autocomplete="current-password"
          size="large"
        >
          <template #prefix><Lock :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <div class="form-options mb-5 flex items-center">
        <AppCheckbox v-model="form.rememberMe">记住我(会话保持 30 天)</AppCheckbox>
      </div>
      <AppButton type="primary" html-type="submit" block size="large" :loading="submitting">
        登录
      </AppButton>
    </AppForm>

    <div class="auth-footer-links mt-6 text-center text-xs text-ink-faint">
      还没有账号?
      <router-link to="/register" class="font-medium text-brand transition-colors hover:text-brand/75">
        注册新租户
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

const schema: FormSchema = {
  username: z
    .string()
    .min(1, '请输入账号')
    .regex(USERNAME_PATTERN, '账号需为 3-32 位小写字母、数字、下划线或连字符'),
  password: z.string().min(1, '请输入密码'),
};

async function onSubmit() {
  submitting.value = true;
  try {
    await auth.login(form.username.trim(), form.password, form.rememberMe);
    message.success('登录成功');
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
        message.error('账号已被封禁,无法登录');
      } else if (/账号或密码错误|用户名或密码错误/.test(error.message)) {
        passwordItem.value?.setError(error.message);
      } else {
        message.error(error.message);
      }
    } else {
      message.error('登录失败,请稍后重试');
    }
  } finally {
    submitting.value = false;
  }
}
</script>
