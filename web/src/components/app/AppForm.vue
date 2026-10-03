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

/**
 * 校验全部已注册字段。
 *
 * 契约（务必保持）：resolve boolean，**永不 reject**。校验未通过是预期控制流，
 * 不是异常 —— 抛出去会流进应用的错误处理机制，把例行的「填错了」变成报错。
 *
 * 调用方必须检查返回值：
 *
 * ```ts
 * const ok = await formRef.value?.validate();
 * if (!ok) return;               // ← 正确
 *
 * try { await formRef.value?.validate(); } catch { return; }   // ← 错误：catch 是死代码
 * ```
 *
 * 后一种写法会让校验被静默跳过、请求照发（表单非法时仍 POST，表现为后端 4xx 误报）。
 * 本仓库曾因此复制出 4 份同样的 bug，正确范本见 `RuleFormView.vue` 的 `onSubmit`。
 */
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

// 暴露给模板 ref：返回 Promise<boolean>，调用方需判断返回值（见 validateAll 的契约说明）
defineExpose({ validate: validateAll });
</script>
