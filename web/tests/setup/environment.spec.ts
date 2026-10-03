/**
 * 纯 TS 冒烟测试:证明 happy-dom 环境 + vitest globals 生效。
 * 不 import describe/it/expect,依赖 vite.config.ts 中的 `globals: true`。
 */
import { mount } from '@vue/test-utils';

// globals 生效:这些是 vitest 注入的全局,没有显式 import
expect(typeof window).toBe('object');
expect(typeof document.createElement).toBe('function');

// vitest.setup.ts 的装置生效
expect(localStorage.getItem('smoke')).toBeNull();
localStorage.setItem('smoke', 'janus');
expect(localStorage.getItem('smoke')).toBe('janus');
expect(typeof globalThis.matchMedia).toBe('function');
expect(typeof globalThis.ResizeObserver).toBe('function');
expect(typeof globalThis.crypto.randomUUID).toBe('function');

// @vue/test-utils 链路通(纯 TS 分支:挂载一个内联组件)
const wrapper = mount({ template: '<p data-testid="inline">ok</p>' });
expect(wrapper.get('[data-testid="inline"]').text()).toBe('ok');
wrapper.unmount();

describe('storage 隔离', () => {
  it('每个用例开始时 localStorage 为空', () => {
    expect(localStorage.length).toBe(0);
  });

  it('上一个用例写入的值不会泄漏到下一个用例', () => {
    expect(localStorage.getItem('leak')).toBeNull();
    localStorage.setItem('leak', 'x');
    expect(localStorage.length).toBe(1);
  });
});