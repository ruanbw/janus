/**
 * 受限存储环境的测试替身。
 *
 * 不用 `vi.spyOn(Storage.prototype, ...)`:happy-dom 的 Storage 实例会把
 * getItem/setItem 绑定成自有属性,prototype 上的 spy 不生效(实测)。
 * 直接替换 window 上的访问器最贴近浏览器里 localStorage 抛 SecurityError 的行为。
 */

/** 存储访问抛 SecurityError 时应抛出的错误 */
function securityError(): never {
  throw new DOMException('The operation is insecure.', 'SecurityError');
}

const restrictedStorage = {
  getItem: securityError,
  setItem: securityError,
  removeItem: securityError,
  clear: securityError,
  key: securityError,
  length: 0,
};

const originalDescriptors = {
  sessionStorage: Object.getOwnPropertyDescriptor(window, 'sessionStorage'),
  localStorage: Object.getOwnPropertyDescriptor(window, 'localStorage'),
};

function restore(name: 'sessionStorage' | 'localStorage'): void {
  const descriptor = originalDescriptors[name];
  if (descriptor) Object.defineProperty(window, name, descriptor);
}

type StorageName = keyof typeof originalDescriptors;

/** 让指定存储在受限环境(Firefox dom.storage.enabled=false / sandbox iframe / 企业策略)下抛 SecurityError */
export function breakStorage(...names: StorageName[]): void {
  for (const name of names) {
    Object.defineProperty(window, name, { configurable: true, get: () => restrictedStorage });
  }
}

/** 还原真实存储 */
export function restoreStorage(...names: StorageName[]): void {
  for (const name of names) restore(name);
}