<template>
  <span class="inline-flex" role="button" tabindex="0" @click="pick" @keydown.enter="pick">
    <slot />
    <input
      ref="inputRef"
      type="file"
      class="hidden"
      :accept="accept"
      :disabled="disabled"
      @change="onChange"
    />
  </span>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const props = withDefaults(
  defineProps<{
    accept?: string;
    disabled?: boolean;
    /** 返回 false 则阻止继续(拦截默认上传,与 antd before-upload 语义一致) */
    beforeUpload?: (file: File) => boolean | Promise<boolean>;
  }>(),
  { disabled: false },
);

const emit = defineEmits<{ select: [file: File] }>();

const inputRef = ref<HTMLInputElement | null>(null);

function pick(): void {
  if (props.disabled) return;
  inputRef.value?.click();
}

async function onChange(event: Event): Promise<void> {
  const el = event.target as HTMLInputElement;
  const file = el.files && el.files.length > 0 ? el.files[0] : null;
  // 允许重复选择同一文件
  el.value = '';
  if (file === null) return;
  if (props.beforeUpload) {
    const allowed = await props.beforeUpload(file);
    if (allowed === false) return;
  }
  emit('select', file);
}
</script>
