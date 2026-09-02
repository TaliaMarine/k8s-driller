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
  requestSegments: DistSegment[]
  limitSegments: DistSegment[]
  usage: number
  format: (v: number) => string
}>()

/**
 * A two-row meter (SPECS.md dataviz guidance: sequential single hue for
 * "allocated," neutral track for "free," status red only for the
 * capacity-overflow case) — not per-pod categorical color, since identity
 * here comes from position + hover, not hue (a node can host far more pods
 * than a categorical palette can carry). Row A = requests (rarely
 * overflows — the scheduler enforces it). Row B = limits (can legitimately
 * exceed capacity — that's the overcommit risk this chart exists to show).
 * A single usage marker spans both rows: "here's where reality sits"
 * against both the booked baseline and the risk ceiling at once.
 */

const VIEW_W = 1000
const ROW_H = 22
const GAP_Y = 10
const SEG_GAP = 3 // px in viewBox units, the "surface gap" separator between segments
const VIEW_H = ROW_H * 2 + GAP_Y

const requestsTotal = computed(() => props.requestSegments.reduce((sum, s) => sum + s.value, 0))
const limitsTotal = computed(() => props.limitSegments.reduce((sum, s) => sum + s.value, 0))
const scaleMax = computed(() => Math.max(props.capacity, limitsTotal.value, props.usage, 1))
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

const requestLayout = computed(() => layout(props.requestSegments))
const limitLayout = computed(() => layout(props.limitSegments))
const overflowWidth = computed(() => Math.max(limitLayout.value.end - capacityX.value, 0))
const usageX = computed(() => (props.usage / scaleMax.value) * VIEW_W)
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
        {{ format(requestsTotal) }} requested · {{ format(limitsTotal) }} limit ·
        {{ format(usage) }} live of {{ format(capacity) }}
      </span>
    </div>

    <div class="dist-svg-wrap">
      <svg :viewBox="`0 0 ${VIEW_W} ${VIEW_H}`" preserveAspectRatio="none" class="dist-svg">
        <!-- Row A: requests -->
        <rect class="track" x="0" y="0" :width="VIEW_W" :height="ROW_H" rx="4" />
        <rect
          v-for="box in requestLayout.boxes"
          :key="`req-${box.seg.key}`"
          class="seg seg-request"
          :x="box.x"
          y="0"
          :width="box.width"
          :height="ROW_H"
          @mouseenter="showHover(box.seg, box.x)"
          @mouseleave="hideHover"
        >
          <title>{{ box.seg.name }}: {{ format(box.seg.value) }}</title>
        </rect>

        <!-- Row B: limits (may overflow past capacity -> overcommit) -->
        <rect class="track" x="0" :y="ROW_H + GAP_Y" :width="capacityX" :height="ROW_H" rx="4" />
        <rect
          v-for="box in limitLayout.boxes"
          :key="`lim-${box.seg.key}`"
          class="seg seg-limit"
          :x="box.x"
          :y="ROW_H + GAP_Y"
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
          :y="ROW_H + GAP_Y"
          :width="overflowWidth"
          :height="ROW_H"
          rx="4"
        >
          <title>Over capacity by {{ format(limitsTotal - capacity) }}</title>
        </rect>

        <!-- Capacity boundary -->
        <line class="capacity-line" :x1="capacityX" y1="-3" :x2="capacityX" :y2="VIEW_H + 3" />

        <!-- Live usage marker, spanning both rows -->
        <line class="usage-line" :x1="usageX" y1="-2" :x2="usageX" :y2="VIEW_H + 2" />
        <circle class="usage-dot" :cx="usageX" cy="-2" r="5" />
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
      <span><span class="swatch swatch-request" /> Requested</span>
      <span><span class="swatch swatch-limit" /> Limit</span>
      <span v-if="overcommit"><span class="swatch swatch-overflow" /> Over capacity</span>
      <span><span class="swatch swatch-usage" /> Live usage</span>
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
  height: 64px;
  display: block;
  overflow: visible;
}
.track {
  fill: rgb(var(--v-theme-surface-variant));
  opacity: 0.5;
}
.seg-request {
  fill: rgb(var(--v-theme-watch));
}
.seg-limit {
  fill: rgb(var(--v-theme-watch));
  opacity: 0.75;
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
}
.swatch {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 2px;
  margin-right: 4px;
}
.swatch-request {
  background: rgb(var(--v-theme-watch));
}
.swatch-limit {
  background: rgb(var(--v-theme-watch));
  opacity: 0.75;
}
.swatch-overflow {
  background: rgb(var(--v-theme-critical));
}
.swatch-usage {
  background: rgb(var(--v-theme-on-surface));
}
</style>
