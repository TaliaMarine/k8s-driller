<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  label: string
  allocationPct: number // Layer A: configured requests/limits vs capacity (SPECS.md §2.1)
  livePct: number // Layer B: real-time usage, overlaid on Layer A
  overcommit?: boolean
}>()

// Live severity and overcommit are two different questions — "is usage
// actually a problem right now" vs. "could the config become a problem" —
// so they get two separate signals instead of overcommit forcing the live
// bar to critical regardless of actual usage. Live: healthy/warning/critical
// by usage alone. Allocation (below): grey normally, warning when
// overcommitted.
const liveColor = computed(() => {
  if (props.livePct > 90) return 'critical'
  if (props.livePct > 75) return 'warning'
  return 'healthy'
})
const allocationColor = computed(() => (props.overcommit ? 'warning' : 'grey'))
</script>

<template>
  <div class="resource-bar mb-2">
    <div class="d-flex justify-space-between text-caption mb-1">
      <span>{{ label }}</span>
      <span>{{ Math.round(livePct) }}% live / {{ Math.round(allocationPct) }}% allocated</span>
    </div>
    <div class="bar-stack">
      <v-progress-linear
        :model-value="Math.min(allocationPct, 100)"
        height="14"
        :color="allocationColor"
        bg-color="surface-variant"
        rounded
      />
      <v-progress-linear
        :model-value="Math.min(livePct, 100)"
        height="14"
        :color="liveColor"
        class="bar-overlay"
        rounded
      />
    </div>
  </div>
</template>

<style scoped>
.bar-stack {
  position: relative;
}
.bar-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  opacity: 0.85;
}
</style>
