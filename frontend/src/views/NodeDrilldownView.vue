<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useEventSource } from '@/composables/useEventSource'
import type { PodDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'
import DeltaBars from '@/components/DeltaBars.vue'

const props = defineProps<{ name: string }>()
const router = useRouter()

const { status, data: pods } = useEventSource<PodDTO[]>(`/api/v1/stream/nodes/${props.name}`)

const wildWestPods = computed(() => (pods.value ?? []).filter((p) => p.wildWest))

interface Group {
  namespace: string
  controllerKey: string
  controllerLabel: string
  pods: PodDTO[]
}

const groups = computed<Group[]>(() => {
  const byKey = new Map<string, Group>()
  for (const pod of pods.value ?? []) {
    const controllerLabel = pod.controller
      ? `${pod.controller.kind}/${pod.controller.name}`
      : 'Bare pod'
    const key = `${pod.namespace}::${controllerLabel}`
    const group = byKey.get(key)
    if (group) {
      group.pods.push(pod)
    } else {
      byKey.set(key, { namespace: pod.namespace, controllerKey: key, controllerLabel, pods: [pod] })
    }
  }
  return [...byKey.values()].sort((a, b) => a.namespace.localeCompare(b.namespace))
})

function missingChips(pod: PodDTO): string[] {
  const chips = new Set<string>()
  for (const c of pod.containers) {
    if (c.wildWest.missingRequestsCpu) chips.add('MISSING CPU REQUEST')
    if (c.wildWest.missingRequestsMem) chips.add('MISSING MEM REQUEST')
    if (c.wildWest.missingLimitsCpu) chips.add('MISSING CPU LIMIT')
    if (c.wildWest.missingLimitsMem) chips.add('MISSING MEM LIMIT')
  }
  return [...chips]
}

function containerAllocation(
  pod: PodDTO,
  field: 'requestsCpu' | 'requestsMem' | 'limitsCpu' | 'limitsMem',
) {
  return pod.containers.reduce((sum, c) => sum + (c[field] ?? 0), 0)
}
</script>

<template>
  <v-container fluid>
    <div class="d-flex align-center mb-4 ga-2">
      <v-btn icon="mdi-arrow-left" variant="text" @click="router.push({ name: 'dashboard' })" />
      <h2 class="text-h6">{{ name }} drilldown</h2>
      <v-chip size="small" :color="status === 'live' ? 'healthy' : 'warning'" variant="flat">{{
        status
      }}</v-chip>
    </div>

    <v-card v-if="wildWestPods.length" class="mb-6" variant="outlined" color="wildwest">
      <v-card-title class="text-wildwest"
        >Misconfigured workloads (missing requests/limits)</v-card-title
      >
      <v-list>
        <v-list-item v-for="pod in wildWestPods" :key="`${pod.namespace}/${pod.name}`">
          <template #prepend>
            <v-icon icon="mdi-alert" color="wildwest" />
          </template>
          <v-list-item-title>{{ pod.name }}</v-list-item-title>
          <v-list-item-subtitle>Namespace: {{ pod.namespace }}</v-list-item-subtitle>
          <template #append>
            <v-chip
              v-for="chip in missingChips(pod)"
              :key="chip"
              color="wildwest"
              size="small"
              variant="flat"
              class="ml-1"
            >
              {{ chip }}
            </v-chip>
          </template>
        </v-list-item>
      </v-list>
    </v-card>

    <v-card v-for="group in groups" :key="group.controllerKey" class="mb-4">
      <v-card-title>{{ group.namespace }} / {{ group.controllerLabel }}</v-card-title>
      <v-expansion-panels variant="accordion">
        <v-expansion-panel v-for="pod in group.pods" :key="pod.name">
          <v-expansion-panel-title>
            <span class="mr-2">{{ pod.name }}</span>
            <v-chip v-if="pod.oomRisk" color="critical" size="small" class="mr-1">OOM-Risk</v-chip>
            <v-chip v-if="pod.throttlingRisk" color="warning" size="small" class="mr-1"
              >Throttling-Risk</v-chip
            >
            <v-chip v-if="pod.wildWest" color="wildwest" size="small">Wild-West</v-chip>
          </v-expansion-panel-title>
          <v-expansion-panel-text>
            <DeltaBars
              label="CPU"
              :usage="pod.usageCpu"
              :request="containerAllocation(pod, 'requestsCpu') || undefined"
              :limit="containerAllocation(pod, 'limitsCpu') || undefined"
              :format="formatCpu"
              :danger="pod.throttlingRisk"
            />
            <DeltaBars
              label="Memory"
              :usage="pod.usageMem"
              :request="containerAllocation(pod, 'requestsMem') || undefined"
              :limit="containerAllocation(pod, 'limitsMem') || undefined"
              :format="formatMem"
              :danger="pod.oomRisk"
            />
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </v-card>
  </v-container>
</template>
