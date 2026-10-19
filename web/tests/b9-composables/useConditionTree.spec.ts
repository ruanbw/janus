/**
 * 规则条件树编辑状态（useConditionTree）的冻结契约。
 *
 * 背景：RuleFormView.vue 里树节点的增删改全部直接操作 ref 中的嵌套对象，
 * 「组里至少留一条叶子」「新增节点要返回给模板聚焦」「提交前要拿副本」
 * 这些规则只靠调用点自觉。抽 composable 之前先钉死语义，重点三条：
 *
 *  1. **组内最后一条叶子删不掉**。空组对后端是非法的，前端必须在这里拦住，
 *     否则用户会提交一棵后端拒绝的树。
 *  2. **增删只作用于传入的那个组**。深层嵌套下误改根组，会让用户在子组里的
 *     编辑「跳到」根组上，属于最难排查的一类交互 bug。
 *  3. **toPayload 深拷贝**。返回值若与编辑态共享引用，后续序列化/校验里任何
 *     一次就地修改都会反写表单状态。
 */
import { describe, expect, it } from 'vitest';

import { useConditionTree } from '@/composables/useConditionTree';
import type { RuleConditionLeafNode, RuleConditionTree, RuleLogic } from '@/types/api';

function makeLeaf(
  field: RuleConditionLeafNode['leaf']['field'] = 'country',
  values: string[] = [''],
): RuleConditionLeafNode {
  return { type: 'leaf', leaf: { field, operator: 'eq', values } };
}

function makeGroup(
  logic: RuleLogic = 'all',
  children: Array<RuleConditionTree | RuleConditionLeafNode> = [],
): RuleConditionTree {
  return { type: 'group', group: { logic, children } };
}

/** 根组：[叶子, 子组 A(叶子 + 子组 B(两条叶子))]，叶子总数为 4 */
function nestedTree(): RuleConditionTree {
  return makeGroup('all', [
    makeLeaf('country', ['CN']),
    makeGroup('any', [makeLeaf('ua', ['curl']), makeGroup('all', [makeLeaf('path', ['/a']), makeLeaf('path', ['/b'])])]),
  ]);
}

function childGroup(parent: RuleConditionTree, index: number): RuleConditionTree {
  const child = parent.group.children[index];
  if (child?.type !== 'group') throw new Error(`children[${index}] 不是组节点`);
  return child;
}

function childLeaf(parent: RuleConditionTree, index: number): RuleConditionLeafNode {
  const child = parent.group.children[index];
  if (child?.type !== 'leaf') throw new Error(`children[${index}] 不是叶子节点`);
  return child;
}

describe('useConditionTree 初始状态', () => {
  it('Given 不传 initial，When 创建，Then 是一个 all 组加一条 country/eq 的默认叶子', () => {
    const t = useConditionTree();

    expect(t.tree.value).toEqual(makeGroup('all', [makeLeaf()]));
    expect(t.leafCount.value).toBe(1);
  });

  it('Given 传了 initial，When 创建，Then 原样采用且不额外塞默认叶子', () => {
    const initial = makeGroup('any', [makeLeaf('country', ['CN']), makeLeaf('ua', ['curl'])]);

    const t = useConditionTree(initial);

    expect(t.tree.value).toEqual(initial);
    expect(t.leafCount.value).toBe(2);
  });

  it('Given 两个独立实例，When 改动其一的默认叶子 values，Then 另一个不受影响', () => {
    // 默认值若是模块级共享对象/数组，第一条规则编辑会污染后面所有新建表单
    const a = useConditionTree();
    const b = useConditionTree();

    childLeaf(a.tree.value, 0).leaf.values.push('CN');

    expect(childLeaf(b.tree.value, 0).leaf.values).toEqual(['']);
  });
});

