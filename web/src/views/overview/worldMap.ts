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
  /** TopoJSON 要素 id（ISO 3166-1 numeric，数据自带且唯一，用作 v-for 的 key） */
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
 * 加载并缓存国界几何。同一页面反复进入只算一次。
 *
 * 投影是纯函数，缓存形状即可；不缓存顶层的 chunk 请求，靠动态 import 自身的模块缓存。
 */
export function loadCountryShapes(): Promise<CountryShape[]> {
  if (!shapesPromise) {
    shapesPromise = buildCountryShapes().catch((err) => {
      // 失败不留缓存，否则一次临时网络/分包故障就永久空白，只能刷页面
      shapesPromise = null;
      throw err;
    });
  }
  return shapesPromise;
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
  const land = collection.features.filter((f) => alpha2Of(f) !== 'AQ');
  const projection = geoNaturalEarth1().fitExtent(
    [
      [MAP_INSET, MAP_INSET],
      [MAP_WIDTH - MAP_INSET, MAP_HEIGHT - MAP_INSET],
    ],
    { type: 'FeatureCollection', features: land },
  );
  const path = geoPath(projection);

  return land
    .map((f) => ({ id: String(f.id ?? ''), code: alpha2Of(f), d: path(f) ?? '' }))
    .filter((s) => s.d !== '');
}

// 单独抽一层是为了让 alpha2FromNumeric 保持可测、可复用，这里只负责取 id。
function alpha2Of(geometry: { id?: string | number }): string {
  return alpha2FromNumeric(geometry.id);
}
