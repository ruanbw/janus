<template>
  <AuthShell>
    <template #title>忘记密码</template>
    <template #subtitle>输入注册邮箱,我们将发送密码重置链接</template>

    <a-result
      v-if="sent"
      status="success"
      title="重置链接已发送"
      :sub-title="sentHint"
    >
      <template #extra>
        <a-button type="primary" @click="router.push('/login')">返回登录</a-button>
      </template>
    </a-result>

    <a-form v-else :model="form" :rules="rules" layout="vertical" @finish="onSubmit">
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
      <a-button type="primary" html-type="submit" block size="large" :loading="submitting">
        发送重置链接
      </a-button>
    </a-form>

    <div v-if="!sent" class="auth-footer-links">
      <router-link to="/login">返回登录</router-link>
      <span class="dot">·</span>
      <router-link to="/register">注册新租户</router-link>
    </div>
  </AuthShell>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { MailOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { forgotPassword } from '@/api/auth';
import AuthShell from '@/components/AuthShell.vue';
import { ApiError } from '@/types/api';

const router = useRouter();

const submitting = ref(false);
const sent = ref(false);
const form = reactive({ email: '' });

/** 开发环境:重置链接打印在后端容器日志(docker logs cloak-backend-1) */
const sentHint =
  '如果该邮箱已注册,重置链接已发送。开发环境中,链接打印在后端容器日志,请执行 docker logs cloak-backend-1 查看。';

const rules: Record<string, Rule[]> = {
  email: [
    { required: true, message: '请输入邮箱' },
    { type: 'email', message: '邮箱格式不正确' },
  ],
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

<style scoped>
.input-icon {
  color: #8b98a5;
}

.auth-footer-links {
  margin-top: 24px;
  text-align: center;
  font-size: 13px;
  color: #8b98a5;
}

.dot {
  margin: 0 8px;
  color: #cbd5e1;
}
</style>
