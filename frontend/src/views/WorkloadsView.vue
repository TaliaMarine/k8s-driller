<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useEventSource } from '@/composables/useEventSource'
import { useClusterStore } from '@/stores/cluster'
import type { PodDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'
import {
  containerAllocation,
  FILTER_OPTIONS,
  podKey,
  totalAggregate,
  usePodFilters,
} from '@/composables/usePodFilters'
import NodeAllocationBar from '@/components/NodeAllocationBar.vue'
import PodRow from '@/components/PodRow.vue'
import PodDetailPanel from '@/components/PodDetailPanel.vue'
import TreemapPanel from '@/components/TreemapPanel.vue'
import HexDistribution from '@/components/HexDistribution.vue'
import type { TreemapItem } from '@/utils/treemap'

const router = useRouter()
const clusterStore = useClusterStore()

const { status, data: pods } = useEventSource<PodDTO[]>('/api/v1/stream/workloads')

// A dedicated feed, not scopedPods: the Distribution honeycomb is the one
// view that wants not-Ready and recently-deleted pods too (grayed, see
// HexDistribution.vue), which the main pod-list stream above deliberately
// excludes everywhere else (SPECS.md §7.1).
const { data: distPods } = useEventSource<PodDTO[]>('/api/v1/stream/workloads/distribution')

const {
  search,
  namespaceFilter,
  activeFilters,
  includeKubeSystem,
  namespaceOptions,
  scopedPods,
  filteredPods,
  groups,
  clearFilters,
  filtersActive,
  overCpuRequestCount,
  overMemRequestCount,
} = usePodFilters(pods)

// The cluster-wide summary below is deliberately independent of the
// kube-system toggle above — that toggle only controls list noise, not
// what counts as "all workloads" for the aggregate picture.
const allWorkloads = computed(() => totalAggregate(pods.value ?? []))

// Per-pod breakdown for the treemap tab — deliberately sourced from the raw
// pod list, not scopedPods/filteredPods, so it always reflects every
// workload in the cluster regardless of the list filters below (same
// principle as allWorkloads above and NodeDrilldownView's distribution
// segments).
const cpuUsageSegments = computed<TreemapItem[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: p.usageCpu }))
      .filter((s) => s.value > 0) ?? [],
)
const memUsageSegments = computed<TreemapItem[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: p.usageMem }))
      .filter((s) => s.value > 0) ?? [],
)
const cpuRequestSegments = computed<TreemapItem[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'requestsCpu') }))
      .filter((s) => s.value > 0) ?? [],
)
const cpuLimitSegments = computed<TreemapItem[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'limitsCpu') }))
      .filter((s) => s.value > 0) ?? [],
)
const memRequestSegments = computed<TreemapItem[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'requestsMem') }))
      .filter((s) => s.value > 0) ?? [],
)
const memLimitSegments = computed<TreemapItem[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'limitsMem') }))
      .filter((s) => s.value > 0) ?? [],
)

function goToNode(nodeName: string) {
  router.push({ name: 'node-drilldown', params: { name: nodeName } })
}

// Keyed by "namespace/name", not array index, so refreshed SSE pushes (new
// array identity every time) never collapse a panel the user had open.
const openPanels = ref<string[]>([])

const resourceTab = ref('treemap')
const selectedWorkloadKey = ref<string | null>(null)

function selectWorkload(item: TreemapItem) {
  clearFilters()
  search.value = item.name
  selectedWorkloadKey.value = item.key
  // The treemap always covers every workload, kube-system included (same
  // principle as allWorkloads above), but the list below hides kube-system
  // by default — without this, clicking a kube-system cell would apply a
  // filter that then hides its own match.
  const [namespace] = item.key.split('/')
  if (namespace === 'kube-system') includeKubeSystem.value = true
}

function clearAllFilters() {
  clearFilters()
  selectedWorkloadKey.value = null
}

function selectPod(name: string) {
  clearFilters()
  search.value = name
  selectedWorkloadKey.value = null
}
</script>

