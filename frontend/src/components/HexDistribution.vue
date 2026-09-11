<script setup lang="ts">
import { computed } from 'vue'
import type { PodDTO } from '@/types/api'
import { containerAllocation } from '@/composables/usePodFilters'

// A honeycomb of one hexagon per pod (à la Datadog's host map), colored by
// how close usage sits to that pod's own ceiling — the same three-band
// severity scale as MiniRatioBar (healthy / warning past requests /
// critical past 90% of limit), so the palette reads the same as everywhere
// else pressure shows up (SPECS.md §7.2), just denser: many pods at a
// glance instead of one bar each.
const props = defineProps<{
  label: string
  pods: PodDTO[]
  resource: 'cpu' | 'mem'
  format: (v: number) => string
}>()
const emit = defineEmits<{ select: [name: string] }>()

interface HexCell {
  key: string
  name: string
  color: string
  tooltip: string
}

// Mirrors MiniRatioBar's denom/effectiveRequests logic (limit, or 2x
// requests when there's no limit; requests, or half the limit when there's
// no requests), then shades continuously within whichever of the three
// bands the ratio falls in, rather than a single flat color per band, so a
// pod just past its request reads differently from one about to hit 90%.
function hexFill(
  usage: number,
  requests: number,
  limits: number,
): { color: string; ratio: number | null } {
  const denom = limits || (requests ? requests * 2 : 0)
  if (!denom) return { color: 'rgba(var(--v-theme-wildwest), 0.5)', ratio: null }

  const effectiveRequests = requests || limits / 2
  const ratio = usage / denom
  const reqRatio = effectiveRequests / denom

  if (ratio >= 0.9) {
    const t = Math.min((ratio - 0.9) / 0.3, 1)
    return { color: `rgba(var(--v-theme-critical), ${(0.7 + t * 0.3).toFixed(2)})`, ratio }
  }
  if (ratio > reqRatio) {
    const t = (ratio - reqRatio) / Math.max(0.9 - reqRatio, 0.001)
    return { color: `rgba(var(--v-theme-warning), ${(0.45 + t * 0.35).toFixed(2)})`, ratio }
  }
  const t = reqRatio > 0 ? ratio / reqRatio : 0
  return { color: `rgba(var(--v-theme-healthy), ${(0.25 + t * 0.3).toFixed(2)})`, ratio }
}

const cells = computed<HexCell[]>(() =>
  props.pods.map((p) => {
    const usage = props.resource === 'cpu' ? p.usageCpu : p.usageMem
    const requests = containerAllocation(
      p,
      props.resource === 'cpu' ? 'requestsCpu' : 'requestsMem',
    )
    const limits = containerAllocation(p, props.resource === 'cpu' ? 'limitsCpu' : 'limitsMem')
    const { color, ratio } = hexFill(usage, requests, limits)
    const denomLabel = limits ? 'limit' : requests ? 'requests × 2' : null
    const tooltip =
      ratio != null && denomLabel
        ? `${p.name}: ${props.format(usage)} (${(ratio * 100).toFixed(0)}% of ${denomLabel})`
        : `${p.name}: ${props.format(usage)} — no request or limit configured`
    return { key: `${p.namespace}/${p.name}`, name: p.name, color, tooltip }
  }),
)

// A roughly 2.5:1 landscape grid, like the reference host map, regardless
// of how many pods there are.
const columns = computed(() =>
  Math.max(4, Math.min(32, Math.round(Math.sqrt(cells.value.length * 2.5)))),
)
const rows = computed(() => {
  const out: HexCell[][] = []
  for (let i = 0; i < cells.value.length; i += columns.value) {
    out.push(cells.value.slice(i, i + columns.value))
  }
  return out
})
</script>

<template>
  <div class="hex-distribution mb-4">
    <div class="text-caption text-medium-emphasis mb-1">{{ label }} · {{ pods.length }} pods</div>
    <div v-if="cells.length === 0" class="text-caption text-medium-emphasis">No pods.</div>
    <div v-else class="hex-grid">
      <div
        v-for="(row, ri) in rows"
        :key="ri"
        class="hex-row"
        :class="{ 'hex-row-offset': ri % 2 === 1 }"
      >
        <div
          v-for="cell in row"
          :key="cell.key"
          class="hex-cell"
          :style="{ background: cell.color }"
          :title="cell.tooltip"
          @click="emit('select', cell.name)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.hex-grid {
  display: flex;
  flex-direction: column;
}
.hex-row {
  display: flex;
  gap: 3px;
}
.hex-row:not(:first-child) {
  margin-top: -7px;
}
.hex-row-offset {
  margin-left: 14px;
}
.hex-cell {
  width: 24px;
  height: 28px;
  flex-shrink: 0;
  clip-path: polygon(50% 0%, 100% 25%, 100% 75%, 50% 100%, 0% 75%, 0% 25%);
  cursor: pointer;
  transition: opacity 0.15s;
}
.hex-cell:hover {
  opacity: 0.75;
}
</style>
