/**
 * AdminLayout 移动端抽屉：主区域 aria-hidden 与焦点搬移的时序契约。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 主区域已标记 aria-hidden="true"
 *   When  焦点此刻落在主区域子树内
 *   Then  Chrome 会拦截这次属性写入并报错
 *         「Blocked aria-hidden on an element because its descendant retained focus」
 *
 * 真实触发路径：移动端点左上角「打开菜单」。
 *   1. mousedown 先让汉堡按钮成为 document.activeElement；
 *   2. click 处理器把 drawerOpen 置真，同一轮 patch 把 aria-hidden="true"
 *      写到按钮的**祖先**上 —— 此刻焦点还在其中；
 *   3. 抽屉虽经 DialogPortal 传送到 body（在 aria-hidden 之外），但 reka-ui 的
 *      FocusScope 在 watchEffect 内 `await nextTick()` 之后才调
 *      dispatchMountAutoFocus 搬焦点，补不上第 2 步留下的窗口。
 *
 * ── happy-dom 的局限（必读）────────────────────────────────────────────
 * 本用例能真实捕捉到上述竞态：修复前它稳定报出 1 次非法状态，修复后为 0。
 * 但它**不能**覆盖 Chrome 那条原生报错本身 —— happy-dom 不实现
 * 「aria-hidden 的祖先含焦点时拦截属性写入」这条浏览器规则，
 * 也不会打印 Blocked aria-hidden 警告。这里断言的是该规则的前置条件
 * （子树内无焦点），而非规则本身。
 * 真实浏览器仍须人工验证（AGENTS.md 第 5 层门禁）：
 *   DevTools 切到移动端设备模拟（<768px）→ 点左上角「打开菜单」
 *   → Console 不应出现 Blocked aria-hidden on an element because its
 *     descendant retained focus；Elements 面板中主区域 div 上带
 *     aria-hidden="true" 时，DevTools 高亮的 focus 节点不应落在该 div 内。
 */
import { mount } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { afterEach } from 'vitest';
import { nextTick } from 'vue';
import { createMemoryHistory, createRouter } from 'vue-router';

import AdminLayout from '@/layouts/AdminLayout.vue';

const BlankPage = { template: '<div>page</div>' };

// AdminLayout 会把抽屉 teleport 到 body，且 mainRegion() 是全局选择器；
// 上一条用例失败时若不卸载，残留 DOM 会污染下一条的查询结果。
let mounted: ReturnType<typeof mount> | null = null;

afterEach(() => {
  mounted?.unmount();
  mounted = null;
  document.body.innerHTML = '';
});

function mountLayout() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: BlankPage }],
  });
  const pinia = createPinia();
  mounted = mount(AdminLayout, {
    attachTo: document.body,
    global: { plugins: [router, pinia] },
  });
  return mounted;
}

/** 取主区域（承载 header/main/footer、被标记 aria-hidden 的那个 div） */
function mainRegion(): HTMLElement {
  const el = document.querySelector('header')?.parentElement;
  if (!el) throw new Error('未找到主区域容器');
  return el as HTMLElement;
}

/** 当前是否处于「祖先已 aria-hidden、焦点仍在其中」的非法状态 */
function ariaHiddenHoldsFocus(): boolean {
  const region = mainRegion();
  return region.getAttribute('aria-hidden') === 'true' && region.contains(document.activeElement);
}

/**
 * 从当前时刻起逐拍推进微任务队列 + Vue 队列，每一拍拍完检查一次不变式。
 * 返回被观测到的非法状态次数（0 = 全程合法）。
 */
async function sweepAriaHiddenInvariant(steps = 8): Promise<number> {
  let violations = 0;
  for (let i = 0; i < steps; i += 1) {
    if (ariaHiddenHoldsFocus()) violations += 1;
    await nextTick();
    await Promise.resolve();
  }
  return violations + (ariaHiddenHoldsFocus() ? 1 : 0);
}

describe('AdminLayout 移动端抽屉', () => {
  it('打开抽屉的整个过程中，主区域从未出现「已 aria-hidden 却仍持有焦点」', async () => {
    const wrapper = mountLayout();
    await nextTick();

    // 真实浏览器里 mousedown 先聚焦按钮、click 随后触发；
    // happy-dom 的 trigger('click') 不会自动聚焦，这里显式复现浏览器的聚焦。
    const burgerEl = document.querySelector<HTMLButtonElement>('button[aria-label="打开菜单"]');
    if (!burgerEl) throw new Error('未找到汉堡按钮');
    burgerEl.focus();
    expect(document.activeElement).toBe(burgerEl);
    expect(ariaHiddenHoldsFocus()).toBe(false);

    // 关键：不能先 await 再检查 —— 竞态窗口就在 click 处理器跑完、
    // FocusScope 的 dispatchMountAutoFocus 尚未执行的那一拍。
    // 故从 dispatch 返回的同步时刻起连续采样。
    const clickDone = wrapper.get('button[aria-label="打开菜单"]').trigger('click');
    const violations = await sweepAriaHiddenInvariant();
    await clickDone;

    expect(violations).toBe(0);

    // 焦点搬完之后仍需成立，且焦点应落在传送至 body 的抽屉面板里。
    const panel = document.querySelector('.drawer-panel');
    expect(panel).not.toBeNull();
    expect(panel?.contains(document.activeElement)).toBe(true);
  });

  it('抽屉打开时主区域确实被 aria-hidden，关闭后撤销', async () => {
    const wrapper = mountLayout();
    await nextTick();
    expect(mainRegion().hasAttribute('aria-hidden')).toBe(false);

    await wrapper.get('button[aria-label="打开菜单"]').trigger('click');
    // 修复后 aria-hidden 改由抽屉的 openAutoFocus 驱动，比原来晚一拍落地，
    // 这正是消除竞态所必需的延迟 —— 断言的是「最终确实挂上了」。
    // openAutoFocus 由 FocusScope 在其 watchEffect 内 await nextTick() 之后才派发，
    // 故需再推进一个微任务让 Vue 把 patch 落地。
    await nextTick();
    await Promise.resolve();
    expect(mainRegion().getAttribute('aria-hidden')).toBe('true');

    // 关闭按钮在 teleport 出去的抽屉面板里，不在 wrapper 的元素树中
    const closeBtn = document.querySelector<HTMLButtonElement>('button[aria-label="关闭菜单"]');
    expect(closeBtn).not.toBeNull();
    closeBtn?.click();
    await nextTick();
    expect(mainRegion().hasAttribute('aria-hidden')).toBe(false);
  });
});
