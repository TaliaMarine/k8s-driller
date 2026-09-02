<script setup lang="ts">
import { computed } from 'vue'
import type { NodeDTO } from '@/types/api'
import { nodeHealthColor } from '@/utils/format'
import ResourceBar from './ResourceBar.vue'

const props = defineProps<{ node: NodeDTO }>()

const healthColor = computed(() => nodeHealthColor(props.node.health))
</script>

<template>
  <v-card
    :to="`/nodes/${node.name}`"
    :class="['node-card', { 'not-ready': !node.ready }]"
    :variant="node.health === 'Overcommit' ? 'outlined' : 'elevated'"
    :style="{
      borderColor: node.health === 'Overcommit' ? `rgb(var(--v-theme-critical))` : undefined,
    }"
  >
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
      <ResourceBar
        label="CPU"
        :allocation-pct="node.pressure.limitsCpuPct"
        :live-pct="node.pressure.liveCpuPct"
        :overcommit="node.pressure.overcommitCpu"
      />
      <ResourceBar
        label="Memory"
        :allocation-pct="node.pressure.limitsMemPct"
        :live-pct="node.pressure.liveMemPct"
        :overcommit="node.pressure.overcommitMem"
      />

      <v-alert
        v-if="node.pressure.overcommitCpu || node.pressure.overcommitMem"
        type="warning"
        density="compact"
        variant="tonal"
        class="mt-2"
      >
        Overcommit: limits exceed node capacity
        <template v-if="node.pressure.overcommitCpu"
          >(CPU {{ Math.round(node.pressure.limitsCpuPct) }}%)</template
        >
        <template v-if="node.pressure.overcommitMem"
          >(Mem {{ Math.round(node.pressure.limitsMemPct) }}%)</template
        >
      </v-alert>

      <div class="text-caption text-medium-emphasis mt-2">{{ node.podCount }} pods</div>
    </v-card-text>
  </v-card>
</template>

<style scoped>
.node-card.not-ready {
  opacity: 0.6;
}
</style>
