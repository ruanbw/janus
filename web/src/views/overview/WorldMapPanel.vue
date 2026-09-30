<template>
  <div class="panel flex flex-col">
    <div class="panel-hd">
      <div>
        <h2>访问来源地</h2>
        <p>按访客 IP 的离线 GeoIP 库判定国家，占比越高颜色越深。</p>
      </div>
      <span class="badge badge-neutral shrink-0">可定位 {{ locatedCount }} / {{ visits.length }}</span>
    </div>

    <div class="panel-bd flex-1">
      <!-- 一条都定位不出来时没什么可着色的，直接说清楚原因，不画一张全灰的地图 -->
      <div v-if="locatedCount === 0" class="empty flex flex-col items-center gap-1.5 py-10 text-center">
        <span class="text-[13px] font-medium text-ink">本次样本里没有能定位到国家的访问</span>
        <span class="max-w-sm text-[12px] leading-relaxed text-ink-faint">
          私网 / 回环地址与离线库未收录的地址会返回空国家码，这类访问不计入地图占比。
        </span>
      </div>

      <div v-else class="flex flex-col gap-4 lg:flex-row lg:items-start">
        <!--
          地图本体：viewBox 固定坐标系，宽度交给 CSS。
          移动端窄屏下按比例缩到约 340px 宽仍可读，故不需要为移动端换一套图。
        -->
        <div class="min-w-0 flex-1">
          <div
            v-if="mapFailed"
            class="empty flex flex-col items-center gap-1.5 py-10 text-center text-[13px] text-ink-soft"
          >
            <span>世界地图数据加载失败，下方列表仍然可用。</span>
            <AppButton size="sm" variant="outline" @click="retry">
              <template #icon><RefreshCw :size="13" /></template>
              重新加载
            </AppButton>
          </div>
          <svg
            v-else-if="shapes.length > 0"
            :viewBox="`0 0 ${MAP_WIDTH} ${MAP_HEIGHT}`"
            class="block h-auto w-full"
            role="img"
            :aria-label="`世界地图：${topCountries.length ? `访问量最高的 ${topCountries.length} 个国家是 ${topCountries.map((c) => c.name).join('、')}` : '暂无国家分布'}`"
          >
            <path
              v-for="shape in shapes"
              :key="shape.id"
              :d="shape.d"
              class="map-shape"
              :class="fillClassOf(shape.code)"
            >
              <!-- 原生 title：鼠标悬停即出，不引第三方 tooltip 也能读数 -->
              <title>{{ tooltipOf(shape.code) }}</title>
            </path>
          </svg>
          <div v-else class="py-16 text-center text-[12px] text-ink-faint">正在加载世界地图…</div>
        </div>

        <!-- 排行榜：手机没有悬停，触摸端靠这份列表读数 -->
        <ol class="grid shrink-0 gap-x-4 gap-y-1.5 text-[12.5px] sm:grid-cols-2 lg:w-[240px] lg:grid-cols-1">
          <li v-for="item in topCountries" :key="item.code" class="flex items-center gap-2">
            <span class="legend-key shrink-0" :class="`map-lv${levelOf(item.code)}`"></span>
            <span class="truncate text-ink">{{ item.name }}</span>
            <span class="ml-auto shrink-0 font-mono text-ink-soft">
              {{ item.count }} · {{ item.percent }}%
            </span>
          </li>
          <li v-if="restCountries" class="flex items-center gap-2 text-ink-faint">
            <span class="legend-key shrink-0 map-land"></span>
            <span class="truncate">其余 {{ restCountries }} 个国家</span>
            <span class="ml-auto shrink-0 font-mono">合计 {{ restVisits }} 次</span>
          </li>
        </ol>
      </div>
    </div>

    <div v-if="locatedCount > 0" class="panel-ft flex flex-wrap items-center gap-x-4 gap-y-2">
      <span class="legend m-0 p-0">
        <span v-for="item in MAP_LEVELS" :key="item.level" class="legend-item">
          <span class="legend-key" :class="`map-lv${item.level}`"></span>
          {{ item.label }}
        </span>
        <span class="legend-item">
          <span class="legend-key map-land"></span>
          无访问
        </span>
      </span>
      <span class="text-muted">占比按能定位的 {{ locatedCount }} 条样本计算，不等于全量访问结构。</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { RefreshCw } from '@lucide/vue';

import AppButton from '@/components/ui/AppButton.vue';
import type { Visit } from '@/types/api';

import { countryDistribution } from './trafficBreakdown';
import { MAP_HEIGHT, MAP_LEVELS, MAP_WIDTH, loadCountryShapes, mapLevel } from './worldMap';
import type { CountryShape } from './worldMap';

/** 榜单条数：再多地图右侧就撑不下了（每行约 24px + 间距） */
const TOP_COUNTRIES = 10;

const props = defineProps<{ visits: Visit[] }>();

const shapes = ref<CountryShape[]>([]);
const mapFailed = ref(false);

const breakdown = computed(() => countryDistribution(props.visits));
const locatedCount = computed(() => breakdown.value.reduce((sum, c) => sum + c.count, 0));
const topCountries = computed(() => breakdown.value.slice(0, TOP_COUNTRIES));
const restCountries = computed(() => Math.max(0, breakdown.value.length - TOP_COUNTRIES));
const restVisits = computed(() =>
  breakdown.value.slice(TOP_COUNTRIES).reduce((sum, c) => sum + c.count, 0),
);
/** alpha-2 → 该国家在样本内的访问次数与中文名 */
const byCode = computed(() => new Map(breakdown.value.map((c) => [c.code, c])));

function levelOf(code: string): number {
  return mapLevel((byCode.value.get(code)?.count ?? 0) / locatedCount.value);
}

function fillClassOf(code: string): string {
  // 0 档不是地图色，而是「本次样本里没有访问」；SVG 没有 fill 就默认涂黑，
  // 所以必须显式落回底色，否则整张地图会变成一片黑。
  const level = levelOf(code);
  return level === 0 ? 'map-land' : `map-lv${level}`;
}

function tooltipOf(code: string): string {
  const hit = byCode.value.get(code);
  if (!hit) return '无访问';
  const percent = ((hit.count / locatedCount.value) * 100).toFixed(1);
  return `${hit.name}：${hit.count} 次 · ${percent}%`;
}

async function loadShapes() {
  mapFailed.value = false;
  try {
    shapes.value = await loadCountryShapes();
  } catch {
    // 分包/网络失败不该让总览页整体挂掉，降级成「地图没了，榜单还在」
    mapFailed.value = true;
  }
}

// 形状只算一次（worldMap 内部有缓存），组件挂载时拉一次即可
void loadShapes();

function retry() {
  void loadShapes();
}
</script>
