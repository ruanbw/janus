/**
 * dropdown-menu.vue 的 props 转发契约。
 *
 * 背景：封装声明了 `DropdownMenuRootProps`（= MenuProps：open / dir / modal），
 * 却只把 defaultOpen 绑到 <DropdownMenuRoot> 上。Vue 会把 `open`/`dir`/`modal`
 * 当成「已声明的 prop」从 $attrs 摘走，于是既不生效也不透传、也不报错。
 * 其中 `open` 的后果最严重：reka 的 DropdownMenuRoot.js 里
 * `passive: props.open === void 0`，不转发就恒为 true，
 * 于是 v-model:open 永远走非受控分支。
 */
import { mount } from '@vue/test-utils';
import { h } from 'vue';

import { DropdownMenu } from '@/components/ui/dropdown-menu';
import { DropdownMenuContent } from '@/components/ui/dropdown-menu';
import { DropdownMenuItem } from '@/components/ui/dropdown-menu';
import { DropdownMenuTrigger } from '@/components/ui/dropdown-menu';

/**
 * 挂一个带 trigger + content 的下拉根。
 * content 必须真实存在：dir / modal 只作用在 content 上，没有 content
 * 就没有可断言的落点（那是测试脚手架的缺口，不是实现的缺口）。
 * attachTo document.body 是为了读到被 teleport 出去的 content。
 */
function mountRoot(props: Record<string, unknown>) {
  const wrapper = mount(DropdownMenu, {
    props,
    slots: {
      default: () => [
        h(DropdownMenuTrigger, { asChild: true }, { default: () => h('button', 'T') }),
        h(DropdownMenuContent, null, {
          default: () => h(DropdownMenuItem, null, { default: () => 'i' }),
        }),
      ],
    },
    attachTo: document.body,
  });
  mounted.push(wrapper);
  return wrapper;
}

let mounted: ReturnType<typeof mount>[] = [];

afterEach(() => {
  for (const w of mounted) w.unmount();
  mounted = [];
  document.body.innerHTML = '';
});

describe('dropdown-menu.vue — 声明的 props 必须转发到 DropdownMenuRoot', () => {
  it('open=true 时 trigger 的 aria-expanded 为 true（受控分支被真正走到）', () => {
    const wrapper = mountRoot({ open: true });

    expect(wrapper.find('button').attributes('aria-expanded')).toBe('true');

  });

  it('dir=rtl 时下拉内容处于 rtl 阅读方向', () => {
    mountRoot({ open: true, dir: 'rtl' });

    const content = document.body.querySelector('[data-reka-menu-content]');
    expect(content).not.toBeNull();
    expect(content?.getAttribute('dir')).toBe('rtl');

  });

  it('dir=ltr 是默认值，说明上面那条不是因为「碰巧」', () => {
    mountRoot({ open: true });

    const content = document.body.querySelector('[data-reka-menu-content]');
    expect(content?.getAttribute('dir')).toBe('ltr');

  });

  it('modal=false 时不再给外部元素加 aria-hidden（modal 真的被转发）', () => {
    mountRoot({ open: true, modal: false });

    expect(document.body.querySelector('[aria-hidden="true"]')).toBeNull();

  });

  it('modal=true（reka 默认）会给外部元素加 aria-hidden，作为对照', () => {
    mountRoot({ open: true, modal: true });

    expect(document.body.querySelector('[aria-hidden="true"]')).not.toBeNull();

  });

  it('点击 trigger 依旧向外冒泡 update:open（转发 open 不能吞掉既有 emit 契约）', async () => {
    const wrapper = mountRoot({ open: false });

    await wrapper.find('button').trigger('click');

    expect(wrapper.emitted('update:open')).toEqual([[true]]);

  });

  it('defaultOpen 仍走非受控分支：挂载即展开', () => {
    mountRoot({ defaultOpen: true });

    expect(document.body.querySelector('button')?.getAttribute('aria-expanded')).toBe('true');

  });
});