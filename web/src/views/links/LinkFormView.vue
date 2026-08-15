<template>
  <div>
    <PageHeader :title="isEdit ? '编辑短链' : '创建短链'" :description="headerDescription">
      <template #actions>
        <a-button @click="goBack">
          <template #icon><ArrowLeftOutlined /></template>
          返回
        </a-button>
      </template>
    </PageHeader>

    <a-card :bordered="false" class="form-card">
      <a-spin :spinning="loading">
        <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
          <a-form-item name="code" label="短码" :extra="codeExtra">
            <a-input
              v-model:value="form.code"
              placeholder="留空自动生成"
              :maxlength="SHORT_CODE_MAX_LENGTH"
              :disabled="isEdit"
            />
          </a-form-item>

          <a-form-item
            name="linkType"
            label="短链类型"
            extra="跳转型:访问短链后立即重定向到目标 URL(支持多个按顺序轮询)。落地页型:先展示一个中间落地页(在线地址或上传的压缩包),访问者点击后才跳转,可单独统计「点击」数。"
          >
            <a-radio-group v-model:value="form.linkType">
              <a-radio value="redirect">跳转型</a-radio>
              <a-radio value="landing">落地页型</a-radio>
            </a-radio-group>
          </a-form-item>

          <a-form-item
            name="targetUrls"
            label="目标 URL"
            extra="支持配置多个目标 URL,访问时按从上到下的顺序轮询分发。支持任意协议(如 https://、http://、mailto:)。每个 URL 不能包含换行/制表符等控制字符,单个最长 4096 字符。"
          >
            <div class="target-url-list">
              <div
                v-for="(_, index) in form.targetUrls"
                :key="index"
                class="target-url-row"
              >
                <a-input
                  v-model:value="form.targetUrls[index]"
                  placeholder="https://example.com/page"
                />
                <a-button
                  v-if="form.targetUrls.length > 1"
                  type="text"
                  danger
                  class="target-url-remove"
                  @click="removeTargetUrl(index)"
                >
                  <template #icon><MinusCircleOutlined /></template>
                </a-button>
              </div>
              <a-button type="dashed" block class="target-url-add" @click="addTargetUrl">
                <template #icon><PlusOutlined /></template>
                添加目标 URL
              </a-button>
            </div>
          </a-form-item>

          <template v-if="form.linkType === 'landing'">
            <a-form-item
              name="landingSource"
              label="落地页来源"
              extra="URL 地址:落地页直接指向一个在线地址,访问时先重定向到该地址。上传压缩包:上传一个静态站点 zip(必须包含 index.html),由本服务托管,访问时直接展示。"
            >
              <a-radio-group v-model:value="form.landingSource">
                <a-radio value="url">URL 地址</a-radio>
                <a-radio value="upload">上传压缩包</a-radio>
              </a-radio-group>
            </a-form-item>

            <a-form-item
              v-if="form.landingSource === 'url'"
              name="landingUrl"
              label="落地页地址"
              extra="访问落地页型短链时,先重定向到此地址。不能包含控制字符,最长 4096 字符。仅在「落地页来源 = URL 地址」时填写。"
            >
              <a-input v-model:value="form.landingUrl" placeholder="https://example.com/landing" />
            </a-form-item>

            <a-form-item
              v-else
              name="landingFile"
              label="落地页压缩包"
              extra="压缩包必须包含 index.html,作为落地页入口。上传为替换式:再次上传会覆盖旧版本,成功后立即生效,无需重新创建短链。仅支持 .zip 格式。"
            >
              <div class="landing-upload">
                <a-space>
                  <a-upload accept=".zip" :show-upload-list="false" :before-upload="onSelectZip">
                    <a-button :loading="landingUploading">
                      <template #icon><UploadOutlined /></template>
                      {{ form.landingUploaded ? '重新上传压缩包' : '选择 zip 压缩包' }}
                    </a-button>
                  </a-upload>
                  <a-tag v-if="form.landingUploaded" color="success">已上传</a-tag>
                  <a-tag v-else color="default">未上传</a-tag>
                </a-space>
                <div v-if="landingFile" class="landing-file-name">待上传:{{ landingFile.name }}</div>
              </div>
            </a-form-item>
          </template>

          <a-form-item
            name="domainIds"
            label="关联域名"
            extra="仅已激活(active)的域名可关联短链;待激活、校验失败、已停用的域名置灰不可选。同一条短码可在不同域名下指向不同目标,至少选择一个域名。"
          >
            <a-select
              v-model:value="form.domainIds"
              mode="multiple"
              placeholder="选择关联域名"
              :options="domainOptions"
              :max-tag-count="3"
            />
          </a-form-item>

          <a-form-item
            v-if="form.linkType === 'redirect'"
            name="redirectStatus"
            label="重定向方式"
            extra="临时重定向(302):浏览器与搜索引擎不缓存跳转,目标变更后立即生效,适合经常调整目标的场景。永久重定向(301):浏览器与搜索引擎会缓存跳转,目标变更后旧地址可能长期命中缓存。"
          >
            <a-radio-group v-model:value="form.redirectStatus">
              <a-radio value="302">临时重定向(302)</a-radio>
              <a-radio value="301">永久重定向(301)</a-radio>
            </a-radio-group>
          </a-form-item>

          <a-form-item
            v-if="isEdit"
            name="status"
            label="状态"
            extra="启用:短链正常解析并计入访问统计。停用:短链保留但访问时返回未命中(404),可随时重新启用。"
          >
            <a-radio-group v-model:value="form.status">
              <a-radio value="enabled">启用</a-radio>
              <a-radio value="disabled">停用</a-radio>
            </a-radio-group>
          </a-form-item>

          <a-form-item>
            <a-space>
              <a-button type="primary" :loading="submitting" @click="onSubmit">保存</a-button>
              <a-button @click="goBack">取消</a-button>
            </a-space>
          </a-form-item>
        </a-form>
      </a-spin>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import {
  ArrowLeftOutlined,
  MinusCircleOutlined,
  PlusOutlined,
  UploadOutlined,
} from '@ant-design/icons-vue';
import type { FormInstance, Rule } from 'ant-design-vue/es/form';

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