describe('useConditionTree.addLeaf', () => {
  it('Given 只有一条默认叶子的组，When addLeaf，Then 末尾追加默认叶子并返回该节点本身', () => {
    const t = useConditionTree();

    const added = t.addLeaf(t.tree.value);

    expect(t.tree.value.group.children).toHaveLength(2);
    expect(added).toEqual(makeLeaf());
    // 必须是树里那个节点，模板才能拿它定位/聚焦/标错
    expect(t.tree.value.group.children[1]).toBe(added);
    expect(t.leafCount.value).toBe(2);
  });

  it('Given 嵌套子组，When 对子组 addLeaf，Then 只改子组，根组 children 不变', () => {
    const t = useConditionTree(
      makeGroup('all', [makeLeaf('country', ['CN']), makeGroup('any', [makeLeaf('ua', ['curl'])])]),
    );
    const sub = childGroup(t.tree.value, 1);

    t.addLeaf(sub);

    expect(sub.group.children).toHaveLength(2);
    expect(t.tree.value.group.children).toHaveLength(2);
    expect(t.leafCount.value).toBe(3);
  });

  it('Given 连续两次 addLeaf，When 给先返回的叶子赋值，Then 两个叶子的 values 数组互不共享', () => {
    const t = useConditionTree();

    const first = t.addLeaf(t.tree.value);
    const second = t.addLeaf(t.tree.value);
    first.leaf.values.push('CN');

    expect(second.leaf.values).toEqual(['']);
  });
});

describe('useConditionTree.removeLeaf', () => {
  it('Given 组内三条叶子，When 删除中间那条，Then 返回 true 且其余节点顺序不变', () => {
    const t = useConditionTree(
      makeGroup('all', [makeLeaf('country', ['CN']), makeLeaf('ua', ['curl']), makeLeaf('path', ['/a'])]),
    );
    const second = childLeaf(t.tree.value, 1);

    expect(t.removeLeaf(t.tree.value, second)).toBe(true);
    expect(t.tree.value.group.children).toEqual([makeLeaf('country', ['CN']), makeLeaf('path', ['/a'])]);
    expect(t.leafCount.value).toBe(2);
  });

  it('Given 组内只剩一条叶子，When 删除它，Then 返回 false 且树保持不变', () => {
    const t = useConditionTree();
    const only = childLeaf(t.tree.value, 0);

    expect(t.removeLeaf(t.tree.value, only)).toBe(false);
    expect(t.tree.value.group.children).toEqual([makeLeaf()]);
    expect(t.leafCount.value).toBe(1);
  });

  it('Given 叶子属于别的组，When 在根组上删除它，Then 返回 false 且两边都不受影响', () => {
    const t = useConditionTree(
      makeGroup('all', [makeLeaf('country', ['CN']), makeGroup('any', [makeLeaf('ua', ['curl'])])]),
    );
    const nested = childLeaf(childGroup(t.tree.value, 1), 0);

    expect(t.removeLeaf(t.tree.value, nested)).toBe(false);
    expect(t.tree.value.group.children).toHaveLength(2);
    expect(childGroup(t.tree.value, 1).group.children).toHaveLength(1);
  });

  it('Given 叶子在嵌套子组里，When 对该子组 removeLeaf，Then 只影响子组，根组叶子保留', () => {
    const t = useConditionTree(
      makeGroup('all', [
        makeLeaf('country', ['CN']),
        makeGroup('any', [makeLeaf('ua', ['curl']), makeLeaf('path', ['/a'])]),
      ]),
    );
    const sub = childGroup(t.tree.value, 1);

    expect(t.removeLeaf(sub, childLeaf(sub, 0))).toBe(true);
    expect(sub.group.children).toEqual([makeLeaf('path', ['/a'])]);
    expect(t.tree.value.group.children).toHaveLength(2);
    expect(t.leafCount.value).toBe(2);
  });
});

describe('useConditionTree.addGroup', () => {
  it('Given 根组，When addGroup，Then 末尾追加一个 logic=all 的空组并返回该组本身', () => {
    const t = useConditionTree();

    const added = t.addGroup(t.tree.value);

    expect(added).toEqual(makeGroup('all', []));
    expect(t.tree.value.group.children).toHaveLength(2);
    expect(t.tree.value.group.children[1]).toBe(added);
    // 空组不贡献叶子：新增分组后叶子计数不应变化
    expect(t.leafCount.value).toBe(1);
  });

  it('Given 嵌套父组，When addGroup，Then 只挂在父组下，根组不受影响', () => {
    const t = useConditionTree(
      makeGroup('all', [makeLeaf('country', ['CN']), makeGroup('any', [makeLeaf('ua', ['curl'])])]),
    );
    const sub = childGroup(t.tree.value, 1);

    const added = t.addGroup(sub);

    expect(sub.group.children).toEqual([makeLeaf('ua', ['curl']), added]);
    expect(t.tree.value.group.children).toHaveLength(2);
  });
});

