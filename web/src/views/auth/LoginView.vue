<template>
  <div class="auth-page">
    <a-card class="auth-card" :bordered="false">
      <div class="title">CLOAK 后台</div>
      <div class="subtitle">登录以管理你的域名与短链</div>

      <a-form :model="form" :rules="rules" layout="vertical" @finish="onSubmit">
        <a-form-item label="邮箱" name="email">
          <a-input v-model:value="form.email" placeholder="you@example.com" autocomplete="email">
            <template #prefix><MailOutlined /></template>
          </a-input>
        </a-form-item>
        <a-form-item label="密码" name="password">
          <a-input-password
            v-model:value="form.password"
            placeholder="请输入密码"
            autocomplete="current-password"
          >
            <template #prefix><LockOutlined /></template>
          </a-input-password>
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="form.rememberMe">记住我(会话保持 30 天)</a-checkbox>
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" block :loading="submitting">
            登录
          </a-button>
        </a-form-item>
      </a-form>

      <div class="links">
        <router-link to="/forgot-password">忘记密码</router-link>
        <router-link to="/register">注册新租户</router-link>
      </div>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { LockOutlined, MailOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const submitting = ref(false);
const form = reactive({
  email: '',
  password: '',
  rememberMe: true,
});

const rules: Record<string, Rule[]> = {
  email: [
    { required: true, message: '请输入邮箱' },
    { type: 'email', message: '邮箱格式不正确' },
  ],
  password: [{ required: true, message: '请输入密码' }],
};

async function onSubmit() {
  submitting.value = true;
  try {
    await auth.login(form.email.trim(), form.password, form.rememberMe);
    message.success('登录成功');
    const redirect = (route.query.redirect as string) || '/domains';
    router.push(redirect);
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        message.error('账号已被封禁,无法登录');
      } else if (error.status === 401) {
        message.error('邮箱或密码错误;若未完成邮箱验证,请先点击验证邮件中的链接');
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
  font-size: 24px;
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
