// Shared pod search/namespace/wild-west filtering and namespace->controller
// grouping, used by both the per-node drilldown and the cluster-wide
// Workloads view so the two stay in lockstep instead of drifting apart.
import { computed, ref, type Ref } from 'vue'
import type { PodDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'

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

// Thresholds for flagging a pod's own configured request/limit as
// unusually large on its face — independent of node capacity or live usage,
// which the other pressure states already cover. 50 cores / 50GiB is well
// above any normal single-container workload, so a pod that clears either
// one is almost always a typo (an extra zero) or a placeholder value left
// over from copy-pasted YAML.
const HIGH_CPU_THRESHOLD_MILLI = 50_000 // 50 cores
const HIGH_MEM_THRESHOLD_BYTES = 50 * 1024 * 1024 * 1024 // 50GiB

export interface HighAllocation {
  resource: 'cpu' | 'mem'
  value: number
  ratio: number
}

// highAllocations reports which of a pod's own request/limit values (the
// larger of the two, per resource) clear the "implausibly large" thresholds
// above, for the "how overly high" chip on the pod list rows.
export function highAllocations(pod: PodDTO): HighAllocation[] {
  const out: HighAllocation[] = []

  const cpu = Math.max(
    containerAllocation(pod, 'requestsCpu'),
    containerAllocation(pod, 'limitsCpu'),
  )
  if (cpu > HIGH_CPU_THRESHOLD_MILLI) {
    out.push({ resource: 'cpu', value: cpu, ratio: cpu / HIGH_CPU_THRESHOLD_MILLI })
  }

  const mem = Math.max(
    containerAllocation(pod, 'requestsMem'),
    containerAllocation(pod, 'limitsMem'),
  )
  if (mem > HIGH_MEM_THRESHOLD_BYTES) {
    out.push({ resource: 'mem', value: mem, ratio: mem / HIGH_MEM_THRESHOLD_BYTES })
  }

  return out
}

export function highAllocationLabel(high: HighAllocation): string {
  return high.resource === 'cpu'
    ? `High CPU: ${formatCpu(high.value)} cores`
    : `High Memory: ${formatMem(high.value)}`
}

export function highAllocationTooltip(high: HighAllocation): string {
  const name = high.resource === 'cpu' ? 'CPU' : 'Memory'
  const unit = high.resource === 'cpu' ? '50-core' : '50GiB'
  return `${name} request/limit is ${high.ratio.toFixed(1)}x the ${unit} sanity threshold — likely a misconfigured value`
}

// Collapsed to at most two chips — "request(s)" and/or "limit(s)" — with no
// CPU/Mem distinction, since the filters dropdown (FILTER_OPTIONS) already
// covers that granularity; the row chip is just a "something's missing"
// flag.
export function missingChips(pod: PodDTO): string[] {
  const missingRequests = pod.containers.some(
    (c) => c.wildWest.missingRequestsCpu || c.wildWest.missingRequestsMem,
  )
  const missingLimits = pod.containers.some(
    (c) => c.wildWest.missingLimitsCpu || c.wildWest.missingLimitsMem,
  )
  const chips: string[] = []
  if (missingRequests) chips.push('request(s)')
  if (missingLimits) chips.push('limit(s)')
  return chips
}

export interface ResourceAggregate {
  usageCpu: number
  usageMem: number
  requestsCpu: number
  requestsMem: number
  limitsCpu: number
  limitsMem: number
}

function emptyAggregate(): ResourceAggregate {
  return { usageCpu: 0, usageMem: 0, requestsCpu: 0, requestsMem: 0, limitsCpu: 0, limitsMem: 0 }
}

function addPod(agg: ResourceAggregate, pod: PodDTO) {
  agg.usageCpu += pod.usageCpu
  agg.usageMem += pod.usageMem
  agg.requestsCpu += containerAllocation(pod, 'requestsCpu')
  agg.requestsMem += containerAllocation(pod, 'requestsMem')
  agg.limitsCpu += containerAllocation(pod, 'limitsCpu')
  agg.limitsMem += containerAllocation(pod, 'limitsMem')
}

// Cluster-wide usage/requests/limits sum across every given pod — the
// backend's cluster summary only carries requests/live as percentages (no
// limits total), so this is computed client-side from the same pod list
// the lists already stream.
export function totalAggregate(pods: PodDTO[]): ResourceAggregate {
  const agg = emptyAggregate()
  for (const pod of pods) addPod(agg, pod)
  return agg
}

export interface NamespaceAggregate extends ResourceAggregate {
  namespace: string
  podCount: number
}

// Per-namespace usage/requests/limits sums, sorted by namespace name — the
// raw material for both the Namespaces page's distribution charts and its
// per-namespace summary cards.
export function namespaceAggregates(pods: PodDTO[]): NamespaceAggregate[] {
  const byNamespace = new Map<string, NamespaceAggregate>()
  for (const pod of pods) {
    let agg = byNamespace.get(pod.namespace)
    if (!agg) {
      agg = { namespace: pod.namespace, podCount: 0, ...emptyAggregate() }
      byNamespace.set(pod.namespace, agg)
    }
    agg.podCount++
    addPod(agg, pod)
  }
  return [...byNamespace.values()].sort((a, b) => a.namespace.localeCompare(b.namespace))
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
  // kube-system is noisy on most clusters (many small system pods that
  // aren't what someone drilling into workload pressure usually cares
  // about), so it's hidden by default and only a deliberate toggle away.
  const includeKubeSystem = ref(false)

  const scopedPods = computed(() =>
    (pods.value ?? []).filter((p) => includeKubeSystem.value || p.namespace !== 'kube-system'),
  )

  const namespaceOptions = computed(() =>
    [...new Set(scopedPods.value.map((p) => p.namespace))].sort(),
  )

  const overCpuRequestCount = computed(
    () => scopedPods.value.filter((p) => isOverRequest(p, 'cpu')).length,
  )
  const overMemRequestCount = computed(
    () => scopedPods.value.filter((p) => isOverRequest(p, 'mem')).length,
  )

  const filteredPods = computed(() => {
    const term = search.value.trim().toLowerCase()
    return scopedPods.value.filter((pod) => {
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
    includeKubeSystem,
    namespaceOptions,
    scopedPods,
    filteredPods,
    groups,
    clearFilters,
    filtersActive,
    overCpuRequestCount,
    overMemRequestCount,
  }
}