describe('useConditionTree.removeGroup', () => {
  it('Given 父组含 [叶子, 组A, 组B]，When 删除组A，Then 返回 true 且剩余节点相对顺序不变', () => {
    const groupA = makeGroup('any', [makeLeaf('ua', ['curl'])]);
    const groupB = makeGroup('all', [makeLeaf('path', ['/a'])]);
    const t = useConditionTree(makeGroup('all', [makeLeaf('country', ['CN']), groupA, groupB]));

    expect(t.removeGroup(t.tree.value, groupA)).toBe(true);
    expect(t.tree.value.group.children).toEqual([makeLeaf('country', ['CN']), groupB]);
    expect(t.leafCount.value).toBe(2);
  });

  it('Given 该组不在父组下，When removeGroup，Then 返回 false 且树保持不变', () => {
    const t = useConditionTree();
    const detached = makeGroup('all', []);

    expect(t.removeGroup(t.tree.value, detached)).toBe(false);
    expect(t.tree.value.group.children).toEqual([makeLeaf()]);
  });

  it('Given 嵌套父组里的子组，When 从父组删除它，Then 根组 children 不变', () => {
    const t = useConditionTree(nestedTree());
    const root = t.tree.value;
    const groupA = childGroup(root, 1);
    const groupB = childGroup(groupA, 1);

    expect(t.removeGroup(groupA, groupB)).toBe(true);
    expect(groupA.group.children).toEqual([makeLeaf('ua', ['curl'])]);
    expect(root.group.children).toHaveLength(2);
    expect(t.leafCount.value).toBe(2);
  });
});

describe('useConditionTree.setLogic', () => {
  it('Given 根组与子组都是 all，When 把根改成 any，Then 只有根变了', () => {
    const t = useConditionTree(
      makeGroup('all', [makeLeaf('country', ['CN']), makeGroup('all', [makeLeaf('ua', ['curl'])])]),
    );

    t.setLogic(t.tree.value, 'any');

    expect(t.tree.value.group.logic).toBe('any');
    expect(childGroup(t.tree.value, 1).group.logic).toBe('all');
  });

  it('Given 嵌套子组，When 只改子组的 logic，Then 根组 logic 不受影响', () => {
    const t = useConditionTree(
      makeGroup('all', [makeLeaf('country', ['CN']), makeGroup('all', [makeLeaf('ua', ['curl'])])]),
    );

    t.setLogic(childGroup(t.tree.value, 1), 'any');

    expect(childGroup(t.tree.value, 1).group.logic).toBe('any');
    expect(t.tree.value.group.logic).toBe('all');
  });
});

describe('useConditionTree.leafCount', () => {
  it('Given 组套组三层，When 读 leafCount，Then 逐层累加（1 + 1 + 2 = 4）', () => {
    const t = useConditionTree(nestedTree());

    expect(t.leafCount.value).toBe(4);
  });

  it('Given 深层树，When 增删最深一层叶子，Then computed 跟着变', () => {
    const t = useConditionTree(nestedTree());
    const deepest = childGroup(childGroup(t.tree.value, 1), 1);

    t.addLeaf(deepest);
    expect(t.leafCount.value).toBe(5);

    t.removeLeaf(deepest, childLeaf(deepest, 0));
    expect(t.leafCount.value).toBe(4);
  });
});

describe('useConditionTree.toPayload', () => {
  it('Given 任意树，When toPayload，Then 内容等于 tree.value 但不是同一个对象', () => {
    const t = useConditionTree(nestedTree());

    const payload = t.toPayload();

    expect(payload).toEqual(t.tree.value);
    expect(payload).not.toBe(t.tree.value);
  });

  it('Given 取到 payload，When 往根组 push 一条叶子，Then 内部 tree 的 children 不变', () => {
    const t = useConditionTree();

    const payload = t.toPayload();
    payload.group.children.push(makeLeaf('ua', ['curl']));

    expect(t.tree.value.group.children).toHaveLength(1);
    expect(t.leafCount.value).toBe(1);
  });

  it('Given 取到 payload，When 改深层叶子的 values，Then 内部 tree 的同一叶子不受影响', () => {
    // 只复制顶层不算深拷贝：嵌套叶子的 values 数组仍会被共享
    const t = useConditionTree(nestedTree());

    const payload = t.toPayload();
    const payloadDeepest = childGroup(childGroup(payload, 1), 1);
    childLeaf(payloadDeepest, 0).leaf.values.push('US');

    const internalDeepest = childGroup(childGroup(t.tree.value, 1), 1);
    expect(childLeaf(internalDeepest, 0).leaf.values).toEqual(['/a']);
  });
});
