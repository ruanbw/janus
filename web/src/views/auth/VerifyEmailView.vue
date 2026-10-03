<template>
  <AuthShell :show-heading="false">
    <div v-if="verifying" class="py-10 text-center text-ink-faint">
      <AppSpin size="large" />
      <p class="mt-4 text-sm">正在验证邮箱…</p>
    </div>

    <AppResult
      v-else-if="verified"
      status="success"
      title="邮箱验证成功"
      sub-title="你的平台默认域名已激活,现在可以登录后台创建短链了。"
    >
      <template #extra>
        <AppButton type="primary" @click="router.push('/login')">前往登录</AppButton>
      </template>
    </AppResult>

    <template v-else>
      <AppResult
        v-if="failed"
        status="error"
        :title="failTitle"
        :sub-title="failMessage"
      >
        <template #extra>
          <AppButton type="primary" @click="router.push('/login')">返回登录</AppButton>
        </template>
      </AppResult>
      <AppResult v-else status="info" title="请完成邮箱验证" :sub-title="sentHint">
        <template #extra>
          <AppButton type="primary" @click="router.push('/login')">返回登录</AppButton>
        </template>
      </AppResult>

      <div class="mt-8 border-t border-line pt-6">
        <p class="mb-3 text-sm font-medium text-ink">没收到验证邮件?</p>
        <p class="mb-3 text-xs leading-relaxed text-ink-faint">输入注册邮箱,我们将重新发送验证邮件。</p>
        <div class="flex gap-2">
          <AppInput v-model="resendEmail" placeholder="you@example.com" autocomplete="email" class="flex-1" />
          <AppButton :loading="resending" @click="onResend">重新发送</AppButton>
        </div>
        <p v-if="resent" class="mt-2 text-xs text-ok">已发送,请查收邮箱(若该邮箱已注册)。</p>
      </div>
    </template>
  </AuthShell>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { resendVerification, verifyEmail } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import AppInput from '@/components/app/AppInput.vue';
import { ApiError } from '@/types/api';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();

const verifying = ref(false);
const verified = ref(false);
const failed = ref(false);
const failTitle = ref('邮箱验证失败');
const failMessage = ref('');

/** 开发环境:验证链接打印在后端容器日志(docker logs janus-backend-1) */
const sentHint =
  '验证链接已发送到你的邮箱。开发环境中,验证链接打印在后端容器日志中,请执行 docker logs janus-backend-1 查看。';

const resendEmail = ref('');
const resending = ref(false);
const resent = ref(false);

async function onResend() {
  const email = resendEmail.value.trim();
  if (!email) {
    message.error('请输入注册邮箱');
    return;
  }
  resending.value = true;
  try {
    await resendVerification({ email });
    resent.value = true;
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('发送失败,请稍后重试');
  } finally {
    resending.value = false;
  }
}

onMounted(async () => {
  const token = (route.query.token as string) || '';
  if (!token) return;

  verifying.value = true;
  try {
    await verifyEmail({ token });
    verified.value = true;
  } catch (error) {
    failed.value = true;
    if (error instanceof ApiError) {
      failTitle.value = '验证失败(' + error.code + ')';
      failMessage.value = error.message;
    } else {
      failMessage.value = '请稍后重试或重新发送验证邮件';
    }
  } finally {
    verifying.value = false;
  }
});
</script>