const route = useRoute();
const router = useRouter();

const rawId = route.params.id;
const linkId = typeof rawId === 'string' ? Number(rawId) : undefined;
const isEdit = computed(() => linkId !== undefined);

const headerDescription = computed(() =>
  isEdit.value
    ? '修改短链的目标、类型、落地页与关联域名;短码创建后不可修改'
    : '选择短链类型、目标与关联域名,提交后立即生效;短码可留空自动生成',
);

const formRef = ref<FormInstance>();
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

const rules: Record<string, Rule[]> = {
  code: [
    {
      validator: (_rule, value: string) => {
        if (!value) return Promise.resolve();
        if (!SHORT_CODE_PATTERN.test(value)) {
          return Promise.reject(new Error('短码仅允许字母与数字'));
        }
        if (SHORT_CODE_FORBIDDEN_PATTERN.test(value)) {
          return Promise.reject(new Error('短码不能包含易混淆字符 0/O/1/l/I'));
        }
        if (value.length > SHORT_CODE_MAX_LENGTH) {
          return Promise.reject(new Error('短码最长 ' + SHORT_CODE_MAX_LENGTH + ' 位'));
        }
        return Promise.resolve();
      },
    },
  ],
  targetUrls: [
    {
      validator: (_rule, value: string[]) => {
        if (!Array.isArray(value) || value.length === 0) {
          return Promise.reject(new Error('请至少填写一个目标 URL'));
        }
        for (let i = 0; i < value.length; i++) {
          const url = (value[i] ?? '').trim();
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
      validator: (_rule, value: string) => {
        if (form.linkType !== 'landing' || form.landingSource !== 'url') {
          return Promise.resolve();
        }
        const url = (value ?? '').trim();
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
      validator: (_rule, value: number[]) => {
        if (!value || value.length === 0) {
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
  if (isEdit.value && linkId !== undefined) {
    loading.value = true;
    try {
      const data = await getLink(linkId);
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

/** 选择 zip:拦截 antd 默认上传;编辑时立即上传,创建时留待建链后上传 */
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

.target-url-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.target-url-row {
  display: flex;
  align-items: center;
  gap: 4px;
}

.target-url-remove {
  flex-shrink: 0;
}

.target-url-add {
  width: 100%;
}

.landing-upload {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.landing-file-name {
  font-size: 12px;
  color: #334155;
  word-break: break-all;
}
</style>
