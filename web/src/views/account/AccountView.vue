<template>
  <div>
    <PageHeader
      title="账号设置与安全"
      description="查看租户核心账户信息与资源配额，并管理控制台登录密码凭据"
    />

    <!-- 平台超管首次登录设置密码提醒 -->
    <AppAlert
      v-if="auth.tenant?.firstLoginSetup"
      class="mb-6"
      type="warning"
      show-icon
      title="首次登录安全提示"
      message="平台管理员账号尚未设置独立登录密码，请先在右侧完成初始密码设置后再正常使用各项管理功能。"
    />

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2 items-start">
      <!-- 左列: 租户信息与资源配额 -->
      <div class="space-y-6">
        <!-- 租户信息卡片 -->
        <AppCard :padding="false">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <Layers :size="18" class="text-brand" />
              租户信息
            </CardTitle>
            <CardDescription>
              租户基础账户属性与当前生效配置
            </CardDescription>
          </CardHeader>

          <CardContent class="space-y-6 p-6">
            <!-- 结构化的租户信息描述列表 -->
            <div class="rounded-xl border border-line bg-surface-muted/40 p-4 space-y-3">
              <div class="flex items-center justify-between text-xs">
                <span class="text-ink-soft">租户登录邮箱</span>
                <span class="mono font-semibold text-ink">{{ auth.tenant?.email ?? '—' }}</span>
              </div>
              <div class="flex items-center justify-between text-xs border-t border-line/60 pt-2.5">
                <span class="text-ink-soft">租户标识前缀 (Slug)</span>
                <code class="font-mono text-xs font-semibold text-ink bg-surface px-1.5 py-0.5 rounded border border-line">
                  {{ auth.tenant?.slug ?? '-' }}
                </code>
              </div>
              <div class="flex items-center justify-between text-xs border-t border-line/60 pt-2.5">
                <span class="text-ink-soft">当前生效等级</span>
                <AppTag color="info">{{ auth.tenant?.tier?.name ?? '-' }}</AppTag>
              </div>
              <div class="flex items-center justify-between text-xs border-t border-line/60 pt-2.5">
                <span class="text-ink-soft">平台默认域名</span>
                <CopyText
                  :text="auth.tenant?.defaultDomain ?? ''"
                  class="mono text-xs font-semibold text-brand-600 hover:text-brand-700"
                >
                  {{ auth.tenant?.defaultDomain ?? '-' }}
                </CopyText>
              </div>
              <div class="flex items-center justify-between text-xs border-t border-line/60 pt-2.5">
                <span class="text-ink-soft">账号当前状态</span>
                <AppTag :color="statusInfo.color">{{ statusInfo.label }}</AppTag>
              </div>
            </div>
          </CardContent>
        </AppCard>

        <!-- 资源配额卡片 -->
        <AppCard :padding="false">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <Gauge :size="18" class="text-brand" />
              资源配额
            </CardTitle>
            <CardDescription>
              租户可用的短链与自有域名配额上限及当前使用进度
            </CardDescription>
          </CardHeader>

          <CardContent class="space-y-4 p-6">
            <div
              v-for="row in quotaRows"
              :key="row.key"
              class="rounded-xl border border-line bg-surface-muted/40 p-4 space-y-2.5"
            >
              <div class="flex items-baseline justify-between gap-3 text-xs">
                <span class="font-medium text-ink">{{ row.label }}</span>
                <span
                  class="whitespace-nowrap font-mono tabular-nums"
                  :class="row.percent >= 100 ? 'text-err font-bold' : 'text-ink font-semibold'"
                >
                  {{ row.used }} / {{ row.maxLabel }}
                  <span class="font-sans font-normal text-ink-faint">（{{ row.percent }}%）</span>
                </span>
              </div>

              <AppProgress
                :percent="row.percent"
                :status="row.percent >= 100 ? 'exception' : 'active'"
                :stroke-width="8"
                :show-info="false"
              />

              <p
                v-if="row.percent >= 100"
                class="flex items-center gap-1.5 text-2xs font-medium text-err"
              >
                <TriangleAlert :size="13" class="shrink-0" />
                {{ row.exhausted }}
              </p>
              <p
                v-else-if="row.percent >= QUOTA_WARN_PERCENT"
                class="flex items-center gap-1.5 text-2xs font-medium text-warn"
              >
                <TriangleAlert :size="13" class="shrink-0" />
                {{ row.label }}已达 {{ row.percent }}%，接近上限
              </p>
              <p v-else class="text-2xs text-ink-faint">
                {{ row.note }}
              </p>
            </div>
          </CardContent>
        </AppCard>
      </div>

      <!-- 右卡片:修改密码 / 设置初始密码 -->
      <AppCard :padding="false">
        <CardHeader>
          <CardTitle class="flex items-center gap-2">
            <KeyRound :size="18" class="text-brand" />
            {{ isFirstLogin ? '设置初始登录密码' : '修改登录密码' }}
          </CardTitle>
          <CardDescription>
            {{
              isFirstLogin
                ? '平台管理员首次登录，请设置独立登录密码以保障管理后台安全'
                : '定期更换强密码有助于保护您的短链路由资源与访问统计数据安全'
            }}
          </CardDescription>
        </CardHeader>

        <CardContent class="space-y-5 p-6">
          <AppForm ref="pwdFormRef" :model="form" :rules="rules" @finish="onChangePassword">
            <!-- 当前密码(首次登录设置密码时无需输入) -->
            <AppFormItem
              v-if="!isFirstLogin"
              label="当前旧密码"
              name="oldPassword"
              extra="请输入您当前使用的账号密码进行身份验证"
            >
              <AppInput
                v-model="form.oldPassword"
                type="password"
                placeholder="请输入当前密码"
                autocomplete="current-password"
              >
                <template #prefix>
                  <Lock :size="15" />
                </template>
              </AppInput>
            </AppFormItem>

            <!-- 新密码 -->
            <AppFormItem
              :label="isFirstLogin ? '设置登录密码' : '设置新密码'"
              name="newPassword"
              extra="密码长度须至少 8 位，包含字符多样性"
            >
              <AppInput
                v-model="form.newPassword"
                type="password"
                placeholder="请输入新密码（至少 8 位）"
                autocomplete="new-password"
              >
                <template #prefix>
                  <Lock :size="15" />
                </template>
              </AppInput>
            </AppFormItem>

            <!-- 确认新密码 -->
            <AppFormItem
              label="再次确认密码"
              name="confirmPassword"
              extra="请再次输入相同的新密码以防止拼写有误"
            >
              <AppInput
                v-model="form.confirmPassword"
                type="password"
                placeholder="再次输入以确认新密码"
                autocomplete="new-password"
              >
                <template #prefix>
                  <Lock :size="15" />
                </template>
              </AppInput>
            </AppFormItem>
          </AppForm>

          <!-- 密码规则提示小卡片 -->
          <div class="rounded-xl border border-line bg-surface-muted/40 p-4 space-y-2 text-xs">
            <div class="flex items-center gap-1.5 font-semibold text-ink">
              <ShieldCheck :size="15" class="text-brand" />
              密码安全建议
            </div>
            <ul class="list-disc pl-4 space-y-1 text-ink-faint text-2xs leading-relaxed">
              <li>密码长度不得少于 8 位字符；</li>
              <li>建议组合使用大小写英文字母、数字与特殊标点符号；</li>
              <li>请勿使用生日、姓名拼音或常见的连贯弱口令。</li>
            </ul>
          </div>
        </CardContent>

        <!-- CardFooter 提交按钮 -->
        <CardFooter class="flex items-center justify-between border-t border-line bg-surface-muted/30 px-6 py-4">
          <span class="text-xs text-ink-faint">
            修改成功后请牢记新密码
          </span>
          <AppButton
            type="primary"
            :loading="changingPassword"
            @click="submitPasswordForm"
          >
            {{ isFirstLogin ? '确认设置初始密码' : '确认修改密码' }}
          </AppButton>
        </CardFooter>
      </AppCard>
    </div>

    <!-- 访客端错误页面配置卡片 -->
    <div class="mt-6">
      <ErrorPagesCard />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import {
  Gauge,
  KeyRound,
  Layers,
  Lock,
  ShieldCheck,
  TriangleAlert,
} from '@lucide/vue';
import { message } from '@/utils/toast';
import type { FormRule } from '@/components/app/types';

