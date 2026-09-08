<script setup lang="ts">
import TreemapBox from '@/components/TreemapBox.vue'
import type { TreemapItem } from '@/utils/treemap'

defineProps<{
  label: string
  unitLabel: string // what a cell represents, e.g. "workload" or "namespace" — drives the click hint text
  usageItems: TreemapItem[]
  requestItems: TreemapItem[]
  limitItems: TreemapItem[]
  format: (v: number) => string
  selectedKey?: string | null
}>()

const emit = defineEmits<{ select: [item: TreemapItem] }>()
</script>

<template>
  <div class="tm-panel">
    <div class="d-flex align-center justify-space-between mb-2">
      <span class="text-body-2 font-weight-medium">{{ label }}</span>
      <span class="text-caption text-medium-emphasis"
        >click a {{ unitLabel }} to filter the list below</span
      >
    </div>
    <div class="tm-row">
      <TreemapBox
        title="Usage"
        color-class="tm-usage"
        :items="usageItems"
        :format="format"
        :selected-key="selectedKey"
        @select="emit('select', $event)"
      />
      <TreemapBox
        title="Requests"
        color-class="tm-request"
        :items="requestItems"
        :format="format"
        :selected-key="selectedKey"
        @select="emit('select', $event)"
      />
      <TreemapBox
        title="Limits"
        color-class="tm-limit"
        :items="limitItems"
        :format="format"
        :selected-key="selectedKey"
        @select="emit('select', $event)"
      />
    </div>
  </div>
</template>

<style scoped>
.tm-panel {
  margin-bottom: 20px;
}
.tm-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
</style>
