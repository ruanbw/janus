# 总览页世界地图用随包国界 + d3-geo 投影,分析卡片按自身宽度换挡

总览页原本只有 KPI 与两列并排的分析面板,访问明细里一直带着的 `country` 字段(ADR 0009 落地 ip2region 离线库后填充)从没被用起来。这条决策定下:世界地图用 **随包发布的 Natural Earth 110m 国界 + `d3-geo` 的 `geoNaturalEarth1` 投影**做 choropleth(按访问占比分 4 档填色),三样依赖全部动态 `import()`,只在总览页加载;地图通栏置顶独占一行,其余 6 张分析/分布卡片一行三列,卡片内部的条形行按**卡片自身宽度**用 CSS `container query` 换挡。

**已实现(2026-10-01)**:`web/src/views/overview/worldMap.ts`(加载/投影/缓存/分档)+ `WorldMapPanel.vue`(面板,含三态降级)、`trafficBreakdown.ts` 的 `countryDistribution()`、`constants/countries.ts` 的 `alpha2FromNumeric()`、`main.css` 的 `.map-*` 与 `.panel-chart` 规则、`OverviewView.vue` 的网格重排。依赖 `d3-geo@3.1.1` / `topojson-client@3.1.0` / `world-atlas@2.0.2`(ISC 许可,允许再分发)。

## 背景与权衡

**随包发布,不运行时拉 CDN**:CLOAK 是自托管产品,大量部署在内网或离线环境,运行时 fetch 一份 GeoJSON 的代价是「有网才有图」,而且多一个外部故障点和一份没法审计的运行时依赖。`world-atlas` 的 110m 国界是 107KB(打包后 gzip 38.6KB),一次性付掉。否决「运行时拉 CDN GeoJSON」,也否决「构建时把 GeoJSON 烘进 HTML」——后者会把 100KB+ 字符串塞进每个路由的 HTML,失去按需加载的意义。

**真实国界 choropleth,而不是气泡地图**:气泡地图要自己维护一份「每个国家的质心坐标」表和一套投影,投影一旦和实际国界对不上,气泡就会落在海里;choropleth 的填色直接由路径算出,不存在落点问题。否决「零依赖气泡地图」的理由是维护成本换不到更多信息——同样的信息量,真实国界还多给了空间感。

**投影只算一次,缓存的是形状不是原始数据**:`geoNaturalEarth1().fitExtent(...)` 与 176 次 `geoPath()` 是纯计算,耗时在微秒到毫秒级,但没必要每次进页面重跑。缓存 `Promise<CountryShape[]>`,失败时**清掉缓存**再抛——否则一次临时分包/网络故障就永久空白,只能靠刷页面恢复。不缓存顶层的 chunk 请求,动态 `import()` 自身的模块缓存已经够了。

**先滤掉南极洲再 fitExtent**:南极洲横跨整个经度圈,在 `fitExtent` 的包围盒里面积占比不小,留着会把整张图垂直压扁掉近一半。它对「访问来自哪」几乎不携带信息,所以在 fit 计算之前就剔除——注意是剔除而不是画成别的颜色,画出来同样会让 fit 变形。

**分 4 档而不是连续色阶**:访问量分布是长尾的,连续色阶会把 99% 的国家压成几乎同一个浅色,读者看不出差异;4 档配图例,读者一眼能对上「深色 = 主要来源地」。

**0 档必须显式落回底色**:SVG 元素没有 `fill` 时默认涂黑。`mapLevel()` 返回 0 表示「本次样本里没有访问」,如果拼出的类名没有对应样式,整张世界地图会变成一片黑——只有去过的地方是白的。这是实现期实际踩到并修掉的缺陷,写在这里是因为「没有样式」和「样式是黑色」在 SVG 上长得一模一样。

**卡片条形行按自身宽度换挡,不用视口断点**:`.bar-row-lg` 的固定三栏(标签 124px + 条形 + 数值 116px + 间距 24px = 最少 264px)放不进 250px 的卡片。但「这张卡有多宽」不是视口的函数——侧边栏收没收起、列数是几档都在变,视口断点看不见这些。所以 `.panel-chart` 声明 `container-type: inline-size`,`.bar-row` 按容器宽度换挡。窄卡上数值列用 `max-content` 而不是固定 92px,否则 `1204 次 · 44%` 会被挤成两行。

**三列的断点取 xl(1280)而不是 lg(1024)**:桌面侧边栏占 224px。1024px 视口留给内容的只有 ~800px,三等分后每张卡不足 260px,条形图会被标签和读数挤没——那个宽度排两列反而更可读。KPI 卡只放一个数字,`lg` 就够,不跟着变。

## 后果

- 三样依赖合计 ~152KB(gzip ~55KB)只随总览页加载,但**这个成本随包体积走**:`world/dist` 经 go:embed 进二进制,国界数据在磁盘上只有一份,不是问题;若将来换更高精度的国界(50m 约 600KB),要先重新评估。
- `worldMap.ts` 里的 `readTopology` **只能校验、不能重建** topology 对象。arcs 是弧表的真实数据,重建会在 topojson-client 内部炸 `TypeError: Cannot read properties of undefined (reading 'length')`——类型对得上不代表数据能这么造。
- `env.d.ts` 里那条 `declare module 'world-atlas/countries-110m.json'` 不是偷懒:不声明的话 `resolveJsonModule` 会把 107KB JSON 展开成完整字面量类型,`vue-tsc` 从 6.8s 涨到 9.3s。
- 国界要素 id 是 ISO **numeric** 码,访问记录与规则条件里是 **alpha-2**,两者靠 `i18n-iso-countries` 的 `getNumericCodes()` 换算。换数据源时这条不变。
- `world-atlas` 110m 有 3 个要素没有可换算的 id(科索沃、北塞浦路斯等),画成底色;这是数据的已知状态,不是 bug。
- 地图与四张分布图算的都是**样本占比**(访问量前 10 条短链 × 各 50 条明细),不等于全量访问结构。地图面板的 `panel-ft` 与页面底部的样本量说明都要写明这一点。
- 地图数据加载失败(分包 404、缓存损坏等)时降级成一段说明 + 重试按钮,**国家排行榜照常可用**——它只依赖 `countryDistribution()`,不依赖 d3-geo。
