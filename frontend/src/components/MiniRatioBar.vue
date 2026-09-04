<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  label: string
  usage: number
  request?: number
  format: (v: number) => string
}>()

// No request set at all -> ratio is meaningless (that pod is flagged
// Wild-West elsewhere); render nothing rather than a misleading 0%.
const ratio = computed(() => (props.request ? props.usage / props.request : null))

const color = computed(() => {
  if (ratio.value === null) return 'healthy'
  if (ratio.value >= 1) return 'critical'
  if (ratio.value >= 0.9) return 'warning'
  return 'healthy'
})

const widthPct = computed(() => Math.min((ratio.value ?? 0) * 100, 100))
</script>

<template>
  <div
    v-if="ratio !== null"
    class="mini-ratio-bar"
    :title="`${label}: ${format(usage)} / ${format(request ?? 0)} requested (${Math.round(ratio * 100)}%)`"
  >
    <span class="mini-ratio-label">{{ label }}</span>
    <div class="mini-ratio-track">
      <div class="mini-ratio-fill" :class="`bg-${color}`" :style="{ width: `${widthPct}%` }" />
    </div>
  </div>
</template>

<style scoped>
.mini-ratio-bar {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.mini-ratio-label {
  font-size: 10px;
  line-height: 1;
  color: rgb(var(--v-theme-on-surface));
  opacity: 0.6;
}
.mini-ratio-track {
  width: 36px;
  height: 6px;
  border-radius: 3px;
  background: rgb(var(--v-theme-surface-variant));
  overflow: hidden;
}
.mini-ratio-fill {
  height: 100%;
  border-radius: 3px;
}
</style>
