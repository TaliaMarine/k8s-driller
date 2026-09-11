<script setup lang="ts">
import { computed } from 'vue'

// Generic honeycomb layout (à la Datadog's host map): chunks pre-colored
// cells into rows, offsetting alternate rows so they interlock. Shared by
// HexDistribution.vue (one hex per pod) and NodeHexDistribution.vue (one
// hex per node) — only what a cell means and how it's colored differs
// between them; the grid itself doesn't care.
export interface HexCell {
  key: string
  value: string // emitted on click — the caller decides what this identifies
  color: string
  tooltip: string
}

const props = defineProps<{ cells: HexCell[] }>()
const emit = defineEmits<{ select: [value: string] }>()

// A roughly 2.5:1 landscape grid, like the reference host map, regardless
// of how many cells there are.
const columns = computed(() =>
  Math.max(4, Math.min(32, Math.round(Math.sqrt(props.cells.length * 2.5)))),
)
const rows = computed(() => {
  const out: HexCell[][] = []
  for (let i = 0; i < props.cells.length; i += columns.value) {
    out.push(props.cells.slice(i, i + columns.value))
  }
  return out
})
</script>

<template>
  <div v-if="cells.length === 0" class="text-caption text-medium-emphasis">No data.</div>
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
        @click="emit('select', cell.value)"
      />
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
