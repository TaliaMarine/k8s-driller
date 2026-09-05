<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  label: string
  usage: number
  requests?: number
  limits?: number
  format: (v: number) => string
}>()

// Scale against the limit — the real ceiling a pod can be throttled/OOM-killed
// at — falling back to 2x requests when there's no limit, since Kubernetes
// gives no other natural ceiling in that case. When a limit exists but
// requests doesn't, requests is assumed to be half the limit purely so the
// marker below has somewhere sensible to sit — it's never treated as a real
// configured value. With neither set there's nothing to scale against, so no
// bar renders at all (an exclamation mark stands in for it).
const denom = computed(() => {
  if (props.limits) return props.limits
  if (props.requests) return props.requests * 2
  return null
})
const effectiveRequests = computed(() => {
  if (props.requests) return props.requests
  if (props.limits) return props.limits / 2
  return null
})

const hasBar = computed(() => denom.value != null && denom.value > 0)
const ratio = computed(() => (hasBar.value ? props.usage / denom.value! : null))
const widthPct = computed(() => Math.min((ratio.value ?? 0) * 100, 100))
const markerPct = computed(() =>
  hasBar.value && effectiveRequests.value != null
    ? Math.min((effectiveRequests.value / denom.value!) * 100, 100)
    : null,
)

const color = computed(() => {
  if (ratio.value === null) return 'healthy'
  if (ratio.value >= 1) return 'critical'
  if (ratio.value >= 0.9) return 'warning'
  return 'healthy'
})

const tooltip = computed(() => {
  if (!hasBar.value) return `${props.label}: no request or limit configured`
  const denomLabel = props.limits ? 'limit' : 'requests × 2'
  return `${props.label}: ${props.format(props.usage)} / ${props.format(denom.value!)} ${denomLabel}`
})
</script>

<template>
  <div class="mini-ratio-bar" :title="tooltip">
    <span class="mini-ratio-label">{{ label }}</span>
    <div v-if="hasBar" class="mini-ratio-track">
      <div class="mini-ratio-fill" :class="`bg-${color}`" :style="{ width: `${widthPct}%` }" />
      <div v-if="markerPct != null" class="mini-ratio-marker" :style="{ left: `${markerPct}%` }" />
    </div>
    <v-icon v-else icon="mdi-alert" color="wildwest" size="12" />
  </div>
</template>

<style scoped>
.mini-ratio-bar {
  display: flex;
  align-items: center;
  gap: 3px;
}
.mini-ratio-label {
  font-size: 10px;
  line-height: 1;
  width: 10px;
  color: rgb(var(--v-theme-on-surface));
  opacity: 0.6;
}
.mini-ratio-track {
  position: relative;
  width: 44px;
  height: 6px;
  border-radius: 3px;
  background: rgb(var(--v-theme-surface-variant));
  overflow: hidden;
}
.mini-ratio-fill {
  height: 100%;
  border-radius: 3px;
}
.mini-ratio-marker {
  position: absolute;
  top: -1px;
  bottom: -1px;
  width: 2px;
  background: rgb(var(--v-theme-on-surface));
  opacity: 0.7;
}
</style>