import { changePassword } from '@/api/auth';
import { fetchMyTenant } from '@/api/me';
import PageHeader from '@/components/PageHeader.vue';
import ErrorPagesCard from '@/views/settings/ErrorPagesCard.vue';
import { TENANT_STATUS } from '@/constants/dict';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';

const auth = useAuthStore();

const changingPassword = ref(false);
const pwdFormRef = ref();
const form = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' });

const isFirstLogin = computed(() => Boolean(auth.tenant?.firstLoginSetup));

const statusInfo = computed(() => {
  const s = auth.tenant?.status ?? 'pending';
  return TENANT_STATUS[s] ?? { label: '未知', color: 'default' };
});

/** 达到这个百分比开始提醒接近上限；100% 则是硬上限 */
const QUOTA_WARN_PERCENT = 80;

interface QuotaRow {
  key: 'links' | 'domains';
  label: string;
  used: number;
  max: number;
  maxLabel: string;
  percent: number;
  exhausted: string;
  note: string;
}

const quotaRows = computed<QuotaRow[]>(() => {
  const usage = auth.tenant?.usage;
  const rows = [
    {
      key: 'links' as const,
      label: '短链配额',
      used: usage?.links ?? 0,
      max: usage?.maxLinks ?? 0,
      exhausted: '短链配额已用尽，无法创建新短链，请联系管理员升级租户等级',
      note: '包含已启用与已停用的短链；物理删除后才释放名额',
    },
    {
      key: 'domains' as const,
      label: '自有域名配额',
      used: usage?.domains ?? 0,
      max: usage?.maxDomains ?? 0,
      exhausted: '自有域名配额已用尽，无法添加新域名',
      note: '平台默认域名不计入自有域名配额',
    },
  ];
  return rows.map((row) => {
    // max 为 0/缺失时当作不限：显示“不限”，进度条 0%
    if (!row.max) {
      return { ...row, maxLabel: '不限', percent: 0 };
    }
    return {
      ...row,
      maxLabel: String(row.max),
      percent: Math.min(100, Math.round((row.used / row.max) * 100)),
    };
  });
});

