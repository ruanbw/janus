<template>
  <div
    class="animate-pulse bg-surface-strong"
    :class="classes"
    :style="customStyle"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue';

interface Props {
  variant?: 'text' | 'rect' | 'circle';
  width?: string | number;
  height?: string | number;
  rounded?: string;
  class?: string;
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'text',
  width: undefined,
  height: undefined,
  rounded: undefined,
  class: '',
});

const classes = computed(() => {
  const list: string[] = [];

  if (props.variant === 'circle') {
    list.push('rounded-full shrink-0');
  } else if (props.variant === 'rect') {
    list.push(props.rounded || 'rounded-lg');
  } else {
    // text
    list.push(props.rounded || 'rounded-md');
  }

  if (props.class) {
    list.push(props.class);
  }

  return list.join(' ');
});

const customStyle = computed(() => {
  const style: Record<string, string> = {};

  if (props.width !== undefined) {
    style.width = typeof props.width === 'number' ? `${props.width}px` : props.width;
  } else if (props.variant === 'circle') {
    style.width = '2.5rem';
  }

  if (props.height !== undefined) {
    style.height = typeof props.height === 'number' ? `${props.height}px` : props.height;
  } else if (props.variant === 'circle') {
    style.height = '2.5rem';
  } else if (props.variant === 'text') {
    style.height = '1rem';
  } else {
    style.height = '3rem';
  }

  return style;
});
</script>
