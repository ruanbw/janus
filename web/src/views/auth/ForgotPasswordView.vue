<template>
  <div class="auth-page">
    <a-card class="auth-card" :bordered="false">
      <div class="title">忘记密码</div>
      <div class="subtitle">输入注册邮箱,我们将发送密码重置链接</div>

      <a-form :model="form" :rules="rules" layout="vertical" @finish="onSubmit">
        <a-form-item label="邮箱" name="email">
          <a-input v-model:value="form.email" placeholder="you@example.com" autocomplete="email">
            <template #prefix><MailOutlined /></template>
          </a-input>
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" block :loading="submitting">
            发送重置链接
          </a-button>
        </a-form-item>
      </a-form>

      <div class="links">
        <router-link to="/login">返回登录</router-link>
        <router-link to="/register">注册新租户</router-link>
      </div>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { MailOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { forgotPassword } from '@/api/auth';
import { ApiError } from '@/types/api';

const submitting = ref(false);
const form = reactive({ email: '' });

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
    message.success('如果该邮箱已注册,重置链接已发送(开发环境见 docker logs cloak-backend-1)');
    form.email = '';
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('发送失败,请稍后重试');
  } finally {
    submitting.value = false;
  }
}
</script>

<style scoped>
.auth-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1f2d3d 0%, #2b3a4a 100%);
}

.auth-card {
  width: 400px;
  border-radius: 12px;
}

.title {
  font-size: 22px;
  font-weight: 700;
  text-align: center;
}

.subtitle {
  text-align: center;
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 24px;
}

.links {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
}
</style>
