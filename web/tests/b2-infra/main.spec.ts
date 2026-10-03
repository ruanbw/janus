import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

import { breakStorage, restoreStorage } from './restricted-storage';

const mockRouter = {
  isReady: vi.fn(() => Promise.resolve()),
  currentRoute: { value: { path: '/overview' } },
  replace: vi.fn(() => Promise.resolve()),
};

vi.mock('@/router', () => ({ default: mockRouter }));

const mockClear = vi.fn();
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ clear: mockClear }),
}));

let mockUnauthorizedHandler: ((redirectTo?: string) => void) | null = null;
vi.mock('@/utils/request', () => ({
  setUnauthorizedHandler: (handler: (redirectTo?: string) => void) => {
    mockUnauthorizedHandler = handler;
  },
}));

// UI 插件会挂载 Toaster/ConfirmHost,与本组断言无关,替换成空插件
vi.mock('@/components/app', () => ({ default: { install: () => {} } }));

/** 构造一个 unhandledrejection 事件(happy-dom 无 PromiseRejectionEvent) */
function rejectionEvent(reason: unknown): Event {
  const event = new Event('unhandledrejection', { cancelable: true }) as Event & { reason?: unknown };
  event.reason = reason;
  return event;
}

const CHUNK_ERROR = new Error('Failed to fetch dynamically imported module: /assets/x.js');

describe('main.ts 全局兜底处理器', () => {
  beforeAll(async () => {
    document.body.innerHTML = '<div id="app"></div>';
    // reload spy 只装一次:happy-dom 的 location.reload 重新 spy 会保留旧调用记录
    vi.spyOn(window.location, 'reload').mockImplementation(() => {});
    // 导入 main.ts 会把整个应用依赖图拉进来(经mock 后剩 App/router/pinia),
    // 全量并行跑时首次 transform 可能超过默认 10s hook 超时
    await import('@/main');
  }, 60_000);

  beforeEach(() => {
    mockClear.mockClear();
    mockRouter.isReady.mockReset().mockResolvedValue(undefined);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(window.location.reload).mockClear();
  });

  afterEach(() => {
    restoreStorage('sessionStorage');
    vi.restoreAllMocks();
    vi.spyOn(window.location, 'reload').mockImplementation(() => {});
  });

  it('chunk 失败被我们接管后要 preventDefault,避免浏览器再打一遍原生 Uncaught (in promise)', () => {
    const event = rejectionEvent(CHUNK_ERROR);
    window.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
  });

  it('sessionStorage 受限时:兜底处理器自身不抛,且不自动重载', () => {
    breakStorage('sessionStorage');

    expect(() => window.dispatchEvent(rejectionEvent(CHUNK_ERROR))).not.toThrow();
    expect(window.location.reload).not.toHaveBeenCalled();
  });

  it('chunk 失败且计数未满:自动重载一次并记账', () => {
    window.dispatchEvent(rejectionEvent(CHUNK_ERROR));

    expect(window.location.reload).toHaveBeenCalledTimes(1);
    expect(sessionStorage.getItem('chunk_reload_/')).toBe('1');
  });

  it('非 chunk 类拒绝不触发重载,也不吞掉浏览器默认上报', () => {
    const event = rejectionEvent(new Error('业务请求失败'));
    window.dispatchEvent(event);

    expect(window.location.reload).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });

  it('资源加载失败(img/script)不记成 [Global Error] undefined', () => {
    const consoleError = vi.mocked(console.error);

    const event = new Event('error') as Event & { error?: unknown };
    Object.defineProperty(event, 'target', { value: document.createElement('img') });
    window.dispatchEvent(event);

    expect(
      consoleError.mock.calls.filter((call) => String(call[0]).includes('[Global Error]')),
    ).toEqual([]);
  });

  it('真正的运行时错误仍记为 [Global Error]', () => {
    const consoleError = vi.mocked(console.error);

    const event = new Event('error') as Event & { error?: unknown };
    event.error = new Error('boom');
    window.dispatchEvent(event);

    expect(consoleError).toHaveBeenCalledWith('[Global Error]', expect.any(Error));
  });

  it('401 处理器在 router.isReady() reject 时仍清理登录态并复位 redirecting', async () => {
    mockRouter.isReady.mockRejectedValue(new Error('initial navigation failed'));

    expect(mockUnauthorizedHandler).not.toBeNull();
    mockUnauthorizedHandler?.('/links');
    await vi.waitFor(() => expect(mockClear).toHaveBeenCalled());

    // 复位后第二次 401 仍能触发(旧实现里 redirecting 卡在 true 会永久失效)
    mockClear.mockClear();
    mockRouter.isReady.mockResolvedValue(undefined);
    mockUnauthorizedHandler?.('/rules');
    await vi.waitFor(() => expect(mockClear).toHaveBeenCalled());
  });
});