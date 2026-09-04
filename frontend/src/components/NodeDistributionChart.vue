<script setup lang="ts">
import { computed, reactive } from 'vue'

export interface DistSegment {
  key: string
  name: string
  value: number
}

const props = defineProps<{
  label: string
  capacity: number
  usageSegments: DistSegment[]
  requestSegments: DistSegment[]
  limitSegments: DistSegment[]
  usage: number // authoritative node-level live usage (metrics-server) — may exceed sum(usageSegments) by unattributed overhead, so it drives the marker line, not the usage row's own length
  format: (v: number) => string
}>()

/**
 * A three-row meter — usage, requests, limits — each showing its own
 * per-pod breakdown, so a real split is visible on every one of the three
 * numbers this chart exists to compare (SPECS.md dataviz guidance).
 * usage/requests/limits form a natural ordinal progression (actual ->
 * reserved -> ceiling), so all three rows share one hue at three distinct
 * lightness steps — an ordinal ramp, not per-pod categorical color, since
 * a node can host far more pods than any palette can carry; pod identity
 * comes from position + hover instead. Status red is reserved for the
 * limits-overflow (overcommit) case. A shared marker line spans all three
 * rows at the live-usage position, so its relationship to both the
 * request baseline and the limit ceiling reads at a glance.
 */

const VIEW_W = 1000
const ROW_H = 22
const GAP_Y = 8
const SEG_GAP = 3 // px in viewBox units, the "surface gap" separator between segments
const ROW_STRIDE = ROW_H + GAP_Y
const VIEW_H = ROW_H * 3 + GAP_Y * 2

const usageY = 0
const requestY = ROW_STRIDE
const limitY = ROW_STRIDE * 2

const requestsTotal = computed(() => props.requestSegments.reduce((sum, s) => sum + s.value, 0))
const limitsTotal = computed(() => props.limitSegments.reduce((sum, s) => sum + s.value, 0))
const scaleMax = computed(() =>
  Math.max(props.capacity, limitsTotal.value, requestsTotal.value, props.usage, 1),
)
const capacityX = computed(() => (props.capacity / scaleMax.value) * VIEW_W)

function layout(segments: DistSegment[]) {
  let x = 0
  const boxes: { seg: DistSegment; x: number; width: number }[] = []
  for (const seg of segments) {
    const width = Math.max((seg.value / scaleMax.value) * VIEW_W - SEG_GAP, 0)
    boxes.push({ seg, x, width })
    x += (seg.value / scaleMax.value) * VIEW_W
  }
  return { boxes, end: x }
}

const usageLayout = computed(() => layout(props.usageSegments))
const requestLayout = computed(() => layout(props.requestSegments))
const limitLayout = computed(() => layout(props.limitSegments))
const overflowWidth = computed(() => Math.max(limitLayout.value.end - capacityX.value, 0))
const usageMarkerX = computed(() => (props.usage / scaleMax.value) * VIEW_W)
const overcommit = computed(() => limitsTotal.value > props.capacity)

const hover = reactive<{ visible: boolean; name: string; value: number; x: number }>({
  visible: false,
  name: '',
  value: 0,
  x: 0,
})

function showHover(seg: DistSegment, x: number) {
  hover.visible = true
  hover.name = seg.name
  hover.value = seg.value
  hover.x = x
}
function hideHover() {
  hover.visible = false
}
</script>

