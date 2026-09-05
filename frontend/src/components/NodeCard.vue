<script setup lang="ts">
import { computed } from 'vue'
import type { NodeDTO } from '@/types/api'
import { formatCpu, formatMem, nodeHealthColor } from '@/utils/format'
import ResourceBar from './ResourceBar.vue'
import NodeAllocationBar from './NodeAllocationBar.vue'

const props = defineProps<{ node: NodeDTO }>()

const healthColor = computed(() => nodeHealthColor(props.node.health))

// The API only carries pressure as percentages of node capacity — recover
// absolute values for the allocation bar and the requests/limits captions
// from those percentages and the node's own capacity.
const cpuUsage = computed(() => (props.node.pressure.liveCpuPct / 100) * props.node.capacityCpu)
const cpuRequests = computed(
  () => (props.node.pressure.requestsCpuPct / 100) * props.node.capacityCpu,
)
const cpuLimits = computed(() => (props.node.pressure.limitsCpuPct / 100) * props.node.capacityCpu)

const memUsage = computed(() => (props.node.pressure.liveMemPct / 100) * props.node.capacityMemory)
const memRequests = computed(
  () => (props.node.pressure.requestsMemPct / 100) * props.node.capacityMemory,
)
const memLimits = computed(
  () => (props.node.pressure.limitsMemPct / 100) * props.node.capacityMemory,
)
</script>

<template>
  <v-card :to="`/nodes/${node.name}`" :class="['node-card', { 'not-ready': !node.ready }]">
    <v-card-item>
      <template #prepend>
        <v-icon icon="mdi-server" />
      </template>
      <v-card-title>{{ node.name }}</v-card-title>
      <template #append>
        <v-chip :color="healthColor" size="small" variant="flat">{{ node.health }}</v-chip>
      </template>
    </v-card-item>

    <v-card-text>
      <ResourceBar label="CPU" unit="pct" :usage="cpuUsage" :capacity="node.capacityCpu" />
      <NodeAllocationBar
        :usage="cpuUsage"
        :requests="cpuRequests"
        :limits="cpuLimits"
        :capacity="node.capacityCpu"
        :format="formatCpu"
      />

      <ResourceBar
        label="Memory"
        unit="absolute"
        :usage="memUsage"
        :capacity="node.capacityMemory"
        :format="formatMem"
      />
      <NodeAllocationBar
        :usage="memUsage"
        :requests="memRequests"
        :limits="memLimits"
        :capacity="node.capacityMemory"
        :format="formatMem"
      />

      <div class="text-caption text-medium-emphasis mt-2">
        Requested: CPU {{ Math.round(node.pressure.requestsCpuPct) }}% · Mem
        {{ Math.round(node.pressure.requestsMemPct) }}%
      </div>
      <div class="text-caption text-medium-emphasis">
        Limits set: CPU {{ Math.round(node.pressure.limitsCpuPct) }}% · Mem
        {{ Math.round(node.pressure.limitsMemPct) }}%
      </div>

      <div class="text-caption text-medium-emphasis mt-2">
        {{ node.podCount }} pods
        <span v-if="node.podsOverRequests > 0" class="text-warning">
          · {{ node.podsOverRequests }} over requests
        </span>
      </div>
    </v-card-text>
  </v-card>
</template>

<style scoped>
.node-card.not-ready {
  opacity: 0.6;
}
</style>
