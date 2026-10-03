/**
 * vitest 全局装置（setupFiles）。
 * 原则:happy-dom 已内置的能力不重复 polyfill,只在缺失时兜底,
 * 避免掩盖真实运行时的行为差异。
 */
import { afterEach, beforeEach } from 'vitest';

// 1. 浏览器存储隔离:测试间残留 local/sessionStorage 会造成顺序相关失败
beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

// 2. matchMedia:happy-dom 默认不实现,组件里的 prefers-color-scheme / 响应式断点会炸
if (typeof globalThis.matchMedia !== 'function') {
  globalThis.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as typeof globalThis.matchMedia;
}

// 3. ResizeObserver:reka-ui / 图表容器挂载时会构造它
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof globalThis.ResizeObserver;
}

// 4. crypto.randomUUID:用于生成测试数据的 ID
if (typeof globalThis.crypto?.randomUUID !== 'function') {
  let seq = 0;
  const randomUUID = () => {
    seq += 1;
    const hex = seq.toString(16).padStart(12, '0');
    return `00000000-0000-4000-8000-${hex}` as `${string}-${string}-${string}-${string}-${string}`;
  };
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    value: randomUUID,
    configurable: true,
    writable: true,
  });
}