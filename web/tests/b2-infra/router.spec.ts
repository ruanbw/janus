import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { breakStorage, restoreStorage } from './restricted-storage';

const mockMessageError = vi.fn();

vi.mock('@/utils/toast', () => ({
  message: {
    error: mockMessageError,
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}));

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    initialized: true,
    isAuthenticated: true,
    isSuperAdmin: true,
    fetchMe: vi.fn(),
  }),
}));

// 让 LinksView 的懒加载失败,复现「部署更新后 chunk 失效」
vi.mock('@/views/links/LinksView.vue', () => ({
  get default() {
    throw new Error('Failed to fetch dynamically imported module: /assets/LinksView.js');
  },
}));

const { default: router } = await import('@/router');

/** 受限存储环境:sessionStorage 读写都抛 SecurityError */
function breakSessionStorage(): void {
  breakStorage('sessionStorage');
}

describe('router chunk 失败自动恢复', () => {
  beforeEach(() => {
    mockMessageError.mockClear();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(window.location, 'assign').mockImplementation(() => {});
  });

  afterEach(() => {
    restoreStorage('sessionStorage');
    vi.restoreAllMocks();
  });

  it('sessionStorage 受限时:不自动重载、不抛异常,改为提示手动重试', async () => {
    breakSessionStorage();

    await expect(router.push('/links')).rejects.toThrow(/dynamically imported module/);

    expect(window.location.assign).not.toHaveBeenCalled();
    expect(mockMessageError).toHaveBeenCalledWith('页面资源加载失败，请检查网络连接后刷新重试');
  });

  it('首次失败且计数未满:自动重载一次', async () => {
    await expect(router.push('/links')).rejects.toThrow(/dynamically imported module/);

    expect(window.location.assign).toHaveBeenCalledWith('/links');
    expect(sessionStorage.getItem('chunk_reload_/links')).toBe('1');
  });

  it('已重载过一次仍失败:停止自动重载并提示手动重试', async () => {
    sessionStorage.setItem('chunk_reload_/links', '1');

    await expect(router.push('/links')).rejects.toThrow(/dynamically imported module/);

    expect(window.location.assign).not.toHaveBeenCalled();
    expect(sessionStorage.getItem('chunk_reload_/links')).toBeNull();
    expect(mockMessageError).toHaveBeenCalledWith('页面资源加载失败，请检查网络连接后刷新重试');
  });
});