<template>
  <v-container fluid class="workloads-view">
    <div class="d-flex align-center mb-2 ga-2">
      <div>
        <h1 class="text-h6 mb-0">Workloads</h1>
        <div class="text-caption text-medium-emphasis">
          {{ scopedPods.length }} pods across the cluster
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
          <v-tab value="treemap" prepend-icon="mdi-chart-tree">By workload</v-tab>
          <v-tab value="bars" prepend-icon="mdi-chart-bar">Bars</v-tab>
          <v-tab value="distribution" prepend-icon="mdi-hexagon-outline">Distribution</v-tab>
        </v-tabs>
        <v-window v-model="resourceTab" class="flex-grow-1">
          <v-window-item value="treemap">
            <v-card-text>
              <TreemapPanel
                label="CPU"
                unit-label="workload"
                :usage-items="cpuUsageSegments"
                :request-items="cpuRequestSegments"
                :limit-items="cpuLimitSegments"
                :format="formatCpu"
                :selected-key="selectedWorkloadKey"
                @select="selectWorkload"
              />
              <TreemapPanel
                label="Memory"
                unit-label="workload"
                :usage-items="memUsageSegments"
                :request-items="memRequestSegments"
                :limit-items="memLimitSegments"
                :format="formatMem"
                :selected-key="selectedWorkloadKey"
                @select="selectWorkload"
              />
            </v-card-text>
          </v-window-item>
          <v-window-item value="bars">
            <v-card-text>
              <div class="text-body-1 font-weight-medium mb-2">
                All workloads: usage vs requests/limits
              </div>
              <NodeAllocationBar
                :usage="allWorkloads.usageCpu"
                :requests="allWorkloads.requestsCpu"
                :limits="allWorkloads.limitsCpu"
                :capacity="clusterStore.summary?.totalCapacityCpu"
                :format="formatCpu"
              />
              <NodeAllocationBar
                :usage="allWorkloads.usageMem"
                :requests="allWorkloads.requestsMem"
                :limits="allWorkloads.limitsMem"
                :capacity="clusterStore.summary?.totalCapacityMem"
                :format="formatMem"
              />
            </v-card-text>
          </v-window-item>
          <v-window-item value="distribution">
            <v-card-text>
              <HexDistribution
                label="CPU"
                :pods="distPods ?? []"
                resource="cpu"
                :format="formatCpu"
                @select="selectPod"
              />
              <HexDistribution
                label="Memory"
                :pods="distPods ?? []"
                resource="mem"
                :format="formatMem"
                @select="selectPod"
              />
            </v-card-text>
          </v-window-item>
        </v-window>
      </div>
    </v-card>

    <div class="d-flex flex-wrap ga-3 mb-4">
      <v-chip
        :color="overCpuRequestCount > 0 ? 'critical' : 'healthy'"
        variant="tonal"
        size="small"
      >
        <v-icon v-if="overCpuRequestCount > 0" start icon="mdi-alert" />
        {{ overCpuRequestCount }} / {{ scopedPods.length }} pods over CPU request
      </v-chip>
      <v-chip
        :color="overMemRequestCount > 0 ? 'critical' : 'healthy'"
        variant="tonal"
        size="small"
      >
        <v-icon v-if="overMemRequestCount > 0" start icon="mdi-alert" />
        {{ overMemRequestCount }} / {{ scopedPods.length }} pods over memory request
      </v-chip>
    </div>

    <v-card class="mb-5" variant="flat" border>
      <v-card-text class="d-flex flex-wrap align-center ga-3">
        <v-text-field
          v-model="search"
          label="Search pod name"
          prepend-inner-icon="mdi-magnify"
          density="compact"
          hide-details
          clearable
          style="max-width: 240px"
        />
        <v-select
          v-model="namespaceFilter"
          :items="namespaceOptions"
          label="Namespace"
          density="compact"
          hide-details
          clearable
          style="max-width: 220px"
        />
        <v-select
          v-model="activeFilters"
          :items="FILTER_OPTIONS"
          item-title="label"
          item-value="value"
          label="Filters"
          multiple
          density="compact"
          hide-details
          clearable
          style="max-width: 220px"
        >
          <template #selection="{ index }">
            <span v-if="index === 0" class="text-caption">
              {{ activeFilters.length }} filter{{ activeFilters.length === 1 ? '' : 's' }}
            </span>
          </template>
          <template #item="{ item, props: itemProps }">
            <v-list-item v-bind="itemProps" :title="undefined">
              <template #prepend="{ isSelected }">
                <v-checkbox-btn :model-value="isSelected" />
              </template>
              {{ item.label }}
            </v-list-item>
          </template>
        </v-select>
        <v-switch
          v-model="includeKubeSystem"
          label="kube-system"
          color="watch"
          density="compact"
          hide-details
          inset
        />
        <v-spacer />
        <span class="text-caption text-medium-emphasis"
          >{{ filteredPods.length }} / {{ scopedPods.length }} pods</span
        >
        <v-btn v-if="filtersActive" size="small" variant="text" @click="clearAllFilters"
          >Clear filters</v-btn
        >
      </v-card-text>
    </v-card>

    <v-alert v-if="!pods" type="info" variant="tonal" class="mb-4">Waiting for pod data…</v-alert>
    <v-alert v-else-if="filteredPods.length === 0" type="info" variant="tonal" class="mb-4">
      No pods match the current filters.
    </v-alert>

    <v-card v-for="group in groups" :key="group.controllerKey" class="mb-4" variant="flat" border>
      <v-card-title class="text-body-1 d-flex align-center ga-2">
        <v-icon icon="mdi-folder-outline" size="small" />
        {{ group.namespace }}
        <v-icon icon="mdi-chevron-right" size="small" class="text-medium-emphasis" />
        {{ group.controllerLabel }}
        <v-chip size="x-small" variant="tonal" class="ml-1">{{ group.pods.length }}</v-chip>
      </v-card-title>
      <v-divider />
      <v-expansion-panels v-model="openPanels" multiple variant="accordion">
        <v-expansion-panel v-for="pod in group.pods" :key="podKey(pod)" :value="podKey(pod)">
          <v-expansion-panel-title>
            <PodRow
              :pod="pod"
              show-node-link
              :cpu-requests-total="allWorkloads.requestsCpu"
              :mem-requests-total="allWorkloads.requestsMem"
              @go-to-node="goToNode"
            />
          </v-expansion-panel-title>
          <v-expansion-panel-text>
            <PodDetailPanel :pod="pod" />
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </v-card>
  </v-container>
</template>

<style scoped>
.resource-tabs {
  flex: 0 0 160px;
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.12);
}
</style>
