import { afterEach, describe, expect, it } from 'vitest';

import { getCookie, getCsrfToken } from '@/utils/request';

/** happy-dom 的 document.cookie 是只读 accessor，测试里改用可写替身 */
function stubCookie(raw: string): void {
  let value = raw;
  Object.defineProperty(document, 'cookie', {
    configurable: true,
    get: () => value,
    set: (next: string) => {
      value = next;
    },
  });
}

afterEach(() => {
  delete (document as { cookie?: unknown }).cookie;
});

/** sandbox iframe(无 allow-same-origin)下读取 document.cookie 会抛 SecurityError */
function breakCookieAccess(): void {
  Object.defineProperty(document, 'cookie', {
    configurable: true,
    get() {
      throw new DOMException('The operation is insecure.', 'SecurityError');
    },
  });
}

describe('utils/request 读取 CSRF cookie', () => {
  it('document.cookie 不可读时返回 undefined 而不是抛 SecurityError', () => {
    breakCookieAccess();

    expect(getCookie('janus_csrf')).toBeUndefined();
    expect(getCsrfToken()).toBeUndefined();
  });

  it('cookie 可读时正常解码取值', () => {
    stubCookie('other=1; janus_csrf=abc%20123');

    expect(getCookie('janus_csrf')).toBe('abc 123');
  });

  it('cookie 取不到目标项时返回 undefined', () => {
    stubCookie('other=1');

    expect(getCookie('janus_csrf')).toBeUndefined();
  });
});