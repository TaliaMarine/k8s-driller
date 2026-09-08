<script setup lang="ts">
import { computed, reactive } from 'vue'
import { squarify, type TreemapItem, type TreemapCell } from '@/utils/treemap'

const props = defineProps<{
  title: string
  items: TreemapItem[]
  format: (v: number) => string
  colorClass: 'tm-usage' | 'tm-request' | 'tm-limit'
  selectedKey?: string | null
}>()

const emit = defineEmits<{ select: [item: TreemapItem] }>()

const total = computed(() => props.items.reduce((sum, i) => sum + i.value, 0))
const cells = computed<TreemapCell[]>(() => squarify(props.items, 0, 0, 100, 100))

function pct(cell: TreemapCell): number {
  return total.value > 0 ? Math.round((cell.item.value / total.value) * 100) : 0
}

const hover = reactive<{ cell: TreemapCell | null; left: number; top: number }>({
  cell: null,
  left: 0,
  top: 0,
})

// Fixed-position, computed from the hovered cell's own bounding box and
// clamped against the viewport — a purely percentage-based tooltip (like
// NodeDistributionChart's) clips off-screen for cells flush against a box's
// left/right edge, since a treemap (unlike a single-row bar) puts cells at
// every edge, not just the ends.
function showHover(cell: TreemapCell, event: MouseEvent) {
  hover.cell = cell
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const tooltipHalfWidth = 90
  hover.left = Math.min(
    Math.max(rect.left + rect.width / 2, tooltipHalfWidth),
    window.innerWidth - tooltipHalfWidth,
  )
  hover.top = rect.top
}
</script>

<template>
  <div class="tm-box">
    <div class="tm-box-header text-caption text-medium-emphasis">
      <span>{{ title }}</span>
      <span>{{ format(total) }}</span>
    </div>
    <div class="tm-canvas" :class="colorClass">
      <div
        v-for="cell in cells"
        :key="cell.item.key"
        class="tm-cell"
        :class="{ 'tm-cell--selected': cell.item.key === selectedKey }"
        :style="{
          left: `${cell.x}%`,
          top: `${cell.y}%`,
          width: `${cell.width}%`,
          height: `${cell.height}%`,
        }"
        @mouseenter="showHover(cell, $event)"
        @mouseleave="hover.cell = null"
        @click="emit('select', cell.item)"
      >
        <span v-if="cell.width > 14 && cell.height > 22" class="tm-cell-label">{{
          cell.item.name
        }}</span>
      </div>
      <div v-if="!cells.length" class="tm-empty text-caption text-medium-emphasis">No data</div>
    </div>
    <Teleport to="body">
      <div
        v-if="hover.cell"
        class="tm-tooltip"
        :style="{ left: `${hover.left}px`, top: `${hover.top}px` }"
      >
        <span class="font-weight-medium">{{ hover.cell.item.name }}</span>
        <span class="text-medium-emphasis"
          >{{ format(hover.cell.item.value) }} · {{ pct(hover.cell) }}%</span
        >
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.tm-box {
  flex: 1 1 220px;
  min-width: 0;
}
.tm-box-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 4px;
}
.tm-canvas {
  position: relative;
  height: 160px;
  border-radius: 6px;
  overflow: visible;
  background: rgb(var(--v-theme-surface-variant));
  opacity: 1;
}
.tm-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.tm-cell {
  position: absolute;
  box-sizing: border-box;
  border: 1.5px solid rgb(var(--v-theme-surface));
  border-radius: 2px;
  cursor: pointer;
  transition: filter 0.1s ease;
  overflow: hidden;
}
.tm-cell:hover {
  filter: brightness(1.18);
  z-index: 1;
}
.tm-cell--selected {
  outline: 2px solid rgb(var(--v-theme-on-surface));
  outline-offset: -2px;
  z-index: 2;
}
.tm-cell-label {
  display: block;
  font-size: 10px;
  line-height: 1.3;
  padding: 3px 5px;
  color: rgb(var(--v-theme-on-surface));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
}
/* Usage / requests / limits share one hue at three lightness steps, matching
   NodeDistributionChart's ordinal usage->reserved->ceiling ramp — identity
   within a box comes from tooltip + label, not per-cell categorical color,
   since a box can hold far more workloads than any palette can carry. */
.tm-usage .tm-cell {
  background: rgb(var(--v-theme-watch));
}
.tm-request .tm-cell {
  background: color-mix(in srgb, rgb(var(--v-theme-watch)) 58%, rgb(var(--v-theme-surface)));
}
.tm-limit .tm-cell {
  background: color-mix(in srgb, rgb(var(--v-theme-watch)) 30%, rgb(var(--v-theme-surface)));
}
.tm-tooltip {
  position: fixed;
  transform: translate(-50%, calc(-100% - 6px));
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-theme-on-surface), 0.15);
  border-radius: 4px;
  padding: 4px 8px;
  font-size: 12px;
  white-space: nowrap;
  display: flex;
  gap: 6px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
  pointer-events: none;
  z-index: 3;
}
</style>
