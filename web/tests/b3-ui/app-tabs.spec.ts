/**
 * AppTabs 系列的合法结构契约：role="tab" 必须通过 aria-controls 指向一个真实存在的 tabpanel。
 *
 * 背景：reka 的 TabsTrigger 里
 *   `contentId = contentIds.has(value) ? makeContentId(...) : void 0`
 * —— 只有当同一 value 的 TabsContent 真的挂载了，aria-controls 才有值。
 * 本仓库有两处调用（LinksView、ErrorPagesCard）只用了 TabsList + TabsTrigger，
 * 没有 AppTabsContent，于是这些 tab 的 aria-controls 恒为 undefined，
 * 读屏器点过去没有任何落点。
 *
 * 本文件只锁「封装层能表达合法结构」这一条：
 * 有 AppTabsContent 时 aria-controls 必须与 panel 的 id 对得上。
 * 调用方的结构问题在报告里说明（调用方不归本 lane）。
 */
import { mount } from '@vue/test-utils';
import { h, nextTick } from 'vue';

import AppTabs from '@/components/app/AppTabs.vue';
import AppTabsContent from '@/components/app/AppTabsContent.vue';
import AppTabsList from '@/components/app/AppTabsList.vue';
import AppTabsTrigger from '@/components/app/AppTabsTrigger.vue';

/**
 * 搭出完整合法的 tabs 结构（含 AppTabsContent）。
 * reka 在 TabsContent 的 onMounted 里才 registerContent，trigger 的 aria-controls
 * 依赖 contentIds 这个 computed，所以断言前必须让出一个 tick。
 */
async function mountWithContent(modelValue = 'a') {
  const wrapper = mount(AppTabs, {
    props: { modelValue },
    slots: {
      default: () => [
        h(AppTabsList, null, {
          default: () => [
            h(AppTabsTrigger, { value: 'a' }, { default: () => 'A' }),
            h(AppTabsTrigger, { value: 'b' }, { default: () => 'B' }),
          ],
        }),
        h(AppTabsContent, { value: 'a' }, { default: () => '面板A' }),
        h(AppTabsContent, { value: 'b' }, { default: () => '面板B' }),
      ],
    },
  });
  await nextTick();
  return wrapper;
}

describe('AppTabs — 合法结构下 aria-controls 指向真实 panel', () => {
  it('每个 trigger 的 aria-controls 都等于对应 panel 的 id', async () => {
    const wrapper = await mountWithContent();

    const triggers = wrapper.findAll('[role="tab"]');
    expect(triggers).toHaveLength(2);

    for (const trigger of triggers) {
      const controls = trigger.attributes('aria-controls');
      // aria-controls 必须存在
      expect(controls).toBeTruthy();
      // 且必须真的指向同一棵树里存在的一个 panel 元素
      expect(wrapper.find(`#${controls}`).exists()).toBe(true);
    }

    wrapper.unmount();
  });

  it('panel 带 role="tabpanel" 且被 aria-labelledby 关联回 trigger', async () => {
    const wrapper = await mountWithContent();

    const panel = wrapper.find('[role="tabpanel"]');
    expect(panel.exists()).toBe(true);
    expect(panel.attributes('id')).toBe(wrapper.find('[role="tab"]').attributes('aria-controls'));
    expect(panel.attributes('aria-labelledby')).toBe(wrapper.find('[role="tab"]').attributes('id'));

    wrapper.unmount();
  });

  it('trigger 与 panel 的 value 一一对应（不能串号）', async () => {
    const wrapper = await mountWithContent('a');

    const [tabA, tabB] = wrapper.findAll('[role="tab"]');
    const panels = wrapper.findAll('[role="tabpanel"]');

    expect(tabA.attributes('aria-controls')).toBe(panels[0].attributes('id'));
    expect(tabB.attributes('aria-controls')).toBe(panels[1].attributes('id'));
    expect(tabA.attributes('aria-controls')).not.toBe(tabB.attributes('aria-controls'));

    wrapper.unmount();
  });

  it('AppTabsContent 的 class 真的落到了 panel 上（不被静默丢弃）', () => {
    const wrapper = mount(AppTabs, {
      props: { modelValue: 'a' },
      slots: {
        default: () => [
          h(AppTabsList, null, { default: () => h(AppTabsTrigger, { value: 'a' }, { default: () => 'A' }) }),
          h(AppTabsContent, { value: 'a', class: 'panel-cls' }, { default: () => '面板A' }),
        ],
      },
    });

    expect(wrapper.find('[role="tabpanel"]').classes()).toContain('panel-cls');

    wrapper.unmount();
  });

  it('AppTabsList 的 class 真的落到了 list 上（不被静默丢弃）', () => {
    const wrapper = mount(AppTabs, {
      props: { modelValue: 'a' },
      slots: {
        default: () => [
          h(AppTabsList, { class: 'list-cls' }, { default: () => h(AppTabsTrigger, { value: 'a' }, { default: () => 'A' }) }),
          h(AppTabsContent, { value: 'a' }, { default: () => '面板A' }),
        ],
      },
    });

    expect(wrapper.find('[role="tablist"]').classes()).toContain('list-cls');

    wrapper.unmount();
  });

  it('AppTabs 的 class 与自身布局类合并后一起落到 tabs 根上', () => {
    const wrapper = mount(AppTabs, {
      props: { modelValue: 'a', class: 'tabs-cls' },
      slots: {
        default: () => [
          h(AppTabsList, null, { default: () => h(AppTabsTrigger, { value: 'a' }, { default: () => 'A' }) }),
          h(AppTabsContent, { value: 'a' }, { default: () => '面板A' }),
        ],
      },
    });

    const root = wrapper.find('[dir]');
    expect(root.classes()).toContain('tabs-cls');
    // 组件自身的布局类不能被调用方的 class 顶掉
    expect(root.classes()).toContain('flex');

    wrapper.unmount();
  });

  it('AppTabsTrigger 的 class 真的落到了 trigger 上（不被静默丢弃）', () => {
    const wrapper = mount(AppTabs, {
      props: { modelValue: 'a' },
      slots: {
        default: () => [
          h(AppTabsList, null, {
            default: () => h(AppTabsTrigger, { value: 'a', class: 'trg-cls' }, { default: () => 'A' }),
          }),
          h(AppTabsContent, { value: 'a' }, { default: () => '面板A' }),
        ],
      },
    });

    expect(wrapper.find('[role="tab"]').classes()).toContain('trg-cls');

    wrapper.unmount();
  });
});