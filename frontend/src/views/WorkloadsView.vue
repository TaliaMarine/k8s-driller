<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useEventSource } from '@/composables/useEventSource'
import type { PodDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'
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

const router = useRouter()

const { status, data: pods } = useEventSource<PodDTO[]>('/api/v1/stream/workloads')

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

function goToNode(nodeName: string) {
  router.push({ name: 'node-drilldown', params: { name: nodeName } })
}

// Keyed by "namespace/name", not array index, so refreshed SSE pushes (new
// array identity every time) never collapse a panel the user had open.
const openPanels = ref<string[]>([])
</script>

<template>
  <v-container fluid class="workloads-view">
    <div class="d-flex align-center mb-2 ga-2">
      <div>
        <h1 class="text-h6 mb-0">Workloads</h1>
        <div class="text-caption text-medium-emphasis">
          {{ pods?.length ?? 0 }} pods across the cluster
        </div>
      </div>
      <v-spacer />
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
              <div class="d-flex flex-column ga-1 pod-usage-stack">
                <MiniRatioBar
                  label="C"
                  :usage="pod.usageCpu"
                  :requests="containerAllocation(pod, 'requestsCpu') || undefined"
                  :limits="containerAllocation(pod, 'limitsCpu') || undefined"
                  :format="formatCpu"
                />
                <MiniRatioBar
                  label="M"
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
              <div class="d-flex align-center ga-2 ml-auto workload-row-end">
                <v-btn
                  icon="mdi-server"
                  size="x-small"
                  variant="text"
                  :title="`Go to node ${pod.nodeName}`"
                  @click.stop="goToNode(pod.nodeName)"
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
.workload-row-end {
  flex-shrink: 0;
}
.pod-usage-stack {
  flex-shrink: 0;
}
</style>
