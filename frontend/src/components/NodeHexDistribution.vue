<script setup lang="ts">
import { computed } from 'vue'
import type { NodeDTO } from '@/types/api'
import { nodeHealthColor } from '@/utils/format'
import HexGrid from './HexGrid.vue'
import type { HexCell } from './HexGrid.vue'

// One hexagon per node, colored by the same health classification as
// everywhere else a node's status appears (NodeCard's border/chip) —
// SPECS.md §7.2's "learn the palette once" — rather than a bespoke
// usage-ratio gradient like HexDistribution.vue uses for pods: a node's
// Health already IS the severity verdict (Healthy / CPU Pressure / Mem
// Pressure / Overcommit / Not Ready / Unschedulable), so there's no ratio
// left to compute.
const props = defineProps<{ nodes: NodeDTO[] }>()
const emit = defineEmits<{ select: [name: string] }>()

const cells = computed<HexCell[]>(() =>
  props.nodes.map((n) => ({
    key: n.name,
    value: n.name,
    color: `rgb(var(--v-theme-${nodeHealthColor(n.health)}))`,
    tooltip: `${n.name}: ${n.health} (CPU ${Math.round(n.pressure.liveCpuPct)}% · Mem ${Math.round(n.pressure.liveMemPct)}%)`,
  })),
)
</script>

<template>
  <div class="node-hex-distribution mb-4">
    <div class="text-caption text-medium-emphasis mb-1">{{ nodes.length }} nodes</div>
    <HexGrid :cells="cells" @select="emit('select', $event)" />
  </div>
</template>
