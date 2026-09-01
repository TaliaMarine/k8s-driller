<script setup lang="ts">
import { computed } from 'vue'
import { useClusterStore } from '@/stores/cluster'
import NodeCard from '@/components/NodeCard.vue'

const clusterStore = useClusterStore()
const summary = computed(() => clusterStore.summary)
</script>

<template>
  <v-container fluid>
    <v-alert v-if="!summary" type="info" variant="tonal" class="mb-4">
      Waiting for the first snapshot from the cluster stream…
    </v-alert>

    <v-card v-else class="mb-6" variant="tonal">
      <v-card-text class="d-flex flex-wrap ga-6">
        <div>
          <div class="text-caption text-medium-emphasis">CPU</div>
          <div class="text-h6">
            {{ Math.round(summary.totalRequestsCpuPct) }}% alloc /
            {{ Math.round(summary.totalLiveCpuPct) }}% live
          </div>
        </div>
        <div>
          <div class="text-caption text-medium-emphasis">Memory</div>
          <div class="text-h6">
            {{ Math.round(summary.totalRequestsMemPct) }}% alloc /
            {{ Math.round(summary.totalLiveMemPct) }}% live
          </div>
        </div>
        <div>
          <div class="text-caption text-medium-emphasis">Nodes</div>
          <div class="text-h6">{{ summary.nodes.length }}</div>
        </div>
      </v-card-text>
    </v-card>

    <v-row v-if="summary">
      <v-col v-for="node in summary.nodes" :key="node.name" cols="12" sm="6" md="4" lg="3">
        <NodeCard :node="node" />
      </v-col>
    </v-row>
  </v-container>
</template>
