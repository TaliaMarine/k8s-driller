<script setup lang="ts">
import { computed, reactive } from 'vue'
import type { Sample } from '@/types/api'

const props = defineProps<{
  label: string
  samples: Sample[]
  requestValue?: number
  limitValue?: number
  format: (v: number) => string
}>()

/**
 * A single-hue usage-over-time line + area, with dashed reference lines for
 * the pod's current request/limit — the same "one hue, ordinal reference
 * lines" approach as NodeDistributionChart, applied to a time series instead
 * of a part-to-whole breakdown (SPECS.md dataviz guidance: sequential
 * magnitude data gets one hue, not a rainbow).
 */

const VIEW_W = 1000
const VIEW_H = 160
const PAD_TOP = 6

const values = computed(() => props.samples.map((s) => s.v))
const scaleMax = computed(
  () => Math.max(...values.value, props.requestValue ?? 0, props.limitValue ?? 0, 1) * 1.08,
)

function yFor(v: number): number {
  return VIEW_H - (v / scaleMax.value) * (VIEW_H - PAD_TOP)
}

const linePath = computed(() => {
  const n = props.samples.length
  if (n === 0) return ''
  return values.value
    .map((v, i) => `${i === 0 ? 'M' : 'L'} ${(i / Math.max(n - 1, 1)) * VIEW_W} ${yFor(v)}`)
    .join(' ')
})

const areaPath = computed(() => {
  const n = props.samples.length
  if (n === 0) return ''
  return `${linePath.value} L ${VIEW_W} ${VIEW_H} L 0 ${VIEW_H} Z`
})

const requestY = computed(() => (props.requestValue != null ? yFor(props.requestValue) : undefined))
const limitY = computed(() => (props.limitValue != null ? yFor(props.limitValue) : undefined))

const hover = reactive<{ visible: boolean; x: number; value: number; t: string }>({
  visible: false,
  x: 0,
  value: 0,
  t: '',
})

function onMove(e: MouseEvent) {
  const target = e.currentTarget as SVGElement
  const rect = target.getBoundingClientRect()
  const n = props.samples.length
  if (n === 0) return
  const frac = Math.min(Math.max((e.clientX - rect.left) / rect.width, 0), 1)
  const idx = Math.round(frac * (n - 1))
  const sample = props.samples[idx]
  hover.visible = true
  hover.x = (idx / Math.max(n - 1, 1)) * VIEW_W
  hover.value = sample.v
  hover.t = new Date(sample.t).toLocaleString()
}
function onLeave() {
  hover.visible = false
}
</script>

<template>
  <div class="history-chart">
    <div class="text-caption text-medium-emphasis mb-1">{{ label }}</div>
    <div class="history-svg-wrap">
      <svg
        :viewBox="`0 0 ${VIEW_W} ${VIEW_H}`"
        preserveAspectRatio="none"
        class="history-svg"
        @mousemove="onMove"
        @mouseleave="onLeave"
      >
        <line
          v-if="limitY != null"
          class="limit-line"
          x1="0"
          :y1="limitY"
          :x2="VIEW_W"
          :y2="limitY"
        />
        <line
          v-if="requestY != null"
          class="request-line"
          x1="0"
          :y1="requestY"
          :x2="VIEW_W"
          :y2="requestY"
        />
        <path class="area" :d="areaPath" />
        <path class="line" :d="linePath" />
        <line
          v-if="hover.visible"
          class="hover-line"
          :x1="hover.x"
          y1="0"
          :x2="hover.x"
          :y2="VIEW_H"
        />
      </svg>
      <div
        v-if="hover.visible"
        class="history-tooltip"
        :style="{ left: `${(hover.x / VIEW_W) * 100}%` }"
      >
        <span class="font-weight-medium">{{ format(hover.value) }}</span>
        <span class="text-medium-emphasis">{{ hover.t }}</span>
      </div>
    </div>
    <div class="history-legend text-caption text-medium-emphasis">
      <span><span class="swatch swatch-usage" /> Usage</span>
      <span v-if="requestValue != null"><span class="swatch swatch-request" /> Request</span>
      <span v-if="limitValue != null"><span class="swatch swatch-limit" /> Limit</span>
    </div>
  </div>
</template>

<style scoped>
.history-chart {
  margin-bottom: 16px;
}
.history-svg-wrap {
  position: relative;
}
.history-svg {
  width: 100%;
  height: 120px;
  display: block;
  overflow: visible;
}
.area {
  fill: rgb(var(--v-theme-watch));
  opacity: 0.15;
}
.line {
  fill: none;
  stroke: rgb(var(--v-theme-watch));
  stroke-width: 2;
}
.request-line {
  stroke: rgb(var(--v-theme-watch));
  stroke-opacity: 0.6;
  stroke-width: 1.5;
  stroke-dasharray: 6 4;
}
.limit-line {
  stroke: rgb(var(--v-theme-on-surface));
  stroke-opacity: 0.35;
  stroke-width: 1.5;
  stroke-dasharray: 6 4;
}
.hover-line {
  stroke: rgb(var(--v-theme-on-surface));
  stroke-opacity: 0.3;
  stroke-width: 1;
}
.history-tooltip {
  position: absolute;
  top: 0;
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
.history-legend {
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
.swatch-usage {
  background: rgb(var(--v-theme-watch));
}
.swatch-request {
  border-bottom: 2px dashed rgb(var(--v-theme-watch));
  background: transparent;
  height: 0;
  vertical-align: middle;
}
.swatch-limit {
  border-bottom: 2px dashed rgba(var(--v-theme-on-surface), 0.5);
  background: transparent;
  height: 0;
  vertical-align: middle;
}
</style>
