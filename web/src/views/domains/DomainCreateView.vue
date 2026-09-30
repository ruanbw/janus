<template>
  <div>
    <PageHeader
      title="添加自有域名"
      description="将自有域名的 DNS 解析指向本服务器，系统将自动校验解析并申请签发 HTTPS 证书"
    >
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回域名列表
        </AppButton>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <!-- 左侧 2 列:主表单 Card -->
      <div class="lg:col-span-2">
        <AppCard :padding="false">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <Globe :size="18" class="text-brand-600 dark:text-brand-400" />
              域名基本信息
            </CardTitle>
            <CardDescription>
              填写的域名必须由您所有且具备 DNS 解析配置权限。添加后请尽快完成解析配置。
            </CardDescription>
          </CardHeader>

          <CardContent class="space-y-5">
            <AppForm
              ref="formRef"
              :model="formState as unknown as Record<string, unknown>"
              :rules="createRules"
              @finish="onSubmit"
            >
              <!-- 域名 FQDN 输入框 -->
              <AppFormItem
                name="fqdn"
                label="域名 FQDN"
                extra="仅支持英文字母、数字与连字符，以点分多级标签（如 links.example.com）。不可包含 http://、https:// 或路径后缀。"
              >
                <AppInput
                  v-model="formState.fqdn"
                  placeholder="例如 links.example.com"
                  @press-enter="onSubmit"
                >
                  <template #prefix>
                    <Globe :size="15" />
                  </template>
                </AppInput>
              </AppFormItem>

              <!-- 描述 Textarea -->
              <AppFormItem
                name="description"
                label="备注描述"
                extra="可选，用于记录该域名的业务用途，方便团队在域名列表中快速识别（最长 200 字）。"
              >
                <AppTextarea
                  v-model="formState.description"
                  placeholder="例如：市场推广主站短链、海外营销投放域名"
                  :maxlength="200"
                  :rows="3"
                  show-count
                />
              </AppFormItem>

              <!-- 合规提示 Alert -->
              <AppAlert
                type="warning"
                show-icon
                title="DNS 解析配置提醒"
                message="添加前请先确认已在您的域名 DNS 解析商处添加对应解析记录。系统将在后台自动轮询检测，若 72 小时内仍未检测到解析，该域名将被标记为「校验失败」。"
              />
            </AppForm>
          </CardContent>

          <!-- CardFooter 操作按钮 -->
          <CardFooter class="flex items-center justify-between border-t border-line bg-surface-muted/30 px-6 py-4">
            <span class="text-xs text-ink-faint">
              添加后计入自有域名配额
            </span>
            <div class="flex items-center gap-3">
              <AppButton @click="goBack">取消</AppButton>
              <AppButton
                type="primary"
                :loading="submitting"
                @click="onSubmit"
              >
                立即添加域名
              </AppButton>
            </div>
          </CardFooter>
        </AppCard>
      </div>

      <!-- 右侧 1 列:DNS 解析指南 Card -->
      <div class="space-y-6 lg:col-span-1">
        <AppCard :padding="false">
          <CardHeader class="pb-3">
            <CardTitle class="flex items-center gap-2 text-sm font-semibold">
              <Server :size="16" class="text-brand-600 dark:text-brand-400" />
              DNS 解析配置指南
            </CardTitle>
            <CardDescription>
              请登录您的域名注册商或 DNS 控制台，添加以下解析记录之一
            </CardDescription>
          </CardHeader>

          <CardContent class="space-y-4">
            <!-- 本服务器 IP 卡片展示 -->
            <div class="rounded-xl border border-line bg-surface-muted/60 p-3.5 space-y-2">
              <div class="flex items-center justify-between text-xs text-ink-soft">
                <span class="font-medium">本服务器 IP 地址</span>
                <span class="text-[11px] text-ink-faint">目标 A 记录</span>
              </div>
              <div class="flex items-center justify-between gap-2">
                <span class="mono text-sm font-bold text-ink">
                  {{ serverIp || '未配置服务器 IP' }}
                </span>
                <CopyText
                  v-if="serverIp"
                  :text="serverIp"
                  class="text-xs font-medium text-brand-600 hover:text-brand-700"
                >
                  复制 IP
                </CopyText>
              </div>
            </div>

            <!-- DNS 解析配置表格 -->
            <div class="space-y-2">
              <div class="text-[11px] font-medium uppercase tracking-wider text-ink-faint">
                推荐解析记录设置
              </div>
              <div class="overflow-hidden rounded-lg border border-line">
                <table class="w-full text-left text-xs">
                  <thead class="border-b border-line bg-surface-muted/70 text-ink-soft">
                    <tr>
                      <th class="px-2.5 py-1.5 font-medium">类型</th>
                      <th class="px-2.5 py-1.5 font-medium">主机记录</th>
                      <th class="px-2.5 py-1.5 font-medium">记录值</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-line text-ink">
                    <tr class="hover:bg-surface-muted/30">
                      <td class="px-2.5 py-2 font-mono font-semibold text-brand-600 dark:text-brand-400">A</td>
                      <td class="px-2.5 py-2 font-mono text-ink-soft">@ 或 子域</td>
                      <td class="px-2.5 py-2 font-mono text-ink truncate max-w-[110px]" :title="serverIp || '服务器 IP'">
                        {{ serverIp || '服务器 IP' }}
                      </td>
                    </tr>
                    <tr class="hover:bg-surface-muted/30">
                      <td class="px-2.5 py-2 font-mono font-semibold text-brand-600 dark:text-brand-400">CNAME</td>
                      <td class="px-2.5 py-2 font-mono text-ink-soft">子域 (如 links)</td>
                      <td class="px-2.5 py-2 font-mono text-ink truncate max-w-[110px]" :title="platformDomain || '平台域名'">
                        {{ platformDomain || '平台域名' }}
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- 自动签发 SSL 证书说明 -->
            <div class="rounded-xl border border-brand-200 bg-brand-50/50 p-3.5 text-xs text-brand-900 dark:border-brand-500/20 dark:bg-brand-500/5 dark:text-brand-200 space-y-1.5">
              <div class="flex items-center gap-1.5 font-semibold text-brand-700 dark:text-brand-300">
                <ShieldCheck :size="15" />
                自动签发 HTTPS 证书
              </div>
              <p class="text-[11px] leading-relaxed text-brand-800/80 dark:text-brand-300/80">
                DNS 解析生效后，系统将自动通过 ACME 协议向 Let's Encrypt 申请 SSL 证书并自动保持续期，无需手动上传证书。
              </p>
            </div>

            <!-- 72 小时轮询说明 -->
            <div class="flex items-start gap-2 rounded-lg border border-line-strong/60 bg-surface/50 p-3 text-[11px] leading-relaxed text-ink-faint">
              <Clock :size="14" class="mt-0.5 shrink-0 text-ink-soft" />
              <span>
                系统将在后台以指数退避间隔持续检测 DNS 指向；若 72 小时后仍未连通，状态将转为「校验失败」，届时可检查 DNS 后重新触发校验。
              </span>
            </div>
          </CardContent>
        </AppCard>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ArrowLeft, Clock, Globe, Server, ShieldCheck } from '@lucide/vue';