const rules: Record<string, FormRule[]> = {
  oldPassword: [{ required: true, message: '请输入当前密码' }],
  newPassword: [
    { required: true, message: '请输入新密码' },
    { min: 8, message: '密码至少 8 位' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (_rule, value) => {
        if (!value || value === form.newPassword) return Promise.resolve();
        return Promise.reject(new Error('两次输入的密码不一致'));
      },
    },
  ],
};

async function load() {
  try {
    const me = await auth.fetchMe();
    if (!me) {
      auth.tenant = await fetchMyTenant();
    }
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  }
}

onMounted(load);

async function submitPasswordForm() {
  try {
    await pwdFormRef.value?.validate();
  } catch {
    return;
  }
  await onChangePassword();
}

async function onChangePassword() {
  changingPassword.value = true;
  try {
    await changePassword({
      oldPassword: isFirstLogin.value ? undefined : form.oldPassword,
      newPassword: form.newPassword,
    });
    message.success(isFirstLogin.value ? '初始密码已成功设置' : '登录密码已成功修改');
    form.oldPassword = '';
    form.newPassword = '';
    form.confirmPassword = '';
    if (isFirstLogin.value) {
      await load();
    }
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('修改失败，请稍后重试');
  } finally {
    changingPassword.value = false;
  }
}
</script>
