<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useEventSource } from '@/composables/useEventSource'
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
import type { TreemapItem } from '@/utils/treemap'

const props = defineProps<{ name: string }>()
const router = useRouter()

const { status, data: allPods } = useEventSource<PodDTO[]>('/api/v1/stream/workloads')

// Scoped to this one namespace before it ever reaches usePodFilters, so
// every count/group/aggregate downstream is already namespace-local.
const nsPods = computed(() => (allPods.value ?? []).filter((p) => p.namespace === props.name))

const { search, activeFilters, filteredPods, groups, clearFilters, filtersActive } =
  usePodFilters(nsPods)

const summary = computed(() => totalAggregate(nsPods.value))

// Per-pod breakdown for the treemap tab — sourced from every pod in the
// namespace, not filteredPods, so it always reflects the whole namespace
// regardless of the list filters below.
const cpuUsageSegments = computed<TreemapItem[]>(() =>
  nsPods.value
    .map((p) => ({ key: podKey(p), name: p.name, value: p.usageCpu }))
    .filter((s) => s.value > 0),
)
const memUsageSegments = computed<TreemapItem[]>(() =>
  nsPods.value
    .map((p) => ({ key: podKey(p), name: p.name, value: p.usageMem }))
    .filter((s) => s.value > 0),
)
const cpuRequestSegments = computed<TreemapItem[]>(() =>
  nsPods.value
    .map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'requestsCpu') }))
    .filter((s) => s.value > 0),
)
const cpuLimitSegments = computed<TreemapItem[]>(() =>
  nsPods.value
    .map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'limitsCpu') }))
    .filter((s) => s.value > 0),
)
const memRequestSegments = computed<TreemapItem[]>(() =>
  nsPods.value
    .map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'requestsMem') }))
    .filter((s) => s.value > 0),
)
const memLimitSegments = computed<TreemapItem[]>(() =>
  nsPods.value
    .map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'limitsMem') }))
    .filter((s) => s.value > 0),
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
}

function clearAllFilters() {
  clearFilters()
  selectedWorkloadKey.value = null
}
</script>

<template>
  <v-container fluid class="namespace-drilldown">
    <div class="d-flex align-center mb-2 ga-2">
      <v-btn
        icon="mdi-arrow-left"
        variant="text"
        density="comfortable"
        @click="router.push({ name: 'namespaces' })"
      />
      <div>
        <h1 class="text-h6 mb-0">{{ name }}</h1>
        <div class="text-caption text-medium-emphasis">{{ nsPods.length }} pods</div>
      </div>
      <v-spacer />
      <v-chip size="small" :color="status === 'live' ? 'healthy' : 'warning'" variant="outlined">
        {{ status }}
      </v-chip>
    </div>

    <v-card v-if="allPods" class="mb-5" variant="flat" border>
      <div class="d-flex">
        <v-tabs v-model="resourceTab" direction="vertical" color="watch" class="resource-tabs">
          <v-tab value="treemap" prepend-icon="mdi-chart-tree">By workload</v-tab>
          <v-tab value="bars" prepend-icon="mdi-chart-bar">Bars</v-tab>
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
              <div class="text-body-1 font-weight-medium mb-2">Usage vs requests/limits</div>
              <NodeAllocationBar
                :usage="summary.usageCpu"
                :requests="summary.requestsCpu"
                :limits="summary.limitsCpu"
                :format="formatCpu"
              />
              <NodeAllocationBar
                :usage="summary.usageMem"
                :requests="summary.requestsMem"
                :limits="summary.limitsMem"
                :format="formatMem"
              />
            </v-card-text>
          </v-window-item>
        </v-window>
      </div>
    </v-card>

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
        <v-spacer />
        <span class="text-caption text-medium-emphasis"
          >{{ filteredPods.length }} / {{ nsPods.length }} pods</span
        >
        <v-btn v-if="filtersActive" size="small" variant="text" @click="clearAllFilters"
          >Clear filters</v-btn
        >
      </v-card-text>
    </v-card>

    <v-alert v-if="!allPods" type="info" variant="tonal" class="mb-4"
      >Waiting for pod data…</v-alert
    >
    <v-alert v-else-if="filteredPods.length === 0" type="info" variant="tonal" class="mb-4">
      No pods match the current filters.
    </v-alert>

    <v-card v-for="group in groups" :key="group.controllerKey" class="mb-4" variant="flat" border>
      <v-card-title class="text-body-1 d-flex align-center ga-2">
        <v-icon icon="mdi-cube-outline" size="small" />
        {{ group.controllerLabel }}
        <v-chip size="x-small" variant="tonal" class="ml-1">{{ group.pods.length }}</v-chip>
      </v-card-title>
      <v-divider />
      <v-expansion-panels v-model="openPanels" multiple variant="accordion">
        <v-expansion-panel v-for="pod in group.pods" :key="podKey(pod)" :value="podKey(pod)">
          <v-expansion-panel-title>
            <PodRow :pod="pod" show-node-link @go-to-node="goToNode" />
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
