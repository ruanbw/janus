<template>
  <a-modal
    :open="open"
    :title="isEdit ? '编辑短链' : '创建短链'"
    :confirm-loading="submitting"
    ok-text="保存"
    cancel-text="取消"
    width="560px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
      <a-form-item
        name="code"
        label="短码"
        extra="留空则按你的默认长度自动生成;字符集不含 0/O/1/l/I"
      >
        <a-input
          v-model:value="form.code"
          placeholder="留空自动生成"
          :maxlength="32"
          :disabled="isEdit"
        />
      </a-form-item>

      <a-form-item name="targetUrl" label="目标 URL" extra="任意协议(如 https://、mailto:),不能包含控制字符">
        <a-input v-model:value="form.targetUrl" placeholder="https://example.com/page" />
      </a-form-item>

      <a-form-item
        name="domainIds"
        label="关联域名"
        extra="平台默认域名与自有域名均可关联;停用/未激活域名下短码不可访问"
      >
        <a-select
          v-model:value="form.domainIds"
          mode="multiple"
          placeholder="选择关联域名"
          :options="domainOptions"
          :max-tag-count="3"
        />
      </a-form-item>

      <a-form-item name="redirectStatus" label="重定向方式">
        <a-radio-group v-model:value="form.redirectStatus">
          <a-radio value="302">临时重定向(302)</a-radio>
          <a-radio value="301">永久重定向(301)</a-radio>
        </a-radio-group>
      </a-form-item>

      <a-form-item v-if="isEdit" name="status" label="状态">
        <a-radio-group v-model:value="form.status">
          <a-radio value="enabled">启用</a-radio>
          <a-radio value="disabled">停用</a-radio>
        </a-radio-group>
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { message } from 'ant-design-vue';
import type { FormInstance, Rule } from 'ant-design-vue/es/form';

import { createLink, updateLink } from '@/api/links';
import { ApiError } from '@/types/api';
import type { Domain, Link, RedirectStatus } from '@/types/api';

const props = defineProps<{
  open: boolean;
  /** 编辑对象;为空表示创建 */
  link: Link | null;
  /** 可选关联域名列表 */
  domains: Domain[];
}>();

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void;
  (e: 'saved'): void;
}>();

const formRef = ref<FormInstance>();
const submitting = ref(false);

const isEdit = computed(() => props.link !== null);

const form = reactive<{
  code: string;
  targetUrl: string;
  domainIds: number[];
  redirectStatus: RedirectStatus;
  status: 'enabled' | 'disabled';
}>({
  code: '',
  targetUrl: '',
  domainIds: [],
  redirectStatus: '302',
  status: 'enabled',
});

/** 域名下拉:停用/未激活域名标注不可用 */
const domainOptions = computed(() =>
  props.domains.map((d) => {
    const usable = d.status === 'active' || d.status === 'pending';
    return {
      value: d.id,
      label: usable ? d.fqdn : `${d.fqdn}(${d.status === 'active' ? '' : '未激活或已停用'})`,
      disabled: !usable,
    };
  }),
);

const hasControlChars = (value: string) => /[\u0000-\u001f\u007f]/.test(value);
const hasConfusableChar = (value: string) => /[0O1lI]/.test(value);

const rules: Record<string, Rule[]> = {
  code: [
    {
      validator: (_rule, value: string) => {
        if (!value) return Promise.resolve();
        if (!/^[a-zA-Z0-9]+$/.test(value)) {
          return Promise.reject(new Error('短码仅允许字母与数字'));
        }
        if (hasConfusableChar(value)) {
          return Promise.reject(new Error('短码不能包含易混淆字符 0/O/1/l/I'));
        }
        if (value.length > 32) {
          return Promise.reject(new Error('短码最长 32 位'));
        }
        return Promise.resolve();
      },
    },
  ],
  targetUrl: [
    { required: true, message: '请输入目标 URL' },
    {
      validator: (_rule, value: string) => {
        if (!value) return Promise.resolve();
        if (hasControlChars(value)) {
          return Promise.reject(new Error('目标 URL 不能包含控制字符(换行/制表符等)'));
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

// 打开时同步表单
watch(
  () => props.open,
  (open) => {
    if (!open) return;
    if (props.link) {
      // 编辑:由 fqdn 列表反查域名 id
      const fqdnSet = new Set(props.link.domains);
      form.code = props.link.code;
      form.targetUrl = props.link.targetUrl;
      form.redirectStatus = props.link.redirectStatus;
      form.status = props.link.status;
      form.domainIds = props.domains.filter((d) => fqdnSet.has(d.fqdn)).map((d) => d.id);
    } else {
      form.code = '';
      form.targetUrl = '';
      form.redirectStatus = '302';
      form.status = 'enabled';
      // 默认选中所有可用(激活)域名
      form.domainIds = props.domains
        .filter((d) => d.status === 'active')
        .map((d) => d.id);
    }
  },
);

async function onSubmit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  submitting.value = true;
  try {
    const payload = {
      targetUrl: form.targetUrl.trim(),
      domainIds: form.domainIds,
      redirectStatus: form.redirectStatus,
    };
    if (isEdit.value && props.link) {
      await updateLink(props.link.id, {
        ...payload,
        status: form.status,
      });
      message.success('短链已更新');
    } else {
      await createLink({
        ...payload,
        code: form.code.trim() || undefined,
      });
      message.success('短链已创建');
    }
    emit('update:open', false);
    emit('saved');
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        message.error(`短链配额超限:${error.message}`);
      } else if (error.status === 409) {
        message.error(`短码冲突:${error.message}`);
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

function onCancel() {
  emit('update:open', false);
}
</script>
