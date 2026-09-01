<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  label: string
  usage: number
  request?: number
  limit?: number
  format: (v: number) => string
  danger?: boolean // OOM-Risk / Throttling-Risk highlight (SPECS.md §2.3)
}>()

/**
 * The Delta Visualizer: Real Usage <-> Configured Request <-> Configured
 * Limit, three bars scaled to the same max so the gap is visible at a
 * glance (SPECS.md §2.3).
 */
const max = computed(() => Math.max(props.usage, props.request ?? 0, props.limit ?? 0, 1))
const pct = (v?: number) => (v == null ? 0 : (v / max.value) * 100)
</script>

<template>
  <div class="mb-3">
    <div class="text-caption text-medium-emphasis mb-1">{{ label }}</div>
    <div class="d-flex align-center ga-2 mb-1">
      <span class="delta-row-label">Usage</span>
      <v-progress-linear
        :model-value="pct(usage)"
        height="10"
        :color="danger ? 'critical' : 'primary'"
        rounded
      />
      <span class="delta-row-value">{{ format(usage) }}</span>
    </div>
    <div class="d-flex align-center ga-2 mb-1">
      <span class="delta-row-label">Request</span>
      <v-progress-linear :model-value="pct(request)" height="10" color="watch" rounded />
      <span class="delta-row-value">{{ request != null ? format(request) : '—' }}</span>
    </div>
    <div class="d-flex align-center ga-2">
      <span class="delta-row-label">Limit</span>
      <v-progress-linear :model-value="pct(limit)" height="10" color="grey" rounded />
      <span class="delta-row-value">{{ limit != null ? format(limit) : '—' }}</span>
    </div>
  </div>
</template>

<style scoped>
.delta-row-label {
  width: 64px;
  font-size: 0.75rem;
  flex-shrink: 0;
}
.delta-row-value {
  width: 64px;
  font-size: 0.75rem;
  text-align: right;
  flex-shrink: 0;
}
</style>
