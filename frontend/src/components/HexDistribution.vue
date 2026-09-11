<script setup lang="ts">
import { computed } from 'vue'
import type { PodDTO } from '@/types/api'
import { containerAllocation } from '@/composables/usePodFilters'
import HexGrid from './HexGrid.vue'
import type { HexCell } from './HexGrid.vue'

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

// Not-Ready and recently-deleted pods (see PodDTO.deleted — a short
// tombstone grace period on the backend, k8swatch.Store.PruneDeleted)
// override the usage-based severity color entirely: a plain gray scale
// (deleted darker than not-ready) rather than any hue, so they read as "not
// currently a live workload" at a glance regardless of what their last
// known usage/requests/limits were.
const NOT_READY_COLOR = 'rgba(var(--v-theme-on-surface), 0.25)'
const DELETED_COLOR = 'rgba(var(--v-theme-on-surface), 0.55)'

const cells = computed<HexCell[]>(() =>
  props.pods.map((p) => {
    const usage = props.resource === 'cpu' ? p.usageCpu : p.usageMem
    const key = `${p.namespace}/${p.name}`

    if (p.deleted) {
      return { key, value: p.name, color: DELETED_COLOR, tooltip: `${p.name}: deleted` }
    }
    if (!p.ready) {
      return {
        key,
        value: p.name,
        color: NOT_READY_COLOR,
        tooltip: `${p.name}: not ready (${props.format(usage)})`,
      }
    }

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
    return { key, value: p.name, color, tooltip }
  }),
)
</script>

<template>
  <div class="hex-distribution mb-4">
    <div class="text-caption text-medium-emphasis mb-1">{{ label }} · {{ pods.length }} pods</div>
    <HexGrid :cells="cells" @select="emit('select', $event)" />
  </div>
</template>
