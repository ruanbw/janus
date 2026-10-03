/**
 * 条形图的「扫出」动效：让一组比例值在数据到位时从 0 长到真实值。
 *
 * 为什么不用 CSS 直接 transition `width`：
 * `width` 是布局属性，二十多根条形同时跑 width 过渡会逐帧重排，而这张总览页
 * 每屏就有二十多根条。条形只需要「从左长出来」这一个效果，transform 就能表达，
 * 且完全跑在合成器上。所以 state 里存的是**当前应当显示的比例**，渲染时用它算
 * `scaleX`。
 *
 * 两段行为是刻意分开的：
 * - **入场**（首屏骨架屏 → 实数）：条形从 0 扫到目标，数字与图形作为同一个对象
 *   落定。实现上不需要"先写 0 再抬"——map 是空的，v-for 新插入的条形首次渲染
 *   就是 scaleX(0)，所以只要把目标值推迟到下一帧再写进去，过渡自然从 0 起跑。
 * - **刷新**：map 里还留着上次的比例，条形先以旧值渲染，watcher 随即写入新值，
 *   CSS 过渡自己把它滑过去（含变短的方向）。刷新不是一次新事件，不重播。
 *
 * 键必须**跨卡片稳定且互不重名**：维度用 `前缀:维度名`，排行用 `link:短链id`。
 * 键若不稳定（哪怕只是随数据变短），改一次就会让某根条形从 0 重播，
 * 看起来像冒出了一批新数据。
 *
 * 减弱动效的路径是**不播**，而不是播一个更短的版本：系统已经表明用户不想看位移，
 * 这时内容必须立刻是终值。所以 reduced-motion 下第一帧就是终值，绝不是 scaleX(0)
 * （那等于内容不可见，是故障而不是克制）。CSS 侧把过渡时长归零只是同一结论的
 * 第二道保险，两边落到同一个结局。
 */
import { onScopeDispose, reactive, readonly, ref, watch } from 'vue';
import { usePreferredReducedMotion } from '@vueuse/core';

/** 零星条目也要看得见：不足 2% 时按 2% 画，旁边的比例文字仍然是真实值 */
export const SWEEP_MIN_RATIO = 0.02;

/** 目标比例 → 实际用来画的。0 仍是 0：不给"确实没有访问"的行画一根残条。 */
export function sweepRatio(ratio: number): number {
  if (!(ratio > 0)) return 0;
  return Math.min(1, Math.max(SWEEP_MIN_RATIO, ratio));
}

export interface SweepRow {
  /** 行的稳定标识（见文件头「键」的约定），决定这次动效作用在哪一根条形上 */
  key: string;
  /** 目标比例，0–1 */
  ratio: number;
}

/** 扫出刚发生（正在发生）的那一小段时间：数字在这段时间里略微让位 */
const SETTLE_MS = 300;

export function useSweep(rows: () => SweepRow[]) {
  const reducedMotion = usePreferredReducedMotion();
  /** key → 当前应当显示的比例；渲染读它，watcher 写它 */
  const shown = reactive(new Map<string, number>());
  /** 已播过入场。只有首屏播，刷新不播。 */
  let primed = false;
  /** 扫出期间为 true，让数字让位、随条形一起落定 */
  const settling = ref(false);
  let settleTimer = 0;
  let frame = 0;

  function clearTimers() {
    window.clearTimeout(settleTimer);
    window.clearTimeout(frame);
  }

  watch(
    rows,
    (list) => {
      clearTimers();

      // 没有行 = 还在骨架屏 / 零数据。此刻"抬到目标"等于把一堆 0 写进 map，
      // 后面真数据到达时就不再是入场（primed 被误消耗掉），动画会消失。
      if (list.length === 0) return;

      // 已经离开的维度不留残值：维度集是固定的，但排行会随数据变短，
      // 漏掉这条就会让一根已经不存在的条形继续被 value() 读到。
      const keys = new Set(list.map((row) => row.key));
      for (const key of [...shown.keys()]) {
        if (!keys.has(key)) shown.delete(key);
      }

      const apply = () => {
        for (const row of list) shown.set(row.key, row.ratio);
      };

      if (reducedMotion.value || primed) {
        primed = true;
        settling.value = false;
        apply();
        return;
      }

      primed = true;
      // 推迟到下一帧，确保浏览器先把 scaleX(0) 画出去，再开始过渡。
      frame = window.requestAnimationFrame(() => {
        apply();
        settling.value = true;
        settleTimer = window.setTimeout(() => {
          settling.value = false;
        }, SETTLE_MS);
      });
    },
    // post：条形要先随新数据渲染出来（此刻 scaleX 还是上一次的旧值），
    // 再由 watcher 写入目标值；否则刷新会从"滑动"退化成"重播"。
    { flush: 'post' },
  );

  onScopeDispose(clearTimers);

  /**
   * 渲染期读取：未登记的键按 0 处理，即"还没长出来"，而不是"默认满格"。
   * reduced-motion 下首帧就是终值，所以不会出现一根永远停在 0 的条形。
   */
  function value(key: string): number {
    return shown.get(key) ?? 0;
  }

  // readonly 而非 computed(settling)：computed 收的是 getter，传 ref 进去会被
  // 当成函数调用。settling 本身已是 ref，脚本侧也不该改它。
  return { value, settling: readonly(settling) };
}