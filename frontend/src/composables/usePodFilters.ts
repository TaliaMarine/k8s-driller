// Shared pod search/namespace/wild-west filtering and namespace->controller
// grouping, used by both the per-node drilldown and the cluster-wide
// Workloads view so the two stay in lockstep instead of drifting apart.
import { computed, ref, type Ref } from 'vue'
import type { PodDTO } from '@/types/api'

export interface PodGroup {
  namespace: string
  controllerKey: string
  controllerLabel: string
  pods: PodDTO[]
}

export const FILTER_OPTIONS = [
  { value: 'misconfigured', label: 'Misconfigured' },
  { value: 'missing-cpu-request', label: 'Missing CPU request' },
  { value: 'missing-cpu-limit', label: 'Missing CPU limit' },
  { value: 'missing-mem-request', label: 'Missing mem request' },
  { value: 'missing-mem-limit', label: 'Missing mem limit' },
  { value: 'over-cpu-request', label: 'Over CPU request' },
  { value: 'over-mem-request', label: 'Over mem request' },
  { value: 'oom-risk', label: 'OOM-Risk' },
  { value: 'throttling-risk', label: 'Throttling-Risk' },
]

export function podKey(pod: PodDTO): string {
  return `${pod.namespace}/${pod.name}`
}

export function containerAllocation(
  pod: PodDTO,
  field: 'requestsCpu' | 'requestsMem' | 'limitsCpu' | 'limitsMem',
): number {
  return pod.containers.reduce((sum, c) => sum + (c[field] ?? 0), 0)
}

// Whole-node/whole-cluster over-request check, independent of the filters
// below — matches the always node-wide distribution charts on the
// drilldown page.
export function isOverRequest(pod: PodDTO, resource: 'cpu' | 'mem'): boolean {
  const usage = resource === 'cpu' ? pod.usageCpu : pod.usageMem
  const request = containerAllocation(pod, resource === 'cpu' ? 'requestsCpu' : 'requestsMem')
  return request > 0 && usage > request
}

export function missingChips(pod: PodDTO): string[] {
  const chips = new Set<string>()
  for (const c of pod.containers) {
    if (c.wildWest.missingRequestsCpu) chips.add('CPU request')
    if (c.wildWest.missingRequestsMem) chips.add('Mem request')
    if (c.wildWest.missingLimitsCpu) chips.add('CPU limit')
    if (c.wildWest.missingLimitsMem) chips.add('Mem limit')
  }
  return [...chips]
}

function hasMissing(pod: PodDTO, field: keyof PodDTO['containers'][number]['wildWest']): boolean {
  return pod.containers.some((c) => c.wildWest[field])
}

function matchesFilter(pod: PodDTO, filter: string): boolean {
  switch (filter) {
    case 'misconfigured':
      return pod.wildWest
    case 'missing-cpu-request':
      return hasMissing(pod, 'missingRequestsCpu')
    case 'missing-cpu-limit':
      return hasMissing(pod, 'missingLimitsCpu')
    case 'missing-mem-request':
      return hasMissing(pod, 'missingRequestsMem')
    case 'missing-mem-limit':
      return hasMissing(pod, 'missingLimitsMem')
    case 'over-cpu-request':
      return isOverRequest(pod, 'cpu')
    case 'over-mem-request':
      return isOverRequest(pod, 'mem')
    case 'oom-risk':
      return pod.oomRisk
    case 'throttling-risk':
      return pod.throttlingRisk
    default:
      return true
  }
}

export function usePodFilters(pods: Ref<PodDTO[] | null | undefined>) {
  const search = ref('')
  const namespaceFilter = ref<string | null>(null)
  const activeFilters = ref<string[]>([])

  const namespaceOptions = computed(() =>
    [...new Set((pods.value ?? []).map((p) => p.namespace))].sort(),
  )

  const overCpuRequestCount = computed(
    () => (pods.value ?? []).filter((p) => isOverRequest(p, 'cpu')).length,
  )
  const overMemRequestCount = computed(
    () => (pods.value ?? []).filter((p) => isOverRequest(p, 'mem')).length,
  )

  const filteredPods = computed(() => {
    const term = search.value.trim().toLowerCase()
    return (pods.value ?? []).filter((pod) => {
      if (term && !pod.name.toLowerCase().includes(term)) return false
      if (namespaceFilter.value && pod.namespace !== namespaceFilter.value) return false
      return activeFilters.value.every((f) => matchesFilter(pod, f))
    })
  })

  function clearFilters() {
    search.value = ''
    namespaceFilter.value = null
    activeFilters.value = []
  }
  const filtersActive = computed(
    () => search.value !== '' || namespaceFilter.value !== null || activeFilters.value.length > 0,
  )

  // Stable order carried over from the already-sorted backend payload
  // (SPECS.md §2.2), regrouped namespace -> controller.
  const groups = computed<PodGroup[]>(() => {
    const byKey = new Map<string, PodGroup>()
    for (const pod of filteredPods.value) {
      const controllerLabel = pod.controller
        ? `${pod.controller.kind}/${pod.controller.name}`
        : 'Bare pod'
      const key = `${pod.namespace}::${controllerLabel}`
      const group = byKey.get(key)
      if (group) {
        group.pods.push(pod)
      } else {
        byKey.set(key, {
          namespace: pod.namespace,
          controllerKey: key,
          controllerLabel,
          pods: [pod],
        })
      }
    }
    return [...byKey.values()].sort(
      (a, b) =>
        a.namespace.localeCompare(b.namespace) ||
        a.controllerLabel.localeCompare(b.controllerLabel),
    )
  })

  return {
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
  }
}
