/**
 * 世界地图的国界几何：把 Natural Earth 110m 国界 TopoJSON 投影成一组 SVG path。
 *
 * 为什么不直接画经纬度网格：那不是地图，只是一堆方块，用户看不出「访问来自哪」。
 * 也不在运行时拉 CDN 的 GeoJSON：斗篷常部署在内网/离线环境，拉不到就是一片空白，
 * 而空白没有任何降级价值。几何随前端包一起走，离线可用。
 *
 * 为什么单独一个模块还全走动态 import：国界数据本身 100KB+（压缩后 ~30KB），
 * d3-geo 也要几十 KB，而全站只有总览页的世界地图要它们。拆成懒加载 chunk，
 * 其余页面（规则、证书、短链列表）不为一张地图付这个体积。
 *
 * 投影选 Natural Earth（`geoNaturalEarth1`）而不是墨卡托：墨卡托会把格陵兰放大到
 * 跟非洲一样大、把南极撑到图外，前者是误导、后者是截断。Natural Earth 是等积投影，
 * 面积不失真，正是「按占比给国家填色」需要的前提。
 */
import { alpha2FromNumeric } from '@/constants/countries';
import type { FeatureCollection, Geometry } from 'geojson';
import type { GeometryCollection, Topology } from 'topojson-specification';

/** 地图画布尺寸（viewBox 坐标系，不是 CSS 像素——实际宽度交给 CSS 自适应） */
export const MAP_WIDTH = 880;
export const MAP_HEIGHT = 430;
/** 四周留白，避免贴边的国家被裁掉 */
const MAP_INSET = 8;

export interface CountryShape {
  /**
   * v-for 的 key。
   *
   * 优先用 TopoJSON 要素 id（ISO 3166-1 numeric）。但**不能只靠它**：
   * countries-110m.json 里的 `N. Cyprus` / `Somaliland` / `Kosovo` 三个要素
   * 根本没有 id 字段，`String(f.id ?? '')` 会给它们三个全等的空串 ——
   * 三个兄弟节点共用一个 key，Vue 会告警且 DOM diff 结果不可靠
   * （复用/错位，而不是重建）。所以回退到 properties.name，它是数据里
   * 一定存在且唯一的。
   */
  id: string;
  /** 换算出的 ISO 3166-1 alpha-2 码；空串表示没有对应的访问数据，画成底色 */
  code: string;
  /** SVG path 数据，坐标系同 MAP_WIDTH / MAP_HEIGHT */
  d: string;
}

/**
 * 填色分档：按**占比**而不是绝对次数切。
 *
 * 按次数切的话，一个只有 20 次访问的租户会得到「一片全白 + 一个黑块」，
 * 完全没有地理信息；按占比切则无论样本大小都有层次。
 */
export const MAP_LEVELS = [
  { level: 1, min: 0, label: '< 5%' },
  { level: 2, min: 0.05, label: '5 – 15%' },
  { level: 3, min: 0.15, label: '15 – 35%' },
  { level: 4, min: 0.35, label: '≥ 35%' },
] as const;

/** 占比（0–1）→ 填色档位。0 表示没有访问，走底色。 */
export function mapLevel(ratio: number): number {
  if (!(ratio > 0)) return 0;
  let level = 1;
  for (const item of MAP_LEVELS) {
    if (ratio >= item.min) level = item.level;
  }
  return level;
}

// TopoJSON 规范里的类型没有随包发出来，这里只声明本模块实际用到的字段。
// 其余字段（arcs / transform）原样透传——arcs 是弧表的真实数据，不能重建。
// id 刻意声明为可选：上面那条 key 回退规则正是因为数据里真有几个要素没有它。
type CountryGeometry = { type: 'Polygon' | 'MultiPolygon'; id?: string | number };

/**
 * 校验并收窄拓扑数据。
 *
 * 只检查本模块真正读的那一处（countries 几何集合）；arcs 坏掉会在 topojson-client
 * 内部抛错，由调用方降级，不在这里假装它合法。
 */
function readTopology(input: unknown): Topology<{ countries: GeometryCollection<CountryGeometry> }> {
  const topo = input as Topology<{ countries: GeometryCollection<CountryGeometry> }> | null;
  if (!Array.isArray(topo?.objects?.countries?.geometries)) {
    throw new Error('world-atlas 国界数据缺少 countries 对象');
  }
  return topo;
}

let shapesPromise: Promise<CountryShape[]> | null = null;

/**
 * 自动重试上限（总尝试次数，含第一次）。
 *
 * 为什么值得重试：唯一会真正命中这条失败路径的典型场景是**发版后旧 chunk 被清掉**——
 * 浏览器还在用上一版 index.html 去取地图 chunk，404 一次，而刷新就好。
 * 自动重试一次能无声救回绝大多数这种瞬时故障，不必让用户点「重新加载」。
 * 为什么必须有上限：网络不通或内网离线时重试只会放大请求量，且永远失败；
 * 定死的小整数上限保证最坏情况也只是多一次失败，不会变成循环。
 */
const MAX_LOAD_ATTEMPTS = 2;

/**
 * 加载并缓存国界几何。同一页面反复进入只算一次。
 *
 * 投影是纯函数，缓存形状即可；不缓存顶层的 chunk 请求，靠动态 import 自身的模块缓存。
 */
