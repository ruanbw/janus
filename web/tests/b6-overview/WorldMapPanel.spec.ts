/**
 * 世界地图面板的交互契约。
 *
 * 这一组只钉一件事：**没有 alpha-2 码的要素不能一起高亮**。
 * countries-110m.json 里 N. Cyprus / Somaliland / Kosovo 三个要素没有 id，
 * alpha2FromNumeric 对它们一律返回空串。若高亮用「code 相等」判定，
 * 指针落到其中任意一块上时三块会同时亮起（空串 === 空串），看上去像
 * 「访问来自三个国家」，而实际上这三个国家一个都没数据。
 *
 * 高亮身份必须是**单个要素**（shape.id），而不是会重复的 code。
 */
const shapeMock = vi.hoisted(() => ({
  shapes: [] as Array<{ id: string; code: string; d: string }>,
}));

vi.mock('@/views/overview/worldMap', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/views/overview/worldMap')>();
  return {
    ...actual,
    loadCountryShapes: () => Promise.resolve(shapeMock.shapes),
  };
});

import { mount } from '@vue/test-utils';

import WorldMapPanel from '@/views/overview/WorldMapPanel.vue';

const NO_ID_FEATURES = [
  { id: 'name:N. Cyprus', code: '', d: 'M0,0L10,0L10,10Z' },
  { id: 'name:Somaliland', code: '', d: 'M20,0L30,0L30,10Z' },
  { id: 'name:Kosovo', code: '', d: 'M40,0L50,0L50,10Z' },
];

describe('WorldMapPanel 悬停高亮', () => {
  beforeEach(() => {
    shapeMock.shapes = NO_ID_FEATURES;
  });

  it('悬停一个无 code 的要素时，只有它自己高亮', async () => {
    const wrapper = mount(WorldMapPanel, {
      props: {
        countries: [{ value: 'US', count: 10 }],
        totalVisits: 10,
        coverageTruncated: false,
        userAgentLimit: 500,
      },
    });
    await new Promise((r) => setTimeout(r, 0));

    const paths = wrapper.findAll('svg path');
    expect(paths).toHaveLength(3);

    await paths[0].trigger('mouseenter');

    const active = wrapper.findAll('svg path').filter((p) => p.classes().includes('is-active'));
    expect(active).toHaveLength(1);
    expect(active[0].attributes('d')).toBe(NO_ID_FEATURES[0].d);

    wrapper.unmount();
  });

  it('移到别的要素上，高亮跟着换而不是叠加', async () => {
    const wrapper = mount(WorldMapPanel, {
      props: {
        countries: [{ value: 'US', count: 10 }],
        totalVisits: 10,
        coverageTruncated: false,
        userAgentLimit: 500,
      },
    });
    await new Promise((r) => setTimeout(r, 0));

    const paths = wrapper.findAll('svg path');
    await paths[0].trigger('mouseenter');
    await paths[2].trigger('mouseenter');

    const active = wrapper.findAll('svg path').filter((p) => p.classes().includes('is-active'));
    expect(active).toHaveLength(1);
    expect(active[0].attributes('d')).toBe(NO_ID_FEATURES[2].d);

    wrapper.unmount();
  });

  it('有 code 的国家：地图与右侧榜单仍然双向联动', async () => {
    shapeMock.shapes = [
      { id: '840', code: 'US', d: 'M0,0L10,0L10,10Z' },
      { id: '276', code: 'DE', d: 'M20,0L30,0L30,10Z' },
    ];

    const wrapper = mount(WorldMapPanel, {
      props: {
        countries: [{ value: 'US', count: 10 }],
        totalVisits: 10,
        coverageTruncated: false,
        userAgentLimit: 500,
      },
    });
    await new Promise((r) => setTimeout(r, 0));

    // 榜单悬停有数据的国家 → 地图上对应国家高亮
    const listItem = wrapper.findAll('li').find((li) => li.text().includes('美国'));
    expect(listItem).toBeTruthy();
    await listItem!.trigger('mouseenter');

    const active = wrapper.findAll('svg path').filter((p) => p.classes().includes('is-active'));
    expect(active).toHaveLength(1);
    expect(active[0].attributes('d')).toBe('M0,0L10,0L10,10Z');

    // 地图悬停有数据的国家 → 读数给出中文名与次数占比
    const usPath = wrapper.findAll('svg path')[0];
    await usPath.trigger('mouseenter');
    expect(wrapper.text()).toContain('美国');
    expect(wrapper.text()).toContain('10 次 · 100.0%');

    // 地图悬停一个无数据的国家 → 浮动读数如实说「无访问记录」并带上 alpha-2，
    // 而不是默默消失（用户会以为那里没数据，其实只是这片没人来）
    await wrapper.findAll('svg path')[1].trigger('mouseenter');
    expect(wrapper.text()).toContain('无访问记录');
    expect(wrapper.text()).toContain('[DE]');

    wrapper.unmount();
  });
});