<template>
  <form class="app-form" :class="props.class" @submit.prevent="onSubmit">
    <slot />
  </form>
</template>

<script setup lang="ts">
import { provide, reactive, ref } from 'vue';

import { formContextKey } from './form';
import type { FormItemContext } from './form';
import type { FormSchema } from './form';

const props = defineProps<{
  model: Record<string, unknown>;
  schema?: FormSchema;
  class?: any;
}>();

const emit = defineEmits<{
  finish: [values: Record<string, unknown>];
}>();

const items = new Map<string, FormItemContext>();
const submitCount = ref(0);

const context = reactive({
  model: props.model,
  schema: props.schema ?? {},
  submitCount,
  registerItem(ctx: FormItemContext): void {
    items.set(ctx.name, ctx);
  },
  unregisterItem(ctx: FormItemContext): void {
    if (items.get(ctx.name) === ctx) {
      items.delete(ctx.name);
    }
  },
});

provide(formContextKey, context);

/** 校验全部已注册字段;全部通过返回 true */
async function validateAll(): Promise<boolean> {
  let allOk = true;
  for (const ctx of items.values()) {
    const ok = await ctx.validate();
    if (ok === false) allOk = false;
  }
  return allOk;
}

async function onSubmit(): Promise<void> {
  submitCount.value += 1;
  const ok = await validateAll();
  if (ok) {
    emit('finish', props.model);
  }
}

defineExpose({ validate: validateAll });
</script>
