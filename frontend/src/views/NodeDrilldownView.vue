<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useEventSource } from '@/composables/useEventSource'
import { useClusterStore } from '@/stores/cluster'
import type { PodDTO } from '@/types/api'
import { formatCpu, formatMem, nodeHealthColor } from '@/utils/format'
import {
  containerAllocation,
  FILTER_OPTIONS,
  highAllocationLabel,
  highAllocations,
  highAllocationTooltip,
  missingChips,
  podKey,
  usePodFilters,
} from '@/composables/usePodFilters'
import MiniRatioBar from '@/components/MiniRatioBar.vue'
import PodDetailPanel from '@/components/PodDetailPanel.vue'
import NodeDistributionChart from '@/components/NodeDistributionChart.vue'
import type { DistSegment } from '@/components/NodeDistributionChart.vue'

const props = defineProps<{ name: string }>()
const router = useRouter()
const clusterStore = useClusterStore()

const { status, data: pods } = useEventSource<PodDTO[]>(`/api/v1/stream/nodes/${props.name}`)

const node = computed(() => clusterStore.summary?.nodes.find((n) => n.name === props.name))

const {
  search,
  namespaceFilter,
  activeFilters,
  namespaceOptions,
  filteredPods,
  groups,
  clearFilters,
  filtersActive,
  overCpuRequestCount,
  overMemRequestCount,
} = usePodFilters(pods)

// --- distribution chart: always reflects the whole node, independent of filters below ---

const cpuUsageSegments = computed<DistSegment[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: p.usageCpu }))
      .filter((s) => s.value > 0) ?? [],
)
const memUsageSegments = computed<DistSegment[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: p.usageMem }))
      .filter((s) => s.value > 0) ?? [],
)
const cpuRequestSegments = computed<DistSegment[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'requestsCpu') }))
      .filter((s) => s.value > 0) ?? [],
)
const cpuLimitSegments = computed<DistSegment[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'limitsCpu') }))
      .filter((s) => s.value > 0) ?? [],
)
const memRequestSegments = computed<DistSegment[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'requestsMem') }))
      .filter((s) => s.value > 0) ?? [],
)
const memLimitSegments = computed<DistSegment[]>(
  () =>
    pods.value
      ?.map((p) => ({ key: podKey(p), name: p.name, value: containerAllocation(p, 'limitsMem') }))
      .filter((s) => s.value > 0) ?? [],
)
const cpuUsageTotal = computed(
  () => ((node.value?.pressure.liveCpuPct ?? 0) / 100) * (node.value?.capacityCpu ?? 0),
)
const memUsageTotal = computed(
  () => ((node.value?.pressure.liveMemPct ?? 0) / 100) * (node.value?.capacityMemory ?? 0),
)

// Keyed by "namespace/name", not array index, so refreshed SSE pushes (new
// array identity every time, SPECS.md §2 data flow) never collapse a panel
// the user had open (Vuetify's v-model tracks these values, not positions).
const openPanels = ref<string[]>([])
</script>

<template>
  <v-container fluid class="node-drilldown">
    <div class="d-flex align-center mb-2 ga-2">
      <v-btn
        icon="mdi-arrow-left"
        variant="text"
        density="comfortable"
        @click="router.push({ name: 'dashboard' })"
      />
      <div>
        <h1 class="text-h6 mb-0">{{ name }}</h1>
        <div class="text-caption text-medium-emphasis">
          {{ node?.podCount ?? pods?.length ?? 0 }} pods
          <template v-if="node">· {{ node.ready ? 'Ready' : 'Not Ready' }}</template>
        </div>
      </div>
      <v-spacer />
      <v-chip v-if="node" :color="nodeHealthColor(node.health)" size="small" variant="flat">
        {{ node.health }}
      </v-chip>
      <v-chip size="small" :color="status === 'live' ? 'healthy' : 'warning'" variant="outlined">
        {{ status }}
      </v-chip>
    </div>

    <div class="d-flex flex-wrap ga-3 mb-4">
      <v-chip
        :color="overCpuRequestCount > 0 ? 'critical' : 'healthy'"
        variant="tonal"
        size="small"
      >
        <v-icon v-if="overCpuRequestCount > 0" start icon="mdi-alert" />
        {{ overCpuRequestCount }} / {{ pods?.length ?? 0 }} pods over CPU request
      </v-chip>
      <v-chip
        :color="overMemRequestCount > 0 ? 'critical' : 'healthy'"
        variant="tonal"
        size="small"
      >
        <v-icon v-if="overMemRequestCount > 0" start icon="mdi-alert" />
        {{ overMemRequestCount }} / {{ pods?.length ?? 0 }} pods over memory request
      </v-chip>
    </div>

    <v-card class="mb-5" variant="flat" border>
      <v-card-text>
        <NodeDistributionChart
          label="CPU"
          :capacity="node?.capacityCpu ?? 0"
          :usage-segments="cpuUsageSegments"
          :request-segments="cpuRequestSegments"
          :limit-segments="cpuLimitSegments"
          :usage="cpuUsageTotal"
          :format="formatCpu"
        />
        <NodeDistributionChart
          label="Memory"
          :capacity="node?.capacityMemory ?? 0"
          :usage-segments="memUsageSegments"
          :request-segments="memRequestSegments"
          :limit-segments="memLimitSegments"
          :usage="memUsageTotal"
          :format="formatMem"
        />
      </v-card-text>
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
        <v-spacer />
        <span class="text-caption text-medium-emphasis"
          >{{ filteredPods.length }} / {{ pods?.length ?? 0 }} pods</span
        >
        <v-btn v-if="filtersActive" size="small" variant="text" @click="clearFilters"
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
            <div class="d-flex align-center ga-2 flex-wrap pod-title">
              <v-icon
                v-if="pod.wildWest"
                icon="mdi-alert-circle"
                color="wildwest"
                size="small"
                title="Missing resource configuration"
              />
              <span class="font-weight-medium">{{ pod.name }}</span>
              <v-chip v-if="pod.oomRisk" color="critical" size="small" variant="flat"
                >OOM-Risk</v-chip
              >
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
              <div class="d-flex align-center ga-2 ml-auto mini-ratio-group">
                <MiniRatioBar
                  label="C"
                  :usage="pod.usageCpu"
                  :request="containerAllocation(pod, 'requestsCpu') || undefined"
                  :format="formatCpu"
                />
                <MiniRatioBar
                  label="M"
                  :usage="pod.usageMem"
                  :request="containerAllocation(pod, 'requestsMem') || undefined"
                  :format="formatMem"
                />
              </div>
            </div>
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
.pod-title {
  min-width: 0;
}
.mini-ratio-group {
  flex-shrink: 0;
}
</style>
