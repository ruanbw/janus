<template>
  <DropdownMenuRoot>
    <DropdownMenuTrigger as-child>
      <button
        ref="trigger"
        type="button"
        aria-label="切换主题"
        class="flex h-8 w-8 items-center justify-center rounded-lg text-ink-soft transition-colors hover:bg-surface-strong hover:text-ink"
        :title="`主题:${currentLabel}`"
      >
        <component :is="ICONS[theme.mode]" :size="16" />
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent
        :side-offset="6"
        align="end"
        class="z-[75] min-w-40 rounded-xl border border-line bg-surface p-1.5 shadow-xl"
      >
        <DropdownMenuItem
          v-for="option in OPTIONS"
          :key="option.value"
          class="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-[13px] text-ink outline-none data-[highlighted]:bg-brand-50 data-[highlighted]:text-brand-700 dark:data-[highlighted]:bg-brand-500/15 dark:data-[highlighted]:text-brand-300"
          @select="onSelect(option.value)"
        >
          <component :is="option.icon" :size="14" class="shrink-0 text-ink-soft" />
          <span class="flex-1">{{ option.label }}</span>
          <Check
            v-if="theme.mode === option.value"
            :size="14"
            class="shrink-0 text-brand-600 dark:text-brand-400"
          />
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { Check, Monitor, Moon, Sun } from '@lucide/vue';
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuTrigger,
} from 'reka-ui';

import { useThemeStore, type ThemeMode } from '@/stores/theme';

const theme = useThemeStore();

const trigger = ref<HTMLButtonElement | null>(null);

const OPTIONS: { value: ThemeMode; label: string; icon: typeof Sun }[] = [
  { value: 'light', label: '浅色', icon: Sun },
  { value: 'dark', label: '深色', icon: Moon },
  { value: 'system', label: '跟随系统', icon: Monitor },
];

const ICONS = { light: Sun, dark: Moon, system: Monitor } as const;
const LABELS = { light: '浅色', dark: '深色', system: '跟随系统' } as const;

/** 跟随系统时带上当前解析出来的实际模式,浅色系统下也能看出深色图标 */
const currentLabel = computed(() =>
  theme.mode === 'system'
    ? `${LABELS.system}(${theme.isDark ? '深色' : '浅色'})`
    : LABELS[theme.mode],
);

/**
 * 圆形展开动画以切换按钮中心为圆心(不是点击点 —— 点击点在菜单里,方向感会跑偏)。
 * 延后两帧再切主题:让 reka-ui 的菜单先完成关闭并从 DOM 卸载,
 * 否则菜单会被定格进过渡的新快照里,动画结束后才突然消失。
 */
function onSelect(next: ThemeMode): void {
  const el = trigger.value;
  const origin = el
    ? { x: el.getBoundingClientRect().left + el.offsetWidth / 2, y: el.getBoundingClientRect().top + el.offsetHeight / 2 }
    : { x: window.innerWidth / 2, y: 0 };
  requestAnimationFrame(() =>
    requestAnimationFrame(() => {
      void theme.setModeAnimated(next, origin);
    }),
  );
}
</script>
