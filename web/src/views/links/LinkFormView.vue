<template>
  <div>
    <PageHeader :title="isEdit ? '编辑短链' : '创建短链'" :description="headerDescription">
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回
        </AppButton>
      </template>
    </PageHeader>

    <AppCard :bordered="false" class="form-card">
      <AppSpin :spinning="loading">
        <AppForm ref="formRef" :model="form as unknown as Record<string, unknown>" :rules="rules">
          <AppFormItem name="code" label="短码" :extra="codeExtra">
            <AppInput
              v-model="form.code"
              placeholder="留空自动生成"
              :maxlength="SHORT_CODE_MAX_LENGTH"
              :disabled="isEdit"
            />
          </AppFormItem>

          <AppFormItem
            name="linkType"
            label="短链类型"
            extra="跳转型:访问短链后立即重定向到目标 URL(支持多个按顺序轮询)。落地页型:先展示一个中间落地页(在线地址或上传的压缩包),访问者点击后才跳转,可单独统计「点击」数。"
          >
            <AppRadioGroup v-model="form.linkType">
              <AppRadio value="redirect">跳转型</AppRadio>
              <AppRadio value="landing">落地页型</AppRadio>
            </AppRadioGroup>
          </AppFormItem>

          <AppFormItem
            name="targetUrls"
            label="目标 URL"
            extra="支持配置多个目标 URL,访问时按从上到下的顺序轮询分发。支持任意协议(如 https://、http://、mailto:)。每个 URL 不能包含换行/制表符等控制字符,单个最长 4096 字符。"
          >
            <div class="flex flex-col gap-2">
              <div
                v-for="(_, index) in form.targetUrls"
                :key="index"
                class="flex items-center gap-1.5"
              >
                <AppInput
                  v-model="form.targetUrls[index]"
                  placeholder="https://example.com/page"
                />
                <AppButton
                  v-if="form.targetUrls.length > 1"
                  type="text"
                  danger
                  class="shrink-0"
                  @click="removeTargetUrl(index)"
                >
                  <template #icon><CircleMinus :size="15" /></template>
                </AppButton>
              </div>
              <AppButton type="dashed" block @click="addTargetUrl">
                <template #icon><Plus :size="15" /></template>
                添加目标 URL
              </AppButton>
            </div>
          </AppFormItem>

          <template v-if="form.linkType === 'landing'">
            <AppFormItem
              name="landingSource"
              label="落地页来源"
              extra="URL 地址:落地页直接指向一个在线地址,访问时先重定向到该地址。上传压缩包:上传一个静态站点 zip(必须包含 index.html),由本服务托管,访问时直接展示。"
            >
              <AppRadioGroup v-model="form.landingSource">
                <AppRadio value="url">URL 地址</AppRadio>
                <AppRadio value="upload">上传压缩包</AppRadio>
              </AppRadioGroup>
            </AppFormItem>

            <AppFormItem
              v-if="form.landingSource === 'url'"
              name="landingUrl"
              label="落地页地址"
              extra="访问落地页型短链时,先重定向到此地址。不能包含控制字符,最长 4096 字符。仅在「落地页来源 = URL 地址」时填写。"
            >
              <AppInput v-model="form.landingUrl" placeholder="https://example.com/landing" />
            </AppFormItem>

            <AppFormItem
              v-else
              name="landingFile"
              label="落地页压缩包"
              extra="压缩包必须包含 index.html,作为落地页入口。上传为替换式:再次上传会覆盖旧版本,成功后立即生效,无需重新创建短链。仅支持 .zip 格式。"
            >
              <div class="flex flex-col gap-2">
                <div class="flex items-center gap-2.5">
                  <AppUpload accept=".zip" :before-upload="onSelectZip">
                    <AppButton :loading="landingUploading">
                      <template #icon><Upload :size="15" /></template>
                      {{ form.landingUploaded ? '重新上传压缩包' : '选择 zip 压缩包' }}
                    </AppButton>
                  </AppUpload>
                  <AppTag v-if="form.landingUploaded" color="success">已上传</AppTag>
                  <AppTag v-else color="default">未上传</AppTag>
                </div>
                <div v-if="landingFile" class="break-all text-xs text-ink-soft">
                  待上传:{{ landingFile.name }}
                </div>
              </div>
            </AppFormItem>
          </template>

          <AppFormItem
            name="domainIds"
            label="关联域名"
            extra="仅已激活(active)的域名可关联短链;待激活、校验失败、已停用的域名置灰不可选。同一条短码可在不同域名下指向不同目标,至少选择一个域名。"
          >
            <AppSelect
              v-model="form.domainIds"
              multiple
              placeholder="选择关联域名"
              :options="domainOptions"
              :max-tag-count="3"
            />
          </AppFormItem>

          <AppFormItem
            v-if="form.linkType === 'redirect'"
            name="redirectStatus"
            label="重定向方式"
            extra="临时重定向(302):浏览器与搜索引擎不缓存跳转,目标变更后立即生效,适合经常调整目标的场景。永久重定向(301):浏览器与搜索引擎会缓存跳转,目标变更后旧地址可能长期命中缓存。"
          >
            <AppRadioGroup v-model="form.redirectStatus">
              <AppRadio value="302">临时重定向(302)</AppRadio>
              <AppRadio value="301">永久重定向(301)</AppRadio>
            </AppRadioGroup>
          </AppFormItem>

          <AppFormItem
            v-if="isEdit"
            name="status"
            label="状态"
            extra="启用:短链正常解析并计入访问统计。停用:短链保留但访问时返回未命中(404),可随时重新启用。"
          >
            <AppRadioGroup v-model="form.status">
              <AppRadio value="enabled">启用</AppRadio>
              <AppRadio value="disabled">停用</AppRadio>
            </AppRadioGroup>
          </AppFormItem>

          <AppFormItem>
            <AppSpace>
              <AppButton type="primary" :loading="submitting" @click="onSubmit">保存</AppButton>
              <AppButton @click="goBack">取消</AppButton>
            </AppSpace>
          </AppFormItem>
        </AppForm>
      </AppSpin>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft, CircleMinus, Plus, Upload } from '@lucide/vue';
