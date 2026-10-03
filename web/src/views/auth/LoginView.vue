<template>
  <AuthShell>
    <template #title>登录后台</template>
    <template #subtitle>管理你的域名、短链与访问统计</template>

    <AppForm :model="form" :schema="schema" @finish="onSubmit">
      <AppFormItem ref="emailItem" label="邮箱" name="email">
        <AppInput v-model="form.email" placeholder="you@example.com" autocomplete="email" size="large">
          <template #prefix><Mail :size="16" /></template>
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
      <div class="form-options mb-5 flex items-center justify-between">
        <AppCheckbox v-model="form.rememberMe">记住我(会话保持 30 天)</AppCheckbox>
        <router-link to="/forgot-password" class="text-xs font-medium text-brand transition-colors hover:text-brand/75">
          忘记密码
        </router-link>
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
import { Lock, Mail } from '@lucide/vue';
import { z } from 'zod';
import type { FormSchema } from '@/components/app/form';

import AuthShell from '@/components/AuthShell.vue';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import { message } from '@/utils/toast';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const submitting = ref(false);
const emailItem = ref<{ setError: (m: string) => void } | null>(null);
const passwordItem = ref<{ setError: (m: string) => void } | null>(null);
const form = reactive({
  email: '',
  password: '',
  rememberMe: true,
});

const schema: FormSchema = {
  email: z.string().min(1, '请输入邮箱').email('邮箱格式不正确'),
  password: z.string().min(1, '请输入密码'),
};

async function onSubmit() {
  submitting.value = true;
  try {
    await auth.login(form.email.trim(), form.password, form.rememberMe);
    message.success('登录成功');
    // 仅允许站内单斜杠路径;拒绝 //evil.com 这类协议相对地址
    const rawRedirect = route.query.redirect;
    const redirect =
      typeof rawRedirect === 'string' && rawRedirect.startsWith('/') && !rawRedirect.startsWith('//')
        ? rawRedirect
        : '/domains';
    // 超管首次登录(尚无密码):先到账号设置页设置初始密码
    if (auth.tenant?.firstLoginSetup) {
      router.push('/account');
    } else {
      router.push(redirect);
    }
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        message.error('账号已被封禁,无法登录');
      } else if (/未验证|验证邮件/.test(error.message)) {
        emailItem.value?.setError(error.message);
      } else if (/邮箱或密码错误/.test(error.message)) {
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
