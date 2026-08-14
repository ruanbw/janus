<template>
  <AuthShell>
    <template #title>登录后台</template>
    <template #subtitle>管理你的域名、短链与访问统计</template>

    <a-form :model="form" :rules="rules" layout="vertical" @finish="onSubmit">
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
      <a-form-item label="密码" name="password">
        <a-input-password
          v-model:value="form.password"
          placeholder="请输入密码"
          autocomplete="current-password"
          size="large"
        >
          <template #prefix><LockOutlined class="input-icon" /></template>
        </a-input-password>
      </a-form-item>
      <div class="form-options">
        <a-checkbox v-model:checked="form.rememberMe">记住我(会话保持 30 天)</a-checkbox>
        <router-link to="/forgot-password" class="forgot-link">忘记密码</router-link>
      </div>
      <a-button type="primary" html-type="submit" block size="large" :loading="submitting">
        登录
      </a-button>
    </a-form>

    <div class="auth-footer-links">
      还没有账号?
      <router-link to="/register">注册新租户</router-link>
    </div>
  </AuthShell>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { LockOutlined, MailOutlined } from '@ant-design/icons-vue';
import type { Rule } from 'ant-design-vue/es/form';

import AuthShell from '@/components/AuthShell.vue';
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
    // 仅允许站内路径,防止 ?redirect= 外部地址
    const rawRedirect = route.query.redirect;
    const redirect =
      typeof rawRedirect === 'string' && rawRedirect.startsWith('/') ? rawRedirect : '/domains';
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
      } else {
        // 后端区分「邮箱或密码错误」与「邮箱未验证,请查收验证邮件」,直接透出更准确
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
.input-icon {
  color: #8b98a5;
}

.form-options {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.forgot-link {
  font-size: 13px;
}

.auth-footer-links {
  margin-top: 24px;
  text-align: center;
  font-size: 13px;
  color: #8b98a5;
}
</style>
