<script setup lang="ts">
import { computed } from 'vue'

// Extracts the stability/restart picture from the pod's own parsed YAML —
// independent of whichever manifest tab (Pod / parent controller) is
// currently selected in the YAML tree next to this table (see
// PodManifestView.vue). Every section is skipped entirely when the source
// data doesn't have it, rather than showing an empty table.
const props = defineProps<{ podData: Record<string, unknown> | null }>()

type YamlObj = Record<string, unknown>

function obj(v: unknown): YamlObj {
  return v != null && typeof v === 'object' && !Array.isArray(v) ? (v as YamlObj) : {}
}
function arr(v: unknown): unknown[] {
  return Array.isArray(v) ? v : []
}
function str(v: unknown): string | undefined {
  if (v == null) return undefined
  if (v instanceof Date) return v.toISOString()
  return String(v)
}
function num(v: unknown): number | undefined {
  return typeof v === 'number' ? v : undefined
}
function date(v: unknown): Date | null {
  if (v instanceof Date) return v
  if (typeof v === 'string' && v !== '') return new Date(v)
  return null
}

function formatAge(d: Date): string {
  const ms = Date.now() - d.getTime()
  if (ms < 0) return '0s'
  const seconds = Math.floor(ms / 1000)
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d${hours}h`
  if (hours > 0) return `${hours}h${minutes}m`
  if (minutes > 0) return `${minutes}m`
  return `${seconds}s`
}

const metadata = computed(() => obj(props.podData?.metadata))
const spec = computed(() => obj(props.podData?.spec))
const status = computed(() => obj(props.podData?.status))

const creationDate = computed(() => date(metadata.value.creationTimestamp))
const age = computed(() => (creationDate.value ? formatAge(creationDate.value) : undefined))
const startDate = computed(() => date(status.value.startTime))

const overview = computed(() => {
  const rows: { label: string; value: string }[] = []
  if (age.value) rows.push({ label: 'Age', value: age.value })
  if (str(status.value.phase)) rows.push({ label: 'Phase', value: str(status.value.phase)! })
  if (str(status.value.qosClass))
    rows.push({ label: 'QoS class', value: str(status.value.qosClass)! })
  if (str(spec.value.nodeName)) rows.push({ label: 'Node', value: str(spec.value.nodeName)! })
  if (startDate.value) rows.push({ label: 'Started', value: startDate.value.toLocaleString() })
  return rows
})

const containerStatuses = computed(() => arr(status.value.containerStatuses).map(obj))

const restarts = computed(() =>
  containerStatuses.value.map((c) => ({
    name: str(c.name) ?? '?',
    restartCount: num(c.restartCount) ?? 0,
  })),
)
const totalRestarts = computed(() => restarts.value.reduce((sum, c) => sum + c.restartCount, 0))

interface TerminationEvent {
  container: string
  kind: 'Running (waiting before)' | 'Currently terminated' | 'Previous termination'
  reason?: string
  exitCode?: number
  finishedAt?: string
}

const terminationEvents = computed<TerminationEvent[]>(() => {
  const out: TerminationEvent[] = []
  for (const c of containerStatuses.value) {
    const name = str(c.name) ?? '?'
    const state = obj(c.state)
    const lastState = obj(c.lastState)

    const waiting = obj(state.waiting)
    if (str(waiting.reason)) {
      out.push({ container: name, kind: 'Running (waiting before)', reason: str(waiting.reason) })
    }
    const terminated = obj(state.terminated)
    if (str(terminated.reason) || num(terminated.exitCode) != null) {
      out.push({
        container: name,
        kind: 'Currently terminated',
        reason: str(terminated.reason),
        exitCode: num(terminated.exitCode),
        finishedAt: str(terminated.finishedAt),
      })
    }
    const lastTerminated = obj(lastState.terminated)
    if (str(lastTerminated.reason) || num(lastTerminated.exitCode) != null) {
      out.push({
        container: name,
        kind: 'Previous termination',
        reason: str(lastTerminated.reason),
        exitCode: num(lastTerminated.exitCode),
        finishedAt: str(lastTerminated.finishedAt),
      })
    }
  }
  return out
})

interface ConditionRow {
  type: string
  status: string
  reason?: string
  message?: string
  lastTransitionTime?: string
}

const conditions = computed<ConditionRow[]>(() =>
  arr(status.value.conditions)
    .map(obj)
    .map((c) => ({
      type: str(c.type) ?? '?',
      status: str(c.status) ?? '?',
      reason: str(c.reason),
      message: str(c.message),
      lastTransitionTime: str(c.lastTransitionTime),
    })),
)
</script>

<template>
  <div class="pod-info-table">
    <div v-if="!podData" class="text-caption text-medium-emphasis">No pod data.</div>
    <template v-else>
      <v-table v-if="overview.length" density="compact" class="mb-4 info-table">
        <tbody>
          <tr v-for="row in overview" :key="row.label">
            <td class="font-weight-medium">{{ row.label }}</td>
            <td>{{ row.value }}</td>
          </tr>
        </tbody>
      </v-table>

      <div v-if="restarts.length" class="mb-4">
        <div class="text-caption text-medium-emphasis mb-1">
          Restarts
          <v-chip
            v-if="totalRestarts > 0"
            size="x-small"
            color="warning"
            variant="flat"
            class="ml-1"
          >
            {{ totalRestarts }} total
          </v-chip>
        </div>
        <v-table density="compact" class="info-table">
          <tbody>
            <tr v-for="c in restarts" :key="c.name">
              <td>{{ c.name }}</td>
              <td class="text-right" :class="c.restartCount > 0 ? 'text-warning' : ''">
                {{ c.restartCount }}
              </td>
            </tr>
          </tbody>
        </v-table>
      </div>

      <div v-if="terminationEvents.length" class="mb-4">
        <div class="text-caption text-medium-emphasis mb-1">Termination / crash info</div>
        <v-table density="compact" class="info-table">
          <thead>
            <tr>
              <th>Container</th>
              <th>State</th>
              <th>Reason</th>
              <th class="text-right">Exit</th>
              <th>When</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(e, i) in terminationEvents" :key="i">
              <td>{{ e.container }}</td>
              <td>{{ e.kind }}</td>
              <td :class="e.reason === 'OOMKilled' ? 'text-critical font-weight-medium' : ''">
                {{ e.reason ?? '—' }}
              </td>
              <td class="text-right">{{ e.exitCode ?? '—' }}</td>
              <td>{{ e.finishedAt ? new Date(e.finishedAt).toLocaleString() : '—' }}</td>
            </tr>
          </tbody>
        </v-table>
      </div>

      <div v-if="conditions.length" class="mb-2">
        <div class="text-caption text-medium-emphasis mb-1">Conditions</div>
        <v-table density="compact" class="info-table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Status</th>
              <th>Reason</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in conditions" :key="c.type">
              <td>{{ c.type }}</td>
              <td :class="c.status !== 'True' ? 'text-warning' : ''">{{ c.status }}</td>
              <td :title="c.message">{{ c.reason ?? '—' }}</td>
            </tr>
          </tbody>
        </v-table>
      </div>
    </template>
  </div>
</template>

<style scoped>
.info-table {
  background: transparent;
}
</style>