import type { FormRule } from '@/components/ui/types';

import { listDomains } from '@/api/domains';
import { createLink, getLink, updateLink, uploadLanding } from '@/api/links';
import PageHeader from '@/components/PageHeader.vue';
import {
  SHORT_CODE_FORBIDDEN_PATTERN,
  SHORT_CODE_MAX_LENGTH,
  SHORT_CODE_PATTERN,
} from '@/constants/dict';
import { ApiError, getQuotaUsage } from '@/types/api';
import type { Domain, LandingSource, Link, LinkType, RedirectStatus } from '@/types/api';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();

/** 仅“短链编辑”路由且 ID 为正整数时才是编辑模式;创建路由没有 id 参数,不能把 NaN 当编辑 */
const validLinkId = computed(() => {
  const raw = route.params.id;
  const id = typeof raw === 'string' ? Number(raw) : Number.NaN;
  return Number.isInteger(id) && id > 0 ? id : undefined;
});
const isEdit = computed(() => route.name === 'link-edit' && validLinkId.value !== undefined);

const headerDescription = computed(() =>
  isEdit.value
    ? '修改短链的目标、类型、落地页与关联域名;短码创建后不可修改'
    : '选择短链类型、目标与关联域名,提交后立即生效;短码可留空自动生成',
);

const formRef = ref();
const submitting = ref(false);
const loading = ref(false);
const landingFile = ref<File | null>(null);
const landingUploading = ref(false);

const link = ref<Link | null>(null);
const domains = ref<Domain[]>([]);

const form = reactive<{
  code: string;
  targetUrls: string[];
  domainIds: number[];
  redirectStatus: RedirectStatus;
  linkType: LinkType;
  landingSource: LandingSource;
  landingUrl: string;
  landingUploaded: boolean;
  status: 'enabled' | 'disabled';
}>({
  code: '',
  targetUrls: [''],
  domainIds: [],
  redirectStatus: '302',
  linkType: 'redirect',
  landingSource: 'url',
  landingUrl: '',
  landingUploaded: false,
  status: 'enabled',
});

/** 域名状态备注(非激活域名置灰) */
const DOMAIN_STATUS_NOTE: Record<string, string> = {
  pending: '待激活',
  failed: '校验失败',
  stopped: '已停用',
};

