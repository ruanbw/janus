/**
 * 定位型 content 封装的 props 转发契约（b3 审计补完）。
 *
 * 背景：b3 第一轮补了 40+ 个文件的 `as` / `asChild`，但在三个 **content** 封装上停了手。
 * 这三个文件的 props 最多、语义也最微妙（全是 floating-ui 定位参数），恰好是最容易
 * 「判断不了就不改」的一类，于是整族都留在了「声明了但不转发」的状态。
 *
 * 失效形态与其它族完全一致：Vue 把未转发的 prop 当成「已声明的 prop」从 $attrs 摘走，
 * 既不生效、也不透传、也不报错。
 *
 * 与 round-1 测试的分工：
 *   form-family.spec.ts    → 断言 asChild 不多包一层（语义层）
 *   本文件                 → 断言定位/as 类 prop 真的到达了 reka 原语（传导层）
 */
import { mount, type VueWrapper } from '@vue/test-utils';
import { h, type Component } from 'vue';

import { DropdownMenuContent } from '@/components/ui/dropdown-menu';
import { DropdownMenuItem } from '@/components/ui/dropdown-menu';
import { DropdownMenuRoot } from '@/components/ui/dropdown-menu';
import { DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { PopoverContent } from '@/components/ui/popover';
import { Popover as PopoverRoot } from '@/components/ui/popover';
import { PopoverTrigger } from '@/components/ui/popover';
import { TooltipContent } from '@/components/ui/tooltip';
import { Tooltip as TooltipRoot } from '@/components/ui/tooltip';
import { TooltipTrigger } from '@/components/ui/tooltip';

const mounted: ReturnType<typeof mount>[] = [];

afterEach(() => {
  for (const w of mounted) w.unmount();
  mounted.length = 0;
  document.body.innerHTML = '';
});

function track<T extends ReturnType<typeof mount>>(w: T): T {
  mounted.push(w);
  return w;
}

/**
 * 取出某个封装传给 reka 原语的 vnode props（kebab-case）。
 *
 * 有些契约在 happy-dom 里无法从真实 DOM 观察（浮动定位要 ResizeObserver 与真实布局、
 * closeAutoFocus 要真实焦点回归）。这类断言改为钉住「绑到了原语的 vnode 上」——
 * 这正是「转发有没有发生」的直接证据，比断言 DOM 副作用更精确，也不会被 jsdom 能力绑住。
 */
function vnodePropsOf(wrapper: VueWrapper, c: Component): Record<string, unknown> {
  return wrapper.findComponent(c).vm.$.subTree.props ?? {};
}

/** dropdown-menu 的 content 被 teleport 到 body，只能从 document 上找落点 */
function mountDropdownContent(props: Record<string, unknown>, slot: () => unknown = () => h(DropdownMenuItem, null, { default: () => 'i' })) {
  return track(mount(DropdownMenuRoot, {
    props: { open: true },
    slots: {
      default: () => [
        h(DropdownMenuTrigger, { asChild: true }, { default: () => h('button', 'T') }),
        h(DropdownMenuContent, props, { default: slot }),
      ],
    },
    attachTo: document.body,
  }));
}

describe('dropdown-menu-content — 定位类 prop 必须到达原语', () => {
  it('as="section" 时渲染 section（当前恒为 div）', () => {
    mountDropdownContent({ as: 'section' });

    const el = document.body.querySelector('[data-reka-menu-content]');
    expect(el).not.toBeNull();
    expect(el!.tagName).toBe('SECTION');
  });

  it('side="top" 落到 data-side', () => {
    mountDropdownContent({ side: 'top' });

    expect(document.body.querySelector('[data-reka-menu-content]')!.getAttribute('data-side')).toBe('top');
  });

  it('align="start" 落到 data-align', () => {
    mountDropdownContent({ align: 'start' });

    expect(document.body.querySelector('[data-reka-menu-content]')!.getAttribute('data-align')).toBe('start');
  });

  it('sideOffset / alignOffset 落到 popper 的 CSS 变量（定位偏移真的生效）', () => {
    mountDropdownContent({ sideOffset: 12, alignOffset: 7 });

    // floating-via 用这个 CSS 变量驱动 transform；没有它就等于没传偏移
    const style = document.body.querySelector('[data-reka-menu-content]')!.getAttribute('style') ?? '';
    expect(style).toContain('--reka-dropdown-menu-content-transform-origin');
  });

  it('collisionBoundary 被转发（传入元素后定位上下文拿到它）', () => {
    const boundary = document.createElement('div');
    document.body.appendChild(boundary);

    const wrapper = mountDropdownContent({ collisionBoundary: boundary });

    expect(vnodePropsOf(wrapper, DropdownMenuContent)['collision-boundary']).toBe(boundary);
  });

  it('positionStrategy="fixed" 被转发到 content 的定位上下文', () => {
    const wrapper = mountDropdownContent({ positionStrategy: 'fixed' });

    expect(vnodePropsOf(wrapper, DropdownMenuContent)['position-strategy']).toBe('fixed');
  });
});

describe('dropdown-menu-content — 定位 prop 逐个到达 reka 原语的 vnode', () => {
  it('collisionBoundary / positionStrategy / sticky / sideFlip 等定位 prop 全部绑定', () => {
    const wrapper = mountDropdownContent({
      sticky: 'partial',
      sideFlip: true,
      alignFlip: false,
      hideShiftedArrow: true,
      hideWhenDetached: true,
      avoidCollisions: false,
      updatePositionStrategy: 'optimized',
      prioritizePosition: true,
      disableUpdateOnLayoutShift: false,
      memoDependencies: [],
      reference: undefined,
      loop: false,
    });

    const p = vnodePropsOf(wrapper, DropdownMenuContent);
    for (const key of [
      'sticky',
      'side-flip',
      'align-flip',
      'hide-shifted-arrow',
      'hide-when-detached',
      'avoid-collisions',
      'update-position-strategy',
      'prioritize-position',
      'disable-update-on-layout-shift',
      'memo-dependencies',
      'reference',
      'loop',
    ]) {
      expect(Object.keys(p), `dropdown-menu-content 未绑定 ${key}`).toContain(key);
    }
  });
});

describe('dropdown-menu-content — emit 必须全部转发', () => {
  it('closeAutoFocus 等全部 declareEmits 都绑定到了 reka 原语上', () => {
    const wrapper = mountDropdownContent({});

    const p = vnodePropsOf(wrapper, DropdownMenuContent);
    // ContentEmits = closeAutoFocus / escapeKeyDown / focusOutside / interactOutside / pointerDownOutside
    for (const key of [
      'onCloseAutoFocus',
      'onEscapeKeyDown',
      'onFocusOutside',
      'onInteractOutside',
      'onPointerDownOutside',
    ]) {
      expect(Object.keys(p), `dropdown-menu-content 未转发 ${key}`).toContain(key);
    }
  });
});

describe('popover-content — 定位类 prop 必须到达原语', () => {
  function mountPopoverContent(props: Record<string, unknown>) {
    return track(mount(PopoverRoot, {
      props: { open: true },
      slots: {
        default: () => [
          h(PopoverTrigger, { asChild: true }, { default: () => h('button', 'T') }),
          h(PopoverContent, props, { default: () => 'c' }),
        ],
      },
      attachTo: document.body,
    }));
  }

  it('as="section" 时渲染 section（当前恒为 div）', () => {
    mountPopoverContent({ as: 'section' });

    const el = document.body.querySelector('[data-reka-popper-content-wrapper]')!.firstElementChild!;
    expect(el.tagName).toBe('SECTION');
  });

  it('dir="rtl" 落到 content 的 dir 属性（当前恒为 ltr）', () => {
    mountPopoverContent({ dir: 'rtl' });

    const el = document.body.querySelector('[data-reka-popper-content-wrapper]')!.firstElementChild!;
    expect(el.getAttribute('dir')).toBe('rtl');
  });

  it('side / align 落到定位属性上', () => {
    mountPopoverContent({ side: 'left', align: 'end' });

    const el = document.body.querySelector('[data-reka-popper-content-wrapper]')!.firstElementChild!;
    expect(el.getAttribute('data-side')).toBe('left');
    expect(el.getAttribute('data-align')).toBe('end');
  });

  it('全部定位 prop 绑定到 reka 原语（sticky / positionStrategy / collisionBoundary 等）', () => {
    const boundary = document.createElement('div');
    document.body.appendChild(boundary);

    const wrapper = mountPopoverContent({ sticky: 'partial', positionStrategy: 'fixed', collisionBoundary: boundary });

    const p = vnodePropsOf(wrapper, PopoverContent);
    expect(p['sticky']).toBe('partial');
    expect(p['position-strategy']).toBe('fixed');
    expect(p['collision-boundary']).toBe(boundary);
    for (const key of [
      'side-flip',
      'align-flip',
      'hide-shifted-arrow',
      'hide-when-detached',
      'disable-update-on-layout-shift',
      'memo-dependencies',
      'prioritize-position',
      'reference',
      'disable-outside-pointer-events',
    ]) {
      expect(Object.keys(p), `popover-content 未绑定 ${key}`).toContain(key);
    }
  });
});

describe('tooltip-content — 定位类 prop 必须到达原语', () => {
  function mountTooltipContent(props: Record<string, unknown>) {
    return track(mount(TooltipRoot, {
      props: { open: true },
      slots: {
        default: () => [
          h(TooltipTrigger, null, { default: () => h('button', 'T') }),
          h(TooltipContent, props, { default: () => 'tip' }),
        ],
      },
      attachTo: document.body,
    }));
  }

  it('as="section" 时渲染 section（当前恒为 div）', () => {
    const wrapper = mountTooltipContent({ as: 'section' });

    expect(vnodePropsOf(wrapper, TooltipContent)['as']).toBe('section');
  });

  it('side / sideOffset 到达定位属性', () => {
    const wrapper = mountTooltipContent({ side: 'right', sideOffset: 9 });

    const p = vnodePropsOf(wrapper, TooltipContent);
    expect(p['side']).toBe('right');
    expect(p['side-offset']).toBe(9);
  });

  it('collisionBoundary / positionStrategy / sticky / hideWhenDetached 全部绑定', () => {
    const boundary = document.createElement('div');
    document.body.appendChild(boundary);

    const wrapper = mountTooltipContent({
      collisionBoundary: boundary,
      positionStrategy: 'fixed',
      sticky: 'partial',
      hideWhenDetached: true,
    });

    const p = vnodePropsOf(wrapper, TooltipContent);
    expect(p['collision-boundary']).toBe(boundary);
    expect(p['position-strategy']).toBe('fixed');
    expect(p['sticky']).toBe('partial');
    expect(p['hide-when-detached']).toBe(true);
  });
});

describe('tooltip root — TooltipProvider 的 content 默认项必须转发', () => {
  it('content 里给的 side 作为 tooltip content 的默认 side 生效', () => {
    const wrapper = track(mount(TooltipRoot, {
      props: { open: true, content: { side: 'left' } },
      slots: {
        default: () => [
          h(TooltipTrigger, null, { default: () => h('button', 'T') }),
          h(TooltipContent, null, { default: () => 'tip' }),
        ],
      },
      attachTo: document.body,
    }));

    // provider 把 content 作为默认项下发给所有 tooltip content
    const providerProps = (wrapper.vm.$.subTree as { props?: Record<string, unknown> }).props ?? {};
    expect(providerProps['content']).toEqual({ side: 'left' });
  });
});
