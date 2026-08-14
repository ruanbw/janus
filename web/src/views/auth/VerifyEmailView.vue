<template>
  <AuthShell :show-heading="false">
    <div v-if="verifying" class="center">
      <a-spin size="large" />
      <p>正在验证邮箱…</p>
    </div>

    <a-result
      v-else-if="verified"
      status="success"
      title="邮箱验证成功"
      sub-title="你的平台默认域名已激活,现在可以登录后台创建短链了。"
    >
      <template #extra>
        <a-button type="primary" @click="router.push('/login')">前往登录</a-button>
      </template>
    </a-result>

    <a-result
      v-else-if="failed"
      status="error"
      :title="failTitle"
      :sub-title="failMessage"
    >
      <template #extra>
        <a-button type="primary" @click="router.push('/login')">返回登录</a-button>
      </template>
    </a-result>

    <a-result v-else status="info" title="请完成邮箱验证" :sub-title="sentHint">
      <template #extra>
        <a-button type="primary" @click="router.push('/login')">返回登录</a-button>
      </template>
    </a-result>
  </AuthShell>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { verifyEmail } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import { ApiError } from '@/types/api';

const route = useRoute();
const router = useRouter();

const verifying = ref(false);
const verified = ref(false);
const failed = ref(false);
const failTitle = ref('邮箱验证失败');
const failMessage = ref('');

/** 开发环境:验证链接打印在后端容器日志(docker logs cloak-backend-1) */
const sentHint =
  '验证链接已发送到你的邮箱。开发环境中,验证链接打印在后端容器日志中,请执行 docker logs cloak-backend-1 查看。';

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
      failTitle.value = `验证失败(${error.code})`;
      failMessage.value = error.message;
    } else {
      failMessage.value = '请稍后重试或重新发送验证邮件';
    }
  } finally {
    verifying.value = false;
  }
});
</script>

<style scoped>
.center {
  text-align: center;
  padding: 48px 0;
  color: #5b6b7c;
}

.center p {
  margin-top: 16px;
  font-size: 14px;
}
</style>