/** 域名下拉:仅 active 域名可关联(后端 validateLinkDomains 要求 active) */
const domainOptions = computed(() =>
  domains.value.map((d) => {
    const active = d.status === 'active';
    return {
      value: d.id,
      label: active ? d.fqdn : d.fqdn + '(' + (DOMAIN_STATUS_NOTE[d.status] ?? '不可用') + ')',
      disabled: !active,
    };
  }),
);

/** 短码说明:创建时说明生成规则,编辑时说明不可修改 */
const codeExtra = computed(() =>
  isEdit.value
    ? '短码创建后不可修改。完整短链地址为「https://<域名>/<短码>」,短码即地址最后一段。'
    : '留空则由系统按账号设置的默认长度自动生成。字符集仅含字母与数字,并去除易混淆字符 0/O/1/l/I。自定义短码最长 ' +
      SHORT_CODE_MAX_LENGTH +
      ' 位,创建后不可修改。',
);

const hasControlChars = (value: string) => /[\u0000-\u001f\u007f]/.test(value);

const rules: Record<string, FormRule[]> = {
  code: [
    {
      validator: (_rule, value: unknown) => {
        const v = value as string;
        if (!v) return Promise.resolve();
        if (!SHORT_CODE_PATTERN.test(v)) {
          return Promise.reject(new Error('短码仅允许字母与数字'));
        }
        if (SHORT_CODE_FORBIDDEN_PATTERN.test(v)) {
          return Promise.reject(new Error('短码不能包含易混淆字符 0/O/1/l/I'));
        }
        if (v.length > SHORT_CODE_MAX_LENGTH) {
          return Promise.reject(new Error('短码最长 ' + SHORT_CODE_MAX_LENGTH + ' 位'));
        }
        return Promise.resolve();
      },
    },
  ],
  targetUrls: [
    {
      validator: (_rule, value: unknown) => {
        if (!Array.isArray(value) || value.length === 0) {
          return Promise.reject(new Error('请至少填写一个目标 URL'));
        }
        const urls = value as string[];
        for (let i = 0; i < urls.length; i++) {
          const url = (urls[i] ?? '').trim();
          if (!url) {
            return Promise.reject(new Error('第 ' + (i + 1) + ' 个目标 URL 不能为空'));
          }
          if (hasControlChars(url)) {
            return Promise.reject(new Error('目标 URL 不能包含控制字符(换行/制表符等)'));
          }
          if (url.length > 4096) {
            return Promise.reject(new Error('目标 URL 最长 4096 字符'));
          }
        }
        return Promise.resolve();
      },
    },
  ],
  landingUrl: [
    {
      validator: (_rule, value: unknown) => {
        if (form.linkType !== 'landing' || form.landingSource !== 'url') {
          return Promise.resolve();
        }
        const url = String(value ?? '').trim();
        if (!url) {
          return Promise.reject(new Error('请填写落地页地址'));
        }
        if (hasControlChars(url)) {
          return Promise.reject(new Error('落地页地址不能包含控制字符(换行/制表符等)'));
        }
        if (url.length > 4096) {
          return Promise.reject(new Error('落地页地址最长 4096 字符'));
        }
        return Promise.resolve();
      },
    },
  ],
  domainIds: [
    {
      validator: (_rule, value: unknown) => {
        if (!Array.isArray(value) || value.length === 0) {
          return Promise.reject(new Error('请至少选择一个关联域名'));
        }
        return Promise.resolve();
      },
    },
  ],
};

/** 用短链详情回填表单(编辑模式) */
function applyLink(data: Link) {
  // 由 fqdn 列表反查域名 id
  const fqdnSet = new Set(data.domains);
  form.code = data.code;
  form.targetUrls = data.targetUrls.length > 0 ? [...data.targetUrls] : [''];
  form.redirectStatus = data.redirectStatus;
  form.linkType = data.linkType || 'redirect';
  form.landingSource = data.landingSource || 'url';
  form.landingUrl = data.landingUrl || '';
  form.landingUploaded = data.landingUploaded === true;
  form.status = data.status;
  form.domainIds = domains.value.filter((d) => fqdnSet.has(d.fqdn)).map((d) => d.id);
}

/** 创建模式的默认值 */
function applyDefaults() {
  form.code = '';
  form.targetUrls = [''];
  form.redirectStatus = '302';
  form.linkType = 'redirect';
  form.landingSource = 'url';
  form.landingUrl = '';
  form.landingUploaded = false;
  form.status = 'enabled';
  // 默认选中所有已激活域名
  form.domainIds = domains.value.filter((d) => d.status === 'active').map((d) => d.id);
}