<template>
  <div class="dist-chart">
    <div class="dist-header">
      <span class="text-body-2 font-weight-medium">{{ label }}</span>
      <span class="text-caption text-medium-emphasis dist-summary">
        {{ format(usage) }} live · {{ format(requestsTotal) }} requested ·
        {{ format(limitsTotal) }} limit · {{ format(capacity) }} capacity
      </span>
    </div>

    <div class="dist-svg-wrap">
      <svg :viewBox="`0 0 ${VIEW_W} ${VIEW_H}`" preserveAspectRatio="none" class="dist-svg">
        <!-- Row 1: live usage -->
        <rect class="track" x="0" :y="usageY" :width="VIEW_W" :height="ROW_H" rx="4" />
        <rect
          v-for="box in usageLayout.boxes"
          :key="`usage-${box.seg.key}`"
          class="seg seg-usage"
          :x="box.x"
          :y="usageY"
          :width="box.width"
          :height="ROW_H"
          @mouseenter="showHover(box.seg, box.x)"
          @mouseleave="hideHover"
        >
          <title>{{ box.seg.name }}: {{ format(box.seg.value) }}</title>
        </rect>

        <!-- Row 2: requests -->
        <rect class="track" x="0" :y="requestY" :width="VIEW_W" :height="ROW_H" rx="4" />
        <rect
          v-for="box in requestLayout.boxes"
          :key="`req-${box.seg.key}`"
          class="seg seg-request"
          :x="box.x"
          :y="requestY"
          :width="box.width"
          :height="ROW_H"
          @mouseenter="showHover(box.seg, box.x)"
          @mouseleave="hideHover"
        >
          <title>{{ box.seg.name }}: {{ format(box.seg.value) }}</title>
        </rect>

        <!-- Row 3: limits (may overflow past capacity -> overcommit) -->
        <rect class="track" x="0" :y="limitY" :width="capacityX" :height="ROW_H" rx="4" />
        <rect
          v-for="box in limitLayout.boxes"
          :key="`lim-${box.seg.key}`"
          class="seg seg-limit"
          :x="box.x"
          :y="limitY"
          :width="Math.min(box.width, Math.max(capacityX - box.x - SEG_GAP, 0))"
          :height="ROW_H"
          @mouseenter="showHover(box.seg, box.x)"
          @mouseleave="hideHover"
        >
          <title>{{ box.seg.name }}: {{ format(box.seg.value) }}</title>
        </rect>
        <rect
          v-if="overcommit"
          class="seg seg-overflow"
          :x="capacityX"
          :y="limitY"
          :width="overflowWidth"
          :height="ROW_H"
          rx="4"
        >
          <title>Over capacity by {{ format(limitsTotal - capacity) }}</title>
        </rect>

        <!-- Capacity boundary, spanning all three rows -->
        <line class="capacity-line" :x1="capacityX" y1="-3" :x2="capacityX" :y2="VIEW_H + 3" />

        <!-- Live usage marker, spanning all three rows -->
        <line class="usage-line" :x1="usageMarkerX" y1="-2" :x2="usageMarkerX" :y2="VIEW_H + 2" />
        <circle class="usage-dot" :cx="usageMarkerX" cy="-2" r="5" />
      </svg>

      <div
        v-if="hover.visible"
        class="dist-tooltip"
        :style="{ left: `${(hover.x / VIEW_W) * 100}%` }"
      >
        <span class="font-weight-medium">{{ format(hover.value) }}</span>
        <span class="text-medium-emphasis">{{ hover.name }}</span>
      </div>
    </div>

    <div class="dist-legend text-caption text-medium-emphasis">
      <span><span class="swatch swatch-usage" /> Usage (per pod)</span>
      <span><span class="swatch swatch-request" /> Requested</span>
      <span><span class="swatch swatch-limit" /> Limit</span>
      <span v-if="overcommit"><span class="swatch swatch-overflow" /> Over capacity</span>
      <span><span class="marker-swatch" /> Live total</span>
    </div>
  </div>
</template>

<style scoped>
.dist-chart {
  margin-bottom: 20px;
}
.dist-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 6px;
  flex-wrap: wrap;
  gap: 4px;
}
.dist-svg-wrap {
  position: relative;
  padding-top: 10px;
}
.dist-svg {
  width: 100%;
  height: 88px;
  display: block;
  overflow: visible;
}
.track {
  fill: rgb(var(--v-theme-surface-variant));
  opacity: 0.5;
}
/* Usage / requests / limits form one ordinal progression (actual -> reserved
   -> ceiling) on a single hue, three clearly distinct lightness steps —
   mixed toward the theme's own surface color so each step stays correct in
   both light and dark mode, not just alpha blended (which read as barely
   different from each other before this). */
.seg-usage {
  fill: rgb(var(--v-theme-watch));
}
.seg-request {
  fill: color-mix(in srgb, rgb(var(--v-theme-watch)) 58%, rgb(var(--v-theme-surface)));
}
.seg-limit {
  fill: color-mix(in srgb, rgb(var(--v-theme-watch)) 30%, rgb(var(--v-theme-surface)));
  stroke: rgb(var(--v-theme-watch));
  stroke-width: 1;
  stroke-opacity: 0.6;
}
.seg-overflow {
  fill: rgb(var(--v-theme-critical));
}
.capacity-line {
  stroke: rgb(var(--v-theme-on-surface));
  stroke-opacity: 0.35;
  stroke-width: 2;
}
.usage-line {
  stroke: rgb(var(--v-theme-on-surface));
  stroke-width: 2;
}
.usage-dot {
  fill: rgb(var(--v-theme-on-surface));
  stroke: rgb(var(--v-theme-surface));
  stroke-width: 2;
}
.dist-tooltip {
  position: absolute;
  top: -4px;
  transform: translate(-50%, -100%);
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-theme-on-surface), 0.15);
  border-radius: 4px;
  padding: 4px 8px;
  font-size: 12px;
  white-space: nowrap;
  display: flex;
  gap: 6px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
  pointer-events: none;
  z-index: 1;
}
.dist-legend {
  display: flex;
  gap: 16px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.swatch {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 2px;
  margin-right: 4px;
}
.swatch-usage {
  background: rgb(var(--v-theme-watch));
}
.swatch-request {
  background: color-mix(in srgb, rgb(var(--v-theme-watch)) 58%, rgb(var(--v-theme-surface)));
}
.swatch-limit {
  background: color-mix(in srgb, rgb(var(--v-theme-watch)) 30%, rgb(var(--v-theme-surface)));
  border: 1px solid rgba(var(--v-theme-watch), 0.6);
}
.swatch-overflow {
  background: rgb(var(--v-theme-critical));
}
.marker-swatch {
  display: inline-block;
  width: 2px;
  height: 10px;
  background: rgb(var(--v-theme-on-surface));
  margin-right: 4px;
  vertical-align: middle;
}
</style>
