<script setup lang="ts">
import type { PodDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'
import {
  containerAllocation,
  highAllocationLabel,
  highAllocations,
  highAllocationTooltip,
  missingChips,
} from '@/composables/usePodFilters'
import MiniRatioBar from './MiniRatioBar.vue'

defineProps<{ pod: PodDTO; showNodeLink?: boolean }>()
defineEmits<{ goToNode: [nodeName: string] }>()
</script>

<template>
  <div class="d-flex align-center ga-2 flex-wrap pod-row">
    <div class="d-flex flex-column ga-1 pod-usage-stack">
      <MiniRatioBar
        label="CPU"
        icon="mdi-cpu-64-bit"
        :usage="pod.usageCpu"
        :requests="containerAllocation(pod, 'requestsCpu') || undefined"
        :limits="containerAllocation(pod, 'limitsCpu') || undefined"
        :format="formatCpu"
      />
      <MiniRatioBar
        label="Memory"
        icon="mdi-memory"
        :usage="pod.usageMem"
        :requests="containerAllocation(pod, 'requestsMem') || undefined"
        :limits="containerAllocation(pod, 'limitsMem') || undefined"
        :format="formatMem"
      />
    </div>
    <v-icon
      v-if="pod.wildWest"
      icon="mdi-alert-circle"
      color="wildwest"
      size="small"
      title="Missing resource configuration"
    />
    <span class="font-weight-medium">{{ pod.name }}</span>
    <v-chip v-if="pod.oomRisk" color="critical" size="small" variant="flat">OOM-Risk</v-chip>
    <v-chip v-if="pod.throttlingRisk" color="warning" size="small" variant="flat"
      >Throttling-Risk</v-chip
    >
    <v-chip
      v-for="chip in missingChips(pod)"
      :key="chip"
      color="wildwest"
      size="small"
      variant="outlined"
    >
      Missing {{ chip }}
    </v-chip>
    <v-chip
      v-for="high in highAllocations(pod)"
      :key="high.resource"
      color="warning"
      size="small"
      variant="flat"
      :title="highAllocationTooltip(high)"
    >
      <v-icon start icon="mdi-arrow-up-bold" />
      {{ highAllocationLabel(high) }}
    </v-chip>
    <div v-if="showNodeLink" class="d-flex align-center ga-2 ml-auto pod-row-end">
      <v-btn
        icon="mdi-server"
        size="x-small"
        variant="text"
        :title="`Go to node ${pod.nodeName}`"
        @click.stop="$emit('goToNode', pod.nodeName)"
      />
    </div>
  </div>
</template>

<style scoped>
.pod-row {
  min-width: 0;
  flex: 1 1 auto;
}
.pod-usage-stack {
  flex-shrink: 0;
}
.pod-row-end {
  flex-shrink: 0;
}
</style>