export function loadCountryShapes(): Promise<CountryShape[]> {
  if (!shapesPromise) {
    shapesPromise = loadWithRetry().catch((err) => {
      // 失败不留缓存，否则一次临时网络/分包故障就永久空白，只能刷页面
      shapesPromise = null;
      // 失败必须留痕：调用点（WorldMapPanel）的 catch 是空的，
      // 而 router.onError / unhandledrejection 都不覆盖组件 setup 之后自己发起的
      // 动态 import —— 不在这里记一条，生产环境就是一片无解释的空白地图。
      if (import.meta.env.DEV) {
        console.error('[worldMap] 世界地图几何加载失败', err);
      }
      throw err;
    });
  }
  return shapesPromise;
}

/** 有限次重试；任何一次成功都直接返回，不再重试 */
async function loadWithRetry(): Promise<CountryShape[]> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= MAX_LOAD_ATTEMPTS; attempt += 1) {
    try {
      return await buildCountryShapes();
    } catch (err) {
      lastError = err;
    }
  }
  throw lastError;
}

async function buildCountryShapes(): Promise<CountryShape[]> {
  const [topoMod, { feature }, { geoNaturalEarth1, geoPath }] = await Promise.all([
    import('world-atlas/countries-110m.json'),
    import('topojson-client'),
    import('d3-geo'),
  ]);
  const topo = readTopology(topoMod.default);

  const collection = feature(topo, topo.objects.countries) as unknown as FeatureCollection<Geometry>;
  // 南极洲按 fitExtent 会把整张图压扁一半（它横跨整个经度圈、面积占比不小），
  // 而它对「访问来自哪」几乎不携带信息，直接不进 fit 计算。
  // 同时在这里就把坐标非有限的要素丢掉：它们留在 fitExtent 的输入里会让
  // 投影整体算出 NaN，把**所有**国家的 path 一起毁掉。
  const land = collection.features.filter(
    (f) => alpha2Of(f) !== 'AQ' && !hasNonFiniteGeometry(f.geometry),
  );
  const projection = geoNaturalEarth1().fitExtent(
    [
      [MAP_INSET, MAP_INSET],
      [MAP_WIDTH - MAP_INSET, MAP_HEIGHT - MAP_INSET],
    ],
    { type: 'FeatureCollection', features: land },
  );
  const path = geoPath(projection);

  return land
    .map((f) => ({ id: shapeKeyOf(f), code: alpha2Of(f), d: path(f) ?? '' }))
    .filter((s) => s.d !== '' && !hasNonFiniteCoord(s.d));
}

/**
 * path 里是否混进了非有限坐标。
 *
 * 投影链（topojson 解码 → d3-geo）本身不校验经纬度：坐标里有一个 NaN/Infinity，
 * `path(f)` 就会原样吐出 `MNaN,8` 这种非法 path（`?? ''` 只兜 null/空串，兜不住它）。
 * 那不是「这块地画不出来」，而是整张 SVG 被浏览器判为非法，地图直接不出图。
 *
 * 这里按**整个要素**丢弃而不是就地修正坐标：猜不出那个 NaN 到底属于哪条边，
 * 补一个数只会把错误边界画成一条真实海岸线 —— 宁可少一块地，也不画一条编造的线。
 */
function hasNonFiniteCoord(d: string): boolean {
  // d3-geo 的数字输出只可能是有限数、"NaN"、"Infinity" 或科学计数指数，
  // 所以按子串判定即可，且不会误伤正常坐标。
  return d.includes('NaN') || d.includes('Infinity');
}

/**
 * 几何里是否含非有限坐标（NaN / Infinity），递归检查 Polygon / MultiPolygon。
 *
 * 必须在 fitExtent **之前**判：投影的缩放与平移是对全体要素求包围盒后算出来的，
 * 一个坏点就会把整张图的 scale 变成 NaN。
 */
function hasNonFiniteGeometry(geometry: Geometry): boolean {
  if (geometry.type === 'GeometryCollection') {
    return geometry.geometries.some(hasNonFiniteGeometry);
  }
  if (geometry.type !== 'Polygon' && geometry.type !== 'MultiPolygon') return false;
  // Polygon 是 position[][]，MultiPolygon 再多一层；逐个数字判足够，不需要认位置下标。
  return flattenPositions(geometry.coordinates).some(
    ([lng, lat]) => !Number.isFinite(lng) || !Number.isFinite(lat),
  );
}

/** 把（可能嵌套的）坐标数组摊平成 position 列表 */
function flattenPositions(value: unknown): Array<[number, number]> {
  if (!Array.isArray(value)) return [];
  if (value.length === 0) return [];
  const [head] = value;
  if (typeof head === 'number') return [[value[0] as number, value[1] as number]];
  return value.flatMap(flattenPositions);
}

// 单独抽一层是为了让 alpha2FromNumeric 保持可测、可复用，这里只负责取 id。
function alpha2Of(geometry: { id?: string | number }): string {
  return alpha2FromNumeric(geometry.id);
}

/** 要素的稳定唯一键：id 优先，缺失时回退到 properties.name。 */
function shapeKeyOf(geometry: { id?: string | number; properties?: unknown }): string {
  if (geometry.id !== undefined && geometry.id !== null && String(geometry.id) !== '') {
    return String(geometry.id);
  }
  // name 在 countries-110m.json 里是必带的；仍留一层兜底(序号由调用方补不到时
  // 至少不会退化成空串导致全表共用同一个 key)。
  const name = (geometry.properties as { name?: string } | null | undefined)?.name;
  return name ? `name:${name}` : 'unknown';
}
