// 主题状态:light / dark,class 策略挂在 <html> 上,持久化到 localStorage
import { computed, ref } from 'vue';
import { defineStore } from 'pinia';

export type ThemeMode = 'light' | 'dark';

const STORAGE_KEY = 'cloak-theme';

function initialMode(): ThemeMode {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === 'light' || saved === 'dark') return saved;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(initialMode());
  const isDark = computed(() => mode.value === 'dark');

  /** 把当前模式应用到 <html> 并持久化(必须在 app.mount 前调用一次,避免首屏闪白) */
  function apply(): void {
    document.documentElement.classList.toggle('dark', isDark.value);
    localStorage.setItem(STORAGE_KEY, mode.value);
  }

  function setMode(next: ThemeMode): void {
    mode.value = next;
    apply();
  }

  function toggle(): void {
    setMode(isDark.value ? 'light' : 'dark');
  }

  return { mode, isDark, setMode, toggle, apply };
});