async function loadDomains() {
  try {
    domains.value = await listDomains();
  } catch {
    // 域名加载失败不阻塞表单页;下拉为空时校验会提示选择域名
  }
}

async function init() {
  await loadDomains();
  if (route.name === 'link-edit') {
    const id = validLinkId.value;
    if (id === undefined) {
      message.error('无效的短链 ID');
      router.push({ name: 'links' });
      return;
    }
    loading.value = true;
    try {
      const data = await getLink(id);
      link.value = data;
      applyLink(data);
    } catch (error) {
      if (error instanceof ApiError) message.error(error.message);
      else message.error('加载短链失败,请稍后重试');
      router.push({ name: 'links' });
      return;
    } finally {
      loading.value = false;
    }
  } else {
    applyDefaults();
  }
}

onMounted(init);

/** 新增一行目标 URL(允许存在空行,提交前统一 trim 并在校验中提示空值) */
function addTargetUrl() {
  form.targetUrls.push('');
}

/** 删除指定行目标 URL(至少保留 1 行) */
function removeTargetUrl(index: number) {
  if (form.targetUrls.length <= 1) return;
  form.targetUrls.splice(index, 1);
}

/** 选择 zip:拦截默认上传;编辑时立即上传,创建时留待建链后上传 */
async function onSelectZip(file: File): Promise<boolean> {
  landingFile.value = file;
  if (isEdit.value && link.value) {
    await doUploadLanding(link.value.id);
  } else {
    message.info('落地页压缩包将在短链创建成功后自动上传');
  }
  return false;
}

/** 上传已选 zip(替换式) */
async function doUploadLanding(id: number) {
  if (!landingFile.value) return;
  landingUploading.value = true;
  try {
    const updated = await uploadLanding(id, landingFile.value);
    form.landingUploaded = updated.landingUploaded === true;
    message.success('落地页压缩包已上传');
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('上传失败,请稍后重试');
  } finally {
    landingUploading.value = false;
  }
}

type LinkPayload = {
  targetUrls: string[];
  domainIds: number[];
  redirectStatus?: RedirectStatus;
  linkType?: LinkType;
  landingSource?: LandingSource;
  landingUrl?: string;
};

async function onSubmit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  submitting.value = true;
  try {
    const payload: LinkPayload = {
      targetUrls: form.targetUrls.map((s) => s.trim()),
      domainIds: form.domainIds,
      linkType: form.linkType,
    };
    if (form.linkType === 'redirect') {
      payload.redirectStatus = form.redirectStatus;
    } else {
      payload.landingSource = form.landingSource;
      if (form.landingSource === 'url') {
        payload.landingUrl = form.landingUrl.trim();
      }
    }

    if (isEdit.value && link.value) {
      await updateLink(link.value.id, {
        ...payload,
        status: form.status,
      });
      message.success('短链已更新');
      // 编辑:落地页上传来源且已重新选择文件,则独立上传(替换式)
      if (form.linkType === 'landing' && form.landingSource === 'upload' && landingFile.value) {
        await doUploadLanding(link.value.id);
      }
    } else {
      const created = await createLink({
        ...payload,
        code: form.code.trim() || undefined,
      });
      message.success('短链已创建');
      // 创建:落地页上传来源且已选择文件,则建链后上传
      if (form.linkType === 'landing' && form.landingSource === 'upload' && landingFile.value) {
        await doUploadLanding(created.id);
      }
    }
    router.push({ name: 'links' });
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        const usage = getQuotaUsage(error.details);
        if (usage) {
          message.error('短链配额超限:当前 ' + usage.links + '/' + usage.maxLinks + ' 条短链,已达上限');
        } else {
          message.error('短链配额超限:' + error.message);
        }
      } else if (error.status === 409) {
        message.error('短码冲突:' + error.message);
      } else {
        message.error(error.message);
      }
    } else {
      message.error('保存失败,请稍后重试');
    }
  } finally {
    submitting.value = false;
  }
}

function goBack() {
  router.push({ name: 'links' });
}
</script>

<style scoped>
.form-card {
  max-width: 640px;
}
</style>
