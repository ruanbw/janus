/**
 * 短链的多目标 URL 编辑（增删改、排序、校验）。
 *
 * 三点必须由这里统一保证：
 * 1. 每行有稳定 id —— 模板用它做 v-for key。用下标当 key 时，删掉第 1 行会让
 *    后面所有行「继承」前一行的 DOM 与输入状态，用户正在输入的内容会串行；
 * 2. 不允许删到 0 行 —— 短链没有目标 URL 就是死链，按钮 disabled 之外还要有逻辑兜底；
 * 3. 校验优先级固定 empty > invalidUrl > duplicate，同一份输入永远报同一句话，
 *    用户按提示改完就能过。
 *
 * 协议白名单与后端 validTargetURL 对齐（仅 http/https）：后端把 scheme 限定死，
 * 否则 javascript:/data:/file: 会被原样写进 Location 头。
 */
import { computed, ref } from 'vue';
import type { ComputedRef, Ref } from 'vue';

export interface TargetEntry {
  id: string;
  url: string;
}

export type TargetValidation =
  | { ok: true }
  | { ok: false; reason: 'empty' | 'invalidUrl' | 'duplicate'; index: number };

/**
 * 单调递增的序号：模块级而非实例级，保证同一页面里两个编辑器也不会撞 id。
 * 只需要「唯一」和「稳定」，不需要可读或可预测。
 */
let entrySeq = 0;

function createEntry(url: string): TargetEntry {
  entrySeq += 1;
  return { id: `target-${entrySeq}`, url };
}

/** 是否为带 host 的 http/https 绝对地址（相对路径、裸域名、mailto: 都不是） */
function isHttpUrl(raw: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    return false;
  }
  return parsed.protocol === 'http:' || parsed.protocol === 'https:';
}

export function useLinkTargets(initial?: string[]): {
  targets: Ref<TargetEntry[]>;
  urls: ComputedRef<string[]>;
  canRemove: ComputedRef<boolean>;
  add(url?: string): void;
  replace(id: string, url: string): void;
  remove(id: string): boolean;
  move(id: string, direction: -1 | 1): void;
  validate(): TargetValidation;
} {
  // 空数组同样回落到一条空行：后端要求至少一个目标 URL，表单不能是「零行」。
  const seed = initial && initial.length > 0 ? initial : [''];
  const targets = ref<TargetEntry[]>(seed.map((url) => createEntry(url)));

  /** 提交值口径：trim 后过滤空白行，顺序与 targets 一致 */
  const urls = computed(() => targets.value.map((entry) => entry.url.trim()).filter((url) => url !== ''));

  const canRemove = computed(() => targets.value.length > 1);

  function add(url = ''): void {
    targets.value = [...targets.value, createEntry(url)];
  }

  function replace(id: string, url: string): void {
    const entry = targets.value.find((item) => item.id === id);
    if (entry === undefined) return;
    // 就地改 url：数组与 id 都不变，v-for 不会重建这一行（输入焦点不丢）。
    entry.url = url;
  }

  function remove(id: string): boolean {
    if (!canRemove.value) return false;
    const next = targets.value.filter((entry) => entry.id !== id);
    if (next.length === targets.value.length) return false;
    targets.value = next;
    return true;
  }

  function move(id: string, direction: -1 | 1): void {
    const list = targets.value;
    const index = list.findIndex((entry) => entry.id === id);
    if (index === -1) return;
    const to = index + direction;
    // 越界就是不动，而不是钳到边界：第一条再往前等于「什么都不该发生」。
    if (to < 0 || to >= list.length) return;
    const next = [...list];
    next[index] = list[to];
    next[to] = list[index];
    targets.value = next;
  }

  function validate(): TargetValidation {
    const trimmed = targets.value.map((entry) => entry.url.trim());

    // 三种错误的优先级固定，先报的永远是更能指导用户改的那一条。
    const emptyIndex = trimmed.findIndex((url) => url === '');
    if (emptyIndex !== -1) return { ok: false, reason: 'empty', index: emptyIndex };

    const invalidIndex = trimmed.findIndex((url) => !isHttpUrl(url));
    if (invalidIndex !== -1) return { ok: false, reason: 'invalidUrl', index: invalidIndex };

    const seen = new Set<string>();
    for (let i = 0; i < trimmed.length; i++) {
      // 重复项报**后一次**出现的下标：让用户去改「新加的那条」，
      // 而不是改早已填好、别处可能在用的前一条。
      if (seen.has(trimmed[i])) return { ok: false, reason: 'duplicate', index: i };
      seen.add(trimmed[i]);
    }

    return { ok: true };
  }

  return { targets, urls, canRemove, add, replace, remove, move, validate };
}
