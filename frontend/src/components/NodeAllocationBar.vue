<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  usage: number
  requests: number
  limits: number
  // Omitted (or 0) when there's no natural ceiling to mark — e.g. a
  // namespace summary, which has no capacity of its own — in which case no
  // marker renders at all rather than a misleading one sitting at 0%.
  capacity?: number
  format: (v: number) => string
}>()

// Usage, requests-sum, and limits-sum all scaled to the same max so an
// overcommitted node (limits sum > capacity) is visible as the capacity
// marker sitting inside the bar rather than at its far edge, instead of
// clipping the limits bar to 100% and hiding the overcommit.
const hasCapacity = computed(() => !!props.capacity && props.capacity > 0)
const max = computed(() =>
  Math.max(props.usage, props.requests, props.limits, props.capacity ?? 0, 1),
)
const scale = (v: number) => Math.min((v / max.value) * 100, 100)

const usagePct = computed(() => scale(props.usage))
const requestsPct = computed(() => scale(props.requests))
const limitsPct = computed(() => scale(props.limits))
const capacityPct = computed(() => (hasCapacity.value ? scale(props.capacity!) : 0))

const usageColor = computed(() => {
  if (!hasCapacity.value) return 'healthy'
  if (props.usage >= props.capacity!) return 'critical'
  if (props.usage >= props.capacity! * 0.9) return 'warning'
  return 'healthy'
})
</script>

<template>
  <div class="node-allocation-bar mb-2">
    <div class="bar-stack">
      <v-progress-linear
        :model-value="limitsPct"
        height="10"
        color="grey"
        bg-color="surface-variant"
        rounded
      />
      <v-progress-linear
        :model-value="requestsPct"
        height="10"
        color="watch"
        class="bar-overlay"
        rounded
      />
      <v-progress-linear
        :model-value="usagePct"
        height="10"
        :color="usageColor"
        class="bar-overlay"
        rounded
      />
      <div
        v-if="hasCapacity"
        class="capacity-marker"
        :style="{ left: `${capacityPct}%` }"
        :title="`Capacity: ${format(capacity!)}`"
      />
    </div>
    <div class="d-flex ga-3 text-caption text-medium-emphasis mt-1">
      <span>Usage {{ format(usage) }}</span>
      <span>Req {{ format(requests) }}</span>
      <span>Limit {{ format(limits) }}</span>
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
.capacity-marker {
  position: absolute;
  top: -2px;
  bottom: -2px;
  width: 2px;
  background: rgb(var(--v-theme-on-surface));
}
</style>
