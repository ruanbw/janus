// 主题状态:light / dark / system,class 策略挂在 <html> 上,持久化到 localStorage
import { computed, nextTick, ref } from 'vue';
import { defineStore } from 'pinia';

export type ThemeMode = 'light' | 'dark' | 'system';
/** 实际生效的模式:system 会被解析为 light 或 dark */
export type ResolvedTheme = 'light' | 'dark';
/** 圆形展开动画的圆心,通常是切换按钮的中心 */
export interface TransitionOrigin {
  x: number;
  y: number;
}

const STORAGE_KEY = 'janus-theme';
const DARK_QUERY = '(prefers-color-scheme: dark)';
/** 移动端/桌面端浏览器地址栏跟随页面底色,随主题切换保持一致 */
const THEME_COLOR: Record<ResolvedTheme, string> = { light: '#ffffff', dark: '#10141d' };

/** 圆形展开动画时长,与 main.css 里的 view-transition 规则配套 */
const TRANSITION_MS = 400;
/**
 * 切主题的瞬间禁用全站 transition。
 * 不禁用的话带 transition-colors 的元素会从旧色渐变到新色,新快照截在渐变起点,
 * 圆环展开结束后真实 DOM 还会再跳一次色。方案同 paco.me/writing/disable-theme-transitions
 * 与 VueUse 的 useColorMode(disableTransition)。
 * 只禁 transition 不禁 animation,否则 AppSpin 的 animate-spin 会在 400ms 内僵住。
 */
const CSS_DISABLE_TRANS =
  '*,*::before,*::after{transition:none!important;-webkit-transition:none!important}';

interface ViewTransition {
  readonly ready: Promise<void>;
  readonly finished: Promise<void>;
}

/** startViewTransition / pseudoElement 尚未进入 TS DOM lib,这里补最小类型 */
function startViewTransition(update: () => Promise<void>): ViewTransition | null {
  const start = (document as Document & { startViewTransition?: unknown }).startViewTransition;
  return typeof start === 'function'
    ? (start as (fn: () => Promise<void>) => ViewTransition).call(document, update)
    : null;
}

function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function initialMode(): ThemeMode {
  let saved: string | null = null;
  try {
    saved = localStorage.getItem(STORAGE_KEY);
  } catch {
    // 忽略 localStorage 访问限制异常(隐私模式 / 受限存储环境),按首次访问处理
    saved = null;
  }
  if (saved === 'light' || saved === 'dark' || saved === 'system') return saved;
  // 首次访问默认跟随系统
  return 'system';
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(initialMode());
  /** 跟随系统时,系统当前报告的深浅色 */
  const systemDark = ref(window.matchMedia(DARK_QUERY).matches);
  /** 过渡进行中:重入时直接硬切,避免 startViewTransition 被跳过导致动画错乱 */
  let animating = false;

  /** 实际作用于 <html class="dark"> 的模式 */
  const resolved = computed<ResolvedTheme>(() =>
    mode.value === 'system' ? (systemDark.value ? 'dark' : 'light') : mode.value,
  );
  const isDark = computed(() => resolved.value === 'dark');

  /** 把当前模式应用到 <html> 并持久化(必须在 app.mount 前调用一次,避免首屏闪白) */
  function apply(): void {
    document.documentElement.classList.toggle('dark', isDark.value);
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', THEME_COLOR[resolved.value]);
    try {
      localStorage.setItem(STORAGE_KEY, mode.value);
    } catch {
      // 忽略 localStorage 访问限制异常(隐私模式 / 受限存储环境):主题本次生效,只是不持久化
    }
  }

  function setMode(next: ThemeMode): void {
    mode.value = next;
    apply();
  }

  /**
   * 带圆形展开动画切换模式:以 origin 为圆心,把新主题从圆点"擦"出来。
   *
   * 思路来自 antfu.me 的 toggleDark(credit to @hooray,vuejs/vitepress#2347):
   * View Transitions 默认是整页交叉淡入淡出,这里把它关掉,改成对
   * `::view-transition-old/new(root)` 之一做 clip-path 圆环缩放。
   *
   * 切到深色时是「旧的浅色快照盖在上层被擦掉」,方向与切到浅色相反;
   * 哪一层在上层由 main.css 里 `.dark::view-transition-*` 的 z-index 决定,
   * 因为 class 是在过渡回调里改的,快照绘制时读到的是新状态。
   */
  async function setModeAnimated(next: ThemeMode, origin: TransitionOrigin): Promise<void> {
    if (next === mode.value) return;
    if (animating || prefersReducedMotion()) {
      setMode(next);
      return;
    }

    const { x, y } = origin;
    // 到最远角的距离,保证圆能盖满整个视口
    const endRadius = Math.hypot(
      Math.max(x, window.innerWidth - x),
      Math.max(y, window.innerHeight - y),
    );

    const style = document.createElement('style');
    style.textContent = CSS_DISABLE_TRANS;
    document.head.append(style);
    // 读布局属性强制回流,让禁用 transition 的样式先于 class 变更生效
    void document.documentElement.offsetWidth;

    const transition = startViewTransition(async () => {
      mode.value = next;
      apply();
      // 等 DOM 落地,新快照才截得到新主题
      await nextTick();
    });
    if (!transition) {
      style.remove();
      setMode(next);
      return;
    }
    animating = true;

    try {
      // 被后续过渡跳过时会 reject,此时不该再动画
      const ready = await transition.ready.then(
        () => true,
        () => false,
      );
      if (ready) {
        const clipPath = [
          `circle(0px at ${x}px ${y}px)`,
          `circle(${endRadius}px at ${x}px ${y}px)`,
        ];
        const dark = isDark.value;
        const root = document.documentElement as HTMLElement & {
          animate(
            keyframes: Keyframe[],
            options: KeyframeAnimationOptions & { pseudoElement: string },
          ): Animation;
        };
        try {
          root.animate(
            { clipPath: dark ? [...clipPath].reverse() : clipPath },
            {
              duration: TRANSITION_MS,
              easing: 'ease-out',
              fill: 'forwards',
              pseudoElement: dark ? '::view-transition-old(root)' : '::view-transition-new(root)',
            },
          );
        } catch {
          // 忽略 Web Animations 对 pseudoElement 支持不全导致的 NotSupportedError:
          // 主题此时已经切换成功,只是缺少圆环收尾动画,不该因此抛给调用方
        }
        await transition.finished.catch(() => undefined);
      }
    } finally {
      style.remove();
      animating = false;
    }
  }

  // 跟随系统模式下,系统主题在运行时切换要立即生效,无需刷新
  window.matchMedia(DARK_QUERY).addEventListener('change', (event) => {
    systemDark.value = event.matches;
    if (mode.value === 'system') apply();
  });

  return { mode, resolved, isDark, setMode, setModeAnimated, apply };
});
