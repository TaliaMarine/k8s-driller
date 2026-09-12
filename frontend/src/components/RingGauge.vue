<script setup lang="ts">
import { computed } from 'vue'

// Concentric ring gauge for one resource's Usage/Requests/Limits/Max
// (Grafana's "Kubernetes Compute Resources" panel style) — each ring is a
// full circle scaled to the same max so relative size is comparable, with
// the legend beside it on wide viewports and below on narrow ones.
const props = defineProps<{
  label: string
  usage: number
  request?: number
  limit?: number
  maxUsage?: number
  format: (v: number) => string
  danger?: boolean // OOM-Risk / Throttling-Risk highlight (SPECS.md §2.3)
}>()

const scaleMax = computed(() =>
  Math.max(props.usage, props.request ?? 0, props.limit ?? 0, props.maxUsage ?? 0, 1),
)

const USAGE_COLOR = 'rgb(var(--v-theme-watch))'
const DANGER_COLOR = 'rgb(var(--v-theme-critical))'
// Dark blue, drawn behind Usage in the same lane — Usage is always <= Max,
// so its brighter arc sits on top of (and is bounded by) this one.
const MAX_COLOR = '#01579B'
// Barely a shade above the unfilled track (0.1 alpha below) rather than the
// old 0.55 — Limits is a ceiling to glance at, not a value to compete with
// Usage/Max/Requests for attention.
const LIMIT_COLOR = 'rgba(var(--v-theme-on-surface), 0.2)'

interface RingLayer {
  color: string
  value: number
}
interface Lane {
  key: string
  layers: RingLayer[] // painted in order — later layers sit on top
}

// Usage and Max share one lane instead of each getting their own ring: Max
// (dark blue) paints first as the outer bound, then Usage (blue, or red
// when danger) paints over it, so the overlap itself shows how much
// headroom is left between current usage and the historical peak.
const lanes = computed<Lane[]>(() => {
  const out: Lane[] = []

  const usageMaxLayers: RingLayer[] = []
  if (props.maxUsage != null) usageMaxLayers.push({ color: MAX_COLOR, value: props.maxUsage })
  usageMaxLayers.push({ color: props.danger ? DANGER_COLOR : USAGE_COLOR, value: props.usage })
  out.push({ key: 'usage', layers: usageMaxLayers })

  if (props.request != null) {
    out.push({
      key: 'request',
      layers: [{ color: 'rgb(var(--v-theme-healthy))', value: props.request }],
    })
  }
  if (props.limit != null) {
    out.push({ key: 'limit', layers: [{ color: LIMIT_COLOR, value: props.limit }] })
  }
  return out
})

interface LegendItem {
  key: string
  label: string
  value: number
  color: string
}

const legendItems = computed<LegendItem[]>(() => {
  const out: LegendItem[] = [
    {
      key: 'usage',
      label: 'Usage',
      value: props.usage,
      color: props.danger ? DANGER_COLOR : USAGE_COLOR,
    },
  ]
  if (props.maxUsage != null)
    out.push({ key: 'max', label: 'Max', value: props.maxUsage, color: MAX_COLOR })
  if (props.request != null) {
    out.push({
      key: 'request',
      label: 'Requests',
      value: props.request,
      color: 'rgb(var(--v-theme-healthy))',
    })
  }
  if (props.limit != null)
    out.push({ key: 'limit', label: 'Limits', value: props.limit, color: LIMIT_COLOR })
  return out
})

const SIZE = 200
const CENTER = SIZE / 2
const RING_WIDTH = 14
const RING_GAP = 5
const OUTER_RADIUS = 86

function radiusFor(index: number): number {
  return OUTER_RADIUS - index * (RING_WIDTH + RING_GAP)
}
function dashArray(value: number, radius: number): string {
  const circumference = 2 * Math.PI * radius
  const filled = Math.min(value / scaleMax.value, 1) * circumference
  return `${filled} ${Math.max(circumference - filled, 0)}`
}
</script>

<template>
  <div class="ring-gauge mb-4">
    <div class="text-caption text-medium-emphasis mb-1">{{ label }}</div>
    <v-row no-gutters>
      <v-col cols="12" sm="6" class="d-flex justify-center align-center">
        <svg :viewBox="`0 0 ${SIZE} ${SIZE}`" class="ring-gauge-svg">
          <g v-for="(lane, i) in lanes" :key="lane.key">
            <circle
              :cx="CENTER"
              :cy="CENTER"
              :r="radiusFor(i)"
              fill="none"
              stroke="rgba(var(--v-theme-on-surface), 0.1)"
              :stroke-width="RING_WIDTH"
            />
            <circle
              v-for="layer in lane.layers"
              :key="layer.color"
              :cx="CENTER"
              :cy="CENTER"
              :r="radiusFor(i)"
              fill="none"
              :stroke="layer.color"
              :stroke-width="RING_WIDTH"
              stroke-linecap="round"
              :stroke-dasharray="dashArray(layer.value, radiusFor(i))"
              :transform="`rotate(-90 ${CENTER} ${CENTER})`"
            />
          </g>
        </svg>
      </v-col>
      <v-col cols="12" sm="6" class="ring-gauge-legend">
        <div v-for="item in legendItems" :key="item.key" class="d-flex align-center ga-2 mb-1">
          <span class="ring-swatch" :style="{ background: item.color }" />
          <span class="text-body-2">{{ item.label }}: {{ format(item.value) }}</span>
        </div>
      </v-col>
    </v-row>
  </div>
</template>

<style scoped>
.ring-gauge-svg {
  width: 100%;
  max-width: 170px;
  height: auto;
  display: block;
}
.ring-gauge-legend {
  display: flex;
  flex-direction: column;
  justify-content: center;
}
.ring-swatch {
  width: 10px;
  height: 10px;
  border-radius: 2px;
  flex-shrink: 0;
}
</style>
