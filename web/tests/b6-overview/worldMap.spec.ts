/**
 * worldMap 的几何与加载契约。
 *
 * 三条断言都对着真实缺陷：
 *  1. 非有限坐标（NaN / Infinity）不得产出带 `NaN` 的 `d` —— 那是非法的 SVG path。
 *  2. 动态 import 失败必须**留痕**（旧 chunk 被清掉是发版后的真实故障），
 *     且失败不得污染缓存，否则一次临时故障就永久空白。
 *  3. 自动重试有上限，不能变成死循环。
 */
import { loadCountryShapes, mapLevel } from '@/views/overview/worldMap';

/**
 * 造一个只有几个要素的最小拓扑，绕开真实 110m 数据（体积与可读性都不适合进测试）。
 *
 * 必须走 TopoJSON 原生的 arcs 表达：countries-110m.json 里就是这个结构，
 * 而 topojson-client 会先把 arcs 解码成坐标再交给 d3-geo —— 只有真走完这条链，
 * 坐标里的 NaN 才会真的落到 path 输出上。
 */
function topoOf(features: Array<{ id?: string; ring: Array<[number, number]> }>) {
  const arcs: Array<Array<[number, number]>> = [];
  const geometries = features.map((f) => {
    const index = arcs.length;
    // 闭合环要重复首点，否则 topojson 的 ring() 会自己补点、补出来的点正好是 NaN
    arcs.push([...f.ring, f.ring[0]]);
    return {
      type: 'Polygon',
      id: f.id,
      properties: { name: f.id ?? 'unnamed' },
      arcs: [[index]],
    };
  });
  return {
    type: 'Topology',
    arcs,
    objects: { countries: { type: 'GeometryCollection', geometries } },
  };
}

/** 一个经纬度全部有限的小方块 */
const FINITE_RING: Array<[number, number]> = [
  [0, 0],
  [0, 10],
  [10, 10],
  [10, 0],
];

describe('loadCountryShapes 非有限坐标', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it('含 NaN 坐标的要素被丢弃，不产出带 NaN 的 path', async () => {
    vi.doMock('world-atlas/countries-110m.json', () => ({
      default: topoOf([
        { id: '840', ring: [[0, 0], [0, 10], [Number.NaN, 10], [10, 0]] },
        { id: '276', ring: FINITE_RING },
      ]),
    }));

    const { loadCountryShapes: load } = await import('@/views/overview/worldMap');
    const shapes = await load();

    expect(shapes).toHaveLength(1);
    expect(shapes.every((s) => !s.d.includes('NaN'))).toBe(true);
    expect(shapes[0]?.code).toBe('DE');
  });

  it('含 Infinity 坐标的要素同样被丢弃', async () => {
    vi.doMock('world-atlas/countries-110m.json', () => ({
      default: topoOf([
        { id: '392', ring: [[0, 0], [0, 10], [Number.POSITIVE_INFINITY, 10], [10, 0]] },
        { id: '840', ring: FINITE_RING },
      ]),
    }));

    const { loadCountryShapes: load } = await import('@/views/overview/worldMap');
    const shapes = await load();

    expect(shapes.every((s) => !/NaN|Infinity/i.test(s.d))).toBe(true);
    expect(shapes.map((s) => s.code)).toEqual(['US']);
  });
});

describe('loadCountryShapes 加载失败', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.doUnmock('world-atlas/countries-110m.json');
    vi.restoreAllMocks();
  });

  it('动态 import 失败时记录一次错误（不让故障静默消失）', async () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.doMock('world-atlas/countries-110m.json', () => {
      throw new Error('Failed to fetch dynamically imported module');
    });

    const { loadCountryShapes: load } = await import('@/views/overview/worldMap');

    await expect(load()).rejects.toThrow();
    expect(spy).toHaveBeenCalledTimes(1);
  });

  it('失败不留缓存：下一次调用会重新加载而不是复用 rejected promise', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    let fail = true;
    vi.doMock('world-atlas/countries-110m.json', () => {
      if (fail) throw new Error('chunk 404');
      return { default: topoOf([{ id: '840', ring: FINITE_RING }]) };
    });

    const { loadCountryShapes: load } = await import('@/views/overview/worldMap');
    await expect(load()).rejects.toThrow();

    fail = false;
    const shapes = await load();

    expect(shapes).toHaveLength(1);
  });

  it('瞬时失败（发版后旧 chunk 被清）自动重试一次即成功，无需用户手动点', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    let attempts = 0;
    vi.doMock('world-atlas/countries-110m.json', () => {
      attempts += 1;
      if (attempts === 1) throw new Error('Failed to fetch dynamically imported module');
      return { default: topoOf([{ id: '840', ring: FINITE_RING }]) };
    });

    const { loadCountryShapes: load } = await import('@/views/overview/worldMap');
    const shapes = await load();

    expect(attempts).toBe(2);
    expect(shapes).toHaveLength(1);
  });

  it('自动重试有上限：永久失败只尝试 2 次，绝不循环', async () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    let attempts = 0;
    vi.doMock('world-atlas/countries-110m.json', () => {
      attempts += 1;
      throw new Error('chunk 404');
    });

    const { loadCountryShapes: load } = await import('@/views/overview/worldMap');
    await expect(load()).rejects.toThrow();

    // 上限即行为契约：一次原始请求 + 一次重试，多一次都不允许
    expect(attempts).toBe(2);
    // 且只记一条错误，重试过程本身不刷屏
    expect(spy).toHaveBeenCalledTimes(1);
  });
});

describe('mapLevel', () => {
  it('无访问（ratio 非正数）走 0 档底色', () => {
    expect(mapLevel(0)).toBe(0);
    expect(mapLevel(Number.NaN)).toBe(0);
  });

  it('按占比切档', () => {
    expect(mapLevel(0.01)).toBe(1);
    expect(mapLevel(0.1)).toBe(2);
    expect(mapLevel(0.2)).toBe(3);
    expect(mapLevel(0.9)).toBe(4);
  });
});

describe('loadCountryShapes 静态导出', () => {
  it('导出的是函数（模块被动态 import 后仍可用）', () => {
    expect(typeof loadCountryShapes).toBe('function');
  });
});