import { createDomain } from '@/api/domains';
import PageHeader from '@/components/PageHeader.vue';
import type { FormRule } from '@/components/ui/types';
import { message } from '@/utils/toast';
import { useAuthStore } from '@/stores/auth';
import { ApiError, getQuotaUsage } from '@/types/api';

const router = useRouter();
const auth = useAuthStore();

const formRef = ref();
const submitting = ref(false);
const formState = reactive({ fqdn: '', description: '' });

const serverIp = computed(() => auth.config?.serverIp || '');
const platformDomain = computed(() => auth.config?.platformDomain || '');

/** 域名格式校验(与后端 validFQDN 一致):点分标签,字母/数字/连字符,标签不以连字符开头结尾,总长 ≤253 */
function isValidFQDN(s: string): boolean {
  const value = s.endsWith('.') ? s.slice(0, -1) : s;
  if (!value || value.length > 253) return false;
  return value.split('.').every((label) => {
    if (!label || label.length > 63) return false;
    for (let i = 0; i < label.length; i++) {
      const c = label[i];
      const ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c === '-';
      if (!ok || (c === '-' && (i === 0 || i === label.length - 1))) return false;
    }
    return true;
  });
}

const createRules: Record<string, FormRule[]> = {
  fqdn: [
    { required: true, message: '请输入域名' },
    {
      validator: (_rule, value: unknown) => {
        if (!value) return Promise.resolve();
        return isValidFQDN(value as string)
          ? Promise.resolve()
          : Promise.reject(new Error('域名格式非法:仅支持字母、数字、连字符，标签不能以连字符开头或结尾，如 links.example.com'));
      },
    },
  ],
  description: [{ max: 200, message: '描述最多 200 字' }],
};

function goBack() {
  router.push({ name: 'domains' });
}

async function onSubmit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }
  const fqdn = formState.fqdn.trim().toLowerCase();
  const description = formState.description.trim();
  submitting.value = true;
  try {
    const domain = await createDomain({ fqdn, description });
    message.success('域名 ' + domain.fqdn + ' 已添加，正在等待 DNS 校验');
    router.push({ name: 'domains' });
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        const usage = getQuotaUsage(error.details);
        if (usage) {
          message.error('域名配额超限:自有域名 ' + usage.domains + '/' + usage.maxDomains + ' 已达上限');
        } else {
          message.error('域名配额超限:' + error.message);
        }
      } else if (error.status === 409) {
        message.error('域名已被占用:' + error.message);
      } else if (error.status === 400) {
        message.error('域名不合法:' + error.message);
      } else {
        message.error(error.message);
      }
    } else {
      message.error('添加失败，请稍后重试');
    }
  } finally {
    submitting.value = false;
  }
}
</script>
