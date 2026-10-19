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
      <AppFormItem v-if="setupRequired" label="首次登录校验码" name="setupToken" required>
        <AppInput
          v-model="form.setupToken"
          type="text"
          placeholder="粘贴邮件里的一次性校验码"
          autocomplete="one-time-code"
          size="large"
        />
        <template #extra>
          平台管理员首次登录需要这枚校验码:已发到该账号邮箱;未配置 SMTP 时它打印在后端日志里。
          设好密码后就不再需要它。
        </template>
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
/**
 * 是否需要「首次登录校验码」。
 *
 * 超管首次登录(尚无密码)时,后端 `verifyCredentials` 要求 login 请求带一次性 setup token,
 * 否则恒 401。而登录页以前从不发这个字段,于是浏览器侧首登永远失败 —— 只能靠 curl。
 * 这里只在后端明确回答说「需要 setup token」时才把输入框亮出来,常态登录不会多出一个字段。
 */
const setupRequired = ref(false);
const emailItem = ref<{ setError: (m: string) => void } | null>(null);
const passwordItem = ref<{ setError: (m: string) => void } | null>(null);
const form = reactive({
  email: '',
  password: '',
  rememberMe: true,
  setupToken: '',
});

const schema: FormSchema = {
  email: z.string().min(1, '请输入邮箱').email('邮箱格式不正确'),
  password: z.string().min(1, '请输入密码'),
};

async function onSubmit() {
  submitting.value = true;
  try {
    await auth.login(
      form.email.trim(),
      form.password,
      form.rememberMe,
      form.setupToken.trim() || undefined,
    );
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
      } else if (needsSetupToken(error)) {
        setupRequired.value = true;
        message.info('该账号还没有设置密码:请查收邮件里的「首次登录校验码」,填入下方后重新登录');
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

/** 后端在首登缺 setup token 时的回答(见 internal/httpapi/auth.go consumeSetupToken) */
function needsSetupToken(error: ApiError): boolean {
  return error.status === 401 && /setup\s*token/i.test(error.message);
}
</script>
