import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useThemeStore } from '@/stores/theme';
import { breakStorage, restoreStorage } from './restricted-storage';

afterEach(() => {
  restoreStorage('localStorage');
  Reflect.deleteProperty(document, 'startViewTransition');
  vi.restoreAllMocks();
});

/** 模拟受限存储环境(Firefox dom.storage.enabled=false / sandbox iframe / 企业策略) */
function breakLocalStorage(): void {
  breakStorage('localStorage');
}

/** 让 startViewTransition 可用,并让 root.animate 因不支持 pseudoElement 而抛 NotSupportedError */
function stubUnsupportedPseudoElementAnimation(): void {
  const startViewTransition = (update: () => Promise<void>) => {
    void update();
    return { ready: Promise.resolve(), finished: Promise.resolve() };
  };
  Object.defineProperty(document, 'startViewTransition', {
    configurable: true,
    writable: true,
    value: startViewTransition,
  });
  vi.spyOn(document.documentElement, 'animate').mockImplementation(() => {
    throw new DOMException(
      "Failed to execute 'animate' on 'Element': The provided double value is non-finite.",
      'NotSupportedError',
    );
  });
}

describe('stores/theme 存储受限', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    document.documentElement.classList.remove('dark');
  });

  it('localStorage.getItem 抛 SecurityError 时,建 store 与读取模式都不炸', () => {
    breakLocalStorage();

    const store = useThemeStore();

    expect(store.mode).toBe('system');
  });

  it('localStorage 写不进去时 apply() 仍把主题落到 <html>', () => {
    breakLocalStorage();
    const store = useThemeStore();

    expect(() => store.apply()).not.toThrow();

    store.setMode('dark');
    expect(store.isDark).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });
});

describe('stores/theme 动画收尾失败', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    document.documentElement.classList.remove('dark');
  });

  it('root.animate 抛 NotSupportedError 时 setModeAnimated 不 reject,且主题已切换', async () => {
    stubUnsupportedPseudoElementAnimation();
    const store = useThemeStore();

    await expect(store.setModeAnimated('dark', { x: 10, y: 10 })).resolves.toBeUndefined();

    expect(store.mode).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('动画收尾失败后不留阻塞:下一次切换照常生效', async () => {
    stubUnsupportedPseudoElementAnimation();
    const store = useThemeStore();

    await store.setModeAnimated('dark', { x: 10, y: 10 });
    vi.mocked(document.documentElement.animate).mockImplementation((() => ({
      cancel() {},
      finish() {},
      finished: Promise.resolve(),
    }) as unknown as Animation));
    await store.setModeAnimated('light', { x: 10, y: 10 });

    expect(store.mode).toBe('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });
});