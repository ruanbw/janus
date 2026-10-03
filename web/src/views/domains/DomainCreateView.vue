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
              <Globe :size="18" class="text-brand" />
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
              :schema="createSchema"
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
              <Server :size="16" class="text-brand" />
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
                <span class="text-2xs text-ink-faint">目标 A 记录</span>
              </div>
              <div class="flex items-center justify-between gap-2">
                <span class="mono text-sm font-bold text-ink">
                  {{ serverIp || '未配置服务器 IP' }}
                </span>
                <CopyText
                  v-if="serverIp"
                  :text="serverIp"
                  class="text-xs font-medium text-brand hover:text-brand/75"
                >
                  复制 IP
                </CopyText>
              </div>
            </div>

            <!-- DNS 解析配置表格 -->
            <div class="space-y-2">
              <div class="text-2xs font-medium uppercase tracking-wider text-ink-faint">
                推荐解析记录设置
              </div>
              <AppTable
                :columns="dnsColumns"
                :data-source="dnsDataSource"
                :pagination="false"
                row-key="type"
              >
                <template #cell="{ column, record }">
                  <template v-if="column.key === 'type'">
                    <span class="font-mono font-semibold text-brand">
                      {{ record.type }}
                    </span>
                  </template>
                  <template v-else-if="column.key === 'host'">
                    <span class="font-mono text-ink-soft">
                      {{ record.host }}
                    </span>
                  </template>
                  <template v-else-if="column.key === 'value'">
                    <span class="font-mono text-ink truncate block max-w-[140px]" :title="String(record.value)">
                      {{ record.value }}
                    </span>
                  </template>
                </template>
              </AppTable>
            </div>

            <!-- 自动签发 SSL 证书说明 -->
            <div class="rounded-xl border border-brand/30 bg-brand/10 p-3.5 text-xs text-brand space-y-1.5">
              <div class="flex items-center gap-1.5 font-semibold text-brand">
                <ShieldCheck :size="15" />
                自动签发 HTTPS 证书
              </div>
              <p class="text-2xs leading-relaxed text-brand/80">
                DNS 解析生效后，系统将自动通过 ACME 协议向 Let's Encrypt 申请 SSL 证书并自动保持续期，无需手动上传证书。
              </p>
            </div>

            <!-- 72 小时轮询说明 -->
            <div class="flex items-start gap-2 rounded-lg border border-line-strong/60 bg-surface/50 p-3 text-2xs leading-relaxed text-ink-faint">
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
import { z } from 'zod';

import type { TableColumn } from '@/components/app/types';
import type { FormSchema } from '@/components/app/form';
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

const dnsColumns: TableColumn[] = [
  { key: 'type', dataIndex: 'type', title: '类型', width: 80 },
  { key: 'host', dataIndex: 'host', title: '主机记录', width: 140 },
  { key: 'value', dataIndex: 'value', title: '记录值' },
];

const dnsDataSource = computed<Record<string, unknown>[]>(() => [
  {
    type: 'A',
    host: '@ 或 子域',
    value: serverIp.value || '服务器 IP',
  },
  {
    type: 'CNAME',
    host: '子域 (如 links)',
    value: platformDomain.value || '平台域名',
  },
]);

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

const createSchema: FormSchema = {
  fqdn: z
    .string()
    .min(1, '请输入域名')
    .superRefine((value, ctx) => {
      if (!value) return;
      if (!isValidFQDN(value)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: '域名格式非法:仅支持字母、数字、连字符，标签不能以连字符开头或结尾，如 links.example.com',
        });
      }
    }),
  description: z.string().max(200, '描述最多 200 字'),
};

function goBack() {
  router.push({ name: 'domains' });
}

/**
 * 提交。有两条路径会同时到达这里：
 *  1) 「立即添加域名」按钮的 @click；
 *  2) FQDN 输入框的 @press-enter —— 它与 <form> 的原生隐式提交撞车：
 *     form 内没有 submit 按钮，且恰好只有 1 个阻塞隐式提交的字段（textarea 不阻塞），
 *     浏览器会在同一次回车里既派发 keydown.enter 又隐式提交。
 *
 * 所以第一行就必须同步上锁：否则一次回车打两次 POST /api/domains，
 * 表现为 201 成功后紧跟一个 409，误报「域名已被占用」。
 */
async function onSubmit() {
  if (submitting.value) return;
  submitting.value = true;
  try {
    // validate() resolve boolean、永不 reject：必须判断返回值。
    // 写成 try/catch 会让 catch 成为死代码、校验被静默跳过（契约见 AppForm.vue 的 validateAll）。
    const ok = await formRef.value?.validate();
    if (!ok) return;

    const fqdn = formState.fqdn.trim().toLowerCase();
    const description = formState.description.trim();
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
