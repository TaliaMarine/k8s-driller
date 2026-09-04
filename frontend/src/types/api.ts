// Mirrors the JSON shapes in internal/api (SPECS.md §6.1/§6.2). Kept as
// plain interfaces, not generated, since the API surface is small and
// stable — codegen would be overhead this project doesn't need yet.

export interface NodePressure {
  requestsCpuPct: number
  requestsMemPct: number
  limitsCpuPct: number
  limitsMemPct: number
  liveCpuPct: number
  liveMemPct: number
  overcommitCpu: boolean
  overcommitMem: boolean
}

export type NodeHealth = 'Healthy' | 'CPU Pressure' | 'Mem Pressure' | 'Overcommit'

export interface NodeDTO {
  name: string
  ready: boolean
  capacityCpu: number
  capacityMemory: number
  pressure: NodePressure
  health: NodeHealth
  podCount: number
}

export interface ClusterSummaryDTO {
  nodes: NodeDTO[]
  totalCapacityCpu: number
  totalCapacityMem: number
  totalRequestsCpuPct: number
  totalRequestsMemPct: number
  totalLiveCpuPct: number
  totalLiveMemPct: number
}

export interface ContainerWildWest {
  missingRequestsCpu: boolean
  missingRequestsMem: boolean
  missingLimitsCpu: boolean
  missingLimitsMem: boolean
}

export interface ContainerDTO {
  name: string
  requestsCpu?: number
  requestsMem?: number
  limitsCpu?: number
  limitsMem?: number
  wildWest: ContainerWildWest
}

export interface ControllerRefDTO {
  kind: string
  name: string
}

export interface PodDTO {
  namespace: string
  name: string
  nodeName: string
  phase: string
  controller?: ControllerRefDTO
  containers: ContainerDTO[]
  usageCpu: number
  usageMem: number
  wildWest: boolean
  oomRisk: boolean
  throttlingRisk: boolean
}

export interface RecommendationDTO {
  p95Cpu: number
  p95Mem: number
  recommendedRequestCpu: number
  recommendedLimitCpu: number
  recommendedRequestMem: number
  recommendedLimitMem: number
  wasteful: boolean
}

export type Role = 'admin' | 'viewer'

export interface CurrentUser {
  subject: string
  email: string
  name: string // best-effort display name from OIDC "profile" claims; "" if the provider sent none
  role: Role
  expires: string
}

// Matches the driller.k8s.io/v1alpha1 DrillerUserRole CRD as returned by
// GET /api/v1/admin/users (SPECS.md §5.1, §6.1).
export interface DrillerUserRole {
  metadata: { name: string }
  spec: {
    subject: string
    email?: string
    role: Role
    updatedBy?: string
    updatedAt?: string
  }
}

export interface WebhookSecretRef {
  name: string
  key: string
}

export interface Webhook {
  type: 'slack' | 'generic'
  secretRef: WebhookSecretRef
  enabled: boolean
}

export interface AlertThresholds {
  nodeMemLivePct: number
  nodeCpuLivePct: number
  overcommitEnabled: boolean
  oomRiskEnabled: boolean
  throttlingRiskEnabled: boolean
}

export interface DrillerAlertConfigSpec {
  webhooks: Webhook[]
  thresholds: AlertThresholds
  debounceMinutes: number
}
