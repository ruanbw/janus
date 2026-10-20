/**
 * 规则条件树的编辑状态。
 *
 * 冻结语义见 tests/b9-composables/useConditionTree.spec.ts 的文件头，三条关键约束：
 *
 * 1. 组内删空就拒绝：空组对后端非法，前端在这里拦住，用户不会提交一棵被后端
 *    拒掉的树。注意判据是「删掉后该组 children 为空」，不是「组里只有一条叶子」——
 *    1 叶子 + 1 子组时删叶子仍然合法。
 * 2. 增删只作用于传入的那个组：深层嵌套下按「根组」兜底会让子组的编辑跳到根组，
 *    这里全部以传入引用为准，不做任何向上/向下搜索。
 * 3. toPayload 深拷贝：返回值若与编辑态共享引用，序列化/校验里任何一次就地修改
 *    都会反写表单状态。
 */
import { computed, ref } from 'vue';

import type { RuleCondition, RuleConditionLeafNode, RuleConditionTree, RuleLogic } from '@/types/api';

/** 新增叶子/默认叶子的取值：country 是唯一支持 eq + 下拉的字段，不会一上手就是非法组合 */
function createLeaf(): RuleConditionLeafNode {
  return { type: 'leaf', leaf: { field: 'country', operator: 'eq', values: [''] } };
}

function createGroup(logic: RuleLogic = 'all'): RuleConditionTree {
  return { type: 'group', group: { logic, children: [] } };
}

/** 新建规则的初始形态：一个 all 组 + 一条空条件，用户不至于面对一棵空树 */
function defaultTree(): RuleConditionTree {
  const group = createGroup('all');
  group.group.children.push(createLeaf());
  return group;
}

/** 递归统计叶子：条件个数的展示与「至少一条」的校验都以它为准 */
function countLeaves(node: RuleConditionTree): number {
  let count = 0;
  for (const child of node.group.children) {
    count += child.type === 'leaf' ? 1 : countLeaves(child);
  }
  return count;
}

function cloneLeaf(leaf: RuleConditionLeafNode): RuleConditionLeafNode {
  const condition: RuleCondition = {
    field: leaf.leaf.field,
    operator: leaf.leaf.operator,
    values: [...leaf.leaf.values],
  };
  return { type: 'leaf', leaf: condition };
}

/** 只复制叶子与组的骨架，组之间互不共享 children、values 数组 */
function cloneTree(node: RuleConditionTree): RuleConditionTree {
  return {
    type: 'group',
    group: {
      logic: node.group.logic,
      children: node.group.children.map((child) =>
        child.type === 'leaf' ? cloneLeaf(child) : cloneTree(child),
      ),
    },
  };
}

export function useConditionTree(initial?: RuleConditionTree) {
  // 默认树每次调用都新建：模块级共享对象会让第一条规则的编辑污染后面所有新建表单。
  const tree = ref<RuleConditionTree>(initial ?? defaultTree());

  const leafCount = computed(() => countLeaves(tree.value));

  /**
   * 取回数组里刚插入的元素，而不是新建实例：tree 是深度响应式，children 里存的是
   * 它的响应式代理；返回原始对象的话模板拿到的引用与树里不同（聚焦 / 标错会失效）。
   */
  function addLeaf(group: RuleConditionTree): RuleConditionLeafNode {
    const children = group.group.children;
    children.push(createLeaf());
    return children[children.length - 1] as RuleConditionLeafNode;
  }

  /**
   * 「找不到」按引用判定（indexOf），不做深比较：深比较会把结构相同的另一个叶子
   * 误认成目标，删掉用户在别处刚建的那条。
   */
  function removeLeaf(group: RuleConditionTree, leaf: RuleConditionLeafNode): boolean {
    const children = group.group.children;
    const index = children.indexOf(leaf);
    if (index < 0) return false;
    // 删掉后该组就没有子节点了：空组后端非法，前端必须拦在这里。
    if (children.length - 1 === 0) return false;
    children.splice(index, 1);
    return true;
  }

  function addGroup(parent: RuleConditionTree): RuleConditionTree {
    const children = parent.group.children;
    children.push(createGroup('all'));
    return children[children.length - 1] as RuleConditionTree;
  }

  /**
   * 删除子组不设「至少留一个」限制：空组是合法的中间态（addGroup 就是先建空组
   * 再往里加叶子），用户清空一个分支就该让它消失。
   */
  function removeGroup(parent: RuleConditionTree, child: RuleConditionTree): boolean {
    const children = parent.group.children;
    const index = children.indexOf(child);
    if (index < 0) return false;
    children.splice(index, 1);
    return true;
  }

  /** 只改传入组的 logic；不做继承或向下传播 */
  function setLogic(group: RuleConditionTree, logic: RuleLogic): void {
    group.group.logic = logic;
  }

  /** 提交前取副本：调用方后续的就地修改（序列化前的清洗等）不得反写编辑态 */
  function toPayload(): RuleConditionTree {
    return cloneTree(tree.value);
  }

  return { tree, leafCount, addLeaf, removeLeaf, addGroup, removeGroup, setLogic, toPayload };
}
