<template>
  <AuthShell>
    <template #title>忘记密码</template>
    <template #subtitle>输入注册邮箱,我们将发送密码重置链接</template>

    <AppResult v-if="sent" status="success" title="重置链接已发送" :sub-title="sentHint">
      <template #extra>
        <AppButton type="primary" @click="router.push('/login')">返回登录</AppButton>
      </template>
    </AppResult>

    <AppForm v-else :model="form" :schema="schema" @finish="onSubmit">
      <AppFormItem label="邮箱" name="email">
        <AppInput v-model="form.email" placeholder="you@example.com" autocomplete="email" size="large">
          <template #prefix><Mail :size="16" /></template>
        </AppInput>
      </AppFormItem>
      <AppButton type="primary" html-type="submit" block size="large" :loading="submitting">
        发送重置链接
      </AppButton>
    </AppForm>

    <div v-if="!sent" class="auth-footer-links mt-6 text-center text-sm text-ink-faint">
      <router-link to="/login" class="font-medium text-brand transition-colors hover:text-brand/75">返回登录</router-link>
      <!-- 分隔点继承外层的 text-ink-faint：原先写 text-line-strong，把「描边令牌」
           当成了文字色，深色下对比度只有 1.56:1，几乎看不见。 -->
      <span class="mx-2">·</span>
      <router-link to="/register" class="font-medium text-brand transition-colors hover:text-brand/75">注册新租户</router-link>
    </div>
  </AuthShell>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Mail } from '@lucide/vue';
import { z } from 'zod';
import type { FormSchema } from '@/components/app/form';

import { forgotPassword } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import { ApiError } from '@/types/api';
import { message } from '@/utils/toast';

const router = useRouter();

const submitting = ref(false);
const sent = ref(false);
const form = reactive({ email: '' });

/** 开发环境:重置链接打印在后端容器日志(docker logs janus-backend-1) */
const sentHint =
  '如果该邮箱已注册,重置链接已发送。开发环境中,链接打印在后端容器日志,请执行 docker logs janus-backend-1 查看。';

const schema: FormSchema = {
  email: z.string().min(1, '请输入邮箱').email('邮箱格式不正确'),
};

async function onSubmit() {
  submitting.value = true;
  try {
    await forgotPassword({ email: form.email.trim() });
    sent.value = true;
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('发送失败,请稍后重试');
  } finally {
    submitting.value = false;
  }
}
</script>
