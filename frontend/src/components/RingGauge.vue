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

interface Ring {
  key: string
  label: string
  value: number
  color: string
}

const rings = computed<Ring[]>(() => {
  const out: Ring[] = [
    {
      key: 'usage',
      label: 'Usage',
      value: props.usage,
      color: props.danger ? 'rgb(var(--v-theme-critical))' : 'rgb(var(--v-theme-watch))',
    },
  ]
  if (props.request != null) {
    out.push({
      key: 'request',
      label: 'Requests',
      value: props.request,
      color: 'rgb(var(--v-theme-healthy))',
    })
  }
  if (props.limit != null) {
    out.push({
      key: 'limit',
      label: 'Limits',
      value: props.limit,
      color: 'rgba(var(--v-theme-on-surface), 0.55)',
    })
  }
  if (props.maxUsage != null) {
    out.push({ key: 'max', label: 'Max', value: props.maxUsage, color: 'rgba(198, 40, 40, 0.55)' })
  }
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
          <g v-for="(ring, i) in rings" :key="ring.key">
            <circle
              :cx="CENTER"
              :cy="CENTER"
              :r="radiusFor(i)"
              fill="none"
              stroke="rgba(var(--v-theme-on-surface), 0.1)"
              :stroke-width="RING_WIDTH"
            />
            <circle
              :cx="CENTER"
              :cy="CENTER"
              :r="radiusFor(i)"
              fill="none"
              :stroke="ring.color"
              :stroke-width="RING_WIDTH"
              stroke-linecap="round"
              :stroke-dasharray="dashArray(ring.value, radiusFor(i))"
              :transform="`rotate(-90 ${CENTER} ${CENTER})`"
            />
          </g>
        </svg>
      </v-col>
      <v-col cols="12" sm="6" class="ring-gauge-legend">
        <div v-for="ring in rings" :key="ring.key" class="d-flex align-center ga-2 mb-1">
          <span class="ring-swatch" :style="{ background: ring.color }" />
          <span class="text-body-2">{{ ring.label }}: {{ format(ring.value) }}</span>
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
