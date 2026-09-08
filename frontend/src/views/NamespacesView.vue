<script setup lang="ts">
import { computed, ref } from 'vue'
import { useEventSource } from '@/composables/useEventSource'
import { useClusterStore } from '@/stores/cluster'
import type { PodDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'
import { namespaceAggregates } from '@/composables/usePodFilters'
import NodeDistributionChart from '@/components/NodeDistributionChart.vue'
import type { DistSegment } from '@/components/NodeDistributionChart.vue'
import NodeAllocationBar from '@/components/NodeAllocationBar.vue'
import TreemapPanel from '@/components/TreemapPanel.vue'
import type { TreemapItem } from '@/utils/treemap'

const clusterStore = useClusterStore()
const { status, data: pods } = useEventSource<PodDTO[]>('/api/v1/stream/workloads')

const namespaces = computed(() => namespaceAggregates(pods.value ?? []))

const resourceTab = ref('treemap')
const nsSearch = ref('')
const selectedNamespaceKey = ref<string | null>(null)

const filteredNamespaces = computed(() => {
  const term = nsSearch.value.trim().toLowerCase()
  if (!term) return namespaces.value
  return namespaces.value.filter((ns) => ns.namespace.toLowerCase().includes(term))
})

function selectNamespace(item: TreemapItem) {
  nsSearch.value = item.name
  selectedNamespaceKey.value = item.key
}

function clearNamespaceFilter() {
  nsSearch.value = ''
  selectedNamespaceKey.value = null
}

function segments(
  field: 'usageCpu' | 'usageMem' | 'requestsCpu' | 'requestsMem' | 'limitsCpu' | 'limitsMem',
): DistSegment[] {
  return namespaces.value
    .map((ns) => ({ key: ns.namespace, name: ns.namespace, value: ns[field] }))
    .filter((s) => s.value > 0)
}

const cpuUsageSegments = computed(() => segments('usageCpu'))
const cpuRequestSegments = computed(() => segments('requestsCpu'))
const cpuLimitSegments = computed(() => segments('limitsCpu'))
const memUsageSegments = computed(() => segments('usageMem'))
const memRequestSegments = computed(() => segments('requestsMem'))
const memLimitSegments = computed(() => segments('limitsMem'))

// The authoritative live total (metrics-server, via the cluster summary)
// rather than the sum of per-namespace segments, matching how the per-node
// distribution chart drives its marker line off node-level metrics instead
// of a pod-level sum that can miss unattributed overhead.
const cpuUsageTotal = computed(
  () =>
    ((clusterStore.summary?.totalLiveCpuPct ?? 0) / 100) *
    (clusterStore.summary?.totalCapacityCpu ?? 0),
)
const memUsageTotal = computed(
  () =>
    ((clusterStore.summary?.totalLiveMemPct ?? 0) / 100) *
    (clusterStore.summary?.totalCapacityMem ?? 0),
)
</script>

<template>
  <v-container fluid class="namespaces-view">
    <div class="d-flex align-center mb-2 ga-2">
      <div>
        <h1 class="text-h6 mb-0">Namespaces</h1>
        <div class="text-caption text-medium-emphasis">
          {{ namespaces.length }} namespaces across the cluster
        </div>
      </div>
      <v-spacer />
      <v-chip size="small" :color="status === 'live' ? 'healthy' : 'warning'" variant="outlined">
        {{ status }}
      </v-chip>
    </div>

    <v-card v-if="pods" class="mb-5" variant="flat" border>
      <div class="d-flex">
        <v-tabs v-model="resourceTab" direction="vertical" color="watch" class="resource-tabs">
          <v-tab value="treemap" prepend-icon="mdi-chart-tree">By namespace</v-tab>
          <v-tab value="bars" prepend-icon="mdi-chart-bar">Bars</v-tab>
        </v-tabs>
        <v-window v-model="resourceTab" class="flex-grow-1">
          <v-window-item value="treemap">
            <v-card-text>
              <TreemapPanel
                label="CPU"
                unit-label="namespace"
                :usage-items="cpuUsageSegments"
                :request-items="cpuRequestSegments"
                :limit-items="cpuLimitSegments"
                :format="formatCpu"
                :selected-key="selectedNamespaceKey"
                @select="selectNamespace"
              />
              <TreemapPanel
                label="Memory"
                unit-label="namespace"
                :usage-items="memUsageSegments"
                :request-items="memRequestSegments"
                :limit-items="memLimitSegments"
                :format="formatMem"
                :selected-key="selectedNamespaceKey"
                @select="selectNamespace"
              />
            </v-card-text>
          </v-window-item>
          <v-window-item value="bars">
            <v-card-text>
              <div class="text-body-1 font-weight-medium mb-2">
                Usage / requests / limits by namespace
              </div>
              <NodeDistributionChart
                label="CPU"
                :capacity="clusterStore.summary?.totalCapacityCpu ?? 0"
                :usage-segments="cpuUsageSegments"
                :request-segments="cpuRequestSegments"
                :limit-segments="cpuLimitSegments"
                :usage="cpuUsageTotal"
                :format="formatCpu"
              />
              <NodeDistributionChart
                label="Memory"
                :capacity="clusterStore.summary?.totalCapacityMem ?? 0"
                :usage-segments="memUsageSegments"
                :request-segments="memRequestSegments"
                :limit-segments="memLimitSegments"
                :usage="memUsageTotal"
                :format="formatMem"
              />
            </v-card-text>
          </v-window-item>
        </v-window>
      </div>
    </v-card>

    <v-alert v-if="!pods" type="info" variant="tonal" class="mb-4">Waiting for pod data…</v-alert>

    <div v-else class="d-flex align-center ga-3 mb-4">
      <v-text-field
        v-model="nsSearch"
        label="Search namespace"
        prepend-inner-icon="mdi-magnify"
        density="compact"
        hide-details
        clearable
        style="max-width: 240px"
        @click:clear="clearNamespaceFilter"
      />
      <span class="text-caption text-medium-emphasis"
        >{{ filteredNamespaces.length }} / {{ namespaces.length }} namespaces</span
      >
      <v-btn v-if="nsSearch" size="small" variant="text" @click="clearNamespaceFilter"
        >Clear filter</v-btn
      >
    </div>

    <v-row v-if="pods">
      <v-col v-for="ns in filteredNamespaces" :key="ns.namespace" cols="12" sm="6" md="4" lg="3">
        <v-card :to="`/namespaces/${ns.namespace}`">
          <v-card-item>
            <template #prepend>
              <v-icon icon="mdi-folder-outline" />
            </template>
            <v-card-title>{{ ns.namespace }}</v-card-title>
            <v-card-subtitle>{{ ns.podCount }} pods</v-card-subtitle>
          </v-card-item>
          <v-card-text>
            <NodeAllocationBar
              :usage="ns.usageCpu"
              :requests="ns.requestsCpu"
              :limits="ns.limitsCpu"
              :format="formatCpu"
            />
            <NodeAllocationBar
              :usage="ns.usageMem"
              :requests="ns.requestsMem"
              :limits="ns.limitsMem"
              :format="formatMem"
            />
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<style scoped>
.resource-tabs {
  flex: 0 0 160px;
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.12);
}
</style>
