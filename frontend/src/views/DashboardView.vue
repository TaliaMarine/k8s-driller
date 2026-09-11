<script setup lang="ts">
import { computed, ref } from 'vue'
import { useClusterStore } from '@/stores/cluster'
import type { NodeDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'
import NodeCard from '@/components/NodeCard.vue'
import TreemapPanel from '@/components/TreemapPanel.vue'
import type { TreemapItem } from '@/utils/treemap'
import NodeDistributionChart from '@/components/NodeDistributionChart.vue'
import type { DistSegment } from '@/components/NodeDistributionChart.vue'
import NodeHexDistribution from '@/components/NodeHexDistribution.vue'

const clusterStore = useClusterStore()
const summary = computed(() => clusterStore.summary)

// Recovers absolute usage/requests/limits from the percentages the API
// carries (same derivation NodeCard already does per-card) — here summed
// into one segment per node instead of per pod, for the treemap/bars/
// distribution panel above the grid.
function nodeValue(
  n: NodeDTO,
  resource: 'cpu' | 'mem',
  metric: 'live' | 'requests' | 'limits',
): number {
  const capacity = resource === 'cpu' ? n.capacityCpu : n.capacityMemory
  const p = n.pressure
  const pct =
    resource === 'cpu'
      ? metric === 'live'
        ? p.liveCpuPct
        : metric === 'requests'
          ? p.requestsCpuPct
          : p.limitsCpuPct
      : metric === 'live'
        ? p.liveMemPct
        : metric === 'requests'
          ? p.requestsMemPct
          : p.limitsMemPct
  return (pct / 100) * capacity
}

function segments(resource: 'cpu' | 'mem', metric: 'live' | 'requests' | 'limits') {
  return (summary.value?.nodes ?? [])
    .map((n) => ({ key: n.name, name: n.name, value: nodeValue(n, resource, metric) }))
    .filter((s) => s.value > 0)
}

const cpuUsageSegments = computed<TreemapItem[]>(() => segments('cpu', 'live'))
const cpuRequestSegments = computed<TreemapItem[]>(() => segments('cpu', 'requests'))
const cpuLimitSegments = computed<TreemapItem[]>(() => segments('cpu', 'limits'))
const memUsageSegments = computed<TreemapItem[]>(() => segments('mem', 'live'))
const memRequestSegments = computed<TreemapItem[]>(() => segments('mem', 'requests'))
const memLimitSegments = computed<TreemapItem[]>(() => segments('mem', 'limits'))

const cpuUsageTotal = computed(
  () => ((summary.value?.totalLiveCpuPct ?? 0) / 100) * (summary.value?.totalCapacityCpu ?? 0),
)
const memUsageTotal = computed(
  () => ((summary.value?.totalLiveMemPct ?? 0) / 100) * (summary.value?.totalCapacityMem ?? 0),
)

const resourceTab = ref('treemap')
const selectedNodeKey = ref<string | null>(null)
const nodeFilter = ref('')

const filteredNodes = computed(() => {
  const term = nodeFilter.value.trim().toLowerCase()
  const nodes = summary.value?.nodes ?? []
  return term ? nodes.filter((n) => n.name.toLowerCase().includes(term)) : nodes
})

function selectNode(name: string) {
  nodeFilter.value = name
  selectedNodeKey.value = name
}
function clearNodeFilter() {
  nodeFilter.value = ''
  selectedNodeKey.value = null
}
</script>

<template>
  <v-container fluid>
    <v-alert v-if="!summary" type="info" variant="tonal" class="mb-4">
      Waiting for the first snapshot from the cluster stream…
    </v-alert>

    <template v-else>
      <v-card class="mb-6" variant="tonal">
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

      <v-card class="mb-5" variant="flat" border>
        <div class="d-flex">
          <v-tabs v-model="resourceTab" direction="vertical" color="watch" class="resource-tabs">
            <v-tab value="treemap" prepend-icon="mdi-chart-tree">By Node</v-tab>
            <v-tab value="bars" prepend-icon="mdi-chart-bar">Bars</v-tab>
            <v-tab value="distribution" prepend-icon="mdi-hexagon-outline">Distribution</v-tab>
          </v-tabs>
          <v-window v-model="resourceTab" class="flex-grow-1">
            <v-window-item value="treemap">
              <v-card-text>
                <TreemapPanel
                  label="CPU"
                  unit-label="node"
                  :usage-items="cpuUsageSegments"
                  :request-items="cpuRequestSegments"
                  :limit-items="cpuLimitSegments"
                  :format="formatCpu"
                  :selected-key="selectedNodeKey"
                  @select="(item: TreemapItem) => selectNode(item.name)"
                />
                <TreemapPanel
                  label="Memory"
                  unit-label="node"
                  :usage-items="memUsageSegments"
                  :request-items="memRequestSegments"
                  :limit-items="memLimitSegments"
                  :format="formatMem"
                  :selected-key="selectedNodeKey"
                  @select="(item: TreemapItem) => selectNode(item.name)"
                />
              </v-card-text>
            </v-window-item>
            <v-window-item value="bars">
              <v-card-text>
                <NodeDistributionChart
                  label="CPU"
                  :capacity="summary.totalCapacityCpu"
                  :usage-segments="cpuUsageSegments"
                  :request-segments="cpuRequestSegments"
                  :limit-segments="cpuLimitSegments"
                  :usage="cpuUsageTotal"
                  :format="formatCpu"
                  :selected-key="selectedNodeKey"
                  @select="(seg: DistSegment) => selectNode(seg.name)"
                />
                <NodeDistributionChart
                  label="Memory"
                  :capacity="summary.totalCapacityMem"
                  :usage-segments="memUsageSegments"
                  :request-segments="memRequestSegments"
                  :limit-segments="memLimitSegments"
                  :usage="memUsageTotal"
                  :format="formatMem"
                  :selected-key="selectedNodeKey"
                  @select="(seg: DistSegment) => selectNode(seg.name)"
                />
              </v-card-text>
            </v-window-item>
            <v-window-item value="distribution">
              <v-card-text>
                <NodeHexDistribution :nodes="summary.nodes" @select="selectNode" />
              </v-card-text>
            </v-window-item>
          </v-window>
        </div>
      </v-card>

      <v-card class="mb-5" variant="flat" border>
        <v-card-text class="d-flex flex-wrap align-center ga-3">
          <v-text-field
            v-model="nodeFilter"
            label="Search node name"
            prepend-inner-icon="mdi-magnify"
            density="compact"
            hide-details
            clearable
            style="max-width: 240px"
          />
          <v-spacer />
          <span class="text-caption text-medium-emphasis"
            >{{ filteredNodes.length }} / {{ summary.nodes.length }} nodes</span
          >
          <v-btn v-if="nodeFilter" size="small" variant="text" @click="clearNodeFilter"
            >Clear filter</v-btn
          >
        </v-card-text>
      </v-card>

      <v-row>
        <v-col v-for="node in filteredNodes" :key="node.name" cols="12" sm="6" md="4" lg="3">
          <NodeCard :node="node" />
        </v-col>
      </v-row>
    </template>
  </v-container>
</template>

<style scoped>
.resource-tabs {
  flex: 0 0 160px;
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.12);
}
</style>
