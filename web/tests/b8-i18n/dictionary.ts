/**
 * i18n 字典巡检工具，locale 一致性与插值两组用例共用。
 *
 * 为什么抽出来：`locales.spec.ts` 用它比对两侧字典结构，`translate.spec.ts`
 * 用它从真实文案里挑出可插值的 key。两处各写一份遍历逻辑，形状判定（尤其是
 * 「数组算叶子」这条）迟早会漂移，届时测试之间的结论会互相矛盾。
 */

export type Dictionary = Record<string, unknown>;

/**
 * 递归展开嵌套字典，点号拼接 key。
 *
 * 数组整体视为叶子：展开成 `tags.0 / tags.1` 会把「两种语言的条目数不同」
 * 伪装成 key 缺失，报错位置也会指到实际上不存在翻译的 `links.tags.0` 上。
 */
export function flattenLeaves(node: Dictionary, prefix = ''): Map<string, unknown> {
  const leaves = new Map<string, unknown>();

  for (const [name, value] of Object.entries(node)) {
    const key = prefix === '' ? name : `${prefix}.${name}`;
    const isLeaf = value === null || typeof value !== 'object' || Array.isArray(value);

    if (isLeaf) {
      leaves.set(key, value);
      continue;
    }

    for (const [childKey, childValue] of flattenLeaves(value as Dictionary, key)) {
      leaves.set(childKey, childValue);
    }
  }

  return leaves;
}

/**
 * 取叶子里的字符串，数组叶子逐元素返回。
 * 非 string / string[] 的叶子返回空数组 —— 类型是否合法由调用方单独断言，
 * 否则「值类型写错」会被当成「值里没有文本」而静默放过。
 */
export function leafStrings(value: unknown): string[] {
  if (typeof value === 'string') return [value];
  if (Array.isArray(value)) return value.filter((item): item is string => typeof item === 'string');
  return [];
}

/**
 * 提取 `{name}` 形式的占位符。
 * 只认裸花括号 + `\w`：`{{name}}`、`{n, number}` 这类富文本/ICU 语法不在冻结契约内，
 * 若实现改用它们，这里会返回空集，从而在「占位符一致」与「插值」两处同时报警。
 */
export function extractPlaceholders(text: string): Set<string> {
  return new Set([...text.matchAll(/\{(\w+)\}/g)].map((match) => match[1]!));
}
