<script setup lang="ts">
import { computed } from 'vue'

// A tiny share-of-whole indicator that sits before a pod's CPU/Memory bar:
// how much of the surrounding scope (node capacity, cluster capacity, or
// namespace-wide usage — the caller decides via `total`) this one pod
// accounts for. Deliberately not scaled against the same request/limit
// denominator as MiniRatioBar next to it — that bar is about the pod's own
// ceiling, this pie is about the pod's weight in a larger group.
const props = defineProps<{
  label: string
  value: number
  total?: number
  format: (v: number) => string
}>()

const pct = computed(() => {
  if (!props.total || props.total <= 0) return null
  return Math.min((props.value / props.total) * 100, 100)
})

const style = computed(() => {
  if (pct.value == null) return {}
  return {
    background: `conic-gradient(rgb(var(--v-theme-watch)) ${pct.value}%, rgba(var(--v-theme-on-surface), 0.15) ${pct.value}%)`,
  }
})

const tooltip = computed(() => {
  if (pct.value == null) return `${props.label}: share of total unavailable`
  return `${props.label}: ${props.format(props.value)} (${pct.value.toFixed(1)}% of ${props.format(props.total!)})`
})
</script>

<template>
  <div
    class="pie-ratio"
    :class="{ 'pie-ratio-empty': pct == null }"
    :style="style"
    :title="tooltip"
  />
</template>

<style scoped>
.pie-ratio {
  flex-shrink: 0;
  width: 12px;
  height: 12px;
  border-radius: 50%;
}
.pie-ratio-empty {
  background: transparent;
  border: 1px dashed rgba(var(--v-theme-on-surface), 0.25);
}
</style>
