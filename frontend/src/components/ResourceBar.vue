<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  label: string
  usage: number
  capacity: number
  // CPU reads as a plain percentage of capacity; memory reads as an
  // absolute usage/capacity pair (SPECS.md-style Ki/Mi/Gi units), since a
  // bare "42%" doesn't tell you whether that's 4GiB of 10GiB or 40GiB of
  // 100GiB — a distinction that matters for memory sizing decisions.
  unit: 'pct' | 'absolute'
  format?: (v: number) => string
}>()

const pct = computed(() => (props.capacity > 0 ? (props.usage / props.capacity) * 100 : 0))
const widthPct = computed(() => Math.min(pct.value, 100))

const color = computed(() => {
  if (pct.value >= 100) return 'critical'
  if (pct.value >= 90) return 'warning'
  return 'healthy'
})

const displayValue = computed(() =>
  props.unit === 'pct'
    ? `${Math.round(pct.value)}%`
    : `${props.format!(props.usage)} / ${props.format!(props.capacity)}`,
)
</script>

<template>
  <div class="resource-bar mb-2">
    <div class="d-flex justify-space-between text-caption mb-1">
      <span>{{ label }}</span>
      <span>{{ displayValue }}</span>
    </div>
    <v-progress-linear
      :model-value="widthPct"
      height="14"
      :color="color"
      bg-color="surface-variant"
      rounded
    />
  </div>
</template